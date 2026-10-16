package installationlicense

// TEST-ONLY portable functional admission. The real TELMIVO Manager.Effect is
// used when this file is run in Product under Go1.27. The local Linux Go1.23
// sandbox uses an explicitly labeled compatible stub, NOT the real Manager.
// No real Authority, multi-UID root supervisor, or external provider exists.

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const phase42GrantDomain = "TELMIVO_LIC_A9_PHASE42_BOOT_GRANT_TEST\x00"
const phase42RequestDomain = "TELMIVO_LIC_A9_PHASE42_EFFECT_CLAIM_TEST\x00"

var phase42Denied = errors.New("PHASE42_TEST_ONLY_DENIED")

type phase42Grant struct {
	BootNonce   string `json:"boot_nonce"`
	Session     string `json:"session"`
	Epoch       uint64 `json:"epoch"`
	ExpiresUnix int64  `json:"expires_unix"`
}
type phase42Request struct {
	BootNonce string       `json:"boot_nonce"`
	Epoch     uint64       `json:"epoch"`
	Claim     phase36Claim `json:"claim"`
}
type phase42SignedGrant struct {
	Grant     phase42Grant `json:"grant"`
	Signature string       `json:"signature"`
}
type phase42SignedRequest struct {
	Request   phase42Request `json:"request"`
	Signature string         `json:"signature"`
}

func phase42Signature(key ed25519.PrivateKey, domain string, payload any) string {
	b, _ := json.Marshal(payload)
	return hex.EncodeToString(ed25519.Sign(key, append([]byte(domain), b...)))
}
func phase42Check(pub ed25519.PublicKey, domain string, payload any, sig string) bool {
	proof, err := hex.DecodeString(sig)
	if err != nil || len(proof) != ed25519.SignatureSize {
		return false
	}
	b, err := json.Marshal(payload)
	return err == nil && ed25519.Verify(pub, append([]byte(domain), b...), proof)
}
func phase42BootNonce(t *testing.T) string {
	t.Helper()
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(raw[:])
}

// This is NOT a deployable bridge: the journal and signer are ephemeral TEST
// fixtures. No transport or worker OS identity is verified in this test.
type phase42PortableGate struct {
	mu      sync.Mutex
	manager *Manager
	public  ed25519.PublicKey
	journal *phase37Journal
	nonce   string
	epoch   uint64
	enabled bool
	effects atomic.Int64
}

func phase42NewGate(t *testing.T, m *Manager, pub ed25519.PublicKey, j *phase37Journal) *phase42PortableGate {
	t.Helper()
	return &phase42PortableGate{manager: m, public: pub, journal: j, nonce: phase42BootNonce(t)}
}
func (g *phase42PortableGate) grant(s phase42SignedGrant) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	x := s.Grant
	if !phase42Check(g.public, phase42GrantDomain, x, s.Signature) ||
		x.BootNonce != g.nonce || x.Session != g.manager.runtimeSession ||
		x.Epoch <= g.epoch || x.Epoch == 0 || x.ExpiresUnix <= time.Now().Unix() ||
		x.ExpiresUnix > time.Now().Add(3*time.Minute).Unix() ||
		g.manager.Decision().State != "VALID_ONLINE" {
		return phase42Denied
	}
	g.epoch = x.Epoch
	g.enabled = true
	return nil
}
func (g *phase42PortableGate) effect(s phase42SignedRequest) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	x := s.Request
	c := x.Claim
	if !g.enabled || !phase42Check(g.public, phase42RequestDomain, x, s.Signature) ||
		x.BootNonce != g.nonce || x.Epoch != g.epoch ||
		c.Protocol != phase36Protocol || c.OwnerSession != g.manager.runtimeSession ||
		c.WorkspaceID != "tenantA" || c.ActorID != "actorA" || c.ResourceID != "resourceA" ||
		len(c.RequestID) == 0 || len(c.RequestID) > 96 ||
		c.ExpiresUnix <= time.Now().Unix() || c.ExpiresUnix > time.Now().Add(3*time.Minute).Unix() {
		return phase42Denied
	}
	// The EXACT signed claim is durably fsynced before the real Manager.Effect
	// can call the synthetic callback. Unknown outcomes are NEVER retried.
	if err := g.journal.reserve(c); err != nil {
		return err
	}
	return g.manager.Effect(context.Background(), func() error { g.effects.Add(1); return nil })
}
func phase42GrantFor(g *phase42PortableGate, key ed25519.PrivateKey, epoch uint64) phase42SignedGrant {
	x := phase42Grant{BootNonce: g.nonce, Session: g.manager.runtimeSession, Epoch: epoch, ExpiresUnix: time.Now().Add(time.Minute).Unix()}
	return phase42SignedGrant{Grant: x, Signature: phase42Signature(key, phase42GrantDomain, x)}
}
func phase42RequestFor(g *phase42PortableGate, key ed25519.PrivateKey, epoch uint64, id string) phase42SignedRequest {
	x := phase42Request{BootNonce: g.nonce, Epoch: epoch, Claim: phase36Claim{
		Protocol: phase36Protocol, OwnerSession: g.manager.runtimeSession,
		WorkspaceID: "tenantA", ActorID: "actorA", ResourceID: "resourceA",
		RequestID: id, ExpiresUnix: time.Now().Add(time.Minute).Unix(),
	}}
	return phase42SignedRequest{Request: x, Signature: phase42Signature(key, phase42RequestDomain, x)}
}

