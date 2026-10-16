package installationlicense

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

// These signed V2 tests use the in-memory throwaway Ed25519 issuer established
// by Phase19 and temporary installation directories. They do NOT certify
// Production, genuine Authority/PG16 behavior, or simultaneous worker liveness.
// In particular, a restricted follower here is a documented AVAILABILITY GAP,
// not evidence that a valid multi-worker installation is release-ready.
func phase22SharedInstallationSessions(t *testing.T) (*Manager, *Manager, func(string, int64) []byte) {
	t.Helper()
	leader, signTestLease := phase19TestSignedConcurrentInstallation(t)
	if err := leader.Accept(signTestLease(leader.runtimeSession, 1)); err != nil {
		t.Fatal("phase22 initial synthetic signed lease:", err)
	}
	follower, err := Open(leader.cfg)
	if err != nil {
		t.Fatal("phase22 follower open:", err)
	}
	if follower.runtimeSession == leader.runtimeSession {
		t.Fatal("distinct worker runtimes unexpectedly share a session ID")
	}
	return leader, follower, signTestLease
}

func TestLICA9Phase22DistinctWorkersShareLeaseOnlyOneMayExecute(t *testing.T) {
	leader, follower, _ := phase22SharedInstallationSessions(t)
	if d := leader.Decision(); d.State != "VALID_ONLINE" {
		t.Fatalf("leader unexpectedly restricted: %+v", d)
	}
	if d := follower.Decision(); d.State != "RESTRICTED" || d.Reason != "CONCURRENCY_REFRESH_REQUIRED" || !d.RefreshDue {
		t.Fatalf("distinct runtime must not reuse leader signed lease: %+v", d)
	}
	called := false
	if err := follower.Effect(context.Background(), func() error { called = true; return nil }); !errors.Is(err, ErrRestricted) || called {
		t.Fatalf("follower reused lease: effect=%t, err=%v", called, err)
	}
	// This test intentionally exposes the incompatible availability contract.
	// It MUST NOT be interpreted as acceptance of simultaneous worker service.
	t.Log("LIC_A9_PHASE22_MULTIWORKER_LIVENESS_GAP_STILL_OPEN")
}

func TestLICA9Phase22RefreshLeaderLockAndOtherSessionRestriction(t *testing.T) {
	leader, follower, _ := phase22SharedInstallationSessions(t)
	first, err := leader.acquireRefreshLeader()
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	other, err := follower.acquireRefreshLeader()
	if other != nil {
		other.Close()
		t.Fatal("two refresh leaders admitted")
	}
	if !errors.Is(err, ErrPending) {
		t.Fatalf("follower unexpectedly acquired refresh leader lock: %v", err)
	}
	if d := follower.Decision(); d.State != "RESTRICTED" {
		t.Fatalf("follower was silently authorized: %+v", d)
	}
	t.Log("LIC_A9_PHASE22_SINGLE_REFRESH_LEADER_AND_SESSION_FENCE_CONFLICT_UNRESOLVED")
}

func TestLICA9Phase22RenewalTransfersButCannotSimultaneouslyAuthorizeWorkers(t *testing.T) {
	leader, follower, signTestLease := phase22SharedInstallationSessions(t)
	if err := follower.Accept(signTestLease(follower.runtimeSession, 2)); err != nil {
		t.Fatalf("synthetic signed follower lease: %v", err)
	}
	if d := follower.Decision(); d.State != "VALID_ONLINE" {
		t.Fatalf("signed follower session unexpectedly denied: %+v", d)
	}
	if d := leader.Decision(); d.State != "RESTRICTED" || d.Reason != "CONCURRENCY_REFRESH_REQUIRED" {
		t.Fatalf("old leader improperly retained simultaneous authorization: %+v", d)
	}
	called := false
	if err := leader.Effect(context.Background(), func() error { called = true; return nil }); !errors.Is(err, ErrRestricted) || called {
		t.Fatalf("old leader performed commercial effect after reassignment: %t, %v", called, err)
	}
	t.Log("LIC_A9_PHASE22_SIGNED_SESSION_HANDOFF_REVOKES_PREVIOUS_WORKER")
}

func TestLICA9Phase22ParallelStaleWorkerEffectsAlwaysFailClosed(t *testing.T) {
	leader, follower, _ := phase22SharedInstallationSessions(t)
	var good, bad, denied atomic.Int64
	var workers sync.WaitGroup
	for n := 0; n < 24; n++ {
		workers.Add(1)
		go func(idx int) {
			defer workers.Done()
			if idx%2 == 0 {
				if err := leader.Effect(context.Background(), func() error { good.Add(1); return nil }); err != nil {
					bad.Add(1)
				}
				return
			}
			executed := false
			err := follower.Effect(context.Background(), func() error { executed = true; return nil })
			if !errors.Is(err, ErrRestricted) || executed {
				bad.Add(1)
			} else {
				denied.Add(1)
			}
		}(n)
	}
	workers.Wait()
	if good.Load() != 12 || denied.Load() != 12 || bad.Load() != 0 {
		t.Fatalf("concurrent leader %d, follower denied %d, unexpected %d", good.Load(), denied.Load(), bad.Load())
	}
}
