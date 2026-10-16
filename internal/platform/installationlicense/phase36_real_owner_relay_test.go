package installationlicense

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// This is an executable TEST-ONLY Unix-socket relay for real Manager.Effect.
// It does not modify Product runtime. Its ephemeral Ed25519 peer identity is
// NOT an Authority V2 signing key or a real Worker credential. Its in-memory
// request ledger is NOT crash-safe. Same-UID IPC isolation remains unproven.
const phase36Protocol = "TELMIVO_LIC_A9_PHASE36_TEST_RELAY_V1"
const phase36SignDomain = "TELMIVO_PHASE36_WORKER_CLAIM\x00"

var phase36PolicyDenied = errors.New("PHASE36_TEST_ONLY_DOMAIN_DENIED")
var phase36UnknownEffect = errors.New("PHASE36_TEST_ONLY_UNKNOWN_EFFECT")

type phase36Claim struct {
	Protocol     string `json:"protocol"`
	OwnerSession string `json:"owner_session"`
	WorkspaceID  string `json:"workspace_id"`
	ActorID      string `json:"actor_id"`
	ResourceID   string `json:"resource_id"`
	RequestID    string `json:"request_id"`
	ExpiresUnix  int64  `json:"expires_unix"`
}
type phase36Envelope struct {
	Claim     phase36Claim `json:"claim"`
	Signature string       `json:"signature"`
}

func phase36Sign(claim phase36Claim, private ed25519.PrivateKey) phase36Envelope {
	body, _ := json.Marshal(claim)
	proof := append([]byte(phase36SignDomain), body...)
	return phase36Envelope{Claim: claim, Signature: hex.EncodeToString(ed25519.Sign(private, proof))}
}

type phase36Relay struct {
	manager     *Manager
	listener    *net.UnixListener
	public      ed25519.PublicKey
	session     string
	mu          sync.Mutex
	reserved    map[string]bool
	effectCount atomic.Int64
	failNext    bool
	workers     sync.WaitGroup
	serveDone   chan struct{}
}