type phase42Fixture struct {
	gate        *phase42PortableGate
	manager     *Manager
	signedLease func(string, int64) []byte
	signer      ed25519.PrivateKey
	journal     *phase37Journal
	root        string
}

func phase42NewFixture(t *testing.T) phase42Fixture {
	t.Helper()
	manager, lease := phase19TestSignedConcurrentInstallation(t)
	if err := manager.Accept(lease(manager.runtimeSession, 1)); err != nil {
		t.Fatal(err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "journal")
	if err = os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	// A different synthetic provider signing key cannot validate worker
	// grants. This is a test-only trust boundary, not a live Authority.
	providerPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := phase37Open(root, providerPub)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = journal.close() })
	return phase42Fixture{gate: phase42NewGate(t, manager, pub, journal), manager: manager,
		signedLease: lease, signer: priv, journal: journal, root: root}
}
func TestLICA9Phase42RealManagerRequiresFreshSignedBoot(t *testing.T) {
	f := phase42NewFixture(t)
	req := phase42RequestFor(f.gate, f.signer, 1, "effect42")
	if err := f.gate.effect(req); !errors.Is(err, phase42Denied) {
		t.Fatalf("pre-grant effect=%v", err)
	}
	grant := phase42GrantFor(f.gate, f.signer, 1)
	grant.Grant.BootNonce = "invalid"
	if err := f.gate.grant(grant); !errors.Is(err, phase42Denied) {
		t.Fatal("unsigned modification authorized boot", err)
	}
	if err := f.gate.grant(phase42GrantFor(f.gate, f.signer, 1)); err != nil {
		t.Fatal(err)
	}
	if err := f.gate.effect(req); err != nil {
		t.Fatal(err)
	}
	if f.gate.effects.Load() != 1 {
		t.Fatal("actual Manager effect did not run once")
	}
	if err := f.gate.effect(req); !errors.Is(err, phase37Duplicate) {
		t.Fatal("duplicate journal replay", err)
	}
}
func TestLICA9Phase42PastBootFutureGrantDeniedAfterRestart(t *testing.T) {
	f := phase42NewFixture(t)
	if err := f.gate.grant(phase42GrantFor(f.gate, f.signer, 1)); err != nil {
		t.Fatal(err)
	}
	recorded := phase42GrantFor(f.gate, f.signer, 2)
	restarted := phase42NewGate(t, f.manager, f.signer.Public().(ed25519.PublicKey), f.journal)
	if restarted.nonce == f.gate.nonce {
		t.Fatal("fresh nonce reused")
	}
	if err := restarted.grant(recorded); !errors.Is(err, phase42Denied) {
		t.Fatal("old future grant replayed", err)
	}
	stale := phase42RequestFor(f.gate, f.signer, 1, "previousboot42")
	if err := restarted.effect(stale); !errors.Is(err, phase42Denied) {
		t.Fatal("old boot effect", err)
	}
	if err := restarted.grant(phase42GrantFor(restarted, f.signer, 2)); err != nil {
		t.Fatal(err)
	}
	if err := restarted.effect(phase42RequestFor(restarted, f.signer, 2, "newboot42")); err != nil {
		t.Fatal(err)
	}
	if restarted.effects.Load() != 1 {
		t.Fatal("fresh boot not admitted")
	}
}
func TestLICA9Phase42RevokedRealManagerV2BlocksEvenFreshGrant(t *testing.T) {
	f := phase42NewFixture(t)
	if err := f.gate.grant(phase42GrantFor(f.gate, f.signer, 1)); err != nil {
		t.Fatal(err)
	}
	follower, err := Open(f.manager.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = follower.Accept(f.signedLease(follower.runtimeSession, 2)); err != nil {
		t.Fatal(err)
	}
	req := phase42RequestFor(f.gate, f.signer, 1, "revoked42")
	if err = f.gate.effect(req); !errors.Is(err, ErrRestricted) {
		t.Fatal("revoked Manager accepted", err)
	}
	if f.gate.effects.Load() != 0 {
		t.Fatal("revoked effect executed")
	}
	if err = f.gate.effect(req); !errors.Is(err, phase37Duplicate) {
		t.Fatal("blind retry after denied admission", err)
	}
	if err = f.gate.grant(phase42GrantFor(f.gate, f.signer, 2)); !errors.Is(err, phase42Denied) {
		t.Fatal("revoked session regranted", err)
	}
}
func TestLICA9Phase42InvalidSignerScopeAndExpiredGrantFailClosed(t *testing.T) {
	f := phase42NewFixture(t)
	_, foreign, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.gate.grant(phase42GrantFor(f.gate, foreign, 1)); !errors.Is(err, phase42Denied) {
		t.Fatal("foreign grant", err)
	}
	expired := phase42GrantFor(f.gate, f.signer, 1)
	expired.Grant.ExpiresUnix = time.Now().Unix() - 1
	expired.Signature = phase42Signature(f.signer, phase42GrantDomain, expired.Grant)
	if err = f.gate.grant(expired); !errors.Is(err, phase42Denied) {
		t.Fatal("expired grant", err)
	}
	if err = f.gate.grant(phase42GrantFor(f.gate, f.signer, 1)); err != nil {
		t.Fatal(err)
	}
	req := phase42RequestFor(f.gate, f.signer, 1, "bad-scope42")
	req.Request.Claim.WorkspaceID = "tenantB"
	req.Signature = phase42Signature(f.signer, phase42RequestDomain, req.Request)
	if err = f.gate.effect(req); !errors.Is(err, phase42Denied) {
		t.Fatal("cross tenant", err)
	}
	req = phase42RequestFor(f.gate, foreign, 1, "foreign42")
	if err = f.gate.effect(req); !errors.Is(err, phase42Denied) {
		t.Fatal("foreign request", err)
	}
	req = phase42RequestFor(f.gate, f.signer, 1, "mutated42")
	req.Request.Claim.ResourceID = "other"
	if err = f.gate.effect(req); !errors.Is(err, phase42Denied) {
		t.Fatal("tampered request", err)
	}
	if f.gate.effects.Load() != 0 {
		t.Fatal("denied claims executed")
	}
}
func TestLICA9Phase42DurablePendingDoesNotRetryAfterReopen(t *testing.T) {
	f := phase42NewFixture(t)
	if err := f.gate.grant(phase42GrantFor(f.gate, f.signer, 1)); err != nil {
		t.Fatal(err)
	}
	req := phase42RequestFor(f.gate, f.signer, 1, "pending42")
	if err := f.gate.effect(req); err != nil {
		t.Fatal(err)
	}
	reopened, err := phase37Open(f.root, f.journal.provider)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.close()
	restarted := phase42NewGate(t, f.manager, f.signer.Public().(ed25519.PublicKey), reopened)
	if err = restarted.grant(phase42GrantFor(restarted, f.signer, 2)); err != nil {
		t.Fatal(err)
	}
	replay := phase42RequestFor(restarted, f.signer, 2, "pending42")
	if err = restarted.effect(replay); !errors.Is(err, phase37Duplicate) {
		t.Fatal("persisted unknown executed", err)
	}
	if restarted.effects.Load() != 0 {
		t.Fatal("unknown retry executed")
	}
	outcome, err := reopened.inspect(req.Request.Claim)
	if err != nil || outcome != "RECONCILE_REQUIRED" {
		t.Fatal("unknown outcome lost", outcome, err)
	}
}
func TestLICA9Phase42ConcurrentSameRequestIDOneRealManagerEffect(t *testing.T) {
	f := phase42NewFixture(t)
	if err := f.gate.grant(phase42GrantFor(f.gate, f.signer, 1)); err != nil {
		t.Fatal(err)
	}
	req := phase42RequestFor(f.gate, f.signer, 1, "parallel42")
	var wg sync.WaitGroup
	var successes, duplicates atomic.Int64
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := f.gate.effect(req)
			switch {
			case err == nil:
				successes.Add(1)
			case errors.Is(err, phase37Duplicate):
				duplicates.Add(1)
			default:
				t.Errorf("unexpected concurrent result: %v", err)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 || duplicates.Load() != 11 || f.gate.effects.Load() != 1 {
		t.Fatalf("success=%d duplicate=%d real effects=%d", successes.Load(), duplicates.Load(), f.gate.effects.Load())
	}
}

// The digest used in the durable journal is always the signed FULL claim, not
// merely the bare request ID; no independent provider transaction is inferred.
func TestLICA9Phase42SignedClaimDigestBindsSessionEpochAndBoot(t *testing.T) {
	f := phase42NewFixture(t)
	a := phase42RequestFor(f.gate, f.signer, 1, "digest42")
	b := a
	b.Request.BootNonce = "old-boot"
	if phase42Check(f.signer.Public().(ed25519.PublicKey), phase42RequestDomain, b.Request, a.Signature) {
		t.Fatal("old-boot signed claim verified")
	}
	d1 := sha256.Sum256([]byte(a.Signature))
	d2 := sha256.Sum256([]byte(b.Signature))
	if d1 != d2 {
		t.Fatal("signatures unexpectedly modified by claim mutation")
	}
	b.Request.Claim.OwnerSession = "another-session"
	if phase42Check(f.signer.Public().(ed25519.PublicKey), phase42RequestDomain, b.Request, a.Signature) {
		t.Fatal("cross session signed claim verified")
	}
}

func TestLICA9Phase42RealManagerProcessSessionHandoffRequiresNewBoot(t *testing.T) {
	f := phase42NewFixture(t)
	if err := f.gate.grant(phase42GrantFor(f.gate, f.signer, 1)); err != nil {
		t.Fatal(err)
	}
	oldGrant := phase42GrantFor(f.gate, f.signer, 2)
	next, err := Open(f.manager.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if next.runtimeSession == f.manager.runtimeSession {
		t.Fatal("Manager reused process session")
	}
	if err := next.Accept(f.signedLease(next.runtimeSession, 2)); err != nil {
		t.Fatal(err)
	}
	newGate := phase42NewGate(t, next, f.signer.Public().(ed25519.PublicKey), f.journal)
	if err := newGate.grant(oldGrant); !errors.Is(err, phase42Denied) {
		t.Fatal("old signed boot/session rearmed new Manager", err)
	}
	if err := newGate.grant(phase42GrantFor(newGate, f.signer, 2)); err != nil {
		t.Fatal(err)
	}
	oldEffect := phase42RequestFor(f.gate, f.signer, 1, "old-manager42")
	if err := newGate.effect(oldEffect); !errors.Is(err, phase42Denied) {
		t.Fatal("old session request reached new Manager", err)
	}
	newEffect := phase42RequestFor(newGate, f.signer, 2, "new-manager42")
	if err := newGate.effect(newEffect); err != nil {
		t.Fatal(err)
	}
	if newGate.effects.Load() != 1 || f.gate.effects.Load() != 0 {
		t.Fatal("wrong session ran synthetic callback")
	}
	if err := f.gate.effect(phase42RequestFor(f.gate, f.signer, 1, "old-after-transfer42")); !errors.Is(err, ErrRestricted) {
		t.Fatal("real Manager accepted stale V2 session", err)
	}
}
