//go:build linux

package brokerproof

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func allowed(_ context.Context, fn func() error) error { return fn() }
func req(id string) Request                            { return Request{Protocol: protocol, Operation: operation, ID: id} }
func setup(t *testing.T, gate EffectGate, peerOverride ...func(*net.UnixConn) (uint32, error)) (*Broker, *atomic.Int64) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "isolated")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	var count atomic.Int64
	b, err := Open(root, gate, func() error { count.Add(1); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(peerOverride) > 0 {
		b.peerUID = peerOverride[0]
	}
	t.Cleanup(func() { _ = b.Close() })
	go b.Serve()
	return b, &count
}
func TestExactlyOneSyntheticEffectAndDuplicateRefused(t *testing.T) {
	b, count := setup(t, allowed)
	id := strings.Repeat("a", 32)
	s, err := Call(b.Address(), req(id))
	if err != nil || s != "EXECUTED_TEST_ONLY" {
		t.Fatalf("first %s %v", s, err)
	}
	s, err = Call(b.Address(), req(id))
	if err != nil || s != "DENIED_DUPLICATE_OR_IO" {
		t.Fatalf("duplicate %s %v", s, err)
	}
	if count.Load() != 1 {
		t.Fatalf("effect count %d", count.Load())
	}
}
func TestImmediateLeaderGateRecheckFailsClosed(t *testing.T) {
	var allow atomic.Bool
	allow.Store(false)
	gate := func(_ context.Context, fn func() error) error {
		if !allow.Load() {
			return ErrDenied
		}
		return fn()
	}
	b, count := setup(t, gate)
	s, err := Call(b.Address(), req(strings.Repeat("b", 32)))
	if err != nil || s != "DENIED_LICENSE_OR_EFFECT" {
		t.Fatalf("license gate %s %v", s, err)
	}
	allow.Store(true)
	s, err = Call(b.Address(), req(strings.Repeat("c", 32)))
	if err != nil || s != "EXECUTED_TEST_ONLY" {
		t.Fatalf("new id %s %v", s, err)
	}
	if count.Load() != 1 {
		t.Fatalf("count %d", count.Load())
	}
}
func TestDeniedPendingIdentifierCannotReplayAfterRevocation(t *testing.T) {
	var allow atomic.Bool
	gate := func(_ context.Context, fn func() error) error {
		if !allow.Load() {
			return ErrDenied
		}
		return fn()
	}
	b, count := setup(t, gate)
	id := strings.Repeat("d", 32)
	s, _ := Call(b.Address(), req(id))
	if s != "DENIED_LICENSE_OR_EFFECT" {
		t.Fatal(s)
	}
	allow.Store(true)
	s, _ = Call(b.Address(), req(id))
	if s != "DENIED_DUPLICATE_OR_IO" {
		t.Fatal(s)
	}
	if count.Load() != 0 {
		t.Fatal("uncertain effects retried")
	}
}
func TestUnknownCommandAndDuplicateFieldsNeverExecute(t *testing.T) {
	b, count := setup(t, allowed)
	frames := []string{
		`{"protocol":"TELMIVO_LIC_A9_PHASE23_TEST_ONLY_V1","operation":"DELETE_REAL_FILES","id":"` + strings.Repeat("1", 32) + `"}`,
		`{"protocol":"TELMIVO_LIC_A9_PHASE23_TEST_ONLY_V1","operation":"SYNTHETIC_RECORD_EFFECT","id":"` + strings.Repeat("1", 32) + `","id":"` + strings.Repeat("2", 32) + `"}`,
		`{"protocol":"TELMIVO_LIC_A9_PHASE23_TEST_ONLY_V1","operation":"SYNTHETIC_RECORD_EFFECT","id":"invalid"}`,
		strings.Repeat("Z", maxFrame+1),
	}
	for i, frame := range frames {
		c, err := net.DialTimeout("unix", b.Address(), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = c.Write([]byte(frame + "\n"))
		buf := make([]byte, 128)
		n, _ := c.Read(buf)
		_ = c.Close()
		if !strings.HasPrefix(string(buf[:n]), `{"status":"DENIED_`) {
			t.Fatalf("frame %d: %s", i, string(buf[:n]))
		}
	}
	if count.Load() != 0 {
		t.Fatal("untrusted operation executed")
	}
}
func TestPeerCredentialMismatchFailsClosed(t *testing.T) {
	b, count := setup(t, allowed, func(*net.UnixConn) (uint32, error) { return uint32(os.Geteuid() + 1), nil })
	s, err := Call(b.Address(), req(strings.Repeat("e", 32)))
	if err != nil || s != "DENIED_PEER" || count.Load() != 0 {
		t.Fatalf("peer admission %s %v", s, err)
	}
}
func TestConcurrentSameIDCannotDoubleEffect(t *testing.T) {
	b, count := setup(t, allowed)
	var wg sync.WaitGroup
	var success, denied atomic.Int64
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := Call(b.Address(), req(strings.Repeat("f", 32)))
			if err != nil {
				return
			}
			if s == "EXECUTED_TEST_ONLY" {
				success.Add(1)
			} else if s == "DENIED_DUPLICATE_OR_IO" {
				denied.Add(1)
			}
		}()
	}
	wg.Wait()
	if success.Load() != 1 || denied.Load() != 19 || count.Load() != 1 {
		t.Fatalf("once %d duplicate %d effects %d", success.Load(), denied.Load(), count.Load())
	}
}
func TestDurableReservationSurvivesBrokerRestart(t *testing.T) {
	root := filepath.Join(t.TempDir(), "restart")
	_ = os.Mkdir(root, 0700)
	var count atomic.Int64
	create := func() *Broker {
		b, err := Open(root, allowed, func() error { count.Add(1); return nil })
		if err != nil {
			t.Fatal(err)
		}
		go b.Serve()
		return b
	}
	b := create()
	id := strings.Repeat("9", 32)
	s, e := Call(b.Address(), req(id))
	if e != nil || s != "EXECUTED_TEST_ONLY" {
		t.Fatalf("first %s %v", s, e)
	}
	_ = b.Close()
	b = create()
	defer b.Close()
	s, e = Call(b.Address(), req(id))
	if e != nil || s != "DENIED_DUPLICATE_OR_IO" {
		t.Fatalf("restart %s %v", s, e)
	}
	if count.Load() != 1 {
		t.Fatalf("effect count %d", count.Load())
	}
}
func TestUnsafeRootAndAfterStopFailClosed(t *testing.T) {
	if _, err := Open("/relative/path", allowed, func() error { return nil }); err == nil {
		t.Fatal("relative root allowed")
	}
	root := filepath.Join(t.TempDir(), "unsafe")
	_ = os.Mkdir(root, 0777)
	_ = os.Chmod(root, 0777)
	if _, err := Open(root, allowed, func() error { return nil }); err == nil {
		t.Fatal("world writable root admitted")
	}
	b, _ := setup(t, allowed)
	p := b.Address()
	_ = b.Close()
	if s, err := Call(p, req(strings.Repeat("8", 32))); err == nil {
		t.Fatalf("dead leader accepted %s", s)
	}
}
func TestCallerCannotBypassLeaderGateOrSubmitGenericOperation(t *testing.T) {
	b, count := setup(t, func(context.Context, func() error) error { return fmt.Errorf("LICENSE_RESTRICTED") })
	for _, r := range []Request{req(strings.Repeat("6", 32)), {Protocol: protocol, Operation: "SHELL_COMMAND", ID: strings.Repeat("7", 32)}} {
		s, err := Call(b.Address(), r)
		if err != nil || !strings.HasPrefix(s, "DENIED_") {
			t.Fatalf("unauthorized %s %v", s, err)
		}
	}
	if count.Load() != 0 {
		t.Fatal("business effect bypassed leader")
	}
}
