package ffmpeg

import (
	"context"
	"errors"
	"github.com/naseri-coder/TELMIVO/internal/platform/installationlicense"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type Runner struct {
	slots   chan struct{}
	Timeout time.Duration
	Root    string
}

func New(root string, concurrency int, timeout time.Duration) (*Runner, error) {
	if concurrency < 1 || timeout <= 0 || !filepath.IsAbs(root) {
		return nil, errors.New("invalid media limits")
	}
	if e := os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	return &Runner{slots: make(chan struct{}, concurrency), Timeout: timeout, Root: root}, nil
}
func (r *Runner) Process(ctx context.Context, input string) (string, error) {
	select {
	case r.slots <- struct{}{}:
		defer func() { <-r.slots }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	// Inputs are worker-owned files, never arbitrary URL or user-provided command arguments.
	real, e := filepath.EvalSymlinks(input)
	if e != nil {
		return "", e
	}
	rel, e := filepath.Rel(r.Root, real)
	if e != nil || !filepath.IsLocal(rel) {
		return "", errors.New("input outside temporary root")
	}
	dir, e := os.MkdirTemp(r.Root, "media-")
	if e != nil {
		return "", e
	}
	defer os.RemoveAll(dir)
	output := filepath.Join(dir, "result.mp4")
	limited, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	cmd := exec.CommandContext(limited, "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-protocol_whitelist", "file,pipe", "-i", real, "-map_metadata", "-1", "-c", "copy", "-fs", "104857600", output)
	if e = installationlicense.RuntimeEffect(ctx, cmd.Run); e != nil {
		return "", errors.New("media processing failed")
	}
	// Caller must ingest the file before returning; persistence belongs to the storage adapter.
	return "", errors.New("storage ingestion required; temporary output was cleaned")
}
