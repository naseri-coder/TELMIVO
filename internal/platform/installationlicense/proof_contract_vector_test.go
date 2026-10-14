package installationlicense

import (
    "bytes"
    "crypto/ed25519"
    "crypto/sha256"
    "encoding/hex"
    "testing"
)

// Independent reference vector calculated with Python SHA-256 and RFC8032
// Ed25519 seed 00..1f, NOT copied from the production proof implementation.
// Runs entirely offline and uses no production keys or network.
func TestM2MProofV1IndependentVector(t *testing.T) {
    seed := make([]byte, ed25519.SeedSize)
    for j := range seed { seed[j] = byte(j) }
    privateKey := ed25519.NewKeyFromSeed(seed)
    identity := Identity{
        ID: "22222222-2222-4222-8222-222222222222",
        PrivateKey: privateKey,
    }
    method := "POST"
    route := "/v1/licenses/activate"
    query := ""
    timestamp := "1728936000000"
    nonce := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
    key := "11111111-1111-4111-8111-111111111111"
    body := []byte(`{"authority_epoch":1}`)

    got := proof(identity, method, route, query, timestamp, nonce, key, body)
    if len(got) != 285 {
        t.Fatalf("v1 proof byte length: got %d, want 285", len(got))
    }
    sum := sha256.Sum256(got)
    expected := "9236f56b627a7e6928ec6c70315e7648fdfe5dc8d5dc3e8e0ce7dce533cb31ef"
    if hex.EncodeToString(sum[:]) != expected {
        t.Fatalf("v1 proof changed: got %x, want %s", sum, expected)
    }
    sig := ed25519.Sign(privateKey, got)
    if !ed25519.Verify(privateKey.Public().(ed25519.PublicKey), got, sig) {
        t.Fatal("local Ed25519 signature failed to verify")
    }
    modified := append([]byte(nil), got...)
    modified[len(modified)-1] ^= 1
    if bytes.Equal(got, modified) || ed25519.Verify(privateKey.Public().(ed25519.PublicKey), modified, sig) {
        t.Fatal("proof tamper did not invalidate signature")
    }
}
