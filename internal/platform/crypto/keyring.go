package crypto

import (
	"context"
	"errors"
	"regexp"
)

// KeyProvider can be backed by an external KMS without changing ciphertext records.
// Implementations must return sanitized errors and never log key material.
type KeyProvider interface {
	Active(context.Context) (string, error)
	Store(context.Context, string) (CredentialStore, error)
}
type LocalKeyring struct {
	active string
	stores map[string]CredentialStore
}

var keyIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
var errKey = errors.New("credential key unavailable")

func NewKeyring(active string, keys map[string][]byte) (*LocalKeyring, error) {
	k := &LocalKeyring{active: active, stores: map[string]CredentialStore{}}
	if !keyIDPattern.MatchString(active) {
		return nil, errKey
	}
	for id, key := range keys {
		if !keyIDPattern.MatchString(id) {
			return nil, errKey
		}
		s, e := NewAESStore(id, key)
		if e != nil {
			return nil, errKey
		}
		k.stores[id] = s
	}
	if k.stores[active] == nil {
		return nil, errKey
	}
	return k, nil
}
func (k *LocalKeyring) Active(context.Context) (string, error) { return k.active, nil }
func (k *LocalKeyring) Store(_ context.Context, id string) (CredentialStore, error) {
	if !keyIDPattern.MatchString(id) || k.stores[id] == nil {
		return nil, errKey
	}
	return k.stores[id], nil
}

type RotatingStore struct{ Keys KeyProvider }

func (s RotatingStore) Seal(ctx context.Context, scope string, plain []byte) (EncryptedSecret, error) {
	id, e := s.Keys.Active(ctx)
	if e != nil {
		return EncryptedSecret{}, errKey
	}
	k, e := s.Keys.Store(ctx, id)
	if e != nil {
		return EncryptedSecret{}, errKey
	}
	return k.Seal(ctx, scope, plain)
}
func (s RotatingStore) Open(ctx context.Context, scope string, v EncryptedSecret) ([]byte, error) {
	if !keyIDPattern.MatchString(v.KeyID) {
		return nil, errKey
	}
	k, e := s.Keys.Store(ctx, v.KeyID)
	if e != nil {
		return nil, errKey
	}
	p, e := k.Open(ctx, scope, v)
	if e != nil {
		return nil, errors.New("credential decryption denied")
	}
	return p, nil
}

// Reencrypt validates context even when already active and preserves ciphertext on replay.
func (s RotatingStore) Reencrypt(ctx context.Context, scope string, v EncryptedSecret) (EncryptedSecret, error) {
	p, e := s.Open(ctx, scope, v)
	if e != nil {
		return EncryptedSecret{}, e
	}
	defer clear(p)
	id, e := s.Keys.Active(ctx)
	if e != nil {
		return EncryptedSecret{}, errKey
	}
	if id == v.KeyID {
		return v, nil
	}
	return s.Seal(ctx, scope, p)
}
