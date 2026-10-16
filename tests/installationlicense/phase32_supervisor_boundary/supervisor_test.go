//go:build linux

package supervisor

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	witness "telmivo.local/lic-a9-phase31-durable-witness"
)

const ownerUID = 25111
const outsiderUID = 25112

var helperBinary string

// The standard go test harness stores binaries under a root-only temporary
// directory. Copy its exact -race instrumented binary to a traversable
// disposable path for actual setuid subprocesses.
func TestMain(m *testing.M) {
	if os.Getenv("P32_CHILD") == "1" {
		os.Exit(m.Run())
	}
	dir, err := os.MkdirTemp("", "p32-testbin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err = os.Chmod(dir, 0755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	helperBinary = filepath.Join(dir, "tested.test")
	if err = os.Link(os.Args[0], helperBinary); err != nil {
		in, e := os.Open(os.Args[0])
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(2)
		}
		out, e := os.OpenFile(helperBinary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0755)
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(2)
		}
		_, err = io.Copy(out, in)
		in.Close()
		out.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}
	status := m.Run()
	os.RemoveAll(dir)
	os.Exit(status)
}

type fixture struct {
	srv        *Server
	root       string
	socketDir  string
	witnessDir string
	private    ed25519.PrivateKey
	issuer     ed25519.PublicKey
}

