//go:build linux && amd64

package ipc

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func setup(t *testing.T) (*Broker, ed25519.PrivateKey, *atomic.Int32) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "isolated")
	if e := os.Mkdir(root, 0700); e != nil {
		t.Fatal(e)
	}
	pub, priv, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	count := new(atomic.Int32)
	b, e := Open(root, map[string]Worker{"worker1": {UID: uint32(os.Geteuid()), Workspace: "workspace1", Key: pub}}, func(c Claim) error { count.Add(1); return nil })
	if e != nil {
		t.Fatal(e)
	}
	go b.Serve()
	t.Cleanup(b.Close)
	return b, priv, count
}
func request(priv ed25519.PrivateKey) Wire {
	return Sign(Claim{Protocol: Protocol, RequestID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", WorkerID: "worker1", WorkspaceID: "workspace1", Operation: Operation, Nonce: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", IssuedAt: time.Now().Unix()}, priv)
}
func call(t *testing.T, b *Broker, w Wire) string {
	t.Helper()
	s, e := Call(context.Background(), b.Address(), w)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestSignedEffectAndNoReplay(t *testing.T) {
	b, k, n := setup(t)
	w := request(k)
	if got := call(t, b, w); got != "EXECUTED_TEST_ONLY" {
		t.Fatal(got)
	}
	if got := call(t, b, w); got != "DENIED_REPLAY_OR_IO" {
		t.Fatal(got)
	}
	if n.Load() != 1 {
		t.Fatal(n.Load())
	}
}
func TestBadSignature(t *testing.T) {
	b, k, n := setup(t)
	w := request(k)
	w.Signature = strings.Repeat("0", 128)
	if got := call(t, b, w); got != "DENIED_SIGNATURE" {
		t.Fatal(got)
	}
	if n.Load() != 0 {
		t.Fatal(n.Load())
	}
}
func TestCrossWorkspace(t *testing.T) {
	b, k, _ := setup(t)
	w := request(k)
	w.Claim.WorkspaceID = "other"
	w = Sign(w.Claim, k)
	if got := call(t, b, w); got != "DENIED_IDENTITY" {
		t.Fatal(got)
	}
}
func TestUnknownWorker(t *testing.T) {
	b, k, _ := setup(t)
	w := request(k)
	w.Claim.WorkerID = "unknown"
	w = Sign(w.Claim, k)
	if got := call(t, b, w); got != "DENIED_IDENTITY" {
		t.Fatal(got)
	}
}
func TestStaleSignedRequest(t *testing.T) {
	b, k, _ := setup(t)
	w := request(k)
	w.Claim.IssuedAt -= 120
	w = Sign(w.Claim, k)
	if got := call(t, b, w); got != "DENIED_TIME" {
		t.Fatal(got)
	}
}
func TestNonAllowlistedOperation(t *testing.T) {
	b, k, _ := setup(t)
	w := request(k)
	w.Claim.Operation = "SHELL"
	w = Sign(w.Claim, k)
	if got := call(t, b, w); got != "DENIED_REQUEST" {
		t.Fatal(got)
	}
}
func TestSingleOwnerAfterSocketUnlink(t *testing.T) {
	b, _, _ := setup(t)
	if e := os.Remove(b.Address()); e != nil {
		t.Fatal(e)
	}
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	if another, e := Open(filepath.Dir(b.Address()), map[string]Worker{"worker1": {UID: uint32(os.Geteuid()), Workspace: "workspace1", Key: pub}}, func(Claim) error { return nil }); e == nil {
		another.Close()
		t.Fatal("second owner admitted")
	}
}
func TestSymlinkAncestorDenied(t *testing.T) {
	b, _, _ := setup(t)
	tmp := t.TempDir()
	if e := os.Symlink(filepath.Dir(b.Address()), filepath.Join(tmp, "link")); e != nil {
		t.Fatal(e)
	}
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	if other, e := Open(filepath.Join(tmp, "link"), map[string]Worker{"w": {Key: pub}}, func(Claim) error { return nil }); e == nil {
		other.Close()
		t.Fatal("symlink admitted")
	}
}
func TestInvalidJSONDuplicateDenied(t *testing.T) {
	b, k, n := setup(t)
	w := request(k)
	raw, _ := json.Marshal(w)
	bad := append([]byte(`{"claim":`), raw...)
	_ = bad
	c, e := net.Dial("unix", b.Address())
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(time.Second))
	_, _ = c.Write([]byte(`{"claim":{},"claim":{},"signature":"f"}` + "\n"))
	buf := make([]byte, 200)
	sz, e := c.Read(buf)
	if e != nil {
		t.Fatal(e)
	}
	if string(buf[:sz]) != "DENIED_REQUEST\n" {
		t.Fatal(string(buf[:sz]))
	}
	if n.Load() != 0 {
		t.Fatal(n.Load())
	}
}
func TestConcurrentSameRequestAtMostOnce(t *testing.T) {
	b, k, n := setup(t)
	w := request(k)
	var wg sync.WaitGroup
	var ok atomic.Int32
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := Call(context.Background(), b.Address(), w)
			if e == nil && v == "EXECUTED_TEST_ONLY" {
				ok.Add(1)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 1 || n.Load() != 1 {
		t.Fatalf("executed %d callbacks %d", ok.Load(), n.Load())
	}
}
func TestOwnerCloseNoReuseWhileDraining(t *testing.T) {
	b, k, _ := setup(t)
	_ = k
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); b.Close() }()
	}
	wg.Wait()
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	root := filepath.Dir(b.Address()) // Socket pathname may remain on disk after Close; removing only after shutdown is operator cleanup.
	_ = os.Remove(b.Address())
	other, e := Open(root, map[string]Worker{"worker1": {UID: uint32(os.Geteuid()), Workspace: "workspace1", Key: pub}}, func(Claim) error { return nil })
	if e != nil {
		t.Fatal(e)
	}
	other.Close()
}
func TestUnsafePermissionsRejected(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bad")
	if e := os.Mkdir(root, 0770); e != nil {
		t.Fatal(e)
	}
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	if b, e := Open(root, map[string]Worker{"w": {Key: pub}}, func(Claim) error { return nil }); e == nil {
		b.Close()
		t.Fatal("unsafe mode accepted")
	}
}
