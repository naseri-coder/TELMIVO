package storage

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestWorkspaceStorageLifecycle(t *testing.T) {
	ctx := context.Background()
	a := "11111111-1111-1111-1111-111111111111"
	b := "22222222-2222-2222-2222-222222222222"
	p, e := NewLocal(t.TempDir(), 100)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	s := Scoped{p}
	key, e := ObjectKey(a, "fixture")
	if e != nil {
		t.Fatal(e)
	}
	same, _ := ObjectKey(a, "fixture")
	if same != key {
		t.Fatal("nondeterministic")
	}
	if e = s.Put(ctx, a, key, strings.NewReader("test"), Metadata{Size: 4, ContentType: "text/plain"}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Open(ctx, b, key); e == nil {
		t.Fatal("cross workspace read")
	}
	if e = s.Delete(ctx, b, key); e == nil {
		t.Fatal("cross workspace delete")
	}
	r, e := s.Open(ctx, a, key)
	if e != nil {
		t.Fatal(e)
	}
	data, e := io.ReadAll(r)
	r.Close()
	if e != nil || string(data) != "test" {
		t.Fatal(e)
	}
	m, e := s.Metadata(ctx, a, key)
	if e != nil || m.Size != 4 || m.ContentType != "text/plain" {
		t.Fatal(e)
	}
	if e = s.Put(ctx, a, key, strings.NewReader("expired"), Metadata{Size: 7, ExpiresAt: time.Now().Add(-time.Second)}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Open(ctx, a, key); e == nil {
		t.Fatal("expired accessible")
	}
	if e = p.Cleanup(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = p.Metadata(ctx, key); e == nil {
		t.Fatal("cleanup failed")
	}
	if _, e = ObjectKey("../outside", "fixture"); e == nil {
		t.Fatal("path escape")
	}
}
