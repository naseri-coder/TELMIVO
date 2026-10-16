package ownerproof

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	ledger "telmivo.local/lic-a9-phase29-outcome-ledger"
)

type childBundle struct {
	Root            string
	Journal         string
	AuthorityPublic ed25519.PublicKey
	ProviderPublic  ed25519.PublicKey
	Grant           Grant
}

func spawnBundle(t *testing.T, f *fixture, g Grant, journalDir string) string {
	t.Helper()
	b, err := json.Marshal(childBundle{Root: f.root, Journal: journalDir, AuthorityPublic: f.authorityKey.Public().(ed25519.PublicKey), ProviderPublic: f.providerKey.Public().(ed25519.PublicKey), Grant: g})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "test-bundle.json")
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func readBundle(t *testing.T) childBundle {
	t.Helper()
	b, e := os.ReadFile(os.Getenv("PHASE30_BUNDLE"))
	if e != nil {
		t.Fatal(e)
	}
	var bundle childBundle
	if e = json.Unmarshal(b, &bundle); e != nil {
		t.Fatal(e)
	}
	return bundle
}

func TestSubprocessReserveAndAbruptExit(t *testing.T) {
	if os.Getenv("PHASE30_CHILD_CRASH") != "1" {
		return
	}
	x := readBundle(t)
	s, e := Open(x.Root, "installationA", x.AuthorityPublic)
	if e != nil {
		t.Fatal(e)
	}
	j, e := ledger.Open(x.Journal, x.ProviderPublic)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Begin(x.Grant, intent("abruptChild"), j); e != nil {
		t.Fatal(e)
	}
	// Real process exits after both durable writes, with no cleanup/Settle.
	os.Exit(77)
}
func TestRealProcessCrashAfterPendingReservation(t *testing.T) {
	f := setup(t)
	a := grant(f, "ownerA", "sessionA", 0)
	acquire(t, f, a)
	journalDir := filepath.Join(t.TempDir(), "journal-child")
	if e := os.Mkdir(journalDir, 0700); e != nil {
		t.Fatal(e)
	}
	bundle := spawnBundle(t, f, a, journalDir)
	cmd := exec.Command(os.Args[0], "-test.run=^TestSubprocessReserveAndAbruptExit$")
	cmd.Env = append(os.Environ(), "PHASE30_CHILD_CRASH=1", "PHASE30_BUNDLE="+bundle)
	output, e := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(e, &exitErr) || exitErr.ExitCode() != 77 {
		t.Fatalf("child did not die on sentinel: %v %s", e, output)
	}
	n, e := f.owner.PendingCount()
	if e != nil || n != 1 {
		t.Fatal(n, e)
	}
	j, e := ledger.Open(journalDir, f.providerKey.Public().(ed25519.PublicKey))
	if e != nil {
		t.Fatal(e)
	}
	defer j.Close()
	status, e := j.Inspect("abruptChild")
	if e != nil || status != ledger.ReconcileRequired {
		t.Fatal(status, e)
	}
	if e := f.owner.Acquire(context.Background(), grant(f, "ownerB", "sessionB", 1), f.provider); !errors.Is(e, ErrPending) {
		t.Fatal(e)
	}
}
func TestSubprocessTryAcquire(t *testing.T) {
	if os.Getenv("PHASE30_CHILD_ACQUIRE") != "1" {
		return
	}
	x := readBundle(t)
	s, e := Open(x.Root, "installationA", x.AuthorityPublic)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	err := s.Acquire(context.Background(), x.Grant, NewTestProvider(true))
	if err == nil {
		t.Log("PHASE30_WIN")
		return
	}
	if errors.Is(err, ErrStale) {
		t.Log("PHASE30_STALE")
		return
	}
	t.Fatal(err)
}
func TestTwoProcessesOneMonotonicWinner(t *testing.T) {
	f := setup(t)
	a := grant(f, "ownerA", "sessionA", 0)
	b := grant(f, "ownerB", "sessionB", 0)
	bs := []string{spawnBundle(t, f, a, ""), spawnBundle(t, f, b, "")}
	var wg sync.WaitGroup
	outputs := make([][]byte, 2)
	errs := make([]error, 2)
	for n := 0; n < 2; n++ {
		wg.Add(1)
		go func(j int) {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run=^TestSubprocessTryAcquire$", "-test.v")
			cmd.Env = append(os.Environ(), "PHASE30_CHILD_ACQUIRE=1", "PHASE30_BUNDLE="+bs[j])
			outputs[j], errs[j] = cmd.CombinedOutput()
		}(n)
	}
	wg.Wait()
	wins, stales := 0, 0
	for n := 0; n < 2; n++ {
		if errs[n] != nil {
			t.Fatalf("child failure: %v %s", errs[n], outputs[n])
		}
		if bytes.Contains(outputs[n], []byte("PHASE30_WIN")) {
			wins++
		}
		if bytes.Contains(outputs[n], []byte("PHASE30_STALE")) {
			stales++
		}
	}
	if wins != 1 || stales != 1 {
		t.Fatalf("wins=%d stale=%d", wins, stales)
	}
	epoch, e := f.owner.CurrentEpoch()
	if e != nil || epoch != 1 {
		t.Fatal(epoch, e)
	}
}
