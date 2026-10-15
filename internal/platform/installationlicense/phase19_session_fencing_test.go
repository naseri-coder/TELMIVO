package installationlicense

import (
    "context"
    "crypto/ed25519"
    "crypto/rand"
    "crypto/sha256"
    "errors"
    c "github.com/naseri-coder/TELMIVO/internal/platform/installationlicense/licensecrypto"
    "path/filepath"
    "sync"
    "sync/atomic"
    "testing"
    "time"
)

// Test-only signed V2 fixture. All signing keys are fresh in-memory test
// keys; no real Authority, database, production signer or network is used.
func phase19TestSignedConcurrentInstallation(t *testing.T) (*Manager, func(string, int64) []byte) {
    t.Helper()
    now := time.Now().UTC().Truncate(time.Microsecond)
    rootPub, rootKey, err := ed25519.GenerateKey(rand.Reader)
    if err != nil { t.Fatal(err) }
    signerPub, signerKey, err := ed25519.GenerateKey(rand.Reader)
    if err != nil { t.Fatal(err) }
    ids := make([]string, 6)
    for idx := range ids {
        ids[idx], err = UUID()
        if err != nil { t.Fatal(err) }
    }
    rootID, authorityID, keyID, certID, licenseID, activationID := ids[0], ids[1], ids[2], ids[3], ids[4], ids[5]
    root := c.Root{
        ID: rootID, Version: 1, PublicKey: rootPub, Status: c.Active,
        NotBefore: now.Add(-2 * time.Hour), NotAfter: now.Add(10 * 24 * time.Hour),
    }
    manager, err := Open(Config{
        Directory: filepath.Join(t.TempDir(), "installation"), AuthorityID: authorityID,
        Environment: "TEST", Roots: c.Roots{rootID: root}, RequireConcurrency: true,
    })
    if err != nil { t.Fatal(err) }
    var identity Identity
    if err := manager.read("identity.json", &identity); err != nil { t.Fatal(err) }
    identityHash := sha256.Sum256(identity.PublicKey())
    cert := c.Certificate{
        Version: 1, Algorithm: c.Algorithm, Product: c.Product,
        CertificateID: certID, KeyID: keyID, RootID: rootID, RootVersion: 1,
        PublicKey: signerPub,
        NotBefore: now.Add(-time.Hour).UnixMicro(),
        NotAfter: now.Add(4 * 24 * time.Hour).UnixMicro(),
        TokenType: c.TokenType, Audience: c.Audience,
        AuthorityID: authorityID, Environment: "TEST", MaximumLifetime: c.MaximumLease.Microseconds(),
    }
    certificateBytes, err := c.EncodeCertificate(cert)
    if err != nil { t.Fatal(err) }
    rootSig := ed25519.Sign(rootKey, c.CertificateSigningInput(certificateBytes))
    packed, err := c.PackCertificate(certificateBytes, rootSig)
    if err != nil { t.Fatal(err) }
    certHash := sha256.Sum256(packed)
    makeLease := func(sessionID string, generation int64) []byte {
        leaseID, err := UUID()
        if err != nil { t.Fatal(err) }
        tokenID, err := UUID()
        if err != nil { t.Fatal(err) }
        claims := c.Claims{
            Version: 2, Algorithm: c.Algorithm, Product: c.Product, Type: c.TokenType,
            Issuer: authorityID, Audience: c.Audience,
            LicenseID: licenseID, ActivationID: activationID,
            InstallationID: identity.ID, InstallationKeyHash: identityHash[:],
            Edition: "TEST_ONLY", Capabilities: []string{"product.use"},
            LicenseRevision: 1, ActivationGeneration: 1, AuthorityEpoch: 1,
            IssuedAt: now.UnixMicro(), NotBefore: now.UnixMicro(),
            RefreshAfter: now.Add(time.Hour).UnixMicro(),
            OnlineUntil: now.Add(24 * time.Hour).UnixMicro(),
            OfflineUntil: now.Add(48 * time.Hour).UnixMicro(),
            CommercialExpiresAt: now.Add(72 * time.Hour).UnixMicro(),
            TokenID: tokenID, KID: keyID, Environment: "TEST",
            CertificateID: certID, CertificateHash: certHash[:],
            ConcurrentPolicy: c.ConcurrentPolicy, RuntimeSessionID: sessionID,
            ConcurrencyLeaseID: leaseID, ConcurrencyGeneration: generation,
            RecoveryGeneration: 0, ConflictState: "NORMAL",
            ConcurrencyIssuedAt: now.UnixMicro(),
            ConcurrencyExpiresAt: now.Add(48 * time.Hour).UnixMicro(),
        }
        payload, err := c.EncodeClaims(claims)
        if err != nil { t.Fatal(err) }
        envelope, err := c.EncodeEnvelope(c.Envelope{
            Version: 1, Algorithm: c.Algorithm, Certificate: certificateBytes,
            RootSignature: rootSig, Payload: payload,
            Signature: ed25519.Sign(signerKey, c.LicenseSigningInput(payload)),
        })
        if err != nil { t.Fatal(err) }
        return envelope
    }
    return manager, makeLease
}

