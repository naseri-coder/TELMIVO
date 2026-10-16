package installationlicense

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
)

// A nil pointer means the process has not installed its license Manager.
// One successful installation may be followed only by terminal restriction.
// A nil installation or attempted foreign replacement is terminal until process
// restart; no local Context can override it.
type runtimeBinding struct {
	manager *Manager
}

var (
	installedRuntime atomic.Pointer[runtimeBinding]
	// A mutex and active-effect counter avoid RWMutex writer-preference
	// deadlocks when a protected callback performs a nested CheckRuntime.
	// Do not call InstallRuntime synchronously inside a protected callback.
	runtimeGate    sync.Mutex
	runtimeActive  int
	runtimeDrained = sync.NewCond(&runtimeGate)
)

// InstallRuntime is a one-way process-wide authority transition:
// uninitialized -> installed / restricted; installed -> restricted.
// An attempted replacement with a different Manager fails closed.
// Reinstalling a restricted process never re-enables effects.
func InstallRuntime(m *Manager) {
	runtimeGate.Lock()
	defer runtimeGate.Unlock()

	previous := installedRuntime.Load()
	if previous == nil {
		installedRuntime.Store(&runtimeBinding{manager: m})
		return
	}
	if previous.manager == nil {
		return // already terminally restricted
	}
	if m == previous.manager {
		return // idempotent installation of the same verified owner
	}
	// Deny new admissions before waiting for previously admitted effects.
	installedRuntime.Store(&runtimeBinding{})
	for runtimeActive > 0 {
		runtimeDrained.Wait()
	}
}

// runtimeForEffect permits only the installed process-wide Manager. In
// particular, even a signed Context-bound Manager cannot authorize an effect
// before InstallRuntime. Test/isolated work should explicitly install a
// Manager in a separate process rather than bypassing startup.
func runtimeForEffect(ctx context.Context, globallyRequired bool, global *Manager) (*Manager, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if required, _ := ctx.Value(requiredKey{}).(bool); required && FromContext(ctx) == nil {
		return nil, ErrRestricted
	}
	if !globallyRequired {
		return nil, ErrRestricted
	}
	m := FromContext(ctx)
	if global == nil || (m != nil && m != global) {
		return nil, ErrRestricted
	}
	return global, nil
}

// runtimeForCurrentEffect loads a single coherent installation snapshot.
// Callers must hold runtimeGate while reserving an effect admission.
func runtimeForCurrentEffect(ctx context.Context) (*Manager, error) {
	binding := installedRuntime.Load()
	if binding == nil {
		return runtimeForEffect(ctx, false, nil)
	}
	return runtimeForEffect(ctx, true, binding.manager)
}

// RuntimeDecision returns the actual installed Manager's signed decision when
// the state lock is available. A busy installation lock fails closed rather
// than deadlocking an already-authorized callback that performs a nested read.
func RuntimeDecision(ctx context.Context) Decision {
	runtimeGate.Lock()
	defer runtimeGate.Unlock()
	m, err := runtimeForCurrentEffect(ctx)
	if err != nil {
		return Decision{State: "RESTRICTED", Reason: "RUNTIME_RESTRICTED"}
	}
	return m.decisionNonblocking()
}

func CheckRuntime(ctx context.Context) error {
	runtimeGate.Lock()
	defer runtimeGate.Unlock()
	m, err := runtimeForCurrentEffect(ctx)
	if err != nil {
		return err
	}
	if m.decisionNonblocking().State == "RESTRICTED" {
		return ErrRestricted
	}
	return nil
}
func RuntimeEffect(ctx context.Context, fn func() error) error {
	runtimeGate.Lock()
	m, err := runtimeForCurrentEffect(ctx)
	if err != nil {
		runtimeGate.Unlock()
		return err
	}
	runtimeActive++
	runtimeGate.Unlock()

	defer func() {
		runtimeGate.Lock()
		runtimeActive--
		if runtimeActive == 0 {
			runtimeDrained.Broadcast()
		}
		runtimeGate.Unlock()
	}()
	return m.Effect(ctx, fn)
}
func HTTP(ctx context.Context, client *http.Client, request *http.Request) (*http.Response, error) {
	var r *http.Response
	err := RuntimeEffect(ctx, func() error { var e error; r, e = client.Do(request); return e })
	return r, err
}

type readKey struct{}

func ReadOnly(ctx context.Context) context.Context { return context.WithValue(ctx, readKey{}, true) }
func IsReadOnly(ctx context.Context) bool          { v, _ := ctx.Value(readKey{}).(bool); return v }

type requiredKey struct{}

func RequireContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, requiredKey{}, true)
}

// Settlement permits only durable outcome bookkeeping; it never grants an external effect.
type settlementKey struct{}

func Settlement(ctx context.Context) context.Context {
	return context.WithValue(ctx, settlementKey{}, true)
}
func IsSettlement(ctx context.Context) bool { v, _ := ctx.Value(settlementKey{}).(bool); return v }
