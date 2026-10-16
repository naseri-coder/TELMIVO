package ledger

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	effectgate "telmivo.local/lic-a9-phase28-effect-proof"
)

func intent(id string) effectgate.Intent {
	return effectgate.Intent{RequestID: id, WorkerID: "workerA", WorkspaceID: "tenantA", ActorID: "actorA", ResourceID: "resourceA", Operation: effectgate.SyntheticOperation}
}
func fixture(t *testing.T) (*Ledger, ed25519.PrivateKey, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "private-ledger")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	l, err := Open(dir, pub)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l, private, dir
}
func TestReservationInspectRequiresReconciliation(t *testing.T) {
	l, _, _ := fixture(t)
	i := intent("reqA")
	if err := l.Reserve(i); err != nil {
		t.Fatal(err)
	}
	status, err := l.Inspect(i.RequestID)
	if err != nil || status != ReconcileRequired {
		t.Fatalf("status=%s err=%v", status, err)
	}
}
func TestDuplicateSameAndCrossTenantConflict(t *testing.T) {
	l, _, _ := fixture(t)
	i := intent("reqA")
	if err := l.Reserve(i); err != nil {
		t.Fatal(err)
	}
	if err := l.Reserve(i); !errors.Is(err, ErrDuplicate) {
		t.Fatal(err)
	}
	i.WorkspaceID = "tenantB"
	if err := l.Reserve(i); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
}
func TestSignedEvidenceConfirms(t *testing.T) {
	l, priv, _ := fixture(t)
	i := intent("reqA")
	if err := l.Reserve(i); err != nil {
		t.Fatal(err)
	}
	signed := SyntheticProof(priv, i, "providerReceiptA")
	if err := l.Confirm(signed); err != nil {
		t.Fatal(err)
	}
	status, err := l.Inspect(i.RequestID)
	if err != nil || status != Confirmed {
		t.Fatal(status, err)
	}
	if err := l.Confirm(signed); err != nil {
		t.Fatal(err)
	}
}
func TestInvalidProviderSignatureRejected(t *testing.T) {
	l, priv, _ := fixture(t)
	i := intent("reqA")
	if err := l.Reserve(i); err != nil {
		t.Fatal(err)
	}
	bad := SyntheticProof(priv, i, "providerA")
	bad.Signature = strings.Repeat("0", 128)
	if err := l.Confirm(bad); !errors.Is(err, ErrBadProof) {
		t.Fatal(err)
	}
	status, err := l.Inspect(i.RequestID)
	if err != nil || status != ReconcileRequired {
		t.Fatal(status, err)
	}
}
func TestOtherProviderKeyRejected(t *testing.T) {
	l, _, _ := fixture(t)
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	i := intent("reqA")
	if err := l.Reserve(i); err != nil {
		t.Fatal(err)
	}
	if err := l.Confirm(SyntheticProof(other, i, "providerA")); !errors.Is(err, ErrBadProof) {
		t.Fatal(err)
	}
}
func TestEvidenceBoundToOriginalFullIntent(t *testing.T) {
	l, priv, _ := fixture(t)
	i := intent("reqA")
	if err := l.Reserve(i); err != nil {
		t.Fatal(err)
	}
	hijacked := i
	hijacked.WorkspaceID = "tenantB"
	if err := l.Confirm(SyntheticProof(priv, hijacked, "providerA")); !errors.Is(err, ErrBadProof) {
		t.Fatal(err)
	}
}
func TestUnverifiedReceiptCannotRewriteKnownOutcome(t *testing.T) {
	l, priv, _ := fixture(t)
	i := intent("reqA")
	if err := l.Reserve(i); err != nil {
		t.Fatal(err)
	}
	good := SyntheticProof(priv, i, "providerA")
	if err := l.Confirm(good); err != nil {
		t.Fatal(err)
	}
	conflict := SyntheticProof(priv, i, "providerB")
	if err := l.Confirm(conflict); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
}
func TestRestartPreservesUnknownAndConfirmation(t *testing.T) {
	l, priv, dir := fixture(t)
	i := intent("reqA")
	if err := l.Reserve(i); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir, priv.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.Reserve(i); !errors.Is(err, ErrDuplicate) {
		t.Fatal(err)
	}
	status, err := reopened.Inspect(i.RequestID)
	if err != nil || status != ReconcileRequired {
		t.Fatal(status, err)
	}
	if err := reopened.Confirm(SyntheticProof(priv, i, "providerA")); err != nil {
		t.Fatal(err)
	}
	status, err = l.Inspect(i.RequestID)
	if err != nil || status != Confirmed {
		t.Fatal(status, err)
	}
}
func TestPartialReservedIntentFailClosed(t *testing.T) {
	l, _, dir := fixture(t)
	name := fileName("i-", "reqA")
	if err := os.WriteFile(filepath.Join(dir, name), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := l.Reserve(intent("reqA")); !errors.Is(err, ErrCorrupt) {
		t.Fatal(err)
	}
	if _, err := l.Inspect("reqA"); !errors.Is(err, ErrCorrupt) {
		t.Fatal(err)
	}
}
func TestCorruptReceiptDoesNotBecomeSuccess(t *testing.T) {
	l, _, dir := fixture(t)
	i := intent("reqA")
	if err := l.Reserve(i); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, fileName("o-", i.RequestID)), []byte("{\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Inspect(i.RequestID); !errors.Is(err, ErrCorrupt) {
		t.Fatal(err)
	}
}
func TestSymlinkJournalAndHardlinkBothDenied(t *testing.T) {
	l, _, dir := fixture(t)
	i := intent("reqA")
	foreign := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(foreign, []byte("PRIVATE"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, fileName("i-", i.RequestID))
	if err := os.Symlink(foreign, path); err != nil {
		t.Fatal(err)
	}
	if err := l.Reserve(i); !errors.Is(err, ErrCorrupt) {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(foreign)
	if !bytes.Equal(b, []byte("PRIVATE")) {
		t.Fatal("target altered")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(foreign, path); err != nil {
		t.Fatal(err)
	}
	if err := l.Reserve(i); !errors.Is(err, ErrCorrupt) {
		t.Fatal(err)
	}
}
func TestUnsafeDirectoryRejected(t *testing.T) {
	_, priv, _ := fixture(t)
	dir := filepath.Join(t.TempDir(), "bad-root")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if l, err := Open(dir, priv.Public().(ed25519.PublicKey)); l != nil || !errors.Is(err, ErrUnsafeRoot) {
		t.Fatal(err)
	}
}
func TestUnknownIDDoesNotPermitEffect(t *testing.T) {
	l, _, _ := fixture(t)
	if _, err := l.Inspect("missing"); !errors.Is(err, ErrReconcile) {
		t.Fatal(err)
	}
	if err := l.Confirm(Outcome{RequestID: "missing"}); !errors.Is(err, ErrCorrupt) {
		t.Fatal(err)
	}
}
func TestNoArbitraryOperations(t *testing.T) {
	l, _, _ := fixture(t)
	i := intent("reqA")
	i.Operation = "PAYMENT_TRANSFER"
	if err := l.Reserve(i); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
}
func TestParallelIndependentHandlesSerialize(t *testing.T) {
	l, priv, dir := fixture(t)
	other, err := Open(dir, priv.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	var wg sync.WaitGroup
	var ok, dup atomic.Int32
	for j := 0; j < 32; j++ {
		wg.Add(1)
		go func(j int) {
			defer wg.Done()
			handle := l
			if j%2 == 0 {
				handle = other
			}
			err := handle.Reserve(intent("reqA"))
			if err == nil {
				ok.Add(1)
			} else if errors.Is(err, ErrDuplicate) {
				dup.Add(1)
			} else {
				t.Error(err)
			}
		}(j)
	}
	wg.Wait()
	if ok.Load() != 1 || dup.Load() != 31 {
		t.Fatalf("ok=%d duplicate=%d", ok.Load(), dup.Load())
	}
}

type mockManager struct{ ok bool }

func (m mockManager) Effect(ctx context.Context, fn func() error) error {
	if !m.ok {
		return effectgate.ErrDenied
	}
	return fn()
}

type mockDomain struct{ ok bool }

func (m mockDomain) FinalRecheck(context.Context, effectgate.Intent) error {
	if !m.ok {
		return effectgate.ErrDenied
	}
	return nil
}
func TestPhase28GatewayUsesDurableLedgerAfterRestart(t *testing.T) {
	l, priv, dir := fixture(t)
	var callbacks atomic.Int32
	gate, err := effectgate.New(mockManager{true}, mockDomain{true}, l, func(context.Context, effectgate.Intent) error {
		callbacks.Add(1)
		return errors.New("timeout ambiguous")
	})
	if err != nil {
		t.Fatal(err)
	}
	i := intent("reqA")
	if err := gate.Execute(context.Background(), i); !errors.Is(err, effectgate.ErrUncertain) {
		t.Fatal(err)
	}
	reopened, err := Open(dir, priv.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	second, err := effectgate.New(mockManager{true}, mockDomain{true}, reopened, func(context.Context, effectgate.Intent) error { callbacks.Add(1); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Execute(context.Background(), i); !errors.Is(err, ErrDuplicate) {
		t.Fatal(err)
	}
	if callbacks.Load() != 1 {
		t.Fatal("blind retry occurred", callbacks.Load())
	}
	status, err := reopened.Inspect(i.RequestID)
	if err != nil || status != ReconcileRequired {
		t.Fatal(status, err)
	}
}
func TestPhase28LicenseGateDenialNeverReserves(t *testing.T) {
	l, _, _ := fixture(t)
	gate, _ := effectgate.New(mockManager{false}, mockDomain{true}, l, func(context.Context, effectgate.Intent) error { t.Fatal("effect ran"); return nil })
	if e := gate.Execute(context.Background(), intent("reqA")); !errors.Is(e, effectgate.ErrDenied) {
		t.Fatal(e)
	}
	if _, err := l.Inspect("reqA"); !errors.Is(err, ErrReconcile) {
		t.Fatal(err)
	}
}

// Spawn subprocesses of this very test binary to check genuine cross-process
// advisory-lock serialization. This tests COOPERATIVE processes only.
func TestReserveInChild(t *testing.T) {
	if os.Getenv("PHASE29_CHILD") != "1" {
		return
	}
	dir := os.Getenv("PHASE29_DIR")
	pubRaw, err := os.ReadFile(filepath.Join(os.Getenv("PHASE29_PUB_DIR"), "pub"))
	if err != nil {
		t.Fatal(err)
	}
	l, err := Open(dir, ed25519.PublicKey(pubRaw))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	e := l.Reserve(intent("reqA"))
	if e == nil {
		t.Log("CHILD_RESERVED")
		return
	}
	if errors.Is(e, ErrDuplicate) {
		t.Log("CHILD_DUPLICATE")
		return
	}
	t.Fatal(e)
}
func TestCrossProcessReservationsExactlyOne(t *testing.T) {
	l, priv, dir := fixture(t)
	_ = l
	publicDir := t.TempDir()
	if e := os.WriteFile(filepath.Join(publicDir, "pub"), priv.Public().(ed25519.PublicKey), 0600); e != nil {
		t.Fatal(e)
	}
	const n = 6
	var wg sync.WaitGroup
	var reserved, duplicate atomic.Int32
	for j := 0; j < n; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run=^TestReserveInChild$", "-test.v")
			cmd.Env = append(os.Environ(), "PHASE29_CHILD=1", "PHASE29_DIR="+dir, "PHASE29_PUB_DIR="+publicDir)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("child err %v %s", err, output)
				return
			}
			if bytes.Contains(output, []byte("CHILD_RESERVED")) {
				reserved.Add(1)
			} else if bytes.Contains(output, []byte("CHILD_DUPLICATE")) {
				duplicate.Add(1)
			} else {
				t.Errorf("unrecognized child: %s", output)
			}
		}()
	}
	wg.Wait()
	if reserved.Load() != 1 || duplicate.Load() != n-1 {
		t.Fatalf("reserved=%d duplicates=%d", reserved.Load(), duplicate.Load())
	}
}

func TestWorldReadableRecordDenied(t *testing.T) {
	l, _, dir := fixture(t)
	i := intent("reqA")
	if err := l.Reserve(i); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, fileName("i-", i.RequestID))
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if err := l.Reserve(i); !errors.Is(err, ErrCorrupt) {
		t.Fatal(err)
	}
	if _, err := l.Inspect(i.RequestID); !errors.Is(err, ErrCorrupt) {
		t.Fatal(err)
	}
}
func TestUntrustedReceiptFileSymlinkDenied(t *testing.T) {
	l, _, dir := fixture(t)
	i := intent("reqA")
	if err := l.Reserve(i); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "foreign")
	if err := os.WriteFile(target, []byte("not-a-proof"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, fileName("o-", i.RequestID))); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Inspect(i.RequestID); !errors.Is(err, ErrCorrupt) {
		t.Fatal(err)
	}
}
func TestCrashChildAfterDurableReserve(t *testing.T) {
	if os.Getenv("PHASE29_SIMULATE_CRASH") != "1" {
		return
	}
	dir := os.Getenv("PHASE29_DIR")
	pub, err := os.ReadFile(filepath.Join(os.Getenv("PHASE29_PUB_DIR"), "pub"))
	if err != nil {
		t.Fatal(err)
	}
	l, err := Open(dir, ed25519.PublicKey(pub))
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Reserve(intent("reqCrash")); err != nil {
		t.Fatal(err)
	}
	// Process disappears immediately after fsync and before effect dispatch.
	os.Exit(77)
}
func TestAbruptProcessExitRetainsUnknownNotRetryable(t *testing.T) {
	l, priv, dir := fixture(t)
	publicDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(publicDir, "pub"), priv.Public().(ed25519.PublicKey), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashChildAfterDurableReserve$")
	cmd.Env = append(os.Environ(), "PHASE29_SIMULATE_CRASH=1", "PHASE29_DIR="+dir, "PHASE29_PUB_DIR="+publicDir)
	err := cmd.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 77 {
		t.Fatalf("expected abrupt exit 77, got: %v", err)
	}
	status, err := l.Inspect("reqCrash")
	if err != nil || status != ReconcileRequired {
		t.Fatalf("status=%s err=%v", status, err)
	}
	if err := l.Reserve(intent("reqCrash")); !errors.Is(err, ErrDuplicate) {
		t.Fatal(err)
	}
}
