// Package effectgate is NONPRODUCTION-only. It tests the required ordering of a
// final license Effect gate, business authorization, replay reservation and a
// finite synthetic handler. It does not import or exercise Product Manager.
package effectgate

import (
	"context"
	"errors"
	"sync"
)

var (
	ErrDenied    = errors.New("PHASE28_TEST_ONLY_DENIED")
	ErrDuplicate = errors.New("PHASE28_TEST_ONLY_DUPLICATE")
	ErrUncertain = errors.New("PHASE28_TEST_ONLY_OUTCOME_UNCERTAIN")
)

const SyntheticOperation = "SYNTHETIC_RECORD_EFFECT"

type Intent struct {
	RequestID   string
	WorkerID    string
	WorkspaceID string
	ActorID     string
	ResourceID  string
	Operation   string
}

// EffectExecutor matches the method signature of the genuine Product
// Manager.Effect. The tests use only a synthetic gate, not an installed manager.
type EffectExecutor interface {
	Effect(context.Context, func() error) error
}

type DomainPolicy interface {
	FinalRecheck(context.Context, Intent) error
}

type Reservations interface {
	Reserve(Intent) error
}

type Gateway struct {
	manager   EffectExecutor
	policy    DomainPolicy
	ledger    Reservations
	synthetic func(context.Context, Intent) error
}

func New(manager EffectExecutor, policy DomainPolicy, ledger Reservations, synthetic func(context.Context, Intent) error) (*Gateway, error) {
	if manager == nil || policy == nil || ledger == nil || synthetic == nil {
		return nil, ErrDenied
	}
	return &Gateway{manager: manager, policy: policy, ledger: ledger, synthetic: synthetic}, nil
}

func (g *Gateway) Execute(ctx context.Context, intent Intent) error {
	if g == nil || intent.RequestID == "" || intent.WorkerID == "" || intent.WorkspaceID == "" || intent.ActorID == "" || intent.ResourceID == "" || intent.Operation != SyntheticOperation {
		return ErrDenied
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Critical ordering: domain and ledger admission must occur inside the
	// *actual* final effect authorization (mocked in this isolated proof).
	return g.manager.Effect(ctx, func() error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if g.policy.FinalRecheck(ctx, intent) != nil {
			return ErrDenied
		}
		if err := g.ledger.Reserve(intent); err != nil {
			return err
		}
		// Never blindly rerun on error: the reservation remains consumed.
		if err := g.synthetic(ctx, intent); err != nil {
			return ErrUncertain
		}
		return nil
	})
}

// MemoryLedger is an in-memory test implementation. It is NOT crash durable.
type MemoryLedger struct {
	mu   sync.Mutex
	used map[string]struct{}
}

func NewMemoryLedger() *MemoryLedger { return &MemoryLedger{used: make(map[string]struct{})} }
func (l *MemoryLedger) Reserve(i Intent) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.used[i.RequestID]; ok {
		return ErrDuplicate
	}
	l.used[i.RequestID] = struct{}{}
	return nil
}