func phase36OpenRelay(t *testing.T, manager *Manager, pub ed25519.PublicKey) *phase36Relay {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "owner-only")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	address := filepath.Join(directory, "test-owner.sock")
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: address, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(address, 0600); err != nil {
		t.Fatal(err)
	}
	relay := &phase36Relay{
		manager: manager, listener: ln, public: append(ed25519.PublicKey(nil), pub...),
		session: manager.runtimeSession, reserved: make(map[string]bool), serveDone: make(chan struct{}),
	}
	go relay.serve()
	t.Cleanup(relay.close)
	return relay
}
func (r *phase36Relay) close() {
	_ = r.listener.Close()
	<-r.serveDone // all workers have been registered before Wait
	r.workers.Wait()
}
func (r *phase36Relay) serve() {
	defer close(r.serveDone)
	for {
		conn, err := r.listener.AcceptUnix()
		if err != nil {
			return
		}
		r.workers.Add(1)
		go func() { defer r.workers.Done(); defer conn.Close(); r.process(conn) }()
	}
}
func (r *phase36Relay) process(conn *net.UnixConn) {
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	answer := func(s string) { _, _ = conn.Write([]byte(s + "\n")) }
	reader := bufio.NewReaderSize(io.LimitReader(conn, 2049), 2048)
	line, err := reader.ReadBytes('\n')
	if err != nil || len(line) < 2 || len(line) > 2049 {
		answer("DENIED_FRAME")
		return
	}
	raw := line[:len(line)-1]
	var wire phase36Envelope
	if err := json.Unmarshal(raw, &wire); err != nil {
		answer("DENIED_WIRE")
		return
	}
	canonical, _ := json.Marshal(wire)
	if !bytes.Equal(raw, canonical) {
		answer("DENIED_CANONICAL")
		return
	}
	c := wire.Claim
	if c.Protocol != phase36Protocol || c.OwnerSession != r.session || c.ExpiresUnix <= time.Now().Unix() || c.ExpiresUnix > time.Now().Add(3*time.Minute).Unix() || len(c.RequestID) == 0 || len(c.RequestID) > 96 || strings.ContainsAny(c.RequestID, "/\\\n\x00") {
		answer("DENIED_CLAIM")
		return
	}
	signature, err := hex.DecodeString(wire.Signature)
	body, _ := json.Marshal(c)
	if err != nil || len(signature) != ed25519.SignatureSize || !ed25519.Verify(r.public, append([]byte(phase36SignDomain), body...), signature) {
		answer("DENIED_SIGNATURE")
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.reserved[c.RequestID] {
		answer("DENIED_DUPLICATE")
		return
	}
	// At-most-once within this *process only*. Keep pending even on failures,
	// so unknown callback outcome is never blindly retried in the same relay.
	r.reserved[c.RequestID] = true
	err = r.manager.Effect(context.Background(), func() error {
		if c.WorkspaceID != "tenantA" || c.ActorID != "actorA" || c.ResourceID != "resourceA" {
			return phase36PolicyDenied
		}
		r.effectCount.Add(1)
		if r.failNext {
			r.failNext = false
			return phase36UnknownEffect
		}
		return nil
	})
	switch {
	case err == nil:
		answer("SYNTHETIC_EFFECT_ACCEPTED")
	case errors.Is(err, ErrRestricted):
		answer("DENIED_LICENSE_SESSION")
	case errors.Is(err, phase36UnknownEffect):
		answer("RECONCILE_REQUIRED")
	default:
		answer("DENIED_POLICY_OR_ERROR")
	}
}
func (r *phase36Relay) send(wire phase36Envelope) (string, error) {
	conn, err := net.DialTimeout("unix", r.listener.Addr().String(), 2*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	raw, err := json.Marshal(wire)
	if err != nil {
		return "", err
	}
	if _, err = conn.Write(append(raw, '\n')); err != nil {
		return "", err
	}
	reply, err := bufio.NewReader(conn).ReadString('\n')
	return strings.TrimSpace(reply), err
}

type phase36Fixture struct {
	leader   *Manager
	follower *Manager
	signer   func(string, int64) []byte
	relay    *phase36Relay
	private  ed25519.PrivateKey
}

func phase36Setup(t *testing.T) phase36Fixture {
	t.Helper()
	leader, signer := phase19TestSignedConcurrentInstallation(t)
	if err := leader.Accept(signer(leader.runtimeSession, 1)); err != nil {
		t.Fatal(err)
	}
	follower, err := Open(leader.cfg)
	if err != nil {
		t.Fatal(err)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return phase36Fixture{leader, follower, signer, phase36OpenRelay(t, leader, public), private}
}
func (f phase36Fixture) claim(id string) phase36Envelope {
	return phase36Sign(phase36Claim{
		Protocol: phase36Protocol, OwnerSession: f.leader.runtimeSession,
		WorkspaceID: "tenantA", ActorID: "actorA", ResourceID: "resourceA",
		RequestID: id, ExpiresUnix: time.Now().Add(time.Minute).Unix(),
	}, f.private)
}
func phase36Reply(t *testing.T, r *phase36Relay, signed phase36Envelope, want string) {
	t.Helper()
	reply, err := r.send(signed)
	if err != nil || reply != want {
		t.Fatalf("reply=%q want=%q err=%v", reply, want, err)
	}
}
func TestLICA9Phase36FollowerCannotCallManagerEffectDirectly(t *testing.T) {
	f := phase36Setup(t)
	called := false
	err := f.follower.Effect(context.Background(), func() error { called = true; return nil })
	if !errors.Is(err, ErrRestricted) || called {
		t.Fatalf("invalid follower effect: called=%t err=%v", called, err)
	}
}
func TestLICA9Phase36RealOwnerProcessesDelegatedSyntheticIPC(t *testing.T) {
	f := phase36Setup(t)
	phase36Reply(t, f.relay, f.claim("delegated1"), "SYNTHETIC_EFFECT_ACCEPTED")
	if got := f.relay.effectCount.Load(); got != 1 {
		t.Fatal(got)
	}
}
func TestLICA9Phase36SignedCrossTenantRequestDenied(t *testing.T) {
	f := phase36Setup(t)
	signed := f.claim("crossTenant")
	signed.Claim.WorkspaceID = "tenantB"
	signed = phase36Sign(signed.Claim, f.private)
	phase36Reply(t, f.relay, signed, "DENIED_POLICY_OR_ERROR")
	if f.relay.effectCount.Load() != 0 {
		t.Fatal("cross tenant effect executed")
	}
}
func TestLICA9Phase36UnsignedClaimMutationDenied(t *testing.T) {
	f := phase36Setup(t)
	signed := f.claim("mutated")
	signed.Claim.WorkspaceID = "tenantB"
	phase36Reply(t, f.relay, signed, "DENIED_SIGNATURE")
}
func TestLICA9Phase36WrongOwnerSessionEvenIfSignedDenied(t *testing.T) {
	f := phase36Setup(t)
	signed := f.claim("oldSession")
	signed.Claim.OwnerSession = f.follower.runtimeSession
	signed = phase36Sign(signed.Claim, f.private)
	phase36Reply(t, f.relay, signed, "DENIED_CLAIM")
}
func TestLICA9Phase36SignedRequestReplayDenied(t *testing.T) {
	f := phase36Setup(t)
	signed := f.claim("sameRequest")
	phase36Reply(t, f.relay, signed, "SYNTHETIC_EFFECT_ACCEPTED")
	phase36Reply(t, f.relay, signed, "DENIED_DUPLICATE")
	if f.relay.effectCount.Load() != 1 {
		t.Fatal("replay executed")
	}
}
func TestLICA9Phase36UnknownSyntheticOutcomeMustReconcile(t *testing.T) {
	f := phase36Setup(t)
	f.relay.mu.Lock()
	f.relay.failNext = true
	f.relay.mu.Unlock()
	signed := f.claim("unknownOutcome")
	phase36Reply(t, f.relay, signed, "RECONCILE_REQUIRED")
	phase36Reply(t, f.relay, signed, "DENIED_DUPLICATE")
	if f.relay.effectCount.Load() != 1 {
		t.Fatal("unknown effect retried")
	}
}
func TestLICA9Phase36FormerOwnerRelayDeniedAfterSignedHandoff(t *testing.T) {
	f := phase36Setup(t)
	if err := f.follower.Accept(f.signer(f.follower.runtimeSession, 2)); err != nil {
		t.Fatal(err)
	}
	phase36Reply(t, f.relay, f.claim("postHandoff"), "DENIED_LICENSE_SESSION")
	if f.relay.effectCount.Load() != 0 {
		t.Fatal("old owner executed after handoff")
	}
}
func TestLICA9Phase36ConcurrentSignedIPCRequestsOneExecution(t *testing.T) {
	f := phase36Setup(t)
	wire := f.claim("parallelOne")
	var wg sync.WaitGroup
	var success atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reply, err := f.relay.send(wire)
			if err != nil {
				t.Error(err)
				return
			}
			switch reply {
			case "SYNTHETIC_EFFECT_ACCEPTED":
				success.Add(1)
			case "DENIED_DUPLICATE":
			default:
				t.Errorf("unexpected %s", reply)
			}
		}()
	}
	wg.Wait()
	if success.Load() != 1 || f.relay.effectCount.Load() != 1 {
		t.Fatalf("success=%d effects=%d", success.Load(), f.relay.effectCount.Load())
	}
}
func TestLICA9Phase36ForgedSignerRejected(t *testing.T) {
	f := phase36Setup(t)
	_, foreignKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	wire := f.claim("forged")
	wire = phase36Sign(wire.Claim, foreignKey)
	phase36Reply(t, f.relay, wire, "DENIED_SIGNATURE")
}
