package installationlicense

import (
	"context"
	"net/http"
	"sync/atomic"
)

var runtimeRequired atomic.Bool
var runtimeManager atomic.Pointer[Manager]

// InstallRuntime is mandatory before the application opens business resources.
// Once enabled it cannot be disabled; a nil manager means restricted startup.
func InstallRuntime(m *Manager) { runtimeManager.Store(m); runtimeRequired.Store(true) }
func CheckRuntime(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if required, _ := ctx.Value(requiredKey{}).(bool); required && FromContext(ctx) == nil {
		return ErrRestricted
	}
	m := FromContext(ctx)
	if m == nil && runtimeRequired.Load() {
		m = runtimeManager.Load()
		if m == nil {
			return ErrRestricted
		}
	}
	if m != nil && m.Decision().State == "RESTRICTED" {
		return ErrRestricted
	}
	return nil
}
func RuntimeEffect(ctx context.Context, fn func() error) error {
	if required, _ := ctx.Value(requiredKey{}).(bool); required && FromContext(ctx) == nil {
		return ErrRestricted
	}
	m := FromContext(ctx)
	if m == nil && runtimeRequired.Load() {
		m = runtimeManager.Load()
		if m == nil {
			return ErrRestricted
		}
	}
	if m != nil {
		return m.Effect(ctx, fn)
	}
	return fn()
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
