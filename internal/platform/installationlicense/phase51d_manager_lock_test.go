package installationlicense

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// These direct-lock tests exercise the genuine Manager file-path/hardlink
// guards without needing any signer, credential, provider or Authority server.
func phase51dLockManager(t *testing.T) *Manager {
	t.Helper()
	return &Manager{cfg: Config{Directory: t.TempDir()}}
}

func TestLICA9Phase51DManagerBoundedAdmission(t *testing.T) {
	m := phase51dLockManager(t)
	called := 0
	err := m.lockModeWithWait(context.Background(), syscall.LOCK_EX, time.Second, ErrRestricted, func() error {
		called++
		return nil
	})
	if err != nil || called != 1 {
		t.Fatalf("lock admission failed: err=%v called=%d", err, called)
	}
}

func TestLICA9Phase51DNestedEffectCannotEnter(t *testing.T) {
	m := phase51dLockManager(t)
	called := false
	err := m.lockModeWithWait(context.Background(), syscall.LOCK_EX, 800*time.Millisecond, ErrRestricted, func() error {
		inner := m.lockModeWithWait(context.Background(), syscall.LOCK_EX, 70*time.Millisecond, ErrRestricted, func() error {
			called = true
			return nil
		})
		if !errors.Is(inner, ErrRestricted) {
			t.Errorf("nested effect not restricted: %v", inner)
		}
		return nil
	})
	if err != nil || called {
		t.Fatalf("nested lock admitted callback: err=%v called=%v", err, called)
	}
}

func TestLICA9Phase51DNestedAcceptRemainsPending(t *testing.T) {
	m := phase51dLockManager(t)
	mutated := false
	err := m.lockModeWithWait(context.Background(), syscall.LOCK_EX, 500*time.Millisecond, ErrRestricted, func() error {
		inner := m.lockModeWithWait(context.Background(), syscall.LOCK_EX, 80*time.Millisecond, ErrPending, func() error {
			mutated = true
			return nil
		})
		if !errors.Is(inner, ErrPending) {
			t.Errorf("nested accept unexpectedly admitted: %v", inner)
		}
		return nil
	})
	if err != nil || mutated {
		t.Fatalf("nested lock changed state: err=%v mutated=%v", err, mutated)
	}
}

func TestLICA9Phase51DManagerHardlinkRejected(t *testing.T) {
	m := phase51dLockManager(t)
	p := filepath.Join(m.cfg.Directory, "lock")
	if err := os.WriteFile(p, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(p, filepath.Join(m.cfg.Directory, "other")); err != nil {
		t.Fatal(err)
	}
	called := false
	err := m.lockModeWithWait(context.Background(), syscall.LOCK_EX, time.Second, ErrRestricted, func() error {
		called = true
		return nil
	})
	if !errors.Is(err, ErrState) || called {
		t.Fatalf("hardlink escaped: err=%v called=%v", err, called)
	}
}

func TestLICA9Phase51DManagerSymlinkRejected(t *testing.T) {
	m := phase51dLockManager(t)
	p := filepath.Join(m.cfg.Directory, "real")
	if err := os.WriteFile(p, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(p, filepath.Join(m.cfg.Directory, "lock")); err != nil {
		t.Fatal(err)
	}
	called := false
	err := m.lockModeWithWait(context.Background(), syscall.LOCK_EX, time.Second, ErrRestricted, func() error {
		called = true
		return nil
	})
	if !errors.Is(err, ErrState) || called {
		t.Fatalf("symlink escaped: err=%v called=%v", err, called)
	}
}

func TestLICA9Phase51DManagerCancelledContextDenied(t *testing.T) {
	m := phase51dLockManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := m.lockModeWithWait(ctx, syscall.LOCK_EX, time.Second, ErrRestricted, func() error {
		called = true
		return nil
	})
	if !errors.Is(err, context.Canceled) || called {
		t.Fatalf("cancellation escaped: err=%v called=%v", err, called)
	}
}
