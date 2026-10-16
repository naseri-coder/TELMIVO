package installationlicense

// TEST-ONLY: this is conservative *global* per-journal uncertainty fencing,
// not an operational external provider / Authority transaction protocol.
import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func phase43StrictFixture(t *testing.T) phase42Fixture {
	t.Helper()
	f := phase42NewFixture(t)
	f.gate.strictGlobalPending = true
	return f
}
func phase43ProviderJournal(t *testing.T, f *phase42Fixture) ed25519.PrivateKey {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "provider43")
	if err = os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	j, err := phase37Open(root, pub)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.close() })
	f.journal = j
	f.root = root
	f.gate.journal = j
	return priv
}
func phase43GrantOne(t *testing.T, f phase42Fixture) {
	t.Helper()
	if err := f.gate.grant(phase42GrantFor(f.gate, f.signer, 1)); err != nil {
		t.Fatal(err)
	}
}
func TestLICA9Phase43UnknownDifferentIDDoesNotDispatchSecondRealManagerEffect(t *testing.T) {
	f := phase43StrictFixture(t)
	phase43GrantOne(t, f)
	first := phase42RequestFor(f.gate, f.signer, 1, "first43")
	if err := f.gate.effect(first); err != nil {
		t.Fatal(err)
	}
	if err := f.gate.effect(phase42RequestFor(f.gate, f.signer, 1, "second43")); !errors.Is(err, phase43Unresolved) {
		t.Fatalf("second ID escaped global fence: %v", err)
	}
	if f.gate.effects.Load() != 1 {
		t.Fatal("second callback executed")
	}
	if err := f.gate.effect(first); !errors.Is(err, phase37Duplicate) {
		t.Fatalf("replayed ID status: %v", err)
	}
}
func TestLICA9Phase43NewSignedBootCannotRearmUnresolvedJournal(t *testing.T) {
	f := phase43StrictFixture(t)
	phase43GrantOne(t, f)
	if err := f.gate.effect(phase42RequestFor(f.gate, f.signer, 1, "unknown43")); err != nil {
		t.Fatal(err)
	}
	next := phase42NewGate(t, f.manager, f.signer.Public().(ed25519.PublicKey), f.journal)
	next.strictGlobalPending = true
	if err := next.grant(phase42GrantFor(next, f.signer, 2)); !errors.Is(err, phase43Unresolved) {
		t.Fatalf("rearm accepted while unknown: %v", err)
	}
	if next.enabled || next.epoch != 0 {
		t.Fatal("blocked grant changed fence state")
	}
}
func TestLICA9Phase43ValidSyntheticProviderReceiptAllowsNewDistinctID(t *testing.T) {
	f := phase43StrictFixture(t)
	provider := phase43ProviderJournal(t, &f)
	phase43GrantOne(t, f)
	first := phase42RequestFor(f.gate, f.signer, 1, "verified43")
	if err := f.gate.effect(first); err != nil {
		t.Fatal(err)
	}
	proof := phase37SyntheticReceipt(provider, first.Request.Claim, "test-provider43")
	if err := f.journal.confirm(proof); err != nil {
		t.Fatal(err)
	}
	if err := f.gate.effect(phase42RequestFor(f.gate, f.signer, 1, "after-receipt43")); err != nil {
		t.Fatal(err)
	}
	if f.gate.effects.Load() != 2 {
		t.Fatal("signed receipt did not clear global uncertainty fence")
	}
	if err := f.gate.effect(first); !errors.Is(err, phase37Duplicate) {
		t.Fatal("original ID replayed", err)
	}
}
func TestLICA9Phase43ForgedSyntheticReceiptLeavesAllOtherIDsDenied(t *testing.T) {
	f := phase43StrictFixture(t)
	_ = phase43ProviderJournal(t, &f)
	phase43GrantOne(t, f)
	first := phase42RequestFor(f.gate, f.signer, 1, "forged43")
	if err := f.gate.effect(first); err != nil {
		t.Fatal(err)
	}
	_, fake, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	proof := phase37SyntheticReceipt(fake, first.Request.Claim, "fake-provider43")
	if err := f.journal.confirm(proof); !errors.Is(err, phase37BadReceipt) {
		t.Fatal("forged signed receipt accepted", err)
	}
	if err := f.gate.effect(phase42RequestFor(f.gate, f.signer, 1, "cross-id43")); !errors.Is(err, phase43Unresolved) {
		t.Fatal("forged receipt opened fence", err)
	}
}
func TestLICA9Phase43ConcurrentDistinctRequestsAcrossGatesOneDurableWinner(t *testing.T) {
	f := phase43StrictFixture(t)
	phase43GrantOne(t, f)
	// Independent test gate instances share the SAME durable journal, with a
	// different mutex: global correctness relies on the journal flock.
	other := phase42NewGate(t, f.manager, f.signer.Public().(ed25519.PublicKey), f.journal)
	other.strictGlobalPending = true
	if err := other.grant(phase42GrantFor(other, f.signer, 1)); err != nil {
		t.Fatal(err)
	}
	var allowed, blocked atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			gate := f.gate
			if n%2 == 1 {
				gate = other
			}
			id := "multi-43-" + string(rune('A'+n))
			req := phase42RequestFor(gate, f.signer, 1, id)
			err := gate.effect(req)
			switch {
			case err == nil:
				allowed.Add(1)
			case errors.Is(err, phase43Unresolved):
				blocked.Add(1)
			default:
				t.Errorf("unexpected result on %s: %v", id, err)
			}
		}(i)
	}
	wg.Wait()
	if allowed.Load() != 1 || blocked.Load() != 11 || f.gate.effects.Load()+other.effects.Load() != 1 {
		t.Fatalf("winners=%d blocked=%d callback count=%d", allowed.Load(), blocked.Load(), f.gate.effects.Load()+other.effects.Load())
	}
}
func TestLICA9Phase43PersistedUncertaintyBlocksDifferentIDAfterReopen(t *testing.T) {
	f := phase43StrictFixture(t)
	phase43GrantOne(t, f)
	if err := f.gate.effect(phase42RequestFor(f.gate, f.signer, 1, "persist43")); err != nil {
		t.Fatal(err)
	}
	again, err := phase37Open(f.root, f.journal.provider)
	if err != nil {
		t.Fatal(err)
	}
	defer again.close()
	newer := phase42NewGate(t, f.manager, f.signer.Public().(ed25519.PublicKey), again)
	newer.strictGlobalPending = true
	if err := newer.grant(phase42GrantFor(newer, f.signer, 2)); !errors.Is(err, phase43Unresolved) {
		t.Fatal("reopen cleared pending", err)
	}
	if newer.effects.Load() != 0 {
		t.Fatal("reopen executed side effect")
	}
}
func TestLICA9Phase43MalformedUnknownJournalArtifactFailsClosed(t *testing.T) {
	f := phase43StrictFixture(t)
	phase43GrantOne(t, f)
	if err := os.WriteFile(filepath.Join(f.root, "unexpected-record"), []byte("evil"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := f.gate.effect(phase42RequestFor(f.gate, f.signer, 1, "poison43")); !errors.Is(err, phase37Corrupt) {
		t.Fatal("unknown journal entry accepted", err)
	}
	if f.gate.effects.Load() != 0 {
		t.Fatal("corrupt journal dispatched")
	}
}
func TestLICA9Phase43RevokedRealManagerCannotDispatchUnderGlobalFence(t *testing.T) {
	f := phase43StrictFixture(t)
	phase43GrantOne(t, f)
	follower, err := Open(f.manager.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = follower.Accept(f.signedLease(follower.runtimeSession, 2)); err != nil {
		t.Fatal(err)
	}
	first := phase42RequestFor(f.gate, f.signer, 1, "revoked43")
	if err = f.gate.effect(first); !errors.Is(err, ErrRestricted) {
		t.Fatal("revoked Manager admitted", err)
	}
	if err = f.gate.effect(phase42RequestFor(f.gate, f.signer, 1, "next43")); !errors.Is(err, phase43Unresolved) {
		t.Fatal("revoked uncertain reservation didn't block other ID", err)
	}
	if f.gate.effects.Load() != 0 {
		t.Fatal("revoked Manager dispatched")
	}
}
