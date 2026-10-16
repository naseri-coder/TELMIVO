package installationlicense

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

func phase51dLockFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(t.TempDir(), "flock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestLICA9Phase51DUncontendedExclusiveFlock(t *testing.T) {
	f := phase51dLockFile(t)
	if err := acquireExclusiveFlock(context.Background(), int(f.Fd()), time.Second, ErrRestricted); err != nil {
		t.Fatal(err)
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

func TestLICA9Phase51DNestedExclusiveFlockIsBounded(t *testing.T) {
	a := phase51dLockFile(t)
	b, err := os.OpenFile(a.Name(), os.O_RDWR, 0)
	if err != nil { t.Fatal(err) }
	defer b.Close()
	if err = syscall.Flock(int(a.Fd()), syscall.LOCK_EX); err != nil { t.Fatal(err) }
	start := time.Now()
	err = acquireExclusiveFlock(context.Background(), int(b.Fd()), 65*time.Millisecond, ErrRestricted)
	if !errors.Is(err, ErrRestricted) { t.Fatalf("lock admission: %v", err) }
	if time.Since(start) > 500*time.Millisecond { t.Fatal("nested admission remained blocked") }
	_ = syscall.Flock(int(a.Fd()), syscall.LOCK_UN)
}

func TestLICA9Phase51DExclusiveFlockContextCancellation(t *testing.T) {
	a := phase51dLockFile(t)
	b, err := os.OpenFile(a.Name(), os.O_RDWR, 0)
	if err != nil { t.Fatal(err) }
	defer b.Close()
	_ = syscall.Flock(int(a.Fd()), syscall.LOCK_EX)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(45*time.Millisecond); cancel() }()
	err = acquireExclusiveFlock(ctx, int(b.Fd()), time.Second, ErrRestricted)
	if !errors.Is(err, context.Canceled) { t.Fatalf("lock admission: %v", err) }
	_ = syscall.Flock(int(a.Fd()), syscall.LOCK_UN)
}

func TestLICA9Phase51DExclusiveFlockWaitsForRelease(t *testing.T) {
	a := phase51dLockFile(t)
	b, err := os.OpenFile(a.Name(), os.O_RDWR, 0)
	if err != nil { t.Fatal(err) }
	defer b.Close()
	_ = syscall.Flock(int(a.Fd()), syscall.LOCK_EX)
	var group sync.WaitGroup
	group.Add(1)
	go func() {
		defer group.Done()
		time.Sleep(45*time.Millisecond)
		_ = syscall.Flock(int(a.Fd()), syscall.LOCK_UN)
	}()
	if err = acquireExclusiveFlock(context.Background(), int(b.Fd()), time.Second, ErrPending); err != nil {
		t.Fatal(err)
	}
	_ = syscall.Flock(int(b.Fd()), syscall.LOCK_UN)
	group.Wait()
}

func TestLICA9Phase51DExclusiveFlockRejectsInvalidArguments(t *testing.T) {
	f := phase51dLockFile(t)
	for _, candidate := range []struct {
		ctx context.Context
		wait time.Duration
		busy error
	}{
		{nil, time.Second, ErrPending},
		{context.Background(), 0, ErrPending},
		{context.Background(), time.Second, nil},
	} {
		if err := acquireExclusiveFlock(candidate.ctx, int(f.Fd()), candidate.wait, candidate.busy); !errors.Is(err, ErrState) {
			t.Fatalf("invalid lock argument: %v", err)
		}
	}
}
