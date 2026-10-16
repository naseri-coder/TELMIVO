//go:build linux

package installationlicense

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Linux only: hardlinking an already-written journal record must not permit
// its use because secure reads require Nlink==1. No Production files touched.
func TestLICA9Phase44HardlinkedReservationRejectedBeforeNewEffect(t *testing.T) {
	f := phase43StrictFixture(t)
	phase43GrantOne(t, f)
	first := phase42RequestFor(f.gate, f.signer, 1, "linkrecord44")
	if err := f.gate.effect(first); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(f.root, "r-"+phase37Name(first.Request.Claim.RequestID))
	outside := filepath.Join(t.TempDir(), "second-link")
	if err := os.Link(source, outside); err != nil {
		t.Fatal("native Linux hardlink fixture required:", err)
	}
	if _, err := f.journal.phase43Pending(); !errors.Is(err, phase37Corrupt) {
		t.Fatalf("hardlinked reservation was read: %v", err)
	}
	if err := f.gate.effect(phase42RequestFor(f.gate, f.signer, 1, "after-hardlink44")); !errors.Is(err, phase37Corrupt) {
		t.Fatalf("hardlink bypassed fail-closed: %v", err)
	}
	if f.gate.effects.Load() != 1 {
		t.Fatal("extra callback")
	}
}
