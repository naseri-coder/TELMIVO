package licensecrypto

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"io"
	"sort"
	"strings"
	"unicode/utf8"
)

// V1 is positional binary, NOT JSON/TLV. A field has exactly one position;
// duplicate fields cannot be represented. Appended/repeated fields are rejected.
type writer struct {
	bytes.Buffer
	err error
}

func (w *writer) u16(v uint16) { var b [2]byte; binary.BigEndian.PutUint16(b[:], v); w.Write(b[:]) }
func (w *writer) i64(v int64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(v))
	w.Write(b[:])
}
func (w *writer) blob(v []byte) {
	if len(v) > MaximumArtifact {
		w.err = Malformed
		return
	}
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(len(v)))
	w.Write(b[:])
	w.Write(v)
}
func (w *writer) str(s string) {
	if len(s) > 256 || !utf8.ValidString(s) {
		w.err = Malformed
		return
	}
	w.blob([]byte(s))
}
func (w *writer) finish() ([]byte, error) {
	if w.err != nil || w.Len() > MaximumArtifact {
		return nil, Malformed
	}
	return append([]byte(nil), w.Bytes()...), nil
}

type reader struct {
	*bytes.Reader
	err error
}

func read(b []byte) *reader {
	r := &reader{Reader: bytes.NewReader(b)}
	if len(b) > MaximumArtifact {
		r.err = Malformed
	}
	return r
}
func (r *reader) take(n int) []byte {
	if r.err != nil || n < 0 || n > r.Len() {
		r.err = Malformed
		return nil
	}
	b := make([]byte, n)
	_, r.err = io.ReadFull(r, b)
	return b
}
func (r *reader) u16() uint16 {
	b := r.take(2)
	if len(b) != 2 {
		return 0
	}
	return binary.BigEndian.Uint16(b)
}
func (r *reader) i64() int64 {
	b := r.take(8)
	if len(b) != 8 {
		return 0
	}
	return int64(binary.BigEndian.Uint64(b))
}
func (r *reader) blob() []byte {
	b := r.take(4)
	if len(b) != 4 {
		return nil
	}
	n := binary.BigEndian.Uint32(b)
	if n > MaximumArtifact {
		r.err = Malformed
		return nil
	}
	return r.take(int(n))
}
func (r *reader) str() string {
	b := r.blob()
	if len(b) > 256 || !utf8.Valid(b) {
		r.err = Malformed
	}
	return string(b)
}
func (r *reader) header(magic string) (uint16, string) {
	if string(r.take(4)) != magic {
		r.err = Malformed
	}
	return r.u16(), r.str()
}
func (r *reader) done() error {
	if r.err != nil || r.Len() != 0 {
		return Malformed
	}
	return nil
}
func ValidID(s string) bool {
	if len(s) != 36 || s != strings.ToLower(s) || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return false
	}
	b, e := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	return e == nil && b[6]>>4 == 4 && b[8]>>6 == 2
}
func EncodeCertificate(c Certificate) ([]byte, error) {
	w := &writer{}
	w.WriteString("TSC1")
	w.u16(c.Version)
	w.str(c.Algorithm)
	for _, s := range []string{c.Product, c.CertificateID, c.KeyID, c.RootID} {
		w.str(s)
	}
	w.i64(c.RootVersion)
	w.blob(c.PublicKey)
	w.i64(c.NotBefore)
	w.i64(c.NotAfter)
	for _, s := range []string{c.TokenType, c.Audience, c.AuthorityID, c.Environment} {
		w.str(s)
	}
	w.i64(c.MaximumLifetime)
	w.i64(c.EpochMin)
	w.i64(c.EpochMax)
	return w.finish()
}
func DecodeCertificate(b []byte) (Certificate, error) {
	r := read(b)
	c := Certificate{}
	c.Version, c.Algorithm = r.header("TSC1")
	c.Product = r.str()
	c.CertificateID = r.str()
	c.KeyID = r.str()
	c.RootID = r.str()
	c.RootVersion = r.i64()
	c.PublicKey = r.blob()
	c.NotBefore = r.i64()
	c.NotAfter = r.i64()
	c.TokenType = r.str()
	c.Audience = r.str()
	c.AuthorityID = r.str()
	c.Environment = r.str()
	c.MaximumLifetime = r.i64()
	c.EpochMin = r.i64()
	c.EpochMax = r.i64()
	if err := r.done(); err != nil {
		return Certificate{}, InvalidCertificate
	}
	canonical, err := EncodeCertificate(c)
	if err != nil || !bytes.Equal(b, canonical) {
		return Certificate{}, InvalidCertificate
	}
	return c, nil
}
func CertificateSigningInput(payload []byte) []byte {
	return append([]byte(CertificateDomain), payload...)
}
func LicenseSigningInput(payload []byte) []byte { return append([]byte(LicenseDomain), payload...) }
func EncodeClaims(c Claims) ([]byte, error) {
	w := &writer{}
	w.WriteString("TLP1")
	w.u16(c.Version)
	w.str(c.Algorithm)
	for _, s := range []string{c.Product, c.Type, c.Issuer, c.Audience, c.LicenseID, c.ActivationID, c.InstallationID} {
		w.str(s)
	}
	w.blob(c.InstallationKeyHash)
	w.str(c.Edition)
	if len(c.Capabilities) > 128 {
		return nil, Malformed
	}
	w.u16(uint16(len(c.Capabilities)))
	for i, s := range c.Capabilities {
		if i > 0 && c.Capabilities[i-1] >= s {
			return nil, Malformed
		}
		w.str(s)
	}
	for _, n := range []int64{c.LicenseRevision, c.ActivationGeneration, c.AuthorityEpoch, c.IssuedAt, c.NotBefore, c.RefreshAfter, c.OnlineUntil, c.OfflineUntil, c.CommercialExpiresAt} {
		w.i64(n)
	}
	w.str(c.TokenID)
	w.str(c.KID)
	w.str(c.Environment)
	w.str(c.CertificateID)
	w.blob(c.CertificateHash)
	return w.finish()
}
func DecodeClaims(b []byte) (Claims, error) {
	r := read(b)
	c := Claims{}
	c.Version, c.Algorithm = r.header("TLP1")
	c.Product = r.str()
	c.Type = r.str()
	c.Issuer = r.str()
	c.Audience = r.str()
	c.LicenseID = r.str()
	c.ActivationID = r.str()
	c.InstallationID = r.str()
	c.InstallationKeyHash = r.blob()
	c.Edition = r.str()
	n := r.u16()
	if n > 128 {
		return Claims{}, Malformed
	}
	for i := 0; i < int(n); i++ {
		c.Capabilities = append(c.Capabilities, r.str())
	}
	c.LicenseRevision = r.i64()
	c.ActivationGeneration = r.i64()
	c.AuthorityEpoch = r.i64()
	c.IssuedAt = r.i64()
	c.NotBefore = r.i64()
	c.RefreshAfter = r.i64()
	c.OnlineUntil = r.i64()
	c.OfflineUntil = r.i64()
	c.CommercialExpiresAt = r.i64()
	c.TokenID = r.str()
	c.KID = r.str()
	c.Environment = r.str()
	c.CertificateID = r.str()
	c.CertificateHash = r.blob()
	if err := r.done(); err != nil {
		return Claims{}, err
	}
	canonical, err := EncodeClaims(c)
	if err != nil || !bytes.Equal(b, canonical) {
		return Claims{}, Malformed
	}
	return c, nil
}
func SortedCapabilities(caps []string) ([]string, error) {
	c := append([]string(nil), caps...)
	sort.Strings(c)
	for i, s := range c {
		if s == "" || (i > 0 && c[i-1] == s) {
			return nil, Malformed
		}
	}
	return c, nil
}

