package ownerproof

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	effectgate "telmivo.local/lic-a9-phase28-effect-proof"
	ledger "telmivo.local/lic-a9-phase29-outcome-ledger"
)

type fixture struct {
	owner        *Supervisor
	provider     *TestProvider
	journal      *ledger.Ledger
	authorityKey ed25519.PrivateKey
	providerKey  ed25519.PrivateKey
	root         string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	root := filepath.Join(t.TempDir(), "supervisor")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	_, akey, _ := ed25519.GenerateKey(rand.Reader)
	ppub, pkey, _ := ed25519.GenerateKey(rand.Reader)
	s, err := Open(root, "installationA", akey.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	journalDir := filepath.Join(t.TempDir(), "journal")
	if err := os.Mkdir(journalDir, 0700); err != nil {
		t.Fatal(err)
	}
	j, err := ledger.Open(journalDir, ppub)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	return &fixture{owner: s, provider: NewTestProvider(true), journal: j, authorityKey: akey, providerKey: pkey, root: root}
}
func grant(f *fixture, owner, session string, previous uint64) Grant {
	now := time.Now().Unix()
	return SignTestGrant(f.authorityKey, Grant{Protocol: Protocol, Installation: "installationA", Owner: owner, Session: session, PreviousEpoch: previous, Epoch: previous + 1, NotBeforeUnix: now - 2, ExpiresUnix: now + 120})
}
func intent(id string) effectgate.Intent {
	return effectgate.Intent{RequestID: id, WorkerID: "workerA", WorkspaceID: "tenantA", ActorID: "actorA", ResourceID: "resourceA", Operation: effectgate.SyntheticOperation}
}
func acquire(t *testing.T, f *fixture, g Grant) {
	t.Helper()
	if err := f.owner.Acquire(context.Background(), g, f.provider); err != nil {
		t.Fatal(err)
	}
}

func TestInitialSignedOwnerAndEpoch(t *testing.T) {
	f := setup(t)
	g := grant(f, "ownerA", "sessionA", 0)
	acquire(t, f, g)
	epoch, err := f.owner.CurrentEpoch()
	if err != nil || epoch != 1 {
		t.Fatal(epoch, err)
	}
	if err := f.owner.IsCurrent(g); err != nil {
		t.Fatal(err)
	}
}
func TestInvalidSignatureDenied(t *testing.T) {
	f := setup(t)
	g := grant(f, "ownerA", "sessionA", 0)
	g.Signature = "00"
	if err := f.owner.Acquire(context.Background(), g, f.provider); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
}
func TestWrongInstallationDenied(t *testing.T) {
	f := setup(t)
	g := grant(f, "ownerA", "sessionA", 0)
	g.Installation = "installationB"
	g = SignTestGrant(f.authorityKey, g)
	if err := f.owner.Acquire(context.Background(), g, f.provider); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
}
func TestExpiredAndFutureGrantDenied(t *testing.T) {
	f := setup(t)
	g := grant(f, "ownerA", "sessionA", 0)
	g.ExpiresUnix = time.Now().Unix() - 1
	g = SignTestGrant(f.authorityKey, g)
	if e := f.owner.Acquire(context.Background(), g, f.provider); !errors.Is(e, ErrDenied) {
		t.Fatal(e)
	}
	g = grant(f, "ownerA", "sessionA", 0)
	g.NotBeforeUnix = time.Now().Unix() + 60
	g = SignTestGrant(f.authorityKey, g)
	if e := f.owner.Acquire(context.Background(), g, f.provider); !errors.Is(e, ErrDenied) {
		t.Fatal(e)
	}
}
func TestMissingExternalFencerFailsClosed(t *testing.T) {
	f := setup(t)
	g := grant(f, "ownerA", "sessionA", 0)
	if e := f.owner.Acquire(context.Background(), g, nil); !errors.Is(e, ErrDenied) {
		t.Fatal(e)
	}
	unsupported := NewTestProvider(false)
	if e := f.owner.Acquire(context.Background(), g, unsupported); !errors.Is(e, ErrFenceUnsupported) {
		t.Fatal(e)
	}
	epoch, _ := f.owner.CurrentEpoch()
	if epoch != 0 {
		t.Fatal(epoch)
	}
}
func TestGenerationMustIncrease(t *testing.T) {
	f := setup(t)
	g := grant(f, "ownerA", "sessionA", 0)
	acquire(t, f, g)
	if e := f.owner.Acquire(context.Background(), g, f.provider); !errors.Is(e, ErrStale) {
		t.Fatal(e)
	}
	older := grant(f, "ownerB", "sessionB", 0)
	if e := f.owner.Acquire(context.Background(), older, f.provider); !errors.Is(e, ErrStale) {
		t.Fatal(e)
	}
}
func TestNewOwnerNeedsDifferentSession(t *testing.T) {
	f := setup(t)
	first := grant(f, "ownerA", "sessionA", 0)
	acquire(t, f, first)
	second := grant(f, "ownerA", "sessionA", 1)
	if e := f.owner.Acquire(context.Background(), second, f.provider); !errors.Is(e, ErrStale) {
		t.Fatal(e)
	}
}
func TestCleanHandoffFencesOldOwnerAndProvider(t *testing.T) {
	f := setup(t)
	a := grant(f, "ownerA", "sessionA", 0)
	acquire(t, f, a)
	b := grant(f, "ownerB", "sessionB", 1)
	acquire(t, f, b)
	if e := f.owner.IsCurrent(a); !errors.Is(e, ErrStale) {
		t.Fatal(e)
	}
	if e := f.owner.Begin(a, intent("req-old"), f.journal); !errors.Is(e, ErrStale) {
		t.Fatal(e)
	}
	if e := f.provider.Apply(a.Epoch, "old"); !errors.Is(e, ErrStale) {
		t.Fatal(e)
	}
	if e := f.owner.IsCurrent(b); e != nil {
		t.Fatal(e)
	}
	if e := f.provider.Apply(b.Epoch, "new"); e != nil {
		t.Fatal(e)
	}
	if f.provider.Count() != 1 {
		t.Fatal(f.provider.Count())
	}
}
func TestOwnerCanRestartWithNewSession(t *testing.T) {
	f := setup(t)
	a := grant(f, "ownerA", "sessionA", 0)
	acquire(t, f, a)
	b := grant(f, "ownerA", "sessionRestart", 1)
	acquire(t, f, b)
	if e := f.owner.IsCurrent(a); !errors.Is(e, ErrStale) {
		t.Fatal(e)
	}
}
func TestUnknownOutcomeBlocksTakeover(t *testing.T) {
	f := setup(t)
	a := grant(f, "ownerA", "sessionA", 0)
	acquire(t, f, a)
	if e := f.owner.Begin(a, intent("reqUnknown"), f.journal); e != nil {
		t.Fatal(e)
	}
	next := grant(f, "ownerB", "sessionB", 1)
	if e := f.owner.Acquire(context.Background(), next, f.provider); !errors.Is(e, ErrPending) {
		t.Fatal(e)
	}
	epoch, _ := f.owner.CurrentEpoch()
	if epoch != 1 {
		t.Fatal(epoch)
	}
}
func TestVerifiedProviderReceiptAllowsHandoff(t *testing.T) {
	f := setup(t)
	a := grant(f, "ownerA", "sessionA", 0)
	acquire(t, f, a)
	i := intent("reqPending")
	if e := f.owner.Begin(a, i, f.journal); e != nil {
		t.Fatal(e)
	}
	if e := f.owner.Settle(i.RequestID, f.journal); !errors.Is(e, ErrPending) {
		t.Fatal(e)
	}
	if e := f.journal.Confirm(ledger.SyntheticProof(f.providerKey, i, "syntheticReceipt")); e != nil {
		t.Fatal(e)
	}
	if e := f.owner.Settle(i.RequestID, f.journal); e != nil {
		t.Fatal(e)
	}
	b := grant(f, "ownerB", "sessionB", 1)
	acquire(t, f, b)
	if e := f.provider.Apply(a.Epoch, "stale"); !errors.Is(e, ErrStale) {
		t.Fatal(e)
	}
}
func TestForgedReceiptCannotClearPending(t *testing.T) {
	f := setup(t)
	a := grant(f, "ownerA", "sessionA", 0)
	acquire(t, f, a)
	i := intent("reqPending")
	if e := f.owner.Begin(a, i, f.journal); e != nil {
		t.Fatal(e)
	}
	_, badKey, _ := ed25519.GenerateKey(rand.Reader)
	if e := f.journal.Confirm(ledger.SyntheticProof(badKey, i, "fraud")); !errors.Is(e, ledger.ErrBadProof) {
		t.Fatal(e)
	}
	if e := f.owner.Settle(i.RequestID, f.journal); !errors.Is(e, ErrPending) {
		t.Fatal(e)
	}
}
func TestCrashRestartPreservesPendingAndEpoch(t *testing.T) {
	f := setup(t)
	a := grant(f, "ownerA", "sessionA", 0)
	acquire(t, f, a)
	if e := f.owner.Begin(a, intent("reqCrash"), f.journal); e != nil {
		t.Fatal(e)
	}
	reopened, e := Open(f.root, "installationA", f.authorityKey.Public().(ed25519.PublicKey))
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	epoch, e := reopened.CurrentEpoch()
	if e != nil || epoch != 1 {
		t.Fatal(epoch, e)
	}
	pending, e := reopened.PendingCount()
	if e != nil || pending != 1 {
		t.Fatal(pending, e)
	}
	if e := reopened.Acquire(context.Background(), grant(f, "ownerB", "sessionB", 1), f.provider); !errors.Is(e, ErrPending) {
		t.Fatal(e)
	}
}
func TestCorruptStateFailsClosed(t *testing.T) {
	f := setup(t)
	g := grant(f, "ownerA", "sessionA", 0)
	acquire(t, f, g)
	if e := os.WriteFile(filepath.Join(f.root, "current.json"), []byte("{"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := f.owner.IsCurrent(g); !errors.Is(e, ErrCorrupt) {
		t.Fatal(e)
	}
	if _, e := Open(f.root, "installationA", f.authorityKey.Public().(ed25519.PublicKey)); !errors.Is(e, ErrCorrupt) {
		t.Fatal(e)
	}
}
func TestMissingStateFailsClosed(t *testing.T) {
	f := setup(t)
	if e := os.Remove(filepath.Join(f.root, "current.json")); e != nil {
		t.Fatal(e)
	}
	if _, e := Open(f.root, "installationA", f.authorityKey.Public().(ed25519.PublicKey)); !errors.Is(e, ErrCorrupt) {
		t.Fatal(e)
	}
}
func TestUnsafeDirRejected(t *testing.T) {
	f := setup(t)
	dir := filepath.Join(t.TempDir(), "unsafe")
	if e := os.Mkdir(dir, 0777); e != nil {
		t.Fatal(e)
	}
	if e := os.Chmod(dir, 0777); e != nil {
		t.Fatal(e)
	}
	if _, e := Open(dir, "installationA", f.authorityKey.Public().(ed25519.PublicKey)); !errors.Is(e, ErrDenied) {
		t.Fatal(e)
	}
}
func TestConcurrentAcquiresOneEpochWinner(t *testing.T) {
	f := setup(t)
	g1 := grant(f, "ownerA", "sessionA", 0)
	g2 := grant(f, "ownerB", "sessionB", 0)
	var wg sync.WaitGroup
	var won atomic.Int32
	for _, g := range []Grant{g1, g2} {
		wg.Add(1)
		go func(g Grant) {
			defer wg.Done()
			if e := f.owner.Acquire(context.Background(), g, f.provider); e == nil {
				won.Add(1)
			} else if !errors.Is(e, ErrStale) {
				t.Error(e)
			}
		}(g)
	}
	wg.Wait()
	if won.Load() != 1 {
		t.Fatal(won.Load())
	}
	epoch, _ := f.owner.CurrentEpoch()
	if epoch != 1 {
		t.Fatal(epoch)
	}
}
func TestBeginDuplicateAlwaysPending(t *testing.T) {
	f := setup(t)
	a := grant(f, "ownerA", "sessionA", 0)
	acquire(t, f, a)
	if e := f.owner.Begin(a, intent("req1"), f.journal); e != nil {
		t.Fatal(e)
	}
	if e := f.owner.Begin(a, intent("req1"), f.journal); !errors.Is(e, ErrPending) {
		t.Fatal(e)
	}
}
func TestFailedLedgerReserveRemainsFailClosed(t *testing.T) {
	f := setup(t)
	a := grant(f, "ownerA", "sessionA", 0)
	acquire(t, f, a)
	bad := intent("reqBad")
	bad.Operation = "ARBITRARY_ORDER"
	if e := f.owner.Begin(a, bad, f.journal); !errors.Is(e, ErrPending) {
		t.Fatal(e)
	}
	n, e := f.owner.PendingCount()
	if e != nil || n != 1 {
		t.Fatal(n, e)
	}
}
func TestNoBlindOwnerTransitionWithUnsupportedProvider(t *testing.T) {
	f := setup(t)
	a := grant(f, "ownerA", "sessionA", 0)
	acquire(t, f, a)
	b := grant(f, "ownerB", "sessionB", 1)
	if e := f.owner.Acquire(context.Background(), b, NewTestProvider(false)); !errors.Is(e, ErrFenceUnsupported) {
		t.Fatal(e)
	}
	if e := f.owner.IsCurrent(a); e != nil {
		t.Fatal(e)
	}
}
func TestCancellationDenied(t *testing.T) {
	f := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := f.owner.Acquire(ctx, grant(f, "ownerA", "sessionA", 0), f.provider); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
