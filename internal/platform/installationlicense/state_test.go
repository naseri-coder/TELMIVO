package installationlicense

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	c "github.com/naseri-coder/TELMIVO/internal/platform/installationlicense/licensecrypto"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Manager, func(int64, int64, int64) []byte) {
	t.Helper()
	now := time.Now().UTC()
	rp, rk, _ := ed25519.GenerateKey(rand.Reader)
	sp, sk, _ := ed25519.GenerateKey(rand.Reader)
	rid, _ := UUID()
	aid, _ := UUID()
	kid, _ := UUID()
	cid, _ := UUID()
	lid, _ := UUID()
	activation, _ := UUID()
	root := c.Root{ID: rid, Version: 1, PublicKey: rp, Status: c.Active, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(30 * 24 * time.Hour)}
	dir := filepath.Join(t.TempDir(), "installation")
	m, e := Open(Config{Directory: dir, AuthorityID: aid, Environment: "TEST", Roots: c.Roots{rid: root}})
	if e != nil {
		t.Fatal(e)
	}
	m.now = func() time.Time { return now }
	cert := c.Certificate{Version: 1, Algorithm: c.Algorithm, Product: c.Product, CertificateID: cid, KeyID: kid, RootID: rid, RootVersion: 1, PublicKey: sp, NotBefore: now.Add(-time.Hour).UnixMicro(), NotAfter: now.Add(7 * 24 * time.Hour).UnixMicro(), TokenType: c.TokenType, Audience: c.Audience, AuthorityID: aid, Environment: "TEST", MaximumLifetime: c.MaximumLease.Microseconds()}
	cb, e := c.EncodeCertificate(cert)
	if e != nil {
		t.Fatal(e)
	}
	rs := ed25519.Sign(rk, c.CertificateSigningInput(cb))
	packed, _ := c.PackCertificate(cb, rs)
	ch := sha256.Sum256(packed)
	var i Identity
	if e = m.read("identity.json", &i); e != nil {
		t.Fatal(e)
	}
	ih := sha256.Sum256(i.PublicKey())
	return m, func(epoch, rev, gen int64) []byte {
		tid, _ := UUID()
		claims := c.Claims{Version: 1, Algorithm: c.Algorithm, Product: c.Product, Type: c.TokenType, Issuer: aid, Audience: c.Audience, LicenseID: lid, ActivationID: activation, InstallationID: i.ID, InstallationKeyHash: ih[:], Edition: "TEST", Capabilities: []string{"product.use"}, LicenseRevision: rev, ActivationGeneration: gen, AuthorityEpoch: epoch, IssuedAt: now.UnixMicro(), NotBefore: now.UnixMicro(), RefreshAfter: now.Add(6 * time.Hour).UnixMicro(), OnlineUntil: now.Add(24 * time.Hour).UnixMicro(), OfflineUntil: now.Add(72 * time.Hour).UnixMicro(), CommercialExpiresAt: now.Add(10 * 24 * time.Hour).UnixMicro(), TokenID: tid, KID: kid, Environment: "TEST", CertificateID: cid, CertificateHash: ch[:]}
		p, e := c.EncodeClaims(claims)
		if e != nil {
			t.Fatal(e)
		}
		raw, e := c.EncodeEnvelope(c.Envelope{Version: 1, Algorithm: c.Algorithm, Certificate: cb, RootSignature: rs, Payload: p, Signature: ed25519.Sign(sk, c.LicenseSigningInput(p))})
		if e != nil {
			t.Fatal(e)
		}
		return raw
	}
}
func TestIdentityRestartAndCorruption(t *testing.T) {
	m, _ := fixture(t)
	var i Identity
	m.read("identity.json", &i)
	again, e := Open(m.cfg)
	if e != nil {
		t.Fatal(e)
	}
	var j Identity
	again.read("identity.json", &j)
	if i.ID != j.ID || string(i.PrivateKey) != string(j.PrivateKey) {
		t.Fatal("identity changed")
	}
	os.WriteFile(filepath.Join(m.cfg.Directory, "identity.json"), []byte("corrupt"), 0600)
	if _, e = Open(m.cfg); e == nil {
		t.Fatal("regenerated corrupt identity")
	}
}
func TestLeaseLifecycleAndFences(t *testing.T) {
	m, token := fixture(t)
	if m.Decision().State != "RESTRICTED" {
		t.Fatal("unlicensed access")
	}
	old := token(1, 1, 1)
	if e := m.Accept(old); e != nil {
		t.Fatal(e)
	}
	if m.Decision().State != "VALID_ONLINE" {
		t.Fatal(m.Decision())
	}
	now := m.now()
	m.now = func() time.Time { return now.Add(25 * time.Hour) }
	if m.Decision().State != "VALID_OFFLINE" {
		t.Fatal(m.Decision())
	}
	m.now = func() time.Time { return now.Add(72 * time.Hour) }
	if m.Decision().State != "RESTRICTED" {
		t.Fatal("deadline extended")
	}
	m.now = func() time.Time { return now }
	if m.Decision().State != "RESTRICTED" {
		t.Fatal("clock rollback allowed")
	}
}
func TestRollbackAndWrongIdentity(t *testing.T) {
	m, token := fixture(t)
	old := token(1, 1, 1)
	if e := m.Accept(token(2, 2, 2)); e != nil {
		t.Fatal(e)
	}
	if e := m.Accept(old); e == nil {
		t.Fatal("stale lease accepted")
	}
	other, _ := fixture(t)
	if e := other.Accept(old); e == nil {
		t.Fatal("different identity accepted")
	}
	bad := token(2, 2, 2)
	bad[len(bad)-1] ^= 1
	if e := m.Accept(bad); e == nil {
		t.Fatal("bad signature accepted")
	}
}
func TestEffectRechecksDisk(t *testing.T) {
	m, token := fixture(t)
	if e := m.Accept(token(1, 1, 1)); e != nil {
		t.Fatal(e)
	}
	called := false
	if e := m.Effect(context.Background(), func() error { called = true; return nil }); e != nil || !called {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(m.cfg.Directory, "state.json"), []byte("bad"), 0600)
	called = false
	if e := m.Effect(context.Background(), func() error { called = true; return nil }); e == nil || called {
		t.Fatal("effect escaped restriction")
	}
}
func TestSymlinkAndPermissions(t *testing.T) {
	m, _ := fixture(t)
	os.Chmod(m.cfg.Directory, 0755)
	if _, e := Open(m.cfg); e == nil {
		t.Fatal("unsafe directory accepted")
	}
	os.Chmod(m.cfg.Directory, 0700)
	p := filepath.Join(t.TempDir(), "link")
	os.Symlink(m.cfg.Directory, p)
	cfg := m.cfg
	cfg.Directory = p
	if _, e := Open(cfg); e == nil {
		t.Fatal("symlink accepted")
	}
}
func TestStrictTrustJSON(t *testing.T) {
	if StrictJSON([]byte(`{"a":1,"a":2}`), &map[string]int{}) == nil {
		t.Fatal("duplicate key accepted")
	}
	if StrictJSON([]byte(`{} {}`), &map[string]int{}) == nil {
		t.Fatal("trailing object accepted")
	}
}
func TestGenerationFenceWithoutEpochChange(t *testing.T) {
	m, token := fixture(t)
	if e := m.Accept(token(1, 2, 3)); e != nil {
		t.Fatal(e)
	}
	if e := m.Accept(token(1, 2, 2)); e == nil {
		t.Fatal("generation rollback accepted")
	}
}
func TestTerminalDenialCannotReplayCachedLease(t *testing.T) {
	m, token := fixture(t)
	raw := token(1, 1, 1)
	if e := m.Accept(raw); e != nil {
		t.Fatal(e)
	}
	var s State
	m.read("state.json", &s)
	s.Restricted = "LICENSE_REVOKED"
	m.write("state.json", s)
	if e := m.Accept(raw); e == nil {
		t.Fatal("revocation cleared by cached lease")
	}
}
func TestRestrictedMiddleware(t *testing.T) {
	m, _ := fixture(t)
	calls := 0
	h := Middleware(m, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(200) }))
	for _, test := range []struct {
		method, path string
		want         int
	}{{"GET", "/health/live", 200}, {"GET", "/health/license", 200}, {"POST", "/api/v1/wallet", 403}, {"GET", "/api/v1/export", 200}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(test.method, test.path, nil))
		if w.Code != test.want {
			t.Fatalf("%s: %d", test.path, w.Code)
		}
	}
	if calls != 2 {
		t.Fatal("protected mutation reached handler")
	}
}
func TestObservedExpiryRetainsTimeFloor(t *testing.T) {
	m, token := fixture(t)
	if e := m.Accept(token(1, 1, 1)); e != nil {
		t.Fatal(e)
	}
	now := m.now()
	m.now = func() time.Time { return now.Add(73 * time.Hour) }
	if m.Decision().State != "RESTRICTED" {
		t.Fatal("expired")
	}
	m.now = func() time.Time { return now.Add(30 * time.Hour) }
	if m.Decision().State != "RESTRICTED" {
		t.Fatal("expired lease revived after time rollback")
	}
}
func TestPrivateMaterialCannotEnterGitWorktree(t *testing.T) {
	m, _ := fixture(t)
	dir := t.TempDir()
	if e := os.Mkdir(filepath.Join(dir, ".git"), 0700); e != nil {
		t.Fatal(e)
	}
	cfg := m.cfg
	cfg.Directory = filepath.Join(dir, "identity")
	if _, e := Open(cfg); e == nil {
		t.Fatal("private identity created in repository")
	}
}
func TestProductionRefusesInjectedClock(t *testing.T) {
	m, _ := fixture(t)
	cfg := m.cfg
	cfg.Environment = "PRODUCTION"
	cfg.Clock = time.Now
	if _, e := Open(cfg); e == nil {
		t.Fatal("production clock bypass")
	}
}
func TestWrongKeySameInstallation(t *testing.T) {
	m, token := fixture(t)
	raw := token(1, 1, 1)
	if e := m.Accept(raw); e != nil {
		t.Fatal(e)
	}
	var identity Identity
	if e := m.read("identity.json", &identity); e != nil {
		t.Fatal(e)
	}
	_, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	identity.PrivateKey = key
	if e = m.write("identity.json", identity); e != nil {
		t.Fatal(e)
	}
	if m.Decision().State != "RESTRICTED" {
		t.Fatal("same ID with wrong key granted access")
	}
}
func TestImmutableTrustAndRedactedFormatting(t *testing.T) {
	m, token := fixture(t)
	if e := m.Accept(token(1, 1, 1)); e != nil {
		t.Fatal(e)
	}
	cfg := m.cfg
	again, e := Open(cfg)
	if e != nil {
		t.Fatal(e)
	}
	for id, root := range cfg.Roots {
		root.Status = c.Revoked
		cfg.Roots[id] = root
	}
	if again.Decision().State != "VALID_ONLINE" {
		t.Fatal("caller mutated manager trust")
	}
	var identity Identity
	again.read("identity.json", &identity)
	if !strings.Contains(fmt.Sprintf("%+v", identity), "REDACTED") {
		t.Fatal("identity format exposes private material")
	}
}
func TestSettlementNeverGrantsExternalEffect(t *testing.T) {
	m, _ := fixture(t)
	ctx := Settlement(WithManager(context.Background(), m))
	called := false
	if e := RuntimeEffect(ctx, func() error { called = true; return nil }); e == nil || called {
		t.Fatal("settlement granted external effect")
	}
}
func TestInvalidConfigurationContextIsRestricted(t *testing.T) {
	ctx := RequireContext(context.Background())
	if CheckRuntime(ctx) == nil {
		t.Fatal("required context bypass")
	}
	called := false
	if e := RuntimeEffect(ctx, func() error { called = true; return nil }); e == nil || called {
		t.Fatal("nil configuration effect")
	}
}
