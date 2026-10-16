package installationlicense

// Phase44 TEST ONLY: adversarial filesystem and provenance cases for the
// Phase43 global synthetic journal. These tests do not establish authentic
// external provider confirmation, remote Authority identity, or Production.
import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestLICA9Phase44DirectoryFloodCannotAllocateUnboundedScanOrAdmitEffect(t *testing.T) {
	f := phase43StrictFixture(t)
	phase43GrantOne(t, f)
	// _owner.lock already exists: more than 4096 directory entries. The
	// scanner must ask the OS for at most 4097 names and fail closed.
	for i := 0; i < 4097; i++ {
		path := filepath.Join(f.root, fmt.Sprintf("untrusted-%05d", i))
		if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.journal.phase43Pending(); !errors.Is(err, phase37Corrupt) {
		t.Fatalf("overbudget directory didn't fail closed: %v", err)
	}
	if err := f.gate.effect(phase42RequestFor(f.gate, f.signer, 1, "flood44")); !errors.Is(err, phase37Corrupt) {
		t.Fatalf("request bypassed overbudget journal: %v", err)
	}
	if f.gate.effects.Load() != 0 {
		t.Fatal("effect dispatched despite flood")
	}
}

func TestLICA9Phase44SignedReceiptViaSymlinkCannotSettleUnknown(t *testing.T) {
	f := phase43StrictFixture(t)
	provider := phase43ProviderJournal(t, &f)
	phase43GrantOne(t, f)
	first := phase42RequestFor(f.gate, f.signer, 1, "linkproof44")
	if err := f.gate.effect(first); err != nil {
		t.Fatal(err)
	}
	receipt := phase37SyntheticReceipt(provider, first.Request.Claim, "synthetic-provider44")
	target := filepath.Join(t.TempDir(), "outside-receipt")
	if err := os.WriteFile(target, phase37Encode(receipt), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(f.root, "c-"+phase37Name(first.Request.Claim.RequestID))
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := f.journal.phase43Pending(); !errors.Is(err, phase37Corrupt) {
		t.Fatalf("symlinked receipt escaped O_NOFOLLOW: %v", err)
	}
	if err := f.gate.effect(phase42RequestFor(f.gate, f.signer, 1, "after-symlink44")); !errors.Is(err, phase37Corrupt) {
		t.Fatalf("later request accepted after symlink: %v", err)
	}
	if f.gate.effects.Load() != 1 {
		t.Fatal("second effect dispatched")
	}
}

func TestLICA9Phase44ForgedReceiptOnDiskNeverClearsGlobalFence(t *testing.T) {
	f := phase43StrictFixture(t)
	_ = phase43ProviderJournal(t, &f)
	phase43GrantOne(t, f)
	first := phase42RequestFor(f.gate, f.signer, 1, "forgedondisk44")
	if err := f.gate.effect(first); err != nil {
		t.Fatal(err)
	}
	_, fake, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	forged := phase37SyntheticReceipt(fake, first.Request.Claim, "fake-provider44")
	if err = f.journal.writeExclusive("c-"+phase37Name(first.Request.Claim.RequestID), forged); err != nil {
		t.Fatal(err)
	}
	if _, err = f.journal.phase43Pending(); !errors.Is(err, phase37Corrupt) {
		t.Fatalf("forged receipt wasn't rejected: %v", err)
	}
	if err = f.gate.effect(phase42RequestFor(f.gate, f.signer, 1, "after-forgery44")); !errors.Is(err, phase37Corrupt) {
		t.Fatalf("forged provider unlocked journal: %v", err)
	}
	if f.gate.effects.Load() != 1 {
		t.Fatal("second effect invoked")
	}
}

func TestLICA9Phase44OrphanCorrectlySignedConfirmationFailsClosed(t *testing.T) {
	f := phase43StrictFixture(t)
	provider := phase43ProviderJournal(t, &f)
	bogus := phase42RequestFor(f.gate, f.signer, 1, "orphan44")
	receipt := phase37SyntheticReceipt(provider, bogus.Request.Claim, "synthetic-provider44")
	if err := f.journal.writeExclusive("c-"+phase37Name(bogus.Request.Claim.RequestID), receipt); err != nil {
		t.Fatal(err)
	}
	if err := f.gate.grant(phase42GrantFor(f.gate, f.signer, 1)); !errors.Is(err, phase37Corrupt) {
		t.Fatalf("orphan confirmation enabled fence: %v", err)
	}
	if f.gate.enabled || f.gate.effects.Load() != 0 {
		t.Fatal("orphan enabled synthetic gate")
	}
}

func TestLICA9Phase44ModifiedRecordCannotPassCryptographicallyValidReceipt(t *testing.T) {
	f := phase43StrictFixture(t)
	provider := phase43ProviderJournal(t, &f)
	phase43GrantOne(t, f)
	first := phase42RequestFor(f.gate, f.signer, 1, "mutated44")
	if err := f.gate.effect(first); err != nil {
		t.Fatal(err)
	}
	if err := f.journal.confirm(phase37SyntheticReceipt(provider, first.Request.Claim, "synthetic-provider44")); err != nil {
		t.Fatal(err)
	}
	name := "r-" + phase37Name(first.Request.Claim.RequestID)
	// Controlled test corruption: still canonical JSON and a valid hex hash,
	// but the fsynced reservation now disagrees with the authentic receipt.
	tampered := phase37Record{Version: phase37Version, RequestID: first.Request.Claim.RequestID,
		ClaimDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	if err := os.WriteFile(filepath.Join(f.root, name), phase37Encode(tampered), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.journal.phase43Pending(); !errors.Is(err, phase37Corrupt) {
		t.Fatalf("mismatch escaped cryptographic link check: %v", err)
	}
	if err := f.gate.effect(phase42RequestFor(f.gate, f.signer, 1, "after-mutation44")); !errors.Is(err, phase37Corrupt) {
		t.Fatalf("mutated journal admitted later effect: %v", err)
	}
}
