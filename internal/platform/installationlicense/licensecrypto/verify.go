package licensecrypto

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
)

func validTimestamp(us int64) bool { return us > 0 && us <= 253402300799999999 }
func validStatus(s Status) bool    { return s == Active || s == Retiring }
func VerifyCertificate(payload, signature []byte, roots Roots, p Policy) (Certificate, error) {
	return verifyCertificate(payload, signature, roots, p, false)
}

// ValidateCertificateRegistration permits staging a future-valid certificate,
// but cannot authorize any token. Routine verification never uses this path.
func ValidateCertificateRegistration(payload, signature []byte, roots Roots, p Policy) (Certificate, error) {
	return verifyCertificate(payload, signature, roots, p, true)
}
func verifyCertificate(payload, signature []byte, roots Roots, p Policy, registration bool) (Certificate, error) {
	c, err := DecodeCertificate(payload)
	if err != nil {
		return Certificate{}, err
	}
	// Parsing before root verification is framing/key selection only.
	root, ok := roots[c.RootID]
	if !ok {
		return Certificate{}, UnknownRoot
	}
	if len(root.PublicKey) != ed25519.PublicKeySize || len(signature) != 64 || !ed25519.Verify(root.PublicKey, CertificateSigningInput(payload), signature) {
		return Certificate{}, InvalidCertificate
	}
	if c.Version != 1 {
		return Certificate{}, UnsupportedVersion
	}
	if c.Algorithm != Algorithm {
		return Certificate{}, UnsupportedAlgorithm
	}
	if c.RootVersion != root.Version || c.RootID != root.ID {
		return Certificate{}, InvalidCertificate
	}
	if registration && root.Status == Retiring {
		return Certificate{}, RootRevoked
	}
	if root.Status == Revoked {
		return Certificate{}, RootRevoked
	}
	if !validStatus(root.Status) {
		return Certificate{}, RootExpired
	}
	now := p.Now.UTC()
	if now.IsZero() {
		return Certificate{}, Malformed
	}
	if now.Before(root.NotBefore) {
		return Certificate{}, RootNotYetValid
	}
	if !now.Before(root.NotAfter) {
		return Certificate{}, RootExpired
	}
	if c.Product != Product || c.Product != p.ExpectedProduct {
		return Certificate{}, WrongProduct
	}
	if c.Audience != Audience || c.Audience != p.ExpectedAudience {
		return Certificate{}, WrongAudience
	}
	if c.TokenType != TokenType {
		return Certificate{}, WrongType
	}
	if !ValidID(c.CertificateID) || !ValidID(c.KeyID) || !ValidID(c.RootID) || !ValidID(c.AuthorityID) || len(c.PublicKey) != 32 || !validTimestamp(c.NotBefore) || !validTimestamp(c.NotAfter) || c.NotAfter <= c.NotBefore || c.NotBefore < root.NotBefore.UnixMicro() || c.NotAfter > root.NotAfter.UnixMicro() || c.MaximumLifetime <= 0 || c.MaximumLifetime > MaximumLease.Microseconds() || c.EpochMin < 0 || c.EpochMax < c.EpochMin || (c.EpochMin == 0 && c.EpochMax != 0) {
		return Certificate{}, InvalidCertificate
	}
	if c.AuthorityID != p.ExpectedAuthority || c.Environment != p.ExpectedEnvironment || (c.Environment != "TEST" && c.Environment != "DEVELOPMENT" && c.Environment != "PRODUCTION") {
		return Certificate{}, ScopeMismatch
	}
	if !registration && now.UnixMicro() < c.NotBefore {
		return Certificate{}, SignerNotYetValid
	}
	if now.UnixMicro() >= c.NotAfter {
		return Certificate{}, SignerExpired
	}
	if p.KnownSigners != nil {
		status, ok := p.KnownSigners[c.KeyID]
		if !ok {
			return Certificate{}, UnknownSigner
		}
		if status == Revoked {
			return Certificate{}, SignerRevoked
		}
		if status == Expired {
			return Certificate{}, SignerExpired
		}
		if !validStatus(status) {
			return Certificate{}, SignerNotYetValid
		}
	}
	return c, nil
}
func ValidateClaims(c Claims, cert Certificate) error {
	if c.Version == 2 {
		if c.ConcurrencyIssuedAt < c.IssuedAt || c.ConcurrencyExpiresAt <= c.ConcurrencyIssuedAt || c.ConcurrencyExpiresAt > c.OfflineUntil || c.ConcurrencyExpiresAt-c.ConcurrencyIssuedAt > MaximumLease.Microseconds() || c.ConcurrentPolicy != ConcurrentPolicy || !ValidID(c.RuntimeSessionID) || !ValidID(c.ConcurrencyLeaseID) || c.ConcurrencyGeneration < 1 || c.RecoveryGeneration < 0 || (c.ConflictState != "NORMAL" && c.ConflictState != "CLONE_SUSPECTED") {
			return Malformed
		}
	} else if c.ConcurrentPolicy != "" {
		return Malformed
	}
	if c.Version != 1 && c.Version != 2 {
		return UnsupportedVersion
	}
	if c.Algorithm != Algorithm {
		return UnsupportedAlgorithm
	}
	if c.Product != cert.Product || c.Product != Product {
		return WrongProduct
	}
	if c.Type != TokenType || c.Type != cert.TokenType {
		return WrongType
	}
	if c.Audience != cert.Audience || c.Audience != Audience {
		return WrongAudience
	}
	if c.CertificateID != cert.CertificateID || len(c.CertificateHash) != 32 {
		return InvalidCertificate
	}
	if c.KID != cert.KeyID {
		return UnknownSigner
	}
	if c.Issuer != cert.AuthorityID || c.Environment != cert.Environment {
		return ScopeMismatch
	}
	for _, id := range []string{c.Issuer, c.LicenseID, c.ActivationID, c.InstallationID, c.TokenID, c.KID} {
		if !ValidID(id) {
			return Malformed
		}
	}
	for _, capability := range c.Capabilities {
		if capability == "" {
			return Malformed
		}
	}
	if len(c.InstallationKeyHash) != 32 || c.Edition == "" || c.LicenseRevision < 1 || c.ActivationGeneration < 1 || c.AuthorityEpoch < 1 {
		return Malformed
	}
	if cert.EpochMin > 0 && (c.AuthorityEpoch < cert.EpochMin || c.AuthorityEpoch > cert.EpochMax) {
		return EpochStale
	}
	for _, n := range []int64{c.IssuedAt, c.NotBefore, c.RefreshAfter, c.OnlineUntil, c.OfflineUntil, c.CommercialExpiresAt} {
		if !validTimestamp(n) {
			return Malformed
		}
	}
	if c.IssuedAt > c.NotBefore+ClockTolerance.Microseconds() || c.IssuedAt >= c.RefreshAfter || c.RefreshAfter > c.OnlineUntil || c.OnlineUntil > c.OfflineUntil || c.OfflineUntil > c.CommercialExpiresAt || c.OfflineUntil-c.IssuedAt > MaximumLease.Microseconds() || c.OfflineUntil-c.IssuedAt > cert.MaximumLifetime || c.IssuedAt < cert.NotBefore || c.OfflineUntil > cert.NotAfter {
		return InvalidCertificate
	}
	return nil
}
func Verify(artifact []byte, roots Roots, p Policy) (Result, error) {
	e, err := DecodeEnvelope(artifact)
	if err != nil {
		return Result{}, err
	}
	if e.Version != 1 {
		return Result{}, UnsupportedVersion
	}
	if e.Algorithm != Algorithm {
		return Result{}, UnsupportedAlgorithm
	}
	cert, err := VerifyCertificate(e.Certificate, e.RootSignature, roots, p)
	if err != nil {
		return Result{}, err
	}
	// No license claim is parsed or authorized before signature verification.
	if !ed25519.Verify(ed25519.PublicKey(cert.PublicKey), LicenseSigningInput(e.Payload), e.Signature) {
		return Result{}, InvalidSignature
	}
	c, err := DecodeClaims(e.Payload)
	if err != nil {
		return Result{}, err
	}
	packed, packErr := PackCertificate(e.Certificate, e.RootSignature)
	if packErr != nil {
		return Result{}, packErr
	}
	certHash := sha256.Sum256(packed)
	if !bytes.Equal(c.CertificateHash, certHash[:]) {
		return Result{}, InvalidCertificate
	}
	if err = ValidateClaims(c, cert); err != nil {
		return Result{}, err
	}
	if c.InstallationID != p.InstallationID || !bytes.Equal(c.InstallationKeyHash, p.InstallationKeyHash) {
		return Result{}, InstallationMismatch
	}
	if c.AuthorityEpoch < p.MinimumEpoch {
		return Result{}, EpochStale
	}
	if c.LicenseRevision < p.MinimumRevision {
		return Result{}, RevisionStale
	}
	now := p.Now.UTC()
	n := now.UnixMicro()
	if (!p.LastTrustedTime.IsZero() && now.Before(p.LastTrustedTime.Add(-ClockTolerance))) || c.IssuedAt > n+ClockTolerance.Microseconds() {
		return Result{}, ClockSkew
	}
	if c.NotBefore > n+ClockTolerance.Microseconds() {
		return Result{}, NotYetValid
	}
	if n >= c.CommercialExpiresAt {
		return Result{}, CommercialExpired
	}
	if n >= c.OfflineUntil {
		return Result{}, OfflineExpired
	}
	result := Result{State: "VALID_ONLINE", Claims: c, Certificate: cert}
	if n >= c.OnlineUntil {
		if p.RequireOnline {
			return Result{}, OnlineExpired
		}
		result.State = "VALID_OFFLINE"
		result.Warning = OnlineExpired
	}
	return result, nil
}
