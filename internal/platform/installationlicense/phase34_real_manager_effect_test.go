package installationlicense

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Phase34 deliberately exercises the REAL Manager.Effect implementation in
// this Product package. The helper uses freshly-generated, in-memory test-only
// Ed25519 signing keys and real cryptographic V2 claims. It does not contact a
// live Authority, produce a remote Provider effect, or certify Production.
func phase34LicensedManager(t *testing.T) (*Manager, func(string, int64) []byte) {
	t.Helper()
	manager, signer := phase19TestSignedConcurrentInstallation(t)
	if err := manager.Accept(signer(manager.runtimeSession, 1)); err != nil {
		t.Fatalf("accept signed test V2 lease: %v", err)
	}
	if got := manager.Decision(); got.State != "VALID_ONLINE" {
		t.Fatalf("signed test lease not valid: %+v", got)
	}
	return manager, signer
}

func TestLICA9Phase34RealManagerEffectDispatchesOnlyWithSignedV2Session(t *testing.T) {
	manager, _ := phase34LicensedManager(t)
	calls := 0
	if err := manager.Effect(context.Background(), func() error {
		calls++
		return nil
	}); err != nil {
		t.Fatalf("real signed-session Manager.Effect returned error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("synthetic callback calls=%d, want=1", calls)
	}
}

func TestLICA9Phase34RealManagerEffectBeforeActivationDenied(t *testing.T) {
	manager, _ := phase19TestSignedConcurrentInstallation(t)
	calls := 0
	err := manager.Effect(context.Background(), func() error { calls++; return nil })
	if !errors.Is(err, ErrRestricted) || calls != 0 {
		t.Fatalf("unsigned/unactivated effect executed: calls=%d error=%v", calls, err)
	}
}

func TestLICA9Phase34RealManagerEffectRejectsCanceledContext(t *testing.T) {
	manager, _ := phase34LicensedManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	err := manager.Effect(ctx, func() error { calls++; return nil })
	if !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("canceled effect should not run: calls=%d error=%v", calls, err)
	}
}

func TestLICA9Phase34RealManagerEffectPropagatesFailureWithoutAutoRetry(t *testing.T) {
	manager, _ := phase34LicensedManager(t)
	sentinel := errors.New("PHASE34_SYNTHETIC_UNKNOWN_PROVIDER_OUTCOME")
	calls := 0
	err := manager.Effect(context.Background(), func() error {
		calls++
		return sentinel
	})
	if !errors.Is(err, sentinel) || calls != 1 {
		t.Fatalf("callback must be called exactly once: calls=%d error=%v", calls, err)
	}
	// Manager.Effect itself does not implement a durable idempotency journal.
	// DO NOT automatically invoke it a second time after an unknown outcome.
}

func TestLICA9Phase34RealManagerEffectDeniesRestartedSessionUntilNewSignedV2(t *testing.T) {
	leader, signer := phase34LicensedManager(t)
	restarted, err := Open(leader.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.runtimeSession == leader.runtimeSession {
		t.Fatal("runtime session unexpectedly reused after restart")
	}
	calls := 0
	err = restarted.Effect(context.Background(), func() error { calls++; return nil })
	if !errors.Is(err, ErrRestricted) || calls != 0 {
		t.Fatalf("restarted process reused old signed lease: calls=%d error=%v", calls, err)
	}
	if err = restarted.Accept(signer(restarted.runtimeSession, 2)); err != nil {
		t.Fatalf("accept fresh signed generation and session: %v", err)
	}
	if err = restarted.Effect(context.Background(), func() error { calls++; return nil }); err != nil {
		t.Fatalf("fresh signed session rejected: %v", err)
	}
	if calls != 1 {
		t.Fatalf("fresh session callback count=%d", calls)
	}
	calls = 0
	if err = leader.Effect(context.Background(), func() error { calls++; return nil }); !errors.Is(err, ErrRestricted) || calls != 0 {
		t.Fatalf("old owner should be fenced after signed handoff: calls=%d error=%v", calls, err)
	}
}

func TestLICA9Phase34RealManagerEffectRejectsExplicitRestrictedState(t *testing.T) {
	manager, _ := phase34LicensedManager(t)
	err := manager.lock(func() error {
		_, state, _, e := manager.load()
		if e != nil {
			return e
		}
		state.Restricted = "PHASE34_SYNTHETIC_RESTRICTED"
		return manager.write("state.json", state)
	})
	if err != nil {
		t.Fatalf("persist synthetic restriction: %v", err)
	}
	calls := 0
	err = manager.Effect(context.Background(), func() error { calls++; return nil })
	if !errors.Is(err, ErrRestricted) || calls != 0 {
		t.Fatalf("restricted state executed protected callback: calls=%d err=%v", calls, err)
	}
}

func TestLICA9Phase34RealManagerEffectDeniesExpiredTestLease(t *testing.T) {
	manager, _ := phase34LicensedManager(t)
	// The signed test-only V2 claims expire after 48h. Injecting validation
	// clock is permitted in TEST only, never in the Production Config.
	later := time.Now().Add(55 * time.Hour)
	manager.now = func() time.Time { return later }
	calls := 0
	err := manager.Effect(context.Background(), func() error { calls++; return nil })
	if !errors.Is(err, ErrRestricted) || calls != 0 {
		t.Fatalf("expired signed test lease executed: calls=%d err=%v", calls, err)
	}
}

func TestLICA9Phase34RealManagerEffectConcurrentFollowerNeverDispatches(t *testing.T) {
	leader, _ := phase34LicensedManager(t)
	follower, err := Open(leader.cfg)
	if err != nil {
		t.Fatal(err)
	}
	const n = 12
	var wg sync.WaitGroup
	var leaderCalls, followerCalls, leaderErrors, followerDenied atomic.Int64
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			if index%2 == 0 {
				err := leader.Effect(context.Background(), func() error {
					leaderCalls.Add(1)
					return nil
				})
				if err != nil {
					leaderErrors.Add(1)
				}
				return
			}
			err := follower.Effect(context.Background(), func() error {
				followerCalls.Add(1)
				return nil
			})
			if errors.Is(err, ErrRestricted) {
				followerDenied.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if leaderCalls.Load() != n/2 || followerCalls.Load() != 0 || leaderErrors.Load() != 0 || followerDenied.Load() != n/2 {
		t.Fatalf("real Manager.Effect session fence: leader=%d follower=%d leaderErrors=%d restricted=%d", leaderCalls.Load(), followerCalls.Load(), leaderErrors.Load(), followerDenied.Load())
	}
}
