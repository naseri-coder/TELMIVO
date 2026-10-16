//go:build linux

package bridge

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const testWorkerUID = 25111
const testOutsiderUID = 25112

var childBinary string

func TestMain(m *testing.M) {
	if os.Getenv("PHASE38_CHILD") != "" {
		os.Exit(m.Run())
	}
	root, e := os.MkdirTemp("/tmp", "phase38-exec-")
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(2)
	}
	_ = os.Chmod(root, 0755)
	childBinary = filepath.Join(root, "tested.test")
	if e = os.Link(os.Args[0], childBinary); e != nil {
		src, er := os.Open(os.Args[0])
		if er != nil {
			panic(er)
		}
		dst, er := os.OpenFile(childBinary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0755)
		if er != nil {
			panic(er)
		}
		_, er = dst.ReadFrom(src)
		src.Close()
		dst.Close()
		if er != nil {
			panic(er)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(root)
	os.Exit(code)
}

type fixture struct {
	root, socketDir, journalDir string
	server                      *Bridge
	issuer                      ed25519.PrivateKey
	provider                    ed25519.PrivateKey
}

func makeFixture(t *testing.T) *fixture {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("actual multi-UID test requires isolated root fixture")
	}
	root, e := os.MkdirTemp("/tmp", "lic38-")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	if e = os.Chmod(root, 0755); e != nil {
		t.Fatal(e)
	}
	socketDir := filepath.Join(root, "socket")
	journalDir := filepath.Join(root, "journal")
	if e = os.Mkdir(socketDir, 0750); e != nil {
		t.Fatal(e)
	}
	if e = os.Chown(socketDir, 0, testWorkerUID); e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(socketDir, 0750); e != nil {
		t.Fatal(e)
	}
	if e = os.Mkdir(journalDir, 0700); e != nil {
		t.Fatal(e)
	}
	_, issuer, _ := ed25519.GenerateKey(rand.Reader)
	_, provider, _ := ed25519.GenerateKey(rand.Reader)
	b, e := Open(socketDir, journalDir, "installationA", testWorkerUID, issuer.Public().(ed25519.PublicKey), provider.Public().(ed25519.PublicKey))
	if e != nil {
		t.Fatal(e)
	}
	b.Start()
	f := &fixture{root, socketDir, journalDir, b, issuer, provider}
	t.Cleanup(func() {
		if f.server != nil {
			f.server.Close()
		}
	})
	return f
}
func claim(f *fixture, action string, epoch uint64, id string) Claim {
	now := time.Now().Unix()
	c := Claim{Protocol: Protocol, Action: action, Installation: "installationA", Owner: "ownerA", Session: fmt.Sprintf("session%d", epoch), Epoch: epoch, RequestID: id, NotBefore: now - 5, Expires: now + 120}
	if action == "APPLY" {
		c.Tenant, c.Actor, c.Resource = "tenantA", "actorA", "resourceA"
	}
	return c
}
func runChild(t *testing.T, f *fixture, wire Wire, mode string, uid uint32) string {
	t.Helper()
	raw, _ := json.Marshal(wire)
	cmd := exec.Command(childBinary, "-test.run=^TestP38Child$", "-test.v")
	cmd.Env = append(os.Environ(), "PHASE38_CHILD=1", "PHASE38_PATH="+f.server.Address(), "PHASE38_JOURNAL="+f.journalDir, "PHASE38_SOCKETDIR="+f.socketDir, "PHASE38_WIRE="+string(raw), "PHASE38_MODE="+mode)
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uid, Gid: uid, Groups: []uint32{uid}}}
	out, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("child mode=%s uid=%d: %v\n%s", mode, uid, e, out)
	}
	return string(out)
}
func expect(t *testing.T, f *fixture, wire Wire, want string) {
	t.Helper()
	out := runChild(t, f, wire, "call", testWorkerUID)
	if !strings.Contains(out, "PHASE38_REPLY="+want) {
		t.Fatalf("expected %q: %s", want, out)
	}
}
func TestP38Child(t *testing.T) {
	if os.Getenv("PHASE38_CHILD") == "" {
		return
	}
	var w Wire
	if e := json.Unmarshal([]byte(os.Getenv("PHASE38_WIRE")), &w); e != nil {
		t.Fatal(e)
	}
	path := os.Getenv("PHASE38_PATH")
	switch os.Getenv("PHASE38_MODE") {
	case "call":
		result, e := Call(context.Background(), path, w)
		if e != nil {
			t.Fatal(e)
		}
		t.Log("PHASE38_REPLY=" + result)
	case "outsider":
		_, e := Call(context.Background(), path, w)
		if e == nil {
			t.Fatal("outsider unexpectedly called socket")
		}
		var op *net.OpError
		if !errors.As(e, &op) {
			t.Fatal(e)
		}
		t.Log("PHASE38_KERNEL_DENIED")
	case "tamper":
		for _, p := range []string{path, filepath.Join(os.Getenv("PHASE38_JOURNAL"), "_root.lock"), filepath.Join(os.Getenv("PHASE38_JOURNAL"), "epoch.json")} {
			e := os.Remove(p)
			if !errors.Is(e, syscall.EPERM) && !errors.Is(e, syscall.EACCES) {
				t.Fatalf("untrusted unlink not denied: %s: %v", p, e)
			}
		}
		for _, p := range []string{filepath.Join(os.Getenv("PHASE38_JOURNAL"), "epoch.json"), filepath.Join(os.Getenv("PHASE38_SOCKETDIR"), "forged.sock")} {
			e := os.WriteFile(p, []byte("ATTACK"), 0600)
			if e == nil {
				t.Fatalf("untrusted write allowed to %s", p)
			}
		}
		t.Log("PHASE38_PATH_MUTATION_DENIED")
	default:
		t.Fatal("unknown helper")
	}
}
func TestP38PermittedWorkerUidAndDurableReserve(t *testing.T) {
	f := makeFixture(t)
	expect(t, f, Sign(f.issuer, claim(f, "FENCE", 1, ""), nil), "FENCED_TEST_ONLY")
	one := claim(f, "APPLY", 1, "requestA")
	expect(t, f, Sign(f.issuer, one, nil), "RECONCILE_REQUIRED")
	expect(t, f, Sign(f.issuer, one, nil), "DENIED_DUPLICATE_RECONCILE_REQUIRED")
	epoch, e := f.server.DebugEpoch()
	if e != nil || epoch != 1 {
		t.Fatal(epoch, e)
	}
}
func TestP38KernelRefusesUnapprovedUID(t *testing.T) {
	f := makeFixture(t)
	out := runChild(t, f, Sign(f.issuer, claim(f, "FENCE", 1, ""), nil), "outsider", testOutsiderUID)
	if !strings.Contains(out, "PHASE38_KERNEL_DENIED") {
		t.Fatal(out)
	}
}
func TestP38UnprivilegedWorkerCannotReplaceRootJournal(t *testing.T) {
	f := makeFixture(t)
	out := runChild(t, f, Sign(f.issuer, claim(f, "FENCE", 1, ""), nil), "tamper", testWorkerUID)
	if !strings.Contains(out, "PHASE38_PATH_MUTATION_DENIED") {
		t.Fatal(out)
	}
	expect(t, f, Sign(f.issuer, claim(f, "FENCE", 1, ""), nil), "FENCED_TEST_ONLY")
}
func TestP38SignedTenantAndForgedKeyDenied(t *testing.T) {
	f := makeFixture(t)
	expect(t, f, Sign(f.issuer, claim(f, "FENCE", 1, ""), nil), "FENCED_TEST_ONLY")
	bad := claim(f, "APPLY", 1, "badtenant")
	bad.Tenant = "tenantB"
	expect(t, f, Sign(f.issuer, bad, nil), "DENIED_DOMAIN")
	a := claim(f, "APPLY", 1, "forged")
	_, stranger, _ := ed25519.GenerateKey(rand.Reader)
	expect(t, f, Sign(stranger, a, nil), "DENIED_SIGNATURE")
	modified := Sign(f.issuer, a, nil)
	modified.Claim.Actor = "hacker"
	expect(t, f, modified, "DENIED_SIGNATURE")
}
func TestP38PendingBlocksHandoffUntilSignedReceipt(t *testing.T) {
	f := makeFixture(t)
	first := claim(f, "FENCE", 1, "")
	expect(t, f, Sign(f.issuer, first, nil), "FENCED_TEST_ONLY")
	a := claim(f, "APPLY", 1, "reqA")
	expect(t, f, Sign(f.issuer, a, nil), "RECONCILE_REQUIRED")
	next := claim(f, "FENCE", 2, "")
	expect(t, f, Sign(f.issuer, next, nil), "RECONCILE_REQUIRED")
	receipt := SignReceipt(f.provider, a, "syntheticProviderA")
	_, stranger, _ := ed25519.GenerateKey(rand.Reader)
	forged := SignReceipt(stranger, a, "syntheticProviderA")
	confirm := claim(f, "CONFIRM", 1, "reqA")
	expect(t, f, Sign(f.issuer, confirm, &forged), "DENIED_PROVIDER_PROOF")
	expect(t, f, Sign(f.issuer, next, nil), "RECONCILE_REQUIRED")
	expect(t, f, Sign(f.issuer, confirm, &receipt), "CONFIRMED_TEST_ONLY")
	expect(t, f, Sign(f.issuer, next, nil), "FENCED_TEST_ONLY")
	expect(t, f, Sign(f.issuer, a, nil), "DENIED_STALE_SESSION")
	expect(t, f, Sign(f.issuer, claim(f, "APPLY", 2, "reqA"), nil), "DENIED_DUPLICATE_RECONCILE_REQUIRED")
}
func TestP38RestartRetainsPendingAndBlocksNewEpoch(t *testing.T) {
	f := makeFixture(t)
	expect(t, f, Sign(f.issuer, claim(f, "FENCE", 1, ""), nil), "FENCED_TEST_ONLY")
	wire := Sign(f.issuer, claim(f, "APPLY", 1, "persistA"), nil)
	expect(t, f, wire, "RECONCILE_REQUIRED")
	f.server.Close()
	again, e := Open(f.socketDir, f.journalDir, "installationA", testWorkerUID, f.issuer.Public().(ed25519.PublicKey), f.provider.Public().(ed25519.PublicKey))
	if e != nil {
		t.Fatal(e)
	}
	f.server = again
	again.Start()
	expect(t, f, wire, "DENIED_RESTART_REQUIRES_FRESH_FENCE")
	expect(t, f, Sign(f.issuer, claim(f, "APPLY", 1, "newAfterRestart"), nil), "DENIED_RESTART_REQUIRES_FRESH_FENCE")
	expect(t, f, Sign(f.issuer, claim(f, "FENCE", 2, ""), nil), "RECONCILE_REQUIRED")
}
func TestP38ConcurrentDistinctUIDWorkerProcessesHaveOneWinner(t *testing.T) {
	f := makeFixture(t)
	expect(t, f, Sign(f.issuer, claim(f, "FENCE", 1, ""), nil), "FENCED_TEST_ONLY")
	wire := Sign(f.issuer, claim(f, "APPLY", 1, "parallel"), nil)
	const n = 8
	out := make([]string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); out[i] = runChild(t, f, wire, "call", testWorkerUID) }(i)
	}
	wg.Wait()
	wins := 0
	for _, s := range out {
		if strings.Contains(s, "PHASE38_REPLY=RECONCILE_REQUIRED") {
			wins++
		} else if !strings.Contains(s, "PHASE38_REPLY=DENIED_DUPLICATE_RECONCILE_REQUIRED") {
			t.Fatal(s)
		}
	}
	if wins != 1 {
		t.Fatalf("winners=%d", wins)
	}
}
func TestP38CorruptJournalFailsClosed(t *testing.T) {
	f := makeFixture(t)
	expect(t, f, Sign(f.issuer, claim(f, "FENCE", 1, ""), nil), "FENCED_TEST_ONLY")
	c := claim(f, "APPLY", 1, "corruptA")
	expect(t, f, Sign(f.issuer, c, nil), "RECONCILE_REQUIRED")
	if e := os.WriteFile(filepath.Join(f.journalDir, "r-"+fileID(c.RequestID)), []byte("{"), 0600); e != nil {
		t.Fatal(e)
	}
	expect(t, f, Sign(f.issuer, c, nil), "DENIED_JOURNAL_OR_IO")
	expect(t, f, Sign(f.issuer, claim(f, "FENCE", 2, ""), nil), "RECONCILE_REQUIRED")
}
func TestP38MispermissionedRootRejected(t *testing.T) {
	f := makeFixture(t)
	f.server.Close()
	if e := os.Chmod(f.journalDir, 0777); e != nil {
		t.Fatal(e)
	}
	_, e := Open(f.socketDir, f.journalDir, "installationA", testWorkerUID, f.issuer.Public().(ed25519.PublicKey), f.provider.Public().(ed25519.PublicKey))
	if !errors.Is(e, ErrBoundary) {
		t.Fatal(e)
	}
}
func TestP38StaleOrReplayFenceDenied(t *testing.T) {
	f := makeFixture(t)
	one := Sign(f.issuer, claim(f, "FENCE", 1, ""), nil)
	expect(t, f, one, "FENCED_TEST_ONLY")
	expect(t, f, one, "DENIED_EPOCH")
	expect(t, f, Sign(f.issuer, claim(f, "FENCE", 3, ""), nil), "DENIED_EPOCH")
}

