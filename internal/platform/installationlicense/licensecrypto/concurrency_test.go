package licensecrypto

import (
	"bytes"
	"testing"
	"time"
)

func TestLICA9BConcurrentCodecPolicyAndTimeBounds(t *testing.T) {
	id := "00000000-0000-4000-8000-000000000001"
	now := time.Now().UnixMicro()
	cert := Certificate{KeyID: id, CertificateID: id, Product: Product, TokenType: TokenType, Audience: Audience, AuthorityID: id, Environment: "TEST", MaximumLifetime: MaximumLease.Microseconds(), NotBefore: now - 1000000, NotAfter: now + MaximumLease.Microseconds() + 1000000}
	claims := Claims{Version: 2, Algorithm: Algorithm, Product: Product, Type: TokenType, Issuer: id, Audience: Audience, LicenseID: id, ActivationID: id, InstallationID: id, InstallationKeyHash: bytes.Repeat([]byte{0}, 32), Edition: "TEST_ONLY", Capabilities: []string{"product.use"}, LicenseRevision: 1, ActivationGeneration: 1, AuthorityEpoch: 1, IssuedAt: now, NotBefore: now, RefreshAfter: now + int64(time.Hour/time.Microsecond), OnlineUntil: now + int64(24*time.Hour/time.Microsecond), OfflineUntil: now + int64(72*time.Hour/time.Microsecond), CommercialExpiresAt: now + int64(80*time.Hour/time.Microsecond), TokenID: id, KID: id, Environment: "TEST", CertificateID: id, CertificateHash: bytes.Repeat([]byte{0}, 32), ConcurrentPolicy: ConcurrentPolicy, RuntimeSessionID: id, ConcurrencyLeaseID: id, ConcurrencyGeneration: 1, ConflictState: "NORMAL", ConcurrencyIssuedAt: now, ConcurrencyExpiresAt: now + int64(72*time.Hour/time.Microsecond)}
	if e := ValidateClaims(claims, cert); e != nil {
		t.Fatal("valid concurrent evidence", e)
	}
	encoded, e := EncodeClaims(claims)
	if e != nil {
		t.Fatal(e)
	}
	roundtrip, e := DecodeClaims(encoded)
	if e != nil || roundtrip.ConcurrentPolicy != claims.ConcurrentPolicy || roundtrip.ConcurrencyGeneration != 1 || roundtrip.ConcurrencyExpiresAt != claims.ConcurrencyExpiresAt {
		t.Fatal("canonical signed field roundtrip", e)
	}
	for name, change := range map[string]func(*Claims){
		"unknown_policy":       func(c *Claims) { c.ConcurrentPolicy = "UNKNOWN_POLICY" },
		"unsigned_generation":  func(c *Claims) { c.ConcurrencyGeneration = 0 },
		"wrong_session":        func(c *Claims) { c.RuntimeSessionID = "invalid" },
		"extended_offline":     func(c *Claims) { c.ConcurrencyExpiresAt = c.OfflineUntil + 1 },
		"expired_before_issue": func(c *Claims) { c.ConcurrencyExpiresAt = c.ConcurrencyIssuedAt },
		"conflicted_grant":     func(c *Claims) { c.ConflictState = "RECOVERY_REQUIRED" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := claims
			change(&bad)
			if e := ValidateClaims(bad, cert); e == nil {
				t.Fatal("invalid concurrent authority accepted")
			}
		})
	}
	legacy := claims
	legacy.Version = 1
	if _, e := EncodeClaims(legacy); e == nil {
		t.Fatal("legacy payload silently carried concurrent authority")
	}
}
