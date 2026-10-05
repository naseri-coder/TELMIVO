package media

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRemuxSafety(t *testing.T) {
	data, e := os.ReadFile("../../platform/testfixtures/media/remux.aac")
	if e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "input")
	if e = os.WriteFile(input, data, 0600); e != nil {
		t.Fatal(e)
	}
	f := Format{Kind: "audio", Container: "m4a", SourceContainer: "aac", Processing: true}
	out := filepath.Join(dir, "output")
	if _, e = Validate(context.Background(), input, 1<<20, f); e != nil {
		t.Fatal(e)
	}
	if _, e = Remux(context.Background(), input, out, 1<<20, f); e != nil {
		t.Fatal(e)
	}
	f.Processing = false
	f.SourceContainer = ""
	if _, e = Validate(context.Background(), out, 1<<20, f); e != nil {
		t.Fatal(e)
	}
	f.Processing = true
	f.SourceContainer = "aac"
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	partial := filepath.Join(dir, "cancelled")
	if _, e = Remux(cancelled, input, partial, 1<<20, f); e == nil {
		t.Fatal("cancel ignored")
	}
	if _, e = os.Stat(partial); !os.IsNotExist(e) {
		t.Fatal("partial output retained")
	}
	if _, e = Remux(context.Background(), input, filepath.Join(t.TempDir(), "escape"), 1<<20, f); e == nil {
		t.Fatal("foreign output admitted")
	}
	if _, e = Remux(context.Background(), input, filepath.Join(dir, "too-large"), 1221, f); e == nil {
		t.Fatal("output cap ignored")
	}
	bad := filepath.Join(dir, "bad")
	os.WriteFile(bad, []byte("not media; $(touch injected)"), 0600)
	if _, e = Remux(context.Background(), bad, filepath.Join(dir, "broken"), 1<<20, f); e == nil {
		t.Fatal("invalid input succeeded")
	}
	if _, e = os.Stat(filepath.Join(dir, "broken")); !os.IsNotExist(e) {
		t.Fatal("broken output retained")
	}
}
