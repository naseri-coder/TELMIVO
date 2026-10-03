package media

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// Remux is the one admitted processing plan: validated ADTS AAC -> M4A,
// stream copy. No provider flags, raw URLs, title or client arguments enter argv.
func Remux(ctx context.Context, input, output string, max int64, format Format) (result string, err error) {
	if !format.Processing || format.SourceContainer != "aac" || format.Container != "m4a" || format.Kind != "audio" || max <= 0 || filepath.Dir(input) != filepath.Dir(output) || input == output {
		return "", Unavailable
	}
	st, e := os.Lstat(input)
	if e != nil || !st.Mode().IsRegular() || st.Size() > max {
		return "", TooLarge
	}
	f, e := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return "", Failure("PROCESSING_FAILED")
	}
	f.Close()
	defer func() {
		if err != nil {
			os.Remove(output)
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-protocol_whitelist", "file", "-i", input, "-map", "0:a:0", "-map_metadata", "-1", "-c", "copy", "-threads", "2", "-movflags", "+faststart", "-fs", strconv.FormatInt(max+1, 10), "-f", "mp4", "-y", output)
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if cmd.Run() != nil {
		if ctx.Err() != nil {
			return "", Failure("PROCESSING_TIMEOUT")
		}
		return "", Failure("PROCESSING_FAILED")
	}
	st, e = os.Lstat(output)
	if e != nil || !st.Mode().IsRegular() || st.Size() <= 0 || st.Size() > max {
		return "", TooLarge
	}
	return output, nil
}
