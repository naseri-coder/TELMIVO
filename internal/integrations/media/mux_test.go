package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMuxAVProducesValidatedTelegramVideo(t *testing.T) {
	src := "../../platform/testfixtures/media/compatible.mp4"
	dir := t.TempDir()
	video := filepath.Join(dir, "video")
	audio := filepath.Join(dir, "audio")
	out := filepath.Join(dir, "asset")

	runFFmpeg := func(args ...string) {
		t.Helper()
		cmd := exec.Command("ffmpeg", args...)
		cmd.Env = []string{"PATH=/usr/bin:/bin"}
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("ffmpeg fixture split failed: %v: %s", err, output)
		}
	}
	runFFmpeg("-nostdin", "-v", "error", "-i", src, "-map", "0:v:0", "-an", "-c", "copy", "-f", "mp4", video)
	runFFmpeg("-nostdin", "-v", "error", "-i", src, "-map", "0:a:0", "-vn", "-c", "copy", "-f", "ipod", audio)

	path, err := MuxAV(context.Background(), video, audio, out, 1<<20)
	if err != nil || path != out {
		t.Fatalf("mux failed: path=%q err=%v", path, err)
	}
	asset, err := Validate(context.Background(), out, 1<<20, Format{Kind: "video", Container: "mp4"})
	if err != nil || asset.Kind != "video" || asset.ContentType != "video/mp4" || asset.Bytes <= 0 {
		t.Fatalf("invalid muxed asset: %#v err=%v", asset, err)
	}

	if _, err = MuxAV(context.Background(), video, audio, filepath.Join(t.TempDir(), "escape"), 1<<20); err == nil {
		t.Fatal("foreign output directory admitted")
	}
	st, err := os.Stat(video)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = MuxAV(context.Background(), video, audio, filepath.Join(dir, "too-large"), st.Size()); err == nil {
		t.Fatal("combined byte cap ignored")
	}
}
