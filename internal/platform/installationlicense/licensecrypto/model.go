// Package licensecrypto is a standalone, standard-library-only verifier contract.
// It contains no private key storage, database access, or application entitlement logic.
package licensecrypto

import (
	"context"
	"crypto/ed25519"
	"time"
)

const (
	Product           = "TELMIVO"
	Audience          = "TELMIVO_INSTALLATION"
	TokenType         = "TELMIVO_INSTALLATION_LICENSE"
	Algorithm         = "Ed25519"
	CertificateDomain = "TELMIVO_LICENSE_SIGNER_CERT_V1\x00"
	LicenseDomain     = "TELMIVO_INSTALLATION_LICENSE_V1\x00"
	MaximumLease      = 72 * time.Hour
	ClockTolerance    = 5 * time.Minute
	MaximumArtifact   = 65536
)

type Code string

func (c Code) Error() string { return string(c) }

const (
	Malformed            Code = "MALFORMED_TOKEN"
	UnsupportedVersion   Code = "UNSUPPORTED_VERSION"
	UnsupportedAlgorithm Code = "UNSUPPORTED_ALGORITHM"
	InvalidCertificate   Code = "INVALID_SIGNER_CERTIFICATE"
	UnknownRoot          Code = "UNKNOWN_ROOT_KEY"
	UnknownSigner        Code = "UNKNOWN_SIGNING_KEY"
	SignerNotYetValid    Code = "SIGNER_NOT_YET_VALID"
	SignerExpired        Code = "SIGNER_EXPIRED"
	SignerRevoked        Code = "SIGNER_REVOKED"
	InvalidSignature     Code = "INVALID_SIGNATURE"
	WrongProduct         Code = "WRONG_PRODUCT"
	WrongAudience        Code = "WRONG_AUDIENCE"
	WrongType            Code = "WRONG_TOKEN_TYPE"
	InstallationMismatch Code = "INSTALLATION_MISMATCH"
	NotYetValid          Code = "NOT_YET_VALID"
	OnlineExpired        Code = "ONLINE_LEASE_EXPIRED"
	OfflineExpired       Code = "OFFLINE_LEASE_EXPIRED"
	CommercialExpired    Code = "COMMERCIAL_TERM_EXPIRED"
	EpochStale           Code = "AUTHORITY_EPOCH_STALE"
	RevisionStale        Code = "LICENSE_REVISION_STALE"
	ClockSkew            Code = "CLOCK_SKEW_EXCEEDED"
	ScopeMismatch        Code = "AUTHORITY_SCOPE_MISMATCH"
	RootRevoked          Code = "ROOT_REVOKED"
	RootExpired          Code = "ROOT_EXPIRED"
	RootNotYetValid      Code = "ROOT_NOT_YET_VALID"
)

type Status string

const (
	Pending  Status = "PENDING"
	Active   Status = "ACTIVE"
	Retiring Status = "RETIRING"
	Revoked  Status = "REVOKED"
	Expired  Status = "EXPIRED"
)

type Root struct {
	ID                  string
	Version             int64
	PublicKey           ed25519.PublicKey
	NotBefore, NotAfter time.Time
	Status              Status
}
type Roots map[string]Root

// RootCertifier is an offline boundary. No runtime issuer needs its implementation.
type RootCertifier interface {
	Certify(context.Context, string, []byte) ([]byte, error)
}
type Certificate struct {
	Version                                          uint16
	Algorithm, Product, CertificateID, KeyID, RootID string
	RootVersion                                      int64
	PublicKey                                        []byte
	NotBefore, NotAfter                              int64 // UTC Unix microseconds
	TokenType, Audience, AuthorityID, Environment    string
	MaximumLifetime                                  int64 // microseconds
	EpochMin, EpochMax                               int64 // zero/zero means no certificate epoch restriction
}
type CertifiedSigner struct {
	Certificate      Certificate
	Bytes, Signature []byte
}
type Claims struct {
	Version                                                                           uint16
	Algorithm, Product, Type, Issuer, Audience                                        string
	LicenseID, ActivationID, InstallationID                                           string
	InstallationKeyHash                                                               []byte
	Edition                                                                           string
	Capabilities                                                                      []string
	LicenseRevision, ActivationGeneration, AuthorityEpoch                             int64
	IssuedAt, NotBefore, RefreshAfter, OnlineUntil, OfflineUntil, CommercialExpiresAt int64
	TokenID, KID, Environment                                                         string
	CertificateID                                                                     string
	CertificateHash                                                                   []byte
}
type Policy struct {
	Now                                                                       time.Time
	ExpectedProduct, ExpectedAudience, ExpectedAuthority, ExpectedEnvironment string
	InstallationID                                                            string
	InstallationKeyHash                                                       []byte
	MinimumEpoch, MinimumRevision                                             int64
	KnownSigners                                                              map[string]Status // nil: valid root-certified signer sufficient
	RequireOnline                                                             bool
	LastTrustedTime                                                           time.Time // optional local rollback high-water
}
type Result struct {
	State       string
	Warning     Code
	Claims      Claims
	Certificate Certificate
}