// A separate root-owned supervisor process actually exits without Close() after
// fsync-backed journal reservation. Parent performs explicit trusted recovery
// of the stale socket pathname; the journal and pending fence persist.
func TestP38CrashSupervisorChild(t *testing.T) {
	if os.Getenv("PHASE38_CHILD") != "crash" {
		return
	}
	socketDir := os.Getenv("PHASE38_CRASH_SOCKET")
	journalDir := os.Getenv("PHASE38_CRASH_JOURNAL")
	iRaw, e := hex.DecodeString(os.Getenv("PHASE38_CRASH_ISSUER"))
	if e != nil {
		t.Fatal(e)
	}
	pRaw, e := hex.DecodeString(os.Getenv("PHASE38_CRASH_PROVIDER"))
	if e != nil {
		t.Fatal(e)
	}
	issuer := ed25519.PrivateKey(iRaw)
	provider := ed25519.PrivateKey(pRaw)
	b, e := Open(socketDir, journalDir, "installationA", testWorkerUID, issuer.Public().(ed25519.PublicKey), provider.Public().(ed25519.PublicKey))
	if e != nil {
		t.Fatal(e)
	}
	b.Start()
	for _, c := range []Claim{
		{Protocol: Protocol, Action: "FENCE", Installation: "installationA", Owner: "ownerA", Session: "session1", Epoch: 1, NotBefore: time.Now().Unix() - 3, Expires: time.Now().Unix() + 90},
		{Protocol: Protocol, Action: "APPLY", Installation: "installationA", Owner: "ownerA", Session: "session1", Epoch: 1, RequestID: "crashAfterFsync", Tenant: "tenantA", Actor: "actorA", Resource: "resourceA", NotBefore: time.Now().Unix() - 3, Expires: time.Now().Unix() + 90},
	} {
		wire, _ := json.Marshal(Sign(issuer, c, nil))
		cmd := exec.Command(os.Getenv("PHASE38_WORKER_BINARY"), "-test.run=^TestP38Child$", "-test.v")
		cmd.Env = append(os.Environ(), "PHASE38_CHILD=1", "PHASE38_MODE=call", "PHASE38_PATH="+b.Address(), "PHASE38_WIRE="+string(wire))
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: testWorkerUID, Gid: testWorkerUID, Groups: []uint32{testWorkerUID}}}
		out, runErr := cmd.CombinedOutput()
		if runErr != nil || !strings.Contains(string(out), "PHASE38_REPLY=") {
			t.Fatalf("child worker error %v %s", runErr, out)
		}
		if c.Action == "APPLY" && !strings.Contains(string(out), "PHASE38_REPLY=RECONCILE_REQUIRED") {
			t.Fatalf("journal not confirmed persisted before crash: %s", out)
		}
	}
	os.Exit(77) // NO close/cleanup. The process dies after journal fsync.
}
func TestP38AbruptSupervisorExitDoesNotPermitBlindRetry(t *testing.T) {
	f := makeFixture(t)
	f.server.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestP38CrashSupervisorChild$", "-test.v")
	cmd.Env = append(os.Environ(), "PHASE38_CHILD=crash", "PHASE38_CRASH_SOCKET="+f.socketDir, "PHASE38_CRASH_JOURNAL="+f.journalDir, "PHASE38_CRASH_ISSUER="+hex.EncodeToString(f.issuer), "PHASE38_CRASH_PROVIDER="+hex.EncodeToString(f.provider), "PHASE38_WORKER_BINARY="+childBinary)
	out, e := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(e, &exit) || exit.ExitCode() != 77 {
		t.Fatalf("expected abrupt supervisor exit 77: %v\n%s", e, out)
	}
	path := filepath.Join(f.socketDir, "owner.sock")
	info, e := os.Lstat(path)
	if e != nil || info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("crash should leave stale root-owned socket: %v %v", info, e)
	}
	owner := info.Sys().(*syscall.Stat_t)
	if owner.Uid != 0 || owner.Gid != testWorkerUID {
		t.Fatal("stale socket ownership unexpected")
	}
	stale, e := net.DialTimeout("unix", path, 100*time.Millisecond)
	if e == nil {
		stale.Close()
		t.Fatal("old supervisor listener unexpectedly alive")
	}
	// Only trusted root may unlink the now-confirmed stale, root-owned socket.
	if e = os.Remove(path); e != nil {
		t.Fatal(e)
	}
	reopened, e := Open(f.socketDir, f.journalDir, "installationA", testWorkerUID, f.issuer.Public().(ed25519.PublicKey), f.provider.Public().(ed25519.PublicKey))
	if e != nil {
		t.Fatal(e)
	}
	f.server = reopened
	reopened.Start()
	expect(t, f, Sign(f.issuer, claim(f, "APPLY", 1, "newAfterCrash"), nil), "DENIED_RESTART_REQUIRES_FRESH_FENCE")
	expect(t, f, Sign(f.issuer, claim(f, "FENCE", 2, ""), nil), "RECONCILE_REQUIRED")
}

