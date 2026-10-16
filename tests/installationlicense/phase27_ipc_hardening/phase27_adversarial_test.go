//go:build linux && amd64

package ipc

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestConfiguredUIDMismatchDenied(t *testing.T) {
	root := filepath.Join(t.TempDir(), "owner")
	if e := os.Mkdir(root, 0700); e != nil {
		t.Fatal(e)
	}
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	b, e := Open(root, map[string]Worker{"worker1": {UID: uint32(os.Geteuid() + 1), Workspace: "workspace1", Key: pub}}, func(Claim) error { t.Error("unapproved effect"); return nil })
	if e != nil {
		t.Fatal(e)
	}
	go b.Serve()
	defer b.Close()
	if got := call(t, b, request(priv)); got != "DENIED_IDENTITY" {
		t.Fatal(got)
	}
}
func TestWrongWorkerSigningKeyDenied(t *testing.T) {
	b, _, n := setup(t)
	_, fake, _ := ed25519.GenerateKey(rand.Reader)
	if got := call(t, b, request(fake)); got != "DENIED_SIGNATURE" {
		t.Fatal(got)
	}
	if n.Load() != 0 {
		t.Fatal(n.Load())
	}
}
func TestNonceReuseDifferentRequestDenied(t *testing.T) {
	b, k, n := setup(t)
	w := request(k)
	if call(t, b, w) != "EXECUTED_TEST_ONLY" {
		t.Fatal("first failed")
	}
	w.Claim.RequestID = "cccccccccccccccccccccccccccccccc"
	w = Sign(w.Claim, k)
	if got := call(t, b, w); got != "DENIED_REPLAY_OR_IO" {
		t.Fatal(got)
	}
	if n.Load() != 1 {
		t.Fatal(n.Load())
	}
}
func TestOversizedFrameDenied(t *testing.T) {
	b, _, n := setup(t)
	c, e := net.Dial("unix", b.Address())
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	_, _ = c.Write([]byte(strings.Repeat("x", 4096) + "\n"))
	s, e := bufio.NewReader(c).ReadString('\n')
	if e != nil {
		t.Fatal(e)
	}
	if s != "DENIED_FRAME\n" {
		t.Fatal(s)
	}
	if n.Load() != 0 {
		t.Fatal(n.Load())
	}
}
func TestAmbiguousEffectNeverBlindRetries(t *testing.T) {
	root := filepath.Join(t.TempDir(), "owner")
	if e := os.Mkdir(root, 0700); e != nil {
		t.Fatal(e)
	}
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	var count atomic.Int32
	b, e := Open(root, map[string]Worker{"worker1": {UID: uint32(os.Geteuid()), Workspace: "workspace1", Key: pub}}, func(Claim) error { count.Add(1); return errors.New("unknown downstream outcome") })
	if e != nil {
		t.Fatal(e)
	}
	go b.Serve()
	defer b.Close()
	w := request(priv)
	if got := call(t, b, w); got != "UNKNOWN_RECONCILE_REQUIRED" {
		t.Fatal(got)
	}
	if got := call(t, b, w); got != "DENIED_REPLAY_OR_IO" {
		t.Fatal(got)
	}
	if count.Load() != 1 {
		t.Fatal(count.Load())
	}
}
func TestJournalPreventsReplayAfterOwnerRestart(t *testing.T) {
	root := filepath.Join(t.TempDir(), "owner")
	if e := os.Mkdir(root, 0700); e != nil {
		t.Fatal(e)
	}
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	var count atomic.Int32
	workers := map[string]Worker{"worker1": {UID: uint32(os.Geteuid()), Workspace: "workspace1", Key: pub}}
	b, e := Open(root, workers, func(Claim) error { count.Add(1); return nil })
	if e != nil {
		t.Fatal(e)
	}
	go b.Serve()
	w := request(priv)
	if got := call(t, b, w); got != "EXECUTED_TEST_ONLY" {
		t.Fatal(got)
	}
	b.Close()
	_ = os.Remove(filepath.Join(root, "broker.sock"))
	second, e := Open(root, workers, func(Claim) error { count.Add(1); return nil })
	if e != nil {
		t.Fatal(e)
	}
	go second.Serve()
	defer second.Close()
	if got := call(t, second, w); got != "DENIED_REPLAY_OR_IO" {
		t.Fatal(got)
	}
	if count.Load() != 1 {
		t.Fatal(count.Load())
	}
}
func TestCancelledSocketDoesNotPermitDirectEffect(t *testing.T) {
	b, k, n := setup(t)
	w := request(k)
	b.Close()
	_, err := Call(context.Background(), b.Address(), w)
	if err == nil {
		t.Fatal("closed broker unexpectedly processed call")
	}
	if n.Load() != 0 {
		t.Fatal(n.Load())
	}
}
