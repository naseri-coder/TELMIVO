package media

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Asset struct {
	Path, Name, Kind, ContentType string
	Bytes                         int64
}

func Validate(ctx context.Context, path string, max int64, expected Format) (Asset, error) {
	a := Asset{Path: path, Name: "media." + expected.Container, Kind: expected.Kind}
	st, e := os.Stat(path)
	if e != nil || !st.Mode().IsRegular() || st.Size() <= 0 || st.Size() > max {
		return a, TooLarge
	}
	a.Bytes = st.Size()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-protocol_whitelist", "file", "-show_entries", "format=format_name,duration:stream=codec_type,codec_name", "-of", "json", path)
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	out := &boundedBuffer{max: 32 << 10}
	cmd.Stdout = out
	cmd.Stderr = io.Discard
	if cmd.Run() != nil {
		return a, Failure("INVALID_MEDIA")
	}
	var info struct {
		Format struct {
			Name string `json:"format_name"`
		}
		Streams []struct {
			Type  string `json:"codec_type"`
			Codec string `json:"codec_name"`
		}
	}
	if json.Unmarshal(out.b, &info) != nil || len(info.Streams) == 0 || len(info.Streams) > 4 {
		return a, Failure("INVALID_MEDIA")
	}
	video, audio := false, false
	for _, s := range info.Streams {
		if s.Type == "video" {
			video = true
			if s.Codec != "h264" {
				return a, Unavailable
			}
		} else if s.Type == "audio" {
			audio = true
			if s.Codec != "aac" && s.Codec != "mp3" {
				return a, Unavailable
			}
		} else {
			return a, Failure("INVALID_MEDIA")
		}
	}
	if !audio || expected.Kind == "video" && !video || expected.Kind == "audio" && video {
		return a, Failure("INVALID_MEDIA")
	}
	if expected.Processing && expected.SourceContainer == "aac" {
		if info.Format.Name != "aac" || video {
			return a, Failure("INVALID_MEDIA")
		}
		a.ContentType = "audio/aac"
	} else if expected.Container == "mp3" {
		if !strings.Contains(info.Format.Name, "mp3") {
			return a, Failure("INVALID_MEDIA")
		}
		a.ContentType = "audio/mpeg"
	} else {
		if !strings.Contains(info.Format.Name, "mp4") {
			return a, Failure("INVALID_MEDIA")
		}
		a.ContentType = "video/mp4"
		if expected.Kind == "audio" {
			a.ContentType = "audio/mp4"
		}
	}
	return a, nil
}

type DeliveryResult struct {
	Outcome   string
	MessageID int64
	Retry     bool
}
type Delivery interface {
	DeliverMedia(context.Context, int64, Asset) (DeliveryResult, error)
}
