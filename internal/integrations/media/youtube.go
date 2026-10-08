package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/naseri-coder/TELMIVO/internal/platform/installationlicense"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

func (f rawFormat) effectiveBytes() int64 {
	if f.Bytes > 0 {
		return f.Bytes
	}
	if f.ApproxBytes > 0 {
		return f.ApproxBytes
	}
	return 0
}

func displaySize(n int64) string {
	if n <= 0 {
		return ""
	}
	if n < 1<<20 {
		return fmt.Sprintf(" • %.0f KB", float64(n)/1024)
	}
	return fmt.Sprintf(" • %.1f MB", float64(n)/(1<<20))
}

func formatLabel(kind, container string, height int, bytes int64) string {
	container = strings.ToUpper(container)
	if kind == "video" {
		if height > 0 {
			return fmt.Sprintf("ویدیو %dp %s%s", height, container, displaySize(bytes))
		}
		return fmt.Sprintf("ویدیو %s%s", container, displaySize(bytes))
	}
	return fmt.Sprintf("صوت %s%s", container, displaySize(bytes))
}

func youtubeAudioOnly(f rawFormat) bool {
	if f.DRM || f.ID == "" || len(f.ID) > 100 || f.Protocol != "https" || f.VCodec != "none" || f.ACodec == "none" {
		return false
	}
	if f.Ext != "m4a" && f.Ext != "mp4" {
		return false
	}
	return strings.HasPrefix(strings.ToLower(f.ACodec), "mp4a")
}

func youtubeVideoOnly(f rawFormat) bool {
	if f.DRM || f.ID == "" || len(f.ID) > 100 || f.Protocol != "https" || f.Ext != "mp4" || f.ACodec != "none" || f.Height <= 0 {
		return false
	}
	return strings.HasPrefix(strings.ToLower(f.VCodec), "avc1")
}

func youtubePairDescriptor(video, audio rawFormat) (Format, bool) {
	if !youtubeVideoOnly(video) || !youtubeAudioOnly(audio) {
		return Format{}, false
	}
	total := int64(0)
	if video.effectiveBytes() > 0 && audio.effectiveBytes() > 0 {
		total = video.effectiveBytes() + audio.effectiveBytes()
	}
	sum := sha256.Sum256([]byte("mux-av\x00" + video.ID + "\x00" + audio.ID + "\x00" + video.VCodec + "\x00" + audio.ACodec))
	return Format{
		VideoSourceID:  video.ID,
		AudioSourceID:  audio.ID,
		ProcessingPlan: "mux-av",
		ID:             hex.EncodeToString(sum[:12]),
		Label:          formatLabel("video", "mp4", video.Height, total),
		Kind:           "video",
		Container:      "mp4",
		Codec:          "h264/aac",
		Bytes:          total,
		Processing:     true,
	}, true
}

func youtubeFormats(raw []rawFormat, max int) []Format {
	if max < 1 {
		return nil
	}
	audioSeen := map[string]bool{}
	audios := make([]rawFormat, 0, 4)
	for _, r := range raw {
		if !youtubeAudioOnly(r) || NetworkURL("youtube", r.URL) != nil {
			continue
		}
		key := r.Ext + "|" + r.ACodec + "|" + fmt.Sprint(r.effectiveBytes())
		if audioSeen[key] {
			continue
		}
		audioSeen[key] = true
		audios = append(audios, r)
	}
	sort.SliceStable(audios, func(i, j int) bool {
		return audios[i].effectiveBytes() > audios[j].effectiveBytes()
	})

	type videoChoice struct {
		format Format
		direct bool
		bytes  int64
	}
	choices := map[int]videoChoice{}

	for _, r := range raw {
		if NetworkURL("youtube", r.URL) != nil || r.Height <= 0 {
			continue
		}
		if f, ok := descriptor(r); ok && f.Kind == "video" {
			cur, exists := choices[r.Height]
			if !exists || !cur.direct || f.Bytes > cur.bytes {
				choices[r.Height] = videoChoice{format: f, direct: true, bytes: f.Bytes}
			}
		}
	}

	if len(audios) > 0 {
		bestAudio := audios[0]
		for _, r := range raw {
			if !youtubeVideoOnly(r) || NetworkURL("youtube", r.URL) != nil {
				continue
			}
			f, ok := youtubePairDescriptor(r, bestAudio)
			if !ok {
				continue
			}
			cur, exists := choices[r.Height]
			if !exists || (!cur.direct && f.Bytes > cur.bytes) {
				choices[r.Height] = videoChoice{format: f, bytes: f.Bytes}
			}
		}
	}

	heights := make([]int, 0, len(choices))
	for height := range choices {
		heights = append(heights, height)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(heights)))

	audioSlots := 0
	if len(audios) > 0 {
		if len(heights) == 0 {
			audioSlots = max
			if audioSlots > 2 {
				audioSlots = 2
			}
		} else if max > 1 {
			audioSlots = 1
			if max >= 6 && len(audios) > 1 {
				audioSlots = 2
			}
		}
	}
	videoLimit := max - audioSlots
	out := make([]Format, 0, max)
	for _, height := range heights {
		if len(out) >= videoLimit {
			break
		}
		out = append(out, choices[height].format)
	}

	for _, r := range audios {
		if len(out) >= max || audioSlots == 0 {
			break
		}
		f, ok := descriptor(r)
		if !ok {
			continue
		}
		out = append(out, f)
		audioSlots--
	}
	return out
}

func fetchRaw(ctx context.Context, provider string, f rawFormat, w io.Writer, max int64) (int64, error) {
	if max <= 0 || NetworkURL(provider, f.URL) != nil {
		return 0, Restricted
	}
	client := Client(provider, time.Minute, 3)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return 0, Restricted
	}
	resp, err := installationlicense.HTTP(ctx, client, req)
	if err != nil {
		return 0, Failure("DOWNLOAD_FAILED")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return 0, Restricted
	}
	if resp.StatusCode != 200 {
		return 0, Failure("DOWNLOAD_FAILED")
	}
	return CopyBounded(ctx, w, resp.Body, max)
}

func (x Extractor) FetchComposite(ctx context.Context, s Source, selected Format, videoOut, audioOut io.Writer, max int64) error {
	if selected.ProcessingPlan != "mux-av" || selected.VideoSourceID == "" || selected.AudioSourceID == "" || max <= 0 {
		return Invalid
	}
	info, err := x.info(ctx, s)
	if err != nil {
		return err
	}
	var video, audio rawFormat
	for _, f := range info.Formats {
		if f.ID == selected.VideoSourceID {
			video = f
		}
		if f.ID == selected.AudioSourceID {
			audio = f
		}
	}
	planned, ok := youtubePairDescriptor(video, audio)
	if !ok || planned.ID != selected.ID {
		return Unavailable
	}
	n, err := fetchRaw(ctx, s.Provider, video, videoOut, max)
	if err != nil {
		return err
	}
	if n >= max {
		return TooLarge
	}
	_, err = fetchRaw(ctx, s.Provider, audio, audioOut, max-n)
	return err
}