func setup(t *testing.T) *fixture {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("root required for actual Linux multiple-UID test")
	}
	root, err := os.MkdirTemp("", "lic-a9-phase32-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	if err = os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	socketDir := filepath.Join(root, "ipc")
	witnessDir := filepath.Join(root, "witness")
	if err = os.Mkdir(socketDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err = os.Chown(socketDir, 0, ownerUID); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(socketDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(witnessDir, 0700); err != nil {
		t.Fatal(err)
	}
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	server, err := OpenSecured(socketDir, witnessDir, "testInstallationA", ownerUID, ownerUID, pub, mockEffect{allow: true}, mockPolicy{allow: true})
	if err != nil {
		t.Fatal(err)
	}
	go server.Serve()
	f := &fixture{server, root, socketDir, witnessDir, private, pub}
	t.Cleanup(func() { f.srv.Close() })
	return f
}
func grant(f *fixture, action string, epoch uint64, id string) Wire {
	return wireClaim(f.private, action, epoch, id)
}
func wireClaim(private ed25519.PrivateKey, action string, epoch uint64, id string) Wire {
	now := time.Now().Unix()
	c := Claim{Protocol: Protocol, Action: action, Installation: "testInstallationA", Owner: "ownerA", Session: fmt.Sprintf("session%d", epoch), Epoch: epoch, PreviousEpoch: epoch - 1, RequestID: id, NotBefore: now - 1, ExpiresAt: now + 100}
	if action == ActionApply {
		c.WorkspaceID, c.ActorID, c.ResourceID = "tenantA", "actorA", "recordA"
	}
	return SignedTestWire(c, private)
}
func startHelper(t *testing.T, mode, addr, witnessRoot, socketDir string, w Wire, uid uint32) *exec.Cmd {
	t.Helper()
	wire, _ := json.Marshal(w)
	cmd := exec.Command(helperBinary, "-test.run=^TestP32ChildHelper$", "-test.v")
	cmd.Env = append(os.Environ(), "P32_CHILD=1", "P32_MODE="+mode, "P32_ADDR="+addr, "P32_WITNESS="+witnessRoot, "P32_SOCKETDIR="+socketDir, "P32_JSON="+string(wire))
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uid, Gid: uid, Groups: []uint32{uid}}}
	return cmd
}
func childOutput(t *testing.T, mode string, f *fixture, w Wire, uid uint32) string {
	t.Helper()
	cmd := startHelper(t, mode, f.srv.Address(), f.witnessDir, f.socketDir, w, uid)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s (uid %d) failed: %v\n%s", mode, uid, err, out)
	}
	return string(out)
}
func expectReply(t *testing.T, f *fixture, w Wire, want string) {
	t.Helper()
	out := childOutput(t, "call", f, w, ownerUID)
	if !strings.Contains(out, "P32_REPLY="+want) {
		t.Fatalf("want %q got %s", want, out)
	}
}
func TestP32ChildHelper(t *testing.T) {
	if os.Getenv("P32_CHILD") != "1" {
		return
	}
	var w Wire
	if err := json.Unmarshal([]byte(os.Getenv("P32_JSON")), &w); err != nil {
		t.Fatal(err)
	}
	addr := os.Getenv("P32_ADDR")
	switch os.Getenv("P32_MODE") {
	case "call":
		reply, err := Call(context.Background(), addr, w)
		if err != nil {
			t.Fatal(err)
		}
		t.Log("P32_REPLY=" + reply)
	case "outsider":
		_, err := Call(context.Background(), addr, w)
		if err == nil {
			t.Fatal("unapproved UID connected")
		}
		var op *net.OpError
		if !errors.As(err, &op) {
			t.Fatal("unexpected dial error", err)
		}
		t.Log("P32_DENIED_BY_KERNEL")
	case "tamper":
		forbidden := []string{addr, filepath.Join(os.Getenv("P32_WITNESS"), "witness.lock"), filepath.Join(os.Getenv("P32_WITNESS"), "epoch.json")}
		for _, path := range forbidden {
			err := os.Remove(path)
			if !errors.Is(err, syscall.EACCES) && !errors.Is(err, syscall.EPERM) {
				t.Fatalf("unlink not denied: %s: %v", path, err)
			}
		}
		name := filepath.Join(os.Getenv("P32_SOCKETDIR"), "forged.sock")
		l, err := net.Listen("unix", name)
		if err == nil {
			l.Close()
			t.Fatal("untrusted UID bound socket")
		}
		wpath := filepath.Join(os.Getenv("P32_WITNESS"), "epoch.json")
		if f, err := os.OpenFile(wpath, os.O_WRONLY|os.O_TRUNC, 0600); err == nil {
			f.Close()
			t.Fatal("untrusted UID truncated witness")
		}
		t.Log("P32_HOSTILE_UID_PATH_MUTATION_DENIED")
	default:
		t.Fatal("invalid mode")
	}
}
func TestP32RealWorkerUIDAndSignedFence(t *testing.T) {
	f := setup(t)
	expectReply(t, f, grant(f, ActionFence, 1, ""), "FENCED_TEST_ONLY")
	expectReply(t, f, grant(f, ActionApply, 1, "recordA"), "APPLIED_SYNTHETIC_TEST_ONLY")
	w, err := witness.Open(f.witnessDir)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	applied, err := w.WasApplied("recordA")
	if err != nil || !applied {
		t.Fatal(applied, err)
	}
}
func TestP32DifferentUnregisteredUIDKernelDenied(t *testing.T) {
	f := setup(t)
	out := childOutput(t, "outsider", f, grant(f, ActionFence, 1, ""), outsiderUID)
	if !strings.Contains(out, "P32_DENIED_BY_KERNEL") {
		t.Fatal(out)
	}
}
func TestP32OwnerUIDCannotReplaceSocketOrWitness(t *testing.T) {
	f := setup(t)
	expectReply(t, f, grant(f, ActionFence, 1, ""), "FENCED_TEST_ONLY")
	out := childOutput(t, "tamper", f, grant(f, ActionApply, 1, "recordA"), ownerUID)
	if !strings.Contains(out, "P32_HOSTILE_UID_PATH_MUTATION_DENIED") {
		t.Fatal(out)
	}
	expectReply(t, f, grant(f, ActionApply, 1, "recordA"), "APPLIED_SYNTHETIC_TEST_ONLY")
}
func TestP32NoApplyBeforeFreshGrantAfterRestart(t *testing.T) {
	f := setup(t)
	expectReply(t, f, grant(f, ActionFence, 1, ""), "FENCED_TEST_ONLY")
	old := grant(f, ActionApply, 1, "oldCrash")
	f.srv.Close()
	reopened, err := OpenSecured(f.socketDir, f.witnessDir, "testInstallationA", ownerUID, ownerUID, f.issuer, mockEffect{allow: true}, mockPolicy{allow: true})
	if err != nil {
		t.Fatal(err)
	}
	f.srv = reopened
	go reopened.Serve()
	expectReply(t, f, old, "DENIED_NOT_READY_OR_STALE_SESSION")
	expectReply(t, f, grant(f, ActionFence, 2, ""), "FENCED_TEST_ONLY")
	expectReply(t, f, old, "DENIED_NOT_READY_OR_STALE_SESSION")
	expectReply(t, f, grant(f, ActionApply, 2, "newCrash"), "APPLIED_SYNTHETIC_TEST_ONLY")
}
func TestP32InvalidSignatureFailsClosed(t *testing.T) {
	f := setup(t)
	wire := grant(f, ActionFence, 1, "")
	wire.Signature = strings.Repeat("0", 128)
	expectReply(t, f, wire, "DENIED_SIGNATURE_OR_REQUEST")
}
func TestP32CrossInstallationSignedClaimDenied(t *testing.T) {
	f := setup(t)
	w := grant(f, ActionFence, 1, "")
	w.Claim.Installation = "other"
	w = SignedTestWire(w.Claim, f.private)
	expectReply(t, f, w, "DENIED_SIGNATURE_OR_REQUEST")
}
func TestP32OldEpochDeniedAfterNewFence(t *testing.T) {
	f := setup(t)
	expectReply(t, f, grant(f, ActionFence, 1, ""), "FENCED_TEST_ONLY")
	expectReply(t, f, grant(f, ActionFence, 2, ""), "FENCED_TEST_ONLY")
	expectReply(t, f, grant(f, ActionApply, 1, "old"), "DENIED_NOT_READY_OR_STALE_SESSION")
}
func TestP32ReplayAfterCrashNotReexecuted(t *testing.T) {
	f := setup(t)
	expectReply(t, f, grant(f, ActionFence, 1, ""), "FENCED_TEST_ONLY")
	wire := grant(f, ActionApply, 1, "identical")
	expectReply(t, f, wire, "APPLIED_SYNTHETIC_TEST_ONLY")
	expectReply(t, f, wire, "DENIED_FINAL_GATE_POLICY_OR_REPLAY")
}
func TestP32ParallelOwnerUIDProcessesOneSyntheticEffect(t *testing.T) {
	f := setup(t)
	expectReply(t, f, grant(f, ActionFence, 1, ""), "FENCED_TEST_ONLY")
	signed := grant(f, ActionApply, 1, "sameRequest")
	const n = 4
	cmds := make([]*exec.Cmd, n)
	outputs := make([][]byte, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		cmds[i] = startHelper(t, "call", f.srv.Address(), f.witnessDir, f.socketDir, signed, ownerUID)
		wg.Add(1)
		go func(i int) { defer wg.Done(); outputs[i], errs[i] = cmds[i].CombinedOutput() }(i)
	}
	wg.Wait()
	success := 0
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("child %d %v %s", i, errs[i], outputs[i])
		}
		if strings.Contains(string(outputs[i]), "P32_REPLY=APPLIED_SYNTHETIC_TEST_ONLY") {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("want 1, got %d", success)
	}
}
func TestP32SocketModeAndNamespaceOwner(t *testing.T) {
	f := setup(t)
	dir, err := os.Stat(f.socketDir)
	if err != nil {
		t.Fatal(err)
	}
	st := dir.Sys().(*syscall.Stat_t)
	if st.Uid != 0 || st.Gid != ownerUID || dir.Mode().Perm() != 0750 {
		t.Fatal(st, dir.Mode())
	}
	info, err := os.Stat(f.srv.Address())
	if err != nil {
		t.Fatal(err)
	}
	st = info.Sys().(*syscall.Stat_t)
	if st.Uid != 0 || st.Gid != ownerUID || info.Mode().Perm() != 0660 {
		t.Fatal(st, info.Mode())
	}
}
func TestP32InsecureDirRejected(t *testing.T) {
	f := setup(t)
	_ = f.srv.Close
	if err := os.Chmod(f.socketDir, 0777); err != nil {
		t.Fatal(err)
	}
	if s, err := Open(f.socketDir, f.witnessDir, "testInstallationA", ownerUID, ownerUID, f.issuer); err == nil {
		s.Close()
		t.Fatal("unsafe directory admitted")
	}
}
func TestP32NoActualSideEffectAvailable(t *testing.T) {
	f := setup(t)
	w := grant(f, "ARBITRARY_PAYMENT", 1, "purchase1")
	expectReply(t, f, w, "DENIED_SIGNATURE_OR_REQUEST")
}
func TestP32ExpiredIssuerClaimDenied(t *testing.T) {
	f := setup(t)
	w := grant(f, ActionFence, 1, "")
	w.Claim.ExpiresAt = time.Now().Unix() - 3
	w = SignedTestWire(w.Claim, f.private)
	expectReply(t, f, w, "DENIED_SIGNATURE_OR_REQUEST")
}