// Even if the leaf directories have root owner and restrictive permissions,
// a writable nonsticky ancestor permits a worker to rename/replace their
// pathnames. The supervisor must refuse to open such a boundary.
func TestP38MutableAncestorRefused(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires isolated root test")
	}
	root, err := os.MkdirTemp("/tmp", "lic38-badparent-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	if err = os.Chmod(root, 0777); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(root, "socket")
	journal := filepath.Join(root, "journal")
	if err = os.Mkdir(socket, 0750); err != nil {
		t.Fatal(err)
	}
	if err = os.Chown(socket, 0, testWorkerUID); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(journal, 0700); err != nil {
		t.Fatal(err)
	}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Open(socket, journal, "installationA", testWorkerUID, pub, pub)
	if b != nil {
		b.Close()
		t.Fatal("unsafe writable ancestor accepted")
	}
	if !errors.Is(err, ErrBoundary) {
		t.Fatalf("expected fail-closed unsafe ancestor: %v", err)
	}
}

// Symlinked ancestor paths are untrusted even when the target itself has
// the exact root owner and 0700/0750 permissions.
func TestP38SymlinkAncestorRefused(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires isolated root test")
	}
	root, err := os.MkdirTemp("/tmp", "lic38-linkparent-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	realParent := filepath.Join(root, "actual")
	if err = os.Mkdir(realParent, 0700); err != nil {
		t.Fatal(err)
	}
	linkedParent := filepath.Join(root, "alias")
	if err = os.Symlink(realParent, linkedParent); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(realParent, "socket")
	journal := filepath.Join(realParent, "journal")
	if err = os.Mkdir(socket, 0750); err != nil {
		t.Fatal(err)
	}
	if err = os.Chown(socket, 0, testWorkerUID); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(journal, 0700); err != nil {
		t.Fatal(err)
	}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Open(filepath.Join(linkedParent, "socket"), journal, "installationA", testWorkerUID, pub, pub)
	if b != nil {
		b.Close()
		t.Fatal("symlink ancestor accepted")
	}
	if !errors.Is(err, ErrBoundary) {
		t.Fatalf("expected fail-closed symlink ancestor: %v", err)
	}
}
