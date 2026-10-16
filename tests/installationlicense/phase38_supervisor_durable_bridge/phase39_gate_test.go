//go:build linux

package bridge

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// This adapter is test-only. It validates the identical callback boundary that
// the Product's real Manager.Effect integration supplies. It cannot certify
// the Product Manager or real Authority without the separate Go1.27 test.
func phase39RootGateFixture(t *testing.T, gate func(context.Context, Claim) error) *fixture {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("LIC_A9_PHASE39_UNSUPPORTED: needs isolated Linux root plus real worker UIDs")
	}
	root, err := os.MkdirTemp("/tmp", "lic39-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err = os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	socketDir := filepath.Join(root, "socket")
	journalDir := filepath.Join(root, "journal")
	if err = os.Mkdir(socketDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err = os.Chown(socketDir, 0, testWorkerUID); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(journalDir, 0700); err != nil {
		t.Fatal(err)
	}
	_, issuer, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, provider, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	server, err := Open(socketDir, journalDir, "installationA", testWorkerUID, issuer.Public().(ed25519.PublicKey), provider.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	if err = server.SetIntegrationTestEffectGate(gate); err != nil {
		t.Fatal(err)
	}
	server.Start()
	f := &fixture{root, socketDir, journalDir, server, issuer, provider}
	t.Cleanup(func() {
		if f.server != nil {
			f.server.Close()
		}
	})
	return f
}

func TestP39RootGateInvokedOnceAfterDurableReserve(t *testing.T) {
	var executed atomic.Int64
	f := phase39RootGateFixture(t, func(ctx context.Context, c Claim) error {
		executed.Add(1)
		return nil
	})
	expect(t, f, Sign(f.issuer, claim(f, "FENCE", 1, ""), nil), "FENCED_TEST_ONLY")
	wire := Sign(f.issuer, claim(f, "APPLY", 1, "unique39"), nil)
	expect(t, f, wire, "RECONCILE_REQUIRED")
	expect(t, f, wire, "DENIED_DUPLICATE_RECONCILE_REQUIRED")
	if executed.Load() != 1 {
		t.Fatalf("effect calls=%d", executed.Load())
	}
}

func TestP39GateFailureNeverBlindRetries(t *testing.T) {
	var attempted atomic.Int64
	f := phase39RootGateFixture(t, func(context.Context, Claim) error { attempted.Add(1); return errors.New("synthetic uncertain outcome") })
	expect(t, f, Sign(f.issuer, claim(f, "FENCE", 1, ""), nil), "FENCED_TEST_ONLY")
	w := Sign(f.issuer, claim(f, "APPLY", 1, "unknown39"), nil)
	expect(t, f, w, "RECONCILE_REQUIRED")
	expect(t, f, w, "DENIED_DUPLICATE_RECONCILE_REQUIRED")
	if attempted.Load() != 1 {
		t.Fatal("blind retry happened", attempted.Load())
	}
}

func TestP39UnprivilegedOutsiderNeverReachesGate(t *testing.T) {
	var called atomic.Int64
	f := phase39RootGateFixture(t, func(context.Context, Claim) error { called.Add(1); return nil })
	wire := Sign(f.issuer, claim(f, "FENCE", 1, ""), nil)
	out := runChild(t, f, wire, "outsider", testOutsiderUID)
	if out == "" {
		t.Fatal("missing kernel denial output")
	}
	if called.Load() != 0 {
		t.Fatal("outsider entered effect gate")
	}
}

func TestP39CannotEnableGateAfterStartOrTwice(t *testing.T) {
	f := phase39RootGateFixture(t, func(context.Context, Claim) error { return nil })
	if err := f.server.SetIntegrationTestEffectGate(func(context.Context, Claim) error { return nil }); !errors.Is(err, ErrBoundary) {
		t.Fatal("mutable gate accepted", err)
	}
	if err := f.server.SetIntegrationTestEffectGate(nil); !errors.Is(err, ErrBoundary) {
		t.Fatal("nil gate accepted", err)
	}
}

func TestP39MismatchedSessionDeniedBeforeGate(t *testing.T) {
	var called atomic.Int64
	f := phase39RootGateFixture(t, func(context.Context, Claim) error { called.Add(1); return nil })
	expect(t, f, Sign(f.issuer, claim(f, "FENCE", 1, ""), nil), "FENCED_TEST_ONLY")
	stale := claim(f, "APPLY", 1, "stale39")
	stale.Session = "invalidSession"
	expect(t, f, Sign(f.issuer, stale, nil), "DENIED_STALE_SESSION")
	if called.Load() != 0 {
		t.Fatal("gate invoked for stale session")
	}
}
