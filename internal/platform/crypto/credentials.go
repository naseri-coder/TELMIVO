package crypto

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
)

type EncryptedSecret struct {
	KeyID      string
	Ciphertext []byte
}
type CredentialStore interface {
	Seal(context.Context, string, []byte) (EncryptedSecret, error)
	Open(context.Context, string, EncryptedSecret) ([]byte, error)
}
type AESStore struct {
	keyID string
	aead  cipher.AEAD
}

func NewAESStore(keyID string, key []byte) (*AESStore, error) {
	if len(key) != 32 || keyID == "" {
		return nil, errors.New("256-bit key and key ID required")
	}
	b, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	a, e := cipher.NewGCM(b)
	return &AESStore{keyID: keyID, aead: a}, e
}
func (s *AESStore) Seal(_ context.Context, scope string, plain []byte) (EncryptedSecret, error) {
	if scope == "" || len(plain) == 0 {
		return EncryptedSecret{}, errors.New("scope and secret required")
	}
	n := make([]byte, s.aead.NonceSize())
	if _, e := rand.Read(n); e != nil {
		return EncryptedSecret{}, e
	}
	return EncryptedSecret{KeyID: s.keyID, Ciphertext: s.aead.Seal(n, n, plain, []byte(scope))}, nil
}
func (s *AESStore) Open(_ context.Context, scope string, secret EncryptedSecret) ([]byte, error) {
	if secret.KeyID != s.keyID || len(secret.Ciphertext) < s.aead.NonceSize() {
		return nil, errors.New("invalid encrypted secret")
	}
	n := secret.Ciphertext[:s.aead.NonceSize()]
	return s.aead.Open(nil, n, secret.Ciphertext[len(n):], []byte(scope))
}