// Envelope: magic/version/algorithm, certificate payload/root signature,
// license payload/delegated signature. All blobs use uint32 big-endian lengths.
type Envelope struct {
	Version                                        uint16
	Algorithm                                      string
	Certificate, RootSignature, Payload, Signature []byte
}

func EncodeEnvelope(e Envelope) ([]byte, error) {
	if len(e.RootSignature) != 64 || len(e.Signature) != 64 {
		return nil, Malformed
	}
	w := &writer{}
	w.WriteString("TLE1")
	w.u16(e.Version)
	w.str(e.Algorithm)
	for _, b := range [][]byte{e.Certificate, e.RootSignature, e.Payload, e.Signature} {
		w.blob(b)
	}
	return w.finish()
}
func DecodeEnvelope(b []byte) (Envelope, error) {
	r := read(b)
	e := Envelope{}
	e.Version, e.Algorithm = r.header("TLE1")
	e.Certificate = r.blob()
	e.RootSignature = r.blob()
	e.Payload = r.blob()
	e.Signature = r.blob()
	if r.done() != nil || len(e.RootSignature) != 64 || len(e.Signature) != 64 {
		return Envelope{}, Malformed
	}
	return e, nil
}
func PackCertificate(payload, signature []byte) ([]byte, error) {
	if len(signature) != 64 {
		return nil, InvalidCertificate
	}
	w := &writer{}
	w.WriteString("TCB1")
	w.blob(payload)
	w.blob(signature)
	return w.finish()
}
func UnpackCertificate(b []byte) ([]byte, []byte, error) {
	r := read(b)
	if string(r.take(4)) != "TCB1" {
		return nil, nil, InvalidCertificate
	}
	p, s := r.blob(), r.blob()
	if r.done() != nil || len(s) != 64 {
		return nil, nil, InvalidCertificate
	}
	return p, s, nil
}
