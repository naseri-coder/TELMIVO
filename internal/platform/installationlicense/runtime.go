package installationlicense

import (
	"context"
	"net/http"
	"sync/atomic"
)

var runtimeRequired atomic.Bool
var runtimeManager atomic.Pointer[Manager]

// InstallRuntime enables a permanent process-wide license guard. Publish the
// requirement first: a concurrent effect cannot see an unguarded window.
// A nil manager leaves protected work restricted.
func InstallRuntime(m *Manager) {
	runtimeRequired.Store(true)
	runtimeManager.Store(m)
}

// runtimeForEffect defaults to denial. A manager explicitly bound to context
// can authorize an effect through its own durable verification even before an
// application-wide manager is installed (e.g. a test or an isolated worker).
// Without either manager, absence of initialization never grants an effect.
func runtimeForEffect(ctx context.Context, globallyRequired bool, global *Manager) (*Manager, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if required, _ := ctx.Value(requiredKey{}).(bool); required && FromContext(ctx) == nil {
		return nil, ErrRestricted
	}
	m := FromContext(ctx)
	if m == nil && globallyRequired {
		m = global
	}
	if m == nil {
		return nil, ErrRestricted
	}
	return m, nil
}

func CheckRuntime(ctx context.Context) error {
	m, err := runtimeForEffect(ctx, runtimeRequired.Load(), runtimeManager.Load())
	if err != nil {
		return err
	}
	if m.Decision().State == "RESTRICTED" {
		return ErrRestricted
	}
	return nil
}
func RuntimeEffect(ctx context.Context, fn func() error) error {
	m, err := runtimeForEffect(ctx, runtimeRequired.Load(), runtimeManager.Load())
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
