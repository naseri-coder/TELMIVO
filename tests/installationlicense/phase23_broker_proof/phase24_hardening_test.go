//go:build linux

package brokerproof

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPhase24LiveOwnerCannotBeReplacedAfterSocketUnlinked(t *testing.T) {
	root := filepath.Join(t.TempDir(), "owner")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	var a, b atomic.Int64
	one, err := Open(root, allowed, func() error { a.Add(1); return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer one.Close()
	go one.Serve()
	if err := os.Remove(one.Address()); err != nil {
		t.Fatal(err)
	}
	two, err := Open(root, allowed, func() error { b.Add(1); return nil })
	if err == nil {
		_ = two.Close()
		t.Fatal("SECOND_BROKER_OWNER_ADMITTED_WITH_LIVE_FIRST_OWNER")
	}
}

func TestPhase24SymlinkedAncestorCannotRedirectBrokerRoot(t *testing.T) {
	outer := t.TempDir()
	actual := filepath.Join(outer, "actual")
	if err := os.Mkdir(actual, 0700); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(actual, "isolated")
	if err := os.Mkdir(original, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(outer, "alias")
	if err := os.Symlink(actual, alias); err != nil {
		t.Fatal(err)
	}
	broker, err := Open(filepath.Join(alias, "isolated"), allowed, func() error { return nil })
	if err == nil {
		_ = broker.Close()
		t.Fatal("SYMLINKED_ANCESTOR_REDIRECTION_ACCEPTED")
	}
}

func TestPhase24QueuedRequestMustNotExecuteAfterShutdownInitiated(t *testing.T) {
	root := filepath.Join(t.TempDir(), "shutdown")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	var count atomic.Int64
	b, err := Open(root, func(_ context.Context, fn func() error) error { return fn() }, func() error { count.Add(1); return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	seen := make(chan struct{})
	b.peerUID = func(*net.UnixConn) (uint32, error) { close(seen); return uint32(os.Geteuid()), nil }
	go b.Serve()
	b.mu.Lock()
	conn, err := net.DialTimeout("unix", b.Address(), time.Second)
	if err != nil {
		b.mu.Unlock()
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	_, err = conn.Write([]byte(`{"protocol":"` + protocol + `","operation":"` + operation + `","id":"` + strings.Repeat("8", 32) + `"}` + "\n"))
	if err != nil {
		b.mu.Unlock()
		t.Fatal(err)
	}
	select {
	case <-seen:
	case <-time.After(2 * time.Second):
		b.mu.Unlock()
		t.Fatal("peer not checked")
	}
	// The broker has parsed the peer, and ordinarily enters the mutex queue.
	time.Sleep(50 * time.Millisecond)
	closing := make(chan struct{})
	go func() { _ = b.Close(); close(closing) }()
	select {
	case <-b.cancel:
	case <-time.After(2 * time.Second):
		b.mu.Unlock()
		t.Fatal("shutdown did not begin")
	}
	b.mu.Unlock()
	buf := make([]byte, 128)
	n, _ := conn.Read(buf)
	select {
	case <-closing:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown incomplete")
	}
	if count.Load() != 0 {
		t.Fatalf("REQUEST_EXECUTED_AFTER_SHUTDOWN: %d", count.Load())
	}
	if n > 0 && !strings.Contains(string(buf[:n]), "DENIED_CLOSED") {
		t.Fatalf("unexpected shutdown result %q", string(buf[:n]))
	}
}

func TestPhase24OwnerLockRejectsSymlinkHardlinkAndRelaxedModes(t *testing.T) {
	for _, mode := range []string{"symlink", "hardlink", "relaxed"} {
		t.Run(mode, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "owner-security")
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			owner := filepath.Join(root, "broker-owner")
			target := filepath.Join(root, "other")
			if err := os.WriteFile(target, []byte("test"), 0600); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "symlink":
				if err := os.Symlink(target, owner); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(target, owner); err != nil {
					t.Fatal(err)
				}
			case "relaxed":
				if err := os.WriteFile(owner, []byte("test"), 0644); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(owner, 0644); err != nil {
					t.Fatal(err)
				}
			}
			b, err := Open(root, allowed, func() error { return nil })
			if err == nil {
				_ = b.Close()
				t.Fatal("unsafe broker-owner accepted")
			}
		})
	}
}

func TestPhase24OwnerLockRetainedUntilInflightEffectDrained(t *testing.T) {
	root := filepath.Join(t.TempDir(), "inflight")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	gate := func(_ context.Context, fn func() error) error {
		close(entered)
		<-release
		return fn()
	}
	var count atomic.Int64
	b, err := Open(root, gate, func() error { count.Add(1); return nil })
	if err != nil {
		t.Fatal(err)
	}
	go b.Serve()
	response := make(chan string, 1)
	go func() { s, _ := Call(b.Address(), req(strings.Repeat("2", 32))); response <- s }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("effect not reached")
	}
	// Remove only the Unix listener path to simulate an accidentally lost path;
	// the locked broker-owner inode must still prevent two active owners.
	if err := os.Remove(b.Address()); err != nil {
		t.Fatal(err)
	}
	closed := make(chan struct{})
	go func() { _ = b.Close(); close(closed) }()
	select {
	case <-b.cancel:
	case <-time.After(2 * time.Second):
		t.Fatal("Close not initiated")
	}
	other, err := Open(root, allowed, func() error { return nil })
	if err == nil {
		_ = other.Close()
		t.Fatal("overlapping owner admitted during inflight effect")
	}
	close(release)
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not drain")
	}
	if count.Load() != 1 {
		t.Fatalf("original effect count %d", count.Load())
	}
	<-response
	// After Close finishes all handlers, the exclusive owner may be re-opened.
	after, err := Open(root, allowed, func() error { return nil })
	if err != nil {
		t.Fatalf("ownership never released after drain: %v", err)
	}
	_ = after.Close()
}

func TestPhase24AmbiguousEffectMayNotRetrySameID(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ambiguous")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	var effects atomic.Int64
	b, err := Open(root, allowed, func() error { effects.Add(1); return errors.New("test-only ambiguous downstream") })
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	go b.Serve()
	id := req(strings.Repeat("3", 32))
	first, err := Call(b.Address(), id)
	if err != nil || first != "DENIED_LICENSE_OR_EFFECT" {
		t.Fatalf("ambiguous first %q %v", first, err)
	}
	second, err := Call(b.Address(), id)
	if err != nil || second != "DENIED_DUPLICATE_OR_IO" {
		t.Fatalf("unexpected retry %q %v", second, err)
	}
	if effects.Load() != 1 {
		t.Fatalf("duplicate effect count %d", effects.Load())
	}
}

func TestPhase24ConcurrentCloseIsRaceFreeAndIdempotent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "closing")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	b, err := Open(root, allowed, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	go b.Serve()
	var wg sync.WaitGroup
	wg.Add(8)
	for i := 0; i < 8; i++ {
		go func() { defer wg.Done(); _ = b.Close() }()
	}
	wg.Wait()
	renewed, err := Open(root, allowed, func() error { return nil })
	if err != nil {
		t.Fatalf("concurrent Close leaked owner lock: %v", err)
	}
	_ = renewed.Close()
}
