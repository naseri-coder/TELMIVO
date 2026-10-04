package filemanager

import (
	"context"
	gate "github.com/naseri-coder/TELMIVO/internal/plugins/accessgate"
	"testing"
)

type access struct{ calls int }

func (a *access) Authorize(context.Context, string, int64) (gate.Decision, error) {
	a.calls++
	return gate.Decision{Allowed: false, Code: "DENIED"}, nil
}
func TestFilesFailClosed(t *testing.T) {
	f := File{FileID: "synthetic", UniqueID: "unique", Kind: "document", Title: "Test", Enabled: true}
	if !f.Valid() {
		t.Fatal("valid")
	}
	for _, kind := range []string{"document", "audio", "video", "photo"} {
		f.Kind = kind
		if !f.Valid() {
			t.Fatal(kind)
		}
	}
	a := &access{}
	d, _ := Authorize(context.Background(), f, 1, a)
	if !d.Allowed || a.calls != 0 {
		t.Fatal("public")
	}
	f.Policy = "required"
	for i := 0; i < 2; i++ {
		d, _ = Authorize(context.Background(), f, 1, a)
		if d.Allowed {
			t.Fatal("protected bypass")
		}
	}
	if a.calls != 2 {
		t.Fatal("cached access")
	}
	d, _ = Authorize(context.Background(), f, 1, nil)
	if d.Allowed {
		t.Fatal("missing gate")
	}
	f.Enabled = false
	d, _ = Authorize(context.Background(), f, 1, a)
	if d.Allowed {
		t.Fatal("disabled")
	}
}
