package witness

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	owner "telmivo.local/lic-a9-phase30-owner-failover"
	"testing"
	"time"
)

func smoke(t *testing.T) (*Witness, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "safe")
	if e := os.Mkdir(root, 0700); e != nil {
		t.Fatal(e)
	}
	w, e := Open(root)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { w.Close() })
	return w, root
}
func TestP31FenceApplyAndReopen(t *testing.T) {
	w, root := smoke(t)
	if e := w.Fence(context.Background(), 1); e != nil {
		t.Fatal(e)
	}
	if e := w.Apply(context.Background(), 1, "recordA"); e != nil {
		t.Fatal(e)
	}
	other, e := Open(root)
	if e != nil {
		t.Fatal(e)
	}
	defer other.Close()
	n, e := other.CurrentEpoch()
	if e != nil || n != 1 {
		t.Fatal(n, e)
	}
	ok, e := other.WasApplied("recordA")
	if e != nil || !ok {
		t.Fatal(ok, e)
	}
}
func TestP31StaleOwnerAndReplayRejected(t *testing.T) {
	w, _ := smoke(t)
	ctx := context.Background()
	_ = w.Fence(ctx, 1)
	_ = w.Apply(ctx, 1, "recordA")
	if e := w.Apply(ctx, 1, "recordA"); !errors.Is(e, ErrDuplicate) {
		t.Fatal(e)
	}
	if e := w.Fence(ctx, 2); e != nil {
		t.Fatal(e)
	}
	if e := w.Apply(ctx, 1, "stale"); !errors.Is(e, ErrStale) {
		t.Fatal(e)
	}
	if e := w.Apply(ctx, 2, "recordA"); !errors.Is(e, ErrBadIntent) {
		t.Fatal(e)
	}
}
func TestP31MissingAndCorruptStateDeny(t *testing.T) {
	w, root := smoke(t)
	if e := os.WriteFile(filepath.Join(root, "epoch.json"), []byte("{"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := w.Fence(context.Background(), 1); !errors.Is(e, ErrCorrupt) {
		t.Fatal(e)
	}
	if _, e := Open(root); !errors.Is(e, ErrCorrupt) {
		t.Fatal(e)
	}
}
func TestP31ParallelHandlesOnlyOneApply(t *testing.T) {
	w, root := smoke(t)
	_ = w.Fence(context.Background(), 1)
	second, e := Open(root)
	if e != nil {
		t.Fatal(e)
	}
	defer second.Close()
	var wg sync.WaitGroup
	results := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cur := w
			if i%2 == 0 {
				cur = second
			}
			results <- cur.Apply(context.Background(), 1, "same")
		}(i)
	}
	wg.Wait()
	close(results)
	wins := 0
	dups := 0
	for e := range results {
		if e == nil {
			wins++
		} else if errors.Is(e, ErrDuplicate) {
			dups++
		} else {
			t.Fatal(e)
		}
	}
	if wins != 1 || dups != 19 {
		t.Fatal(wins, dups)
	}
}
func TestP31OwnerCanOnlyAdvanceOneEpoch(t *testing.T) {
	w, _ := smoke(t)
	ctx := context.Background()
	if e := w.Fence(ctx, 2); !errors.Is(e, ErrStale) {
		t.Fatal(e)
	}
	_ = w.Fence(ctx, 1)
	if e := w.Fence(ctx, 1); !errors.Is(e, ErrStale) {
		t.Fatal(e)
	}
}
func TestP31ChildProcess(t *testing.T) {
	if os.Getenv("P31_SMOKE_CHILD") != "1" {
		return
	}
	w, e := Open(os.Getenv("P31_SMOKE_ROOT"))
	if e != nil {
		t.Fatal(e)
	}
	defer w.Close()
	switch os.Getenv("P31_SMOKE_MODE") {
	case "fence":
		e = w.Fence(context.Background(), 1)
	case "apply":
		e = w.Apply(context.Background(), 1, "same")
	case "crash":
		if e = w.Fence(context.Background(), 1); e != nil {
			t.Fatal(e)
		}
		os.Exit(77)
	default:
		t.Fatal("unknown mode")
	}
	if e == nil {
		t.Log("P31_WIN")
		return
	}
	if errors.Is(e, ErrStale) || errors.Is(e, ErrDuplicate) {
		t.Log("P31_LOSE")
		return
	}
	t.Fatal(e)
}
func smokeChild(root, mode string) ([]byte, error) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestP31ChildProcess$", "-test.v")
	cmd.Env = append(os.Environ(), "P31_SMOKE_CHILD=1", "P31_SMOKE_ROOT="+root, "P31_SMOKE_MODE="+mode)
	return cmd.CombinedOutput()
}
func TestP31TwoRealProcessesShareOneWinner(t *testing.T) {
	_, root := smoke(t)
	out := make([][]byte, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); out[i], errs[i] = smokeChild(root, "fence") }(i)
	}
	wg.Wait()
	winners := 0
	losers := 0
	for i := 0; i < 2; i++ {
		if errs[i] != nil {
			t.Fatal(errs[i], string(out[i]))
		}
		if bytes.Contains(out[i], []byte("P31_WIN")) {
			winners++
		}
		if bytes.Contains(out[i], []byte("P31_LOSE")) {
			losers++
		}
	}
	if winners != 1 || losers != 1 {
		t.Fatal(winners, losers)
	}
}
func TestP31AbruptExitRetainsEpoch(t *testing.T) {
	w, root := smoke(t)
	out, e := smokeChild(root, "crash")
	var exit *exec.ExitError
	if !errors.As(e, &exit) || exit.ExitCode() != 77 {
		t.Fatal(e, string(out))
	}
	n, e := w.CurrentEpoch()
	if e != nil || n != 1 {
		t.Fatal(n, e)
	}
}
func TestP31SupervisorHandoffUsesCommonWitness(t *testing.T) {
	w, _ := smoke(t)
	dir := filepath.Join(t.TempDir(), "supervisor")
	if e := os.Mkdir(dir, 0700); e != nil {
		t.Fatal(e)
	}
	_, sk, _ := ed25519.GenerateKey(rand.Reader)
	s, e := owner.Open(dir, "installationA", sk.Public().(ed25519.PublicKey))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	now := time.Now().Unix()
	mk := func(prev uint64, name string) owner.Grant {
		return owner.SignTestGrant(sk, owner.Grant{Protocol: owner.Protocol, Installation: "installationA", Owner: name, Session: "session" + name, PreviousEpoch: prev, Epoch: prev + 1, NotBeforeUnix: now - 1, ExpiresUnix: now + 120})
	}
	ctx := context.Background()
	if e := s.Acquire(ctx, mk(0, "A"), w); e != nil {
		t.Fatal(e)
	}
	if e := s.Acquire(ctx, mk(1, "B"), w); e != nil {
		t.Fatal(e)
	}
	if e := w.Apply(ctx, 1, "old"); !errors.Is(e, ErrStale) {
		t.Fatal(e)
	}
	if e := w.Apply(ctx, 2, "new"); e != nil {
		t.Fatal(e)
	}
}

// A success here REPRODUCES an UNSAFE scenario. It does not certify security.
func TestP31KnownSameUIDDirectoryReplacement_SECURITY_BLOCKER(t *testing.T) {
	w, root := smoke(t)
	_ = w.Fence(context.Background(), 1)
	if e := os.Remove(filepath.Join(root, "witness.lock")); e != nil {
		t.Fatal(e)
	}
	if e := os.Remove(filepath.Join(root, "epoch.json")); e != nil {
		t.Fatal(e)
	}
	replacement, e := Open(root)
	if e != nil {
		t.Fatal(e)
	}
	defer replacement.Close()
	epoch, e := replacement.CurrentEpoch()
	if e != nil || epoch != 0 {
		t.Fatal(epoch, e)
	}
	t.Log("SECURITY BLOCKER: a hostile same-UID writer can replace witness paths")
}
