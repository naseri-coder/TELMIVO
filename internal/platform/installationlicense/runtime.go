package installationlicense

import (
	"context"
	"net/http"
	"sync/atomic"
)

// A nil atomic pointer means that InstallRuntime was never called. After the
// first installation, even a nil Manager is represented by a non-nil binding:
// the process must remain restricted. Publish the entire pair atomically so
// authorization never observes a mixed generation during a replacement.
type runtimeBinding struct {
	manager *Manager
}

var installedRuntime atomic.Pointer[runtimeBinding]

// InstallRuntime publishes one immutable binding; a nil Manager is a durable
// process-local restriction, not an opt-out of license enforcement.
func InstallRuntime(m *Manager) {
	installedRuntime.Store(&runtimeBinding{manager: m})
}

// runtimeForEffect defaults to denial. A manager explicitly bound to context
// can authorize an effect through its own durable verification even before an
// application-wide manager is installed (e.g. a test or an isolated worker).
// Once installed, the process-wide Manager is authoritative: a foreign or
// locally supplied Manager cannot override a restricted global installation.
// Before installation, a deliberately bound Manager may still be used by
// isolated workers, subject to that Manager's own verified lease.
func runtimeForEffect(ctx context.Context, globallyRequired bool, global *Manager) (*Manager, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if required, _ := ctx.Value(requiredKey{}).(bool); required && FromContext(ctx) == nil {
		return nil, ErrRestricted
	}
	m := FromContext(ctx)
	if globallyRequired {
		// An installed restricted/nil runtime is process-wide authority.
		// A foreign context-bound Manager must never override it.
		if global == nil || (m != nil && m != global) {
			return nil, ErrRestricted
		}
		return global, nil
	}
	if m == nil {
		return nil, ErrRestricted
	}
	return m, nil
}

// runtimeForCurrentEffect reads one coherent installation snapshot. In a
// separate isolated worker with no global installation, an explicit context
// Manager is still individually checked; no Manager always denies.
func runtimeForCurrentEffect(ctx context.Context) (*Manager, error) {
	binding := installedRuntime.Load()
	if binding == nil {
		return runtimeForEffect(ctx, false, nil)
	}
	return runtimeForEffect(ctx, true, binding.manager)
}

func CheckRuntime(ctx context.Context) error {
	m, err := runtimeForCurrentEffect(ctx)
	if err != nil {
		return err
	}
	if m.Decision().State == "RESTRICTED" {
		return ErrRestricted
	}
	return nil
}
func RuntimeEffect(ctx context.Context, fn func() error) error {
	m, err := runtimeForCurrentEffect(ctx)
	if err != nil {
		return err
	}
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
