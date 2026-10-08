package licensecrypto

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
)

type ConcurrencyRequest struct {
	ProtocolID           string  `json:"protocol_id"`
	LicenseID            string  `json:"license_id"`
	InstallationID       string  `json:"installation_id"`
	InstallationKeyHash  string  `json:"installation_public_key_hash"`
	ActivationID         string  `json:"activation_id"`
	AuthorityEpoch       int64   `json:"authority_epoch"`
	LicenseRevision      int64   `json:"license_revision"`
	ActivationGeneration int64   `json:"activation_generation"`
	RequestedGeneration  int64   `json:"requested_concurrency_generation"`
	RecoveryGeneration   int64   `json:"recovery_generation"`
	PreviousLeaseID      *string `json:"previous_lease_id"`
	RuntimeSessionID     string  `json:"runtime_session_id"`
	RequestID            string  `json:"request_id"`
}

func (r ConcurrencyRequest) Canonical() ([]byte, error) {
	hash, err := base64.RawURLEncoding.Strict().DecodeString(r.InstallationKeyHash)
	if err != nil || len(hash) != 32 || base64.RawURLEncoding.EncodeToString(hash) != r.InstallationKeyHash ||
		r.ProtocolID != ConcurrentPolicy || !ValidID(r.LicenseID) || !ValidID(r.InstallationID) ||
		!ValidID(r.ActivationID) || !ValidID(r.RuntimeSessionID) || !ValidID(r.RequestID) ||
		r.AuthorityEpoch < 1 || r.LicenseRevision < 1 || r.ActivationGeneration < 1 ||
		r.RequestedGeneration < 0 || r.RecoveryGeneration < 0 || (r.RequestedGeneration == 0 && r.PreviousLeaseID != nil) || (r.RequestedGeneration > 0 && (r.PreviousLeaseID == nil || !ValidID(*r.PreviousLeaseID))) {
		return nil, Malformed
	}
	return json.Marshal(r)
}
func (r ConcurrencyRequest) Fingerprint() ([]byte, error) {
	b, e := r.Canonical()
	if e != nil {
		return nil, e
	}
	h := sha256.Sum256(b)
	return h[:], nil
}
func ParseConcurrencyRequest(b []byte) (ConcurrencyRequest, error) {
	var r ConcurrencyRequest
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&r) != nil {
		return r, Malformed
	}
	canonical, e := r.Canonical()
	if e != nil || !bytes.Equal(b, canonical) {
		return r, Malformed
	}
	return r, nil
}
