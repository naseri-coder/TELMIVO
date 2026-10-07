package media

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

func MuxAV(ctx context.Context, videoPath, audioPath, outputPath string, max int64) (string, error) {
	if max <= 0 || videoPath == "" || audioPath == "" || outputPath == "" {
		return "", Invalid
	}
	videoPath = filepath.Clean(videoPath)
	audioPath = filepath.Clean(audioPath)
	outputPath = filepath.Clean(outputPath)
	dir := filepath.Dir(outputPath)
	if filepath.Dir(videoPath) != dir || filepath.Dir(audioPath) != dir || videoPath == audioPath || outputPath == videoPath || outputPath == audioPath {
		return "", Invalid
	}
	video, err := os.Stat(videoPath)
	if err != nil || !video.Mode().IsRegular() || video.Size() <= 0 {
		return "", Failure("INVALID_MEDIA")
	}
	audio, err := os.Stat(audioPath)
	if err != nil || !audio.Mode().IsRegular() || audio.Size() <= 0 {
		return "", Failure("INVALID_MEDIA")
	}
	if video.Size() > max || audio.Size() > max-video.Size() {
		return "", TooLarge
	}
	partial := outputPath + ".partial"
	_ = os.Remove(partial)
	defer os.Remove(partial)

	cmd := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-nostdin",
		"-v", "error",
		"-protocol_whitelist", "file",
		"-i", videoPath,
		"-i", audioPath,
		"-map", "0:v:0",
		"-map", "1:a:0",
		"-c", "copy",
		"-movflags", "+faststart",
		"-f", "mp4",
		partial,
	)
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err = cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", Failure("PROCESSING_FAILED")
	}
	out, err := os.Stat(partial)
	if err != nil || !out.Mode().IsRegular() || out.Size() <= 0 {
		return "", Failure("PROCESSING_FAILED")
	}
	if out.Size() > max {
		return "", TooLarge
	}
	if err = os.Rename(partial, outputPath); err != nil {
		return "", Failure("PROCESSING_FAILED")
	}
	return outputPath, nil
}
