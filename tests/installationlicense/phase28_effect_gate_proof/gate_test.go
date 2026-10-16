package effectgate

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

type mockEffect struct {
	mu      sync.Mutex
	allowed bool
	called  atomic.Int32
}

func (m *mockEffect) Effect(ctx context.Context, fn func() error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.called.Add(1)
	if !m.allowed {
		return ErrDenied
	}
	return fn()
}

type mockPolicy struct {
	allow  bool
	checks atomic.Int32
}

func (p *mockPolicy) FinalRecheck(_ context.Context, i Intent) error {
	p.checks.Add(1)
	if !p.allow || i.WorkspaceID != "tenantA" || i.ActorID != "actorA" {
		return ErrDenied
	}
	return nil
}
func valid() Intent {
	return Intent{RequestID: "request1", WorkerID: "worker1", WorkspaceID: "tenantA", ActorID: "actorA", ResourceID: "record1", Operation: SyntheticOperation}
}
func fixture(licensed, authorized bool, eff func(context.Context, Intent) error) (*Gateway, *mockEffect, *mockPolicy) {
	manager := &mockEffect{allowed: licensed}
	policy := &mockPolicy{allow: authorized}
	gate, _ := New(manager, policy, NewMemoryLedger(), eff)
	return gate, manager, policy
}
func TestGenuineFinalGateIsNecessary(t *testing.T) {
	var calls atomic.Int32
	g, m, p := fixture(false, true, func(context.Context, Intent) error { calls.Add(1); return nil })
	if err := g.Execute(context.Background(), valid()); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	if calls.Load() != 0 || p.checks.Load() != 0 || m.called.Load() != 1 {
		t.Fatal("missing effect fence")
	}
}
func TestDomainPolicyMustRunInsideFinalGate(t *testing.T) {
	var calls atomic.Int32
	g, m, p := fixture(true, false, func(context.Context, Intent) error { calls.Add(1); return nil })
	if err := g.Execute(context.Background(), valid()); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	if calls.Load() != 0 || m.called.Load() != 1 || p.checks.Load() != 1 {
		t.Fatal("recheck not enforced")
	}
}
func TestTenantTamperingDenied(t *testing.T) {
	var calls atomic.Int32
	g, _, _ := fixture(true, true, func(context.Context, Intent) error { calls.Add(1); return nil })
	i := valid()
	i.WorkspaceID = "tenantB"
	if err := g.Execute(context.Background(), i); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("effect escaped tenant")
	}
}
func TestActorTamperingDenied(t *testing.T) {
	g, _, _ := fixture(true, true, func(context.Context, Intent) error { t.Fatal("effect executed"); return nil })
	i := valid()
	i.ActorID = "bad"
	if err := g.Execute(context.Background(), i); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
}
func TestUnregisteredEffectRejected(t *testing.T) {
	g, m, _ := fixture(true, true, func(context.Context, Intent) error { t.Fatal("unexpected effect"); return nil })
	i := valid()
	i.Operation = "ARBITRARY_SHELL"
	if err := g.Execute(context.Background(), i); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	if m.called.Load() != 0 {
		t.Fatal("unexpected manager call")
	}
}
func TestNoRequestIdRejected(t *testing.T) {
	g, _, _ := fixture(true, true, func(context.Context, Intent) error { t.Fatal("unexpected"); return nil })
	i := valid()
	i.RequestID = ""
	if err := g.Execute(context.Background(), i); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
}
func TestSingleEffectAndReplay(t *testing.T) {
	var calls atomic.Int32
	g, _, _ := fixture(true, true, func(context.Context, Intent) error { calls.Add(1); return nil })
	if e := g.Execute(context.Background(), valid()); e != nil {
		t.Fatal(e)
	}
	if e := g.Execute(context.Background(), valid()); !errors.Is(e, ErrDuplicate) {
		t.Fatal(e)
	}
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
}
func TestUnknownOutcomeNotRetried(t *testing.T) {
	var calls atomic.Int32
	g, _, _ := fixture(true, true, func(context.Context, Intent) error { calls.Add(1); return errors.New("downstream unknown") })
	if e := g.Execute(context.Background(), valid()); !errors.Is(e, ErrUncertain) {
		t.Fatal(e)
	}
	if e := g.Execute(context.Background(), valid()); !errors.Is(e, ErrDuplicate) {
		t.Fatal(e)
	}
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
}
func TestConcurrentAtMostOnce(t *testing.T) {
	var calls atomic.Int32
	g, _, _ := fixture(true, true, func(context.Context, Intent) error { calls.Add(1); return nil })
	var wg sync.WaitGroup
	for j := 0; j < 32; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e := g.Execute(context.Background(), valid())
			if e != nil && !errors.Is(e, ErrDuplicate) {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
}
func TestCancelledContextDenied(t *testing.T) {
	g, m, _ := fixture(true, true, func(context.Context, Intent) error { t.Fatal("unexpected effect"); return nil })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := g.Execute(ctx, valid()); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if m.called.Load() != 0 {
		t.Fatal(m.called.Load())
	}
}
func TestNilDependenciesFailClosed(t *testing.T) {
	if g, e := New(nil, nil, nil, nil); g != nil || !errors.Is(e, ErrDenied) {
		t.Fatal("unsafe constructor")
	}
}
