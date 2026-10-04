package crypto

import (
	"bytes"
	"context"
	"testing"
)

func TestRotation(t *testing.T) {
	ctx := context.Background()
	scope := "workspace-a:installation-1"
	old, e := NewKeyring("v1", map[string][]byte{"v1": bytes.Repeat([]byte{1}, 32)})
	if e != nil {
		t.Fatal(e)
	}
	v, e := (RotatingStore{old}).Seal(ctx, scope, []byte("synthetic-credential"))
	if e != nil {
		t.Fatal(e)
	}
	keys, e := NewKeyring("v2", map[string][]byte{"v1": bytes.Repeat([]byte{1}, 32), "v2": bytes.Repeat([]byte{2}, 32)})
	if e != nil {
		t.Fatal(e)
	}
	s := RotatingStore{keys}
	if _, e = s.Open(ctx, scope, v); e != nil {
		t.Fatal(e)
	}
	n, e := s.Reencrypt(ctx, scope, v)
	if e != nil || n.KeyID != "v2" {
		t.Fatal(e)
	}
	again, e := s.Reencrypt(ctx, scope, n)
	if e != nil || !bytes.Equal(n.Ciphertext, again.Ciphertext) {
		t.Fatal("rotation replay", e)
	}
	if _, e = s.Open(ctx, "workspace-b:installation-1", n); e == nil {
		t.Fatal("context escaped")
	}
	for _, id := range []string{"unknown", "../v2", ""} {
		bad := v
		bad.KeyID = id
		if _, e = s.Open(ctx, scope, bad); e == nil {
			t.Fatal("unknown key accepted")
		}
	}
	fresh, e := s.Seal(ctx, scope, []byte("new-synthetic"))
	if e != nil || fresh.KeyID != "v2" {
		t.Fatal(e)
	}
}
