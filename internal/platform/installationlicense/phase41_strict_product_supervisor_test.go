//go:build linux && lic39 && lic41

package installationlicense

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"

	bridge "telmivo.local/lic-a9-phase38-supervisor-durable-bridge"
)

// Phase41 only exercises a disposable nonproduction integration. The fixture
// calls the real Manager.Effect when compiled in the real Product package.
// The local Go1.23 sandbox uses an explicitly labeled compatible STUB.
func phase41StrictFixture(t *testing.T) *phase39Fixture {
	t.Helper()
	return phase39FixtureOpenMode(t, true)
}
func phase41FreshFence(t *testing.T, f *phase39Fixture, epoch uint64) bridge.Claim {
	t.Helper()
	nonce, err := f.server.TestBootChallenge()
	if err != nil || len(nonce) != 64 {
		t.Fatalf("root-only fresh challenge error: %v", err)
	}
	c := phase39Claim(f, "FENCE", epoch, "")
	c.BootChallenge = nonce
	return c
}
func phase41ReopenStrict(t *testing.T, f *phase39Fixture) {
	t.Helper()
	f.server.Close()
	s, err := bridge.Open(f.socketDir, f.journalDir, "installationA", phase39WorkerUID, f.issuer.Public().(ed25519.PublicKey), f.provider.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	f.server = s
	if err = s.EnableTestFreshBootFence(); err != nil {
		t.Fatal(err)
	}
	if err = s.SetIntegrationTestEffectGate(func(ctx context.Context, c bridge.Claim) error {
		if c.Session != f.manager.runtimeSession {
			return ErrRestricted
		}
		return f.manager.Effect(ctx, func() error { f.effectCount.Add(1); return nil })
	}); err != nil {
		t.Fatal(err)
	}
	s.Start()
}

func TestLICA9Phase41StrictFreshBootRequiredBeforeProductEffect(t *testing.T) {
	f := phase41StrictFixture(t)
	bin := phase39WorkerBinary(t)
	phase39Expect(t, bin, f, bridge.Sign(f.issuer, phase39Claim(f, "FENCE", 1, ""), nil), "DENIED_FRESH_BOOT_CHALLENGE")
	if f.effectCount.Load() != 0 {
		t.Fatal("denied FENCE triggered Manager")
	}
	phase39Expect(t, bin, f, bridge.Sign(f.issuer, phase41FreshFence(t, f, 1), nil), "FENCED_TEST_ONLY")
	w := bridge.Sign(f.issuer, phase39Claim(f, "APPLY", 1, "effect41"), nil)
	phase39Expect(t, bin, f, w, "RECONCILE_REQUIRED")
	phase39Expect(t, bin, f, w, "DENIED_DUPLICATE_RECONCILE_REQUIRED")
	if f.effectCount.Load() != 1 {
		t.Fatalf("Manager callback count=%d", f.effectCount.Load())
	}
}
func TestLICA9Phase41RecordedFutureSignedFenceCannotRearmAfterRestart(t *testing.T) {
	f := phase41StrictFixture(t)
	bin := phase39WorkerBinary(t)
	phase39Expect(t, bin, f, bridge.Sign(f.issuer, phase41FreshFence(t, f, 1), nil), "FENCED_TEST_ONLY")
	future := bridge.Sign(f.issuer, phase41FreshFence(t, f, 2), nil)
	oldNonce := future.Claim.BootChallenge
	phase41ReopenStrict(t, f)
	newNonce, err := f.server.TestBootChallenge()
	if err != nil || newNonce == oldNonce {
		t.Fatal("supervisor reused boot challenge")
	}
	phase39Expect(t, bin, f, future, "DENIED_FRESH_BOOT_CHALLENGE")
	phase39Expect(t, bin, f, bridge.Sign(f.issuer, phase39Claim(f, "APPLY", 1, "stale41"), nil), "DENIED_RESTART_REQUIRES_FRESH_FENCE")
	if f.effectCount.Load() != 0 {
		t.Fatal("old signed boot accessed Manager")
	}
	phase39Expect(t, bin, f, bridge.Sign(f.issuer, phase41FreshFence(t, f, 2), nil), "FENCED_TEST_ONLY")
	phase39Expect(t, bin, f, bridge.Sign(f.issuer, phase39Claim(f, "APPLY", 2, "fresh41"), nil), "RECONCILE_REQUIRED")
	if f.effectCount.Load() != 1 {
		t.Fatalf("fresh session didn't dispatch once: %d", f.effectCount.Load())
	}
}
func TestLICA9Phase41UncertainEffectStillBlocksFreshRearm(t *testing.T) {
	f := phase41StrictFixture(t)
	bin := phase39WorkerBinary(t)
	phase39Expect(t, bin, f, bridge.Sign(f.issuer, phase41FreshFence(t, f, 1), nil), "FENCED_TEST_ONLY")
	w := bridge.Sign(f.issuer, phase39Claim(f, "APPLY", 1, "uncertain41"), nil)
	phase39Expect(t, bin, f, w, "RECONCILE_REQUIRED")
	phase41ReopenStrict(t, f)
	phase39Expect(t, bin, f, bridge.Sign(f.issuer, phase41FreshFence(t, f, 2), nil), "RECONCILE_REQUIRED")
	phase39Expect(t, bin, f, w, "DENIED_RESTART_REQUIRES_FRESH_FENCE")
	if f.effectCount.Load() != 1 {
		t.Fatalf("unknown effect retried: %d", f.effectCount.Load())
	}
}
func TestLICA9Phase41AuthenticV2RevocationPrevailsOverFreshBoot(t *testing.T) {
	f := phase41StrictFixture(t)
	bin := phase39WorkerBinary(t)
	phase39Expect(t, bin, f, bridge.Sign(f.issuer, phase41FreshFence(t, f, 1), nil), "FENCED_TEST_ONLY")
	follower, err := Open(f.manager.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = follower.Accept(f.signer(follower.runtimeSession, 2)); err != nil {
		t.Fatal(err)
	}
	phase39Expect(t, bin, f, bridge.Sign(f.issuer, phase39Claim(f, "APPLY", 1, "revoked41"), nil), "RECONCILE_REQUIRED")
	if f.effectCount.Load() != 0 {
		t.Fatal("old V2 Manager invoked after handoff")
	}
}
func TestLICA9Phase41OutsiderAndSignedClaimTamperingCannotFence(t *testing.T) {
	f := phase41StrictFixture(t)
	bin := phase39WorkerBinary(t)
	valid := bridge.Sign(f.issuer, phase41FreshFence(t, f, 1), nil)
	outsider := phase39WorkerCall(t, bin, f, valid, phase39OutsiderUID)
	if !strings.Contains(outsider, "LIC39_WORKER_DENIED=") && !strings.Contains(outsider, "LIC39_REPLY=DENIED_PEER") {
		t.Fatalf("outsider was admitted: %s", outsider)
	}
	valid.Claim.BootChallenge = strings.Repeat("f", 64)
	phase39Expect(t, bin, f, valid, "DENIED_SIGNATURE")
	phase39Expect(t, bin, f, bridge.Sign(f.issuer, phase41FreshFence(t, f, 1), nil), "FENCED_TEST_ONLY")
	if f.effectCount.Load() != 0 {
		t.Fatal("denied requests reached Manager")
	}
	_, fake, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	phase39Expect(t, bin, f, bridge.Sign(fake, phase39Claim(f, "APPLY", 1, "forged41"), nil), "DENIED_SIGNATURE")
}
