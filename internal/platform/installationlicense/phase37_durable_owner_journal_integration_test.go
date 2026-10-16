package installationlicense

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

// This file runs against the REAL Product Manager on Go1.27, and the local
// Go1.23 compatibility fixture during offline contract development. The
// provider signature and journal are SYNTHETIC, not production services.
func phase37TestJournal(t *testing.T) (phase36Fixture, *phase37Journal, ed25519.PrivateKey, string) {
	t.Helper()
	f := phase36Setup(t)
	root := filepath.Join(t.TempDir(), "journal")
	if e := os.Mkdir(root, 0700); e != nil {
		t.Fatal(e)
	}
	pub, priv, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	j, e := phase37Open(root, pub)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = j.close() })
	f.relay.mu.Lock()
	f.relay.journal = j
	f.relay.mu.Unlock()
	return f, j, priv, root
}
func TestLICA9Phase37RealManagerEffectWithDurableReservation(t *testing.T) {
	f, j, _, _ := phase37TestJournal(t)
	c := f.claim("durable-effect")
	phase36Reply(t, f.relay, c, "SYNTHETIC_EFFECT_ACCEPTED")
	got, e := j.inspect(c.Claim)
	if e != nil || got != "RECONCILE_REQUIRED" || f.relay.effectCount.Load() != 1 {
		t.Fatal(got, e)
	}
}
func TestLICA9Phase37CrashReopenDeniesBlindRetry(t *testing.T) {
	f, j, key, root := phase37TestJournal(t)
	c := f.claim("unknown-crash")
	f.relay.mu.Lock()
	f.relay.failNext = true
	f.relay.mu.Unlock()
	phase36Reply(t, f.relay, c, "RECONCILE_REQUIRED")
	reopened, e := phase37Open(root, key.Public().(ed25519.PublicKey))
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.close()
	f.relay.mu.Lock()
	f.relay.journal = reopened
	f.relay.mu.Unlock()
	phase36Reply(t, f.relay, c, "DENIED_DUPLICATE_OR_RECONCILE_REQUIRED")
	if f.relay.effectCount.Load() != 1 {
		t.Fatal("unsafe retry")
	}
	_ = j
}
func TestLICA9Phase37ValidSignedSyntheticReceipt(t *testing.T) {
	f, j, key, _ := phase37TestJournal(t)
	c := f.claim("confirmed")
	phase36Reply(t, f.relay, c, "SYNTHETIC_EFFECT_ACCEPTED")
	if e := j.confirm(phase37SyntheticReceipt(key, c.Claim, "provider-test")); e != nil {
		t.Fatal(e)
	}
	result, e := j.inspect(c.Claim)
	if e != nil || result != "CONFIRMED_WITH_SYNTHETIC_PROVIDER_EVIDENCE" {
		t.Fatal(result, e)
	}
	phase36Reply(t, f.relay, c, "DENIED_DUPLICATE_OR_RECONCILE_REQUIRED")
}
func TestLICA9Phase37ForgedProofRejected(t *testing.T) {
	f, j, _, _ := phase37TestJournal(t)
	c := f.claim("forged")
	if e := j.reserve(c.Claim); e != nil {
		t.Fatal(e)
	}
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	if e := j.confirm(phase37SyntheticReceipt(key, c.Claim, "forged")); !errors.Is(e, phase37BadReceipt) {
		t.Fatal(e)
	}
}
func TestLICA9Phase37CrossTenantReusesIDNotAllowed(t *testing.T) {
	f, j, _, _ := phase37TestJournal(t)
	c := f.claim("shared-id").Claim
	if e := j.reserve(c); e != nil {
		t.Fatal(e)
	}
	c.WorkspaceID = "other-tenant"
	if e := j.reserve(c); !errors.Is(e, phase37Duplicate) {
		t.Fatal(e)
	}
	if _, e := j.inspect(c); !errors.Is(e, phase37Corrupt) {
		t.Fatal(e)
	}
}
func TestLICA9Phase37TruncatedJournalDenies(t *testing.T) {
	f, j, _, root := phase37TestJournal(t)
	c := f.claim("truncated").Claim
	if e := j.reserve(c); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(root, "r-"+phase37Name(c.RequestID)), []byte("{"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := j.inspect(c); !errors.Is(e, phase37Corrupt) {
		t.Fatal(e)
	}
}
func TestLICA9Phase37ConcurrentJournalHandlesHaveSingleWinner(t *testing.T) {
	f, _, key, root := phase37TestJournal(t)
	claim := f.claim("simultaneous").Claim
	var won, duplicated atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			handle, e := phase37Open(root, key.Public().(ed25519.PublicKey))
			if e != nil {
				t.Error(e)
				return
			}
			defer handle.close()
			e = handle.reserve(claim)
			if e == nil {
				won.Add(1)
			} else if errors.Is(e, phase37Duplicate) {
				duplicated.Add(1)
			} else {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if won.Load() != 1 || duplicated.Load() != 7 {
		t.Fatal(won.Load(), duplicated.Load())
	}
}
func TestLICA9Phase37FormerOwnerStillDeniedByRealManager(t *testing.T) {
	f, j, _, _ := phase37TestJournal(t)
	if e := f.follower.Accept(f.signer(f.follower.runtimeSession, 2)); e != nil {
		t.Fatal(e)
	}
	c := f.claim("revoked")
	phase36Reply(t, f.relay, c, "DENIED_LICENSE_SESSION")
	result, e := j.inspect(c.Claim)
	if e != nil || result != "RECONCILE_REQUIRED" || f.relay.effectCount.Load() != 0 {
		t.Fatal(result, e)
	}
}

// The child deliberately exits immediately after fsync-backed reservation:
// the parent must still observe an unreconciled intent on a fresh handle.
func TestLICA9Phase37AbruptChildExitAfterFsync(t *testing.T) {
	if os.Getenv("LIC_A9_PHASE37_CHILD") == "YES" {
		pub, e := hex.DecodeString(os.Getenv("LIC_A9_PHASE37_PUB"))
		if e != nil {
			t.Fatal(e)
		}
		j, e := phase37Open(os.Getenv("LIC_A9_PHASE37_ROOT"), ed25519.PublicKey(pub))
		if e != nil {
			t.Fatal(e)
		}
		c := phase36Claim{Protocol: phase36Protocol, OwnerSession: "syntheticOwner", WorkspaceID: "tenantA", ActorID: "actorA", ResourceID: "resourceA", RequestID: "child-durable", ExpiresUnix: 123456789}
		if e := j.reserve(c); e != nil {
			t.Fatal(e)
		}
		os.Exit(77)
	}
	_, j, key, root := phase37TestJournal(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestLICA9Phase37AbruptChildExitAfterFsync$")
	cmd.Env = append(os.Environ(), "LIC_A9_PHASE37_CHILD=YES", "LIC_A9_PHASE37_ROOT="+root, "LIC_A9_PHASE37_PUB="+hex.EncodeToString(key.Public().(ed25519.PublicKey)))
	out, e := cmd.CombinedOutput()
	var ex *exec.ExitError
	if !errors.As(e, &ex) || ex.ExitCode() != 77 {
		t.Fatalf("expected abrupt exit code 77: %v %s", e, out)
	}
	c := phase36Claim{Protocol: phase36Protocol, OwnerSession: "syntheticOwner", WorkspaceID: "tenantA", ActorID: "actorA", ResourceID: "resourceA", RequestID: "child-durable", ExpiresUnix: 123456789}
	status, e := j.inspect(c)
	if e != nil || status != "RECONCILE_REQUIRED" {
		t.Fatal(status, e)
	}
	if e = j.reserve(c); !errors.Is(e, phase37Duplicate) {
		t.Fatal(e)
	}
}
