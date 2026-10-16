//go:build integration

package installationlicense

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	c "github.com/naseri-coder/TELMIVO/internal/platform/installationlicense/licensecrypto"
)

// NewTestOnlySignedV2RuntimeForNativeIntegration constructs a real signed V2
// Manager with fresh ephemeral TEST-only Ed25519 root/signer keys. It is not
// compiled into ordinary Product builds (requires -tags=integration).
// Never call this in an actual installation or with nonisolated database URLs.
func NewTestOnlySignedV2RuntimeForNativeIntegration(stateDirectory string) (*Manager, error) {
	if stateDirectory == "" || !strings.HasSuffix(filepath.Base(os.Args[0]), ".test") ||
		os.Getenv("TELMIVO_LIC_A9_NATIVE_FIXTURE") != "YES" ||
		strings.EqualFold(os.Getenv("TELMIVO_ENV"), "production") ||
		strings.EqualFold(os.Getenv("TELMIVO_LICENSE_ENVIRONMENT"), "production") ||
		!isolatedNativeIntegrationDB(os.Getenv("TEST_DATABASE_URL")) ||
		!isolatedNativeIntegrationDB(os.Getenv("TEST_MIGRATION_DATABASE_URL")) {
		return nil, ErrState
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	rootPublic, rootPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	signerPublic, signerPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 8)
	for idx := range ids {
		ids[idx], err = UUID()
		if err != nil {
			return nil, err
		}
	}
	rootID, authorityID, keyID, certID := ids[0], ids[1], ids[2], ids[3]
	licenseID, activationID, leaseID, tokenID := ids[4], ids[5], ids[6], ids[7]
	root := c.Root{
		ID: rootID, Version: 1, PublicKey: rootPublic, Status: c.Active,
		NotBefore: now.Add(-2 * time.Hour), NotAfter: now.Add(10 * 24 * time.Hour),
	}
	manager, err := Open(Config{
		Directory: stateDirectory, AuthorityID: authorityID,
		Environment: "TEST", Roots: c.Roots{rootID: root}, RequireConcurrency: true,
	})
	if err != nil {
		return nil, err
	}
	var identity Identity
	if err = manager.read("identity.json", &identity); err != nil {
		return nil, err
	}
	keyHash := sha256.Sum256(identity.PublicKey())
	certificate := c.Certificate{
		Version: 1, Algorithm: c.Algorithm, Product: c.Product,
		CertificateID: certID, KeyID: keyID, RootID: rootID, RootVersion: 1,
		PublicKey: signerPublic, NotBefore: now.Add(-time.Hour).UnixMicro(),
		NotAfter:  now.Add(4 * 24 * time.Hour).UnixMicro(),
		TokenType: c.TokenType, Audience: c.Audience,
		AuthorityID: authorityID, Environment: "TEST",
		MaximumLifetime: c.MaximumLease.Microseconds(),
	}
	certificateBytes, err := c.EncodeCertificate(certificate)
	if err != nil {
		return nil, err
	}
	rootSignature := ed25519.Sign(rootPrivate, c.CertificateSigningInput(certificateBytes))
	packedCertificate, err := c.PackCertificate(certificateBytes, rootSignature)
	if err != nil {
		return nil, err
	}
	certificateHash := sha256.Sum256(packedCertificate)
	claims := c.Claims{
		Version: 2, Algorithm: c.Algorithm, Product: c.Product, Type: c.TokenType,
		Issuer: authorityID, Audience: c.Audience,
		LicenseID: licenseID, ActivationID: activationID,
		InstallationID: identity.ID, InstallationKeyHash: keyHash[:],
		Edition: "TEST_ONLY", Capabilities: []string{"product.use"},
		LicenseRevision: 1, ActivationGeneration: 1, AuthorityEpoch: 1,
		IssuedAt: now.UnixMicro(), NotBefore: now.UnixMicro(),
		RefreshAfter:        now.Add(time.Hour).UnixMicro(),
		OnlineUntil:         now.Add(24 * time.Hour).UnixMicro(),
		OfflineUntil:        now.Add(48 * time.Hour).UnixMicro(),
		CommercialExpiresAt: now.Add(72 * time.Hour).UnixMicro(),
		TokenID:             tokenID, KID: keyID, Environment: "TEST",
		CertificateID: certID, CertificateHash: certificateHash[:],
		ConcurrentPolicy: c.ConcurrentPolicy, RuntimeSessionID: manager.runtimeSession,
		ConcurrencyLeaseID: leaseID, ConcurrencyGeneration: 1,
		RecoveryGeneration: 0, ConflictState: "NORMAL",
		ConcurrencyIssuedAt:  now.UnixMicro(),
		ConcurrencyExpiresAt: now.Add(48 * time.Hour).UnixMicro(),
	}
	payload, err := c.EncodeClaims(claims)
	if err != nil {
		return nil, err
	}
	envelope, err := c.EncodeEnvelope(c.Envelope{
		Version: 1, Algorithm: c.Algorithm, Certificate: certificateBytes,
		RootSignature: rootSignature, Payload: payload,
		Signature: ed25519.Sign(signerPrivate, c.LicenseSigningInput(payload)),
	})
	if err != nil {
		return nil, err
	}
	if err := manager.Accept(envelope); err != nil {
		return nil, err
	}
	if d := manager.Decision(); d.State != "VALID_ONLINE" {
		return nil, ErrRestricted
	}
	return manager, nil
}

func isolatedNativeIntegrationDB(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "postgres" && u.Scheme != "postgresql" ||
		u.Path != "/telmivo_test" || u.User == nil || u.Host == "" ||
		u.RawQuery != "sslmode=disable" || u.Fragment != "" ||
		(u.Port() != "" && u.Port() != "5432") {
		return false
	}
	switch strings.ToLower(u.Hostname()) {
	case "127.0.0.1", "localhost", "postgres":
		return true
	default:
		return false
	}
}