func TestLICA9OldRuntimeSessionCannotReuseSignedLease(t *testing.T) {
    leader, signedLease := phase19TestSignedConcurrentInstallation(t)
    first := signedLease(leader.runtimeSession, 1)
    if err := leader.Accept(first); err != nil { t.Fatalf("first signed V2: %v", err) }
    if d := leader.Decision(); d.State != "VALID_ONLINE" {
        t.Fatalf("original session rejected: %+v", d)
    }
    restarted, err := Open(leader.cfg)
    if err != nil { t.Fatal(err) }
    if restarted.runtimeSession == leader.runtimeSession { t.Fatal("new process reused session ID") }
    d := restarted.Decision()
    if d.State != "RESTRICTED" || d.Reason != "CONCURRENCY_REFRESH_REQUIRED" || !d.RefreshDue {
        t.Fatalf("old signed lease escaped runtime session boundary: %+v", d)
    }
    called := false
    if err := restarted.Effect(context.Background(), func() error { called = true; return nil }); !errors.Is(err, ErrRestricted) || called {
        t.Fatalf("stale signed lease authorized effect: called=%v err=%v", called, err)
    }
    if err := restarted.Accept(signedLease(restarted.runtimeSession, 2)); err != nil { t.Fatal(err) }
    if d := restarted.Decision(); d.State != "VALID_ONLINE" { t.Fatalf("new signed session denied: %+v", d) }
    if err := restarted.Accept(first); err == nil { t.Fatal("signed generation-1 replay accepted after generation 2") }
}

func TestLICA9ConcurrentStaleSessionNeverRunsEffects(t *testing.T) {
    leader, signedLease := phase19TestSignedConcurrentInstallation(t)
    if err := leader.Accept(signedLease(leader.runtimeSession, 1)); err != nil { t.Fatal(err) }
    stale, err := Open(leader.cfg)
    if err != nil { t.Fatal(err) }
    var authorized, rejected, illicit atomic.Int64
    var wg sync.WaitGroup
    for i := 0; i < 12; i++ {
        wg.Add(1)
        go func(index int) {
            defer wg.Done()
            if index%2 == 0 {
                err := leader.Effect(context.Background(), func() error { authorized.Add(1); return nil })
                if err != nil { illicit.Add(1) }
            } else {
                err := stale.Effect(context.Background(), func() error { illicit.Add(1); return nil })
                if errors.Is(err, ErrRestricted) { rejected.Add(1) } else { illicit.Add(1) }
            }
        }(i)
    }
    wg.Wait()
    if authorized.Load() != 6 || rejected.Load() != 6 || illicit.Load() != 0 {
        t.Fatalf("stale-session race: leader=%d restricted=%d illicit=%d", authorized.Load(), rejected.Load(), illicit.Load())
    }
}
