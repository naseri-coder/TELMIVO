//go:build linux

package bridge

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// Strict fresh-boot challenge is a TEST-ONLY contract: no operational
// Authority signer / credential distribution or Product Manager.Effect.
func phase40FreshFixture(t *testing.T) *fixture {
	return phase40FreshFixtureWithGate(t, nil)
}

func phase40FreshFixtureWithGate(t *testing.T, gate func(context.Context, Claim) error) *fixture {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("LIC_A9_PHASE40_REQUIRES_ISOLATED_LINUX_ROOT")
	}
	root, err := os.MkdirTemp("/tmp", "lic40-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err = os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	socketDir, journalDir := filepath.Join(root, "socket"), filepath.Join(root, "journal")
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
	if err = server.EnableTestFreshBootFence(); err != nil {
		t.Fatal(err)
	}
	if gate != nil {
		if err = server.SetIntegrationTestEffectGate(gate); err != nil {
			t.Fatal(err)
		}
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
func phase40BootClaim(t *testing.T, f *fixture, epoch uint64) Claim {
	t.Helper()
	nonce, err := f.server.TestBootChallenge()
	if err != nil {
		t.Fatal(err)
	}
	c := claim(f, "FENCE", epoch, "")
	c.BootChallenge = nonce
	return c
}
func TestP40FreshBootUnsignedOrPreviousChallengeDeniedBeforeEpochAdvance(t *testing.T) {
	f := phase40FreshFixture(t)
	empty := claim(f, "FENCE", 1, "")
	expect(t, f, Sign(f.issuer, empty, nil), "DENIED_FRESH_BOOT_CHALLENGE")
	empty.BootChallenge = strings.Repeat("a", 64)
	expect(t, f, Sign(f.issuer, empty, nil), "DENIED_FRESH_BOOT_CHALLENGE")
	epoch, err := f.server.DebugEpoch()
	if err != nil || epoch != 0 {
		t.Fatalf("bad FENCE advanced epoch: %v %d", err, epoch)
	}
	signed := phase40BootClaim(t, f, 1)
	expect(t, f, Sign(f.issuer, signed, nil), "FENCED_TEST_ONLY")
	epoch, err = f.server.DebugEpoch()
	if err != nil || epoch != 1 {
		t.Fatalf("genuine fresh challenge rejected: %v %d", err, epoch)
	}
}
func TestP40PreSignedFutureFenceFailsClosedAfterSupervisorRestart(t *testing.T) {
	f := phase40FreshFixture(t)
	old := phase40BootClaim(t, f, 1)
	expect(t, f, Sign(f.issuer, old, nil), "FENCED_TEST_ONLY")
	// Attacker recorded an authenticated *future* FENCE before restart.
	future := phase40BootClaim(t, f, 2)
	recorded := Sign(f.issuer, future, nil)
	oldBoot := future.BootChallenge
	f.server.Close()
	reopened, err := Open(f.socketDir, f.journalDir, "installationA", testWorkerUID, f.issuer.Public().(ed25519.PublicKey), f.provider.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	f.server = reopened
	if err = reopened.EnableTestFreshBootFence(); err != nil {
		t.Fatal(err)
	}
	nonce, err := reopened.TestBootChallenge()
	if err != nil || nonce == oldBoot {
		t.Fatalf("nonce reused or cannot be read: %v", err)
	}
	reopened.Start()
	expect(t, f, recorded, "DENIED_FRESH_BOOT_CHALLENGE")
	epoch, err := reopened.DebugEpoch()
	if err != nil || epoch != 1 {
		t.Fatalf("recorded fence advanced persisted epoch %d %v", epoch, err)
	}
	fresh := phase40BootClaim(t, f, 2)
	expect(t, f, Sign(f.issuer, fresh, nil), "FENCED_TEST_ONLY")
}
func TestP40UnresolvedDurableEffectBlocksEvenFreshSignedRearm(t *testing.T) {
	f := phase40FreshFixture(t)
	expect(t, f, Sign(f.issuer, phase40BootClaim(t, f, 1), nil), "FENCED_TEST_ONLY")
	var called atomic.Int32
	// Callback cannot be installed after Start: reserve via regular
	// synthetic APPLY and confirm that durable unresolved record persists.
	request := Sign(f.issuer, claim(f, "APPLY", 1, "uncertain40"), nil)
	expect(t, f, request, "RECONCILE_REQUIRED")
	expect(t, f, request, "DENIED_DUPLICATE_RECONCILE_REQUIRED")
	if called.Load() != 0 {
		t.Fatal("unexpected effect")
	}
	f.server.Close()
	reopened, err := Open(f.socketDir, f.journalDir, "installationA", testWorkerUID, f.issuer.Public().(ed25519.PublicKey), f.provider.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	f.server = reopened
	if err = reopened.EnableTestFreshBootFence(); err != nil {
		t.Fatal(err)
	}
	reopened.Start()
	expect(t, f, Sign(f.issuer, phase40BootClaim(t, f, 2), nil), "RECONCILE_REQUIRED")
	epoch, err := reopened.DebugEpoch()
	if err != nil || epoch != 1 {
		t.Fatalf("pending journal permitted epoch %d %v", epoch, err)
	}
}
func TestP40NonRootPeerCannotBecomeRootBootSigner(t *testing.T) {
	f := phase40FreshFixture(t)
	wire := Sign(f.issuer, phase40BootClaim(t, f, 1), nil)
	out := runChild(t, f, wire, "outsider", testOutsiderUID)
	if out == "" {
		t.Fatal("missing outsider rejection")
	}
	epoch, err := f.server.DebugEpoch()
	if err != nil || epoch != 0 {
		t.Fatal("untrusted outsider advanced fence")
	}
}
func TestP40BootChallengeModeIsImmutableAndGateCannotBeLateEnabled(t *testing.T) {
	f := phase40FreshFixture(t)
	if err := f.server.EnableTestFreshBootFence(); !errors.Is(err, ErrBoundary) {
		t.Fatalf("strict mode changed after Start %v", err)
	}
	nonce, err := f.server.TestBootChallenge()
	if err != nil || len(nonce) != 64 {
		t.Fatalf("invalid challenge %q %v", nonce, err)
	}
	if err := f.server.SetIntegrationTestEffectGate(func(context.Context, Claim) error { return nil }); !errors.Is(err, ErrBoundary) {
		t.Fatalf("gate enabled after Start %v", err)
	}
}

// This verifies the Phase39 test-only effect callback remains unreachable
// when a signed FUTURE FENCE was produced for a different supervisor boot.
// It does not prove Product Manager.Effect or any real provider behavior.
func TestP40FreshBootFenceGuardsPhase39SyntheticEffectGate(t *testing.T) {
	var calls atomic.Int64
	f := phase40FreshFixtureWithGate(t, func(context.Context, Claim) error { calls.Add(1); return nil })
	first := phase40BootClaim(t, f, 1)
	expect(t, f, Sign(f.issuer, first, nil), "FENCED_TEST_ONLY")
	oldNext := Sign(f.issuer, phase40BootClaim(t, f, 2), nil)
	f.server.Close()
	server, err := Open(f.socketDir, f.journalDir, "installationA", testWorkerUID, f.issuer.Public().(ed25519.PublicKey), f.provider.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	f.server = server
	if err = server.EnableTestFreshBootFence(); err != nil {
		t.Fatal(err)
	}
	if err = server.SetIntegrationTestEffectGate(func(context.Context, Claim) error { calls.Add(1); return nil }); err != nil {
		t.Fatal(err)
	}
	server.Start()
	expect(t, f, oldNext, "DENIED_FRESH_BOOT_CHALLENGE")
	// Old APPLY cannot pass the volatile readyEpoch fence after restart.
	expect(t, f, Sign(f.issuer, claim(f, "APPLY", 1, "replayed40"), nil), "DENIED_RESTART_REQUIRES_FRESH_FENCE")
	if calls.Load() != 0 {
		t.Fatal("stale boot reached Effect adapter")
	}
	expect(t, f, Sign(f.issuer, phase40BootClaim(t, f, 2), nil), "FENCED_TEST_ONLY")
	w := Sign(f.issuer, claim(f, "APPLY", 2, "fresh40"), nil)
	expect(t, f, w, "RECONCILE_REQUIRED")
	expect(t, f, w, "DENIED_DUPLICATE_RECONCILE_REQUIRED")
	if calls.Load() != 1 {
		t.Fatalf("effect callback count=%d", calls.Load())
	}
}
