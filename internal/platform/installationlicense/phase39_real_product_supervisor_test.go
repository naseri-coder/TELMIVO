//go:build linux && lic39

package installationlicense

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	bridge "telmivo.local/lic-a9-phase38-supervisor-durable-bridge"
)

// The integration test is intentionally disabled during normal builds.
// It connects a real Manager.Effect to a ROOT-owned standalone Linux test
// supervisor using a real worker UID, with a test-only callback that has no
// provider/business effect. The genuine Product test is gated on Go1.27 Linux.
const phase39WorkerUID = uint32(25111)
const phase39OutsiderUID = uint32(25112)

type phase39Fixture struct {
	server                *bridge.Bridge
	socketDir, journalDir string
	issuer                ed25519.PrivateKey
	provider              ed25519.PrivateKey
	manager               *Manager
	signer                func(string, int64) []byte
	effectCount           atomic.Int64
}

func phase39FixtureOpen(t *testing.T) *phase39Fixture {
	return phase39FixtureOpenMode(t, false)
}

// Phase41 test gate: strict fresh-boot identity while preserving legacy tests.
func phase39FixtureOpenMode(t *testing.T, strictBoot bool) *phase39Fixture {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("LIC_A9_PHASE39_NOT_RUN: actual root UID and Linux required")
	}
	manager, signer := phase19TestSignedConcurrentInstallation(t)
	if e := manager.Accept(signer(manager.runtimeSession, 1)); e != nil {
		t.Fatal(e)
	}
	root, e := os.MkdirTemp("/tmp", "lic39-int-")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if e = os.Chmod(root, 0755); e != nil {
		t.Fatal(e)
	}
	f := &phase39Fixture{manager: manager, signer: signer}
	f.socketDir = filepath.Join(root, "socket")
	f.journalDir = filepath.Join(root, "journal")
	if e = os.Mkdir(f.socketDir, 0750); e != nil {
		t.Fatal(e)
	}
	if e = os.Chown(f.socketDir, 0, int(phase39WorkerUID)); e != nil {
		t.Fatal(e)
	}
	if e = os.Mkdir(f.journalDir, 0700); e != nil {
		t.Fatal(e)
	}
	_, f.issuer, e = ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	_, f.provider, e = ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	f.server, e = bridge.Open(f.socketDir, f.journalDir, "installationA", phase39WorkerUID, f.issuer.Public().(ed25519.PublicKey), f.provider.Public().(ed25519.PublicKey))
	if e != nil {
		t.Fatal(e)
	}
	if strictBoot {
		if e = f.server.EnableTestFreshBootFence(); e != nil {
			t.Fatal(e)
		}
	}
	if e = f.server.SetIntegrationTestEffectGate(func(ctx context.Context, c bridge.Claim) error {
		if c.Session != f.manager.runtimeSession {
			return ErrRestricted
		}
		return f.manager.Effect(ctx, func() error { f.effectCount.Add(1); return nil })
	}); e != nil {
		t.Fatal(e)
	}
	f.server.Start()
	t.Cleanup(func() {
		if f.server != nil {
			f.server.Close()
		}
	})
	return f
}
func phase39Claim(f *phase39Fixture, action string, epoch uint64, request string) bridge.Claim {
	now := time.Now().Unix()
	c := bridge.Claim{Protocol: bridge.Protocol, Action: action, Installation: "installationA", Owner: "ownerA", Session: f.manager.runtimeSession, Epoch: epoch, RequestID: request, NotBefore: now - 5, Expires: now + 90}
	if action == "APPLY" {
		c.Tenant = "tenantA"
		c.Actor = "actorA"
		c.Resource = "resourceA"
	}
	return c
}
func TestLICA9Phase39WorkerChild(t *testing.T) {
	if os.Getenv("LIC39_CHILD") != "YES" {
		return
	}
	var wire bridge.Wire
	if e := json.Unmarshal([]byte(os.Getenv("LIC39_WIRE")), &wire); e != nil {
		t.Fatal(e)
	}
	reply, e := bridge.Call(context.Background(), os.Getenv("LIC39_SOCKET"), wire)
	if e != nil {
		t.Log("LIC39_WORKER_DENIED=" + e.Error())
		return
	}
	t.Log("LIC39_REPLY=" + reply)
}
func phase39WorkerBinary(t *testing.T) string {
	t.Helper()
	root, e := os.MkdirTemp("/tmp", "lic39-bin-")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if e = os.Chmod(root, 0755); e != nil {
		t.Fatal(e)
	}
	// The original Go test executable can reside in a private 0700 parent
	// inaccessible to the worker UID, so copy it to a disposable root-owned
	// 0755 directory. No production executable or credentials are copied.
	src, e := os.Open(os.Args[0])
	if e != nil {
		t.Fatal(e)
	}
	defer src.Close()
	path := filepath.Join(root, "worker-test")
	dst, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
	if e != nil {
		t.Fatal(e)
	}
	_, e = io.Copy(dst, src)
	closeErr := dst.Close()
	if e != nil {
		t.Fatal(e)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if e = os.Chmod(path, 0755); e != nil {
		t.Fatal(e)
	}
	return path
}
func phase39WorkerCall(t *testing.T, bin string, f *phase39Fixture, w bridge.Wire, uid uint32) string {
	t.Helper()
	payload, e := json.Marshal(w)
	if e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(bin, "-test.run=^TestLICA9Phase39WorkerChild$", "-test.v")
	cmd.Env = []string{"LIC39_CHILD=YES", "LIC39_SOCKET=" + f.server.Address(), "LIC39_WIRE=" + string(payload), "HOME=/tmp"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uid, Gid: uid, Groups: []uint32{uid}}}
	out, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("child uid=%d err=%v log=%s", uid, e, out)
	}
	return string(out)
}
func phase39Expect(t *testing.T, bin string, f *phase39Fixture, w bridge.Wire, expected string) {
	t.Helper()
	out := phase39WorkerCall(t, bin, f, w, phase39WorkerUID)
	if !strings.Contains(out, "LIC39_REPLY="+expected) {
		t.Fatalf("expected=%s got=%s", expected, out)
	}
}
func TestLICA9Phase39RootWorkerRealManagerEffectOnceAfterFsync(t *testing.T) {
	f := phase39FixtureOpen(t)
	bin := phase39WorkerBinary(t)
	phase39Expect(t, bin, f, bridge.Sign(f.issuer, phase39Claim(f, "FENCE", 1, ""), nil), "FENCED_TEST_ONLY")
	wire := bridge.Sign(f.issuer, phase39Claim(f, "APPLY", 1, "ownerEffect39"), nil)
	phase39Expect(t, bin, f, wire, "RECONCILE_REQUIRED")
	phase39Expect(t, bin, f, wire, "DENIED_DUPLICATE_RECONCILE_REQUIRED")
	if f.effectCount.Load() != 1 {
		t.Fatal("real Manager effect escaped once-per-request", f.effectCount.Load())
	}
}
func TestLICA9Phase39RootWorkerRealManagerDeniesRevokedLease(t *testing.T) {
	f := phase39FixtureOpen(t)
	bin := phase39WorkerBinary(t)
	phase39Expect(t, bin, f, bridge.Sign(f.issuer, phase39Claim(f, "FENCE", 1, ""), nil), "FENCED_TEST_ONLY")
	// The second process has a distinct V2 session and a NEW signed lease.
	// Former Owner Manager must refuse an effect even after peer/gate checks.
	follower, e := Open(f.manager.cfg)
	if e != nil {
		t.Fatal(e)
	}
	if e = follower.Accept(f.signer(follower.runtimeSession, 2)); e != nil {
		t.Fatal(e)
	}
	wire := bridge.Sign(f.issuer, phase39Claim(f, "APPLY", 1, "revoked39"), nil)
	phase39Expect(t, bin, f, wire, "RECONCILE_REQUIRED")
	phase39Expect(t, bin, f, wire, "DENIED_DUPLICATE_RECONCILE_REQUIRED")
	if f.effectCount.Load() != 0 {
		t.Fatal("old V2 Owner authorized an effect")
	}
}
func TestLICA9Phase39RootPeerIdentityEnforcedBeforeManager(t *testing.T) {
	f := phase39FixtureOpen(t)
	bin := phase39WorkerBinary(t)
	wire := bridge.Sign(f.issuer, phase39Claim(f, "FENCE", 1, ""), nil)
	out := phase39WorkerCall(t, bin, f, wire, phase39OutsiderUID)
	if !strings.Contains(out, "LIC39_WORKER_DENIED=") && !strings.Contains(out, "LIC39_REPLY=DENIED_PEER") {
		t.Fatalf("outsider unexpectedly admitted: %s", out)
	}
	if f.effectCount.Load() != 0 {
		t.Fatal("outside UID executed effect")
	}
}
func TestLICA9Phase39RootRestartCannotReplayUnresolvedEffect(t *testing.T) {
	f := phase39FixtureOpen(t)
	bin := phase39WorkerBinary(t)
	phase39Expect(t, bin, f, bridge.Sign(f.issuer, phase39Claim(f, "FENCE", 1, ""), nil), "FENCED_TEST_ONLY")
	wire := bridge.Sign(f.issuer, phase39Claim(f, "APPLY", 1, "reopen39"), nil)
	phase39Expect(t, bin, f, wire, "RECONCILE_REQUIRED")
	if f.effectCount.Load() != 1 {
		t.Fatal("missing original Manager callback")
	}
	f.server.Close()
	var e error
	f.server, e = bridge.Open(f.socketDir, f.journalDir, "installationA", phase39WorkerUID, f.issuer.Public().(ed25519.PublicKey), f.provider.Public().(ed25519.PublicKey))
	if e != nil {
		t.Fatal(e)
	}
	if e = f.server.SetIntegrationTestEffectGate(func(ctx context.Context, c bridge.Claim) error {
		return f.manager.Effect(ctx, func() error { f.effectCount.Add(1); return nil })
	}); e != nil {
		t.Fatal(e)
	}
	f.server.Start()
	phase39Expect(t, bin, f, wire, "DENIED_RESTART_REQUIRES_FRESH_FENCE")
	later := bridge.Sign(f.issuer, phase39Claim(f, "FENCE", 2, ""), nil)
	phase39Expect(t, bin, f, later, "RECONCILE_REQUIRED")
	if f.effectCount.Load() != 1 {
		t.Fatal("unknown prior outcome retried")
	}
}
func TestLICA9Phase39RootForgedPeerSignatureCannotRunEffect(t *testing.T) {
	f := phase39FixtureOpen(t)
	bin := phase39WorkerBinary(t)
	phase39Expect(t, bin, f, bridge.Sign(f.issuer, phase39Claim(f, "FENCE", 1, ""), nil), "FENCED_TEST_ONLY")
	_, fake, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	wire := bridge.Sign(fake, phase39Claim(f, "APPLY", 1, "forged39"), nil)
	phase39Expect(t, bin, f, wire, "DENIED_SIGNATURE")
	if f.effectCount.Load() != 0 {
		t.Fatal("forged signature reached Manager")
	}
}
func TestLICA9Phase39RootDomainDenialPrecedesManager(t *testing.T) {
	f := phase39FixtureOpen(t)
	bin := phase39WorkerBinary(t)
	phase39Expect(t, bin, f, bridge.Sign(f.issuer, phase39Claim(f, "FENCE", 1, ""), nil), "FENCED_TEST_ONLY")
	invalid := phase39Claim(f, "APPLY", 1, "foreignTenant39")
	invalid.Tenant = "tenantB"
	phase39Expect(t, bin, f, bridge.Sign(f.issuer, invalid, nil), "DENIED_DOMAIN")
	if f.effectCount.Load() != 0 {
		t.Fatal("cross-tenant request reached Manager")
	}
}
