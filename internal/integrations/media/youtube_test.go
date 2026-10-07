package media

import (
	"strings"
	"testing"
)

func TestYouTubeFormatPlanning(t *testing.T) {
	raw := []rawFormat{
		{ID: "audio-low-a", Ext: "m4a", Protocol: "https", URL: "https://r1.googlevideo.com/a1", VCodec: "none", ACodec: "mp4a.40.5", Bytes: 220000},
		{ID: "audio-low-b", Ext: "m4a", Protocol: "https", URL: "https://r1.googlevideo.com/a2", VCodec: "none", ACodec: "mp4a.40.5", Bytes: 220000},
		{ID: "audio-high", Ext: "m4a", Protocol: "https", URL: "https://r1.googlevideo.com/a3", VCodec: "none", ACodec: "mp4a.40.2", Bytes: 580000},
		{ID: "progressive-360", Ext: "mp4", Protocol: "https", URL: "https://r1.googlevideo.com/p360", VCodec: "avc1.42001E", ACodec: "mp4a.40.2", Bytes: 3000000, Height: 360},
		{ID: "video-720", Ext: "mp4", Protocol: "https", URL: "https://r1.googlevideo.com/v720", VCodec: "avc1.4d401f", ACodec: "none", Bytes: 8000000, Height: 720},
		{ID: "video-1080", Ext: "mp4", Protocol: "https", URL: "https://r1.googlevideo.com/v1080", VCodec: "avc1.640028", ACodec: "none", ApproxBytes: 15000000, Height: 1080},
		{ID: "vp9-1080", Ext: "webm", Protocol: "https", URL: "https://r1.googlevideo.com/vp9", VCodec: "vp9", ACodec: "none", Bytes: 12000000, Height: 1080},
	}

	formats := youtubeFormats(raw, 8)
	if len(formats) != 5 {
		t.Fatalf("expected 3 video + 2 audio formats, got %d: %#v", len(formats), formats)
	}
	seenID := map[string]bool{}
	seenLabel := map[string]bool{}
	videoCount, audioCount, muxCount := 0, 0, 0
	for _, f := range formats {
		if seenID[f.ID] {
			t.Fatalf("duplicate format id: %s", f.ID)
		}
		seenID[f.ID] = true
		if seenLabel[f.Label] {
			t.Fatalf("duplicate visible label: %s", f.Label)
		}
		seenLabel[f.Label] = true
		switch f.Kind {
		case "video":
			videoCount++
			if !strings.Contains(f.Label, "ویدیو") || !strings.Contains(f.Label, "MP4") {
				t.Fatalf("bad video label: %q", f.Label)
			}
			if f.ProcessingPlan == "mux-av" {
				muxCount++
				if f.VideoSourceID == "" || f.AudioSourceID == "" || !f.Processing {
					t.Fatalf("incomplete composite plan: %#v", f)
				}
			}
		case "audio":
			audioCount++
			if !strings.Contains(f.Label, "صوت") || !strings.Contains(f.Label, "M4A") {
				t.Fatalf("bad audio label: %q", f.Label)
			}
		default:
			t.Fatalf("unexpected kind: %q", f.Kind)
		}
	}
	if videoCount != 3 || audioCount != 2 || muxCount != 2 {
		t.Fatalf("videos=%d audio=%d mux=%d", videoCount, audioCount, muxCount)
	}
	if !strings.Contains(formats[0].Label, "1080p") || !strings.Contains(formats[1].Label, "720p") {
		t.Fatalf("video quality order not descending: %#v", formats)
	}

	compact := youtubeFormats(raw, 2)
	if len(compact) != 2 || compact[0].Kind != "video" || compact[1].Kind != "audio" {
		t.Fatalf("small format budget must preserve video + audio choice: %#v", compact)
	}
}

func TestCompositeDescriptorStableAndTamperResistant(t *testing.T) {
	v := rawFormat{ID: "137", Ext: "mp4", Protocol: "https", URL: "https://r1.googlevideo.com/v", VCodec: "avc1.640028", ACodec: "none", Bytes: 1000, Height: 1080}
	a := rawFormat{ID: "140", Ext: "m4a", Protocol: "https", URL: "https://r1.googlevideo.com/a", VCodec: "none", ACodec: "mp4a.40.2", Bytes: 100}
	one, ok := youtubePairDescriptor(v, a)
	if !ok {
		t.Fatal("valid pair rejected")
	}
	two, ok := youtubePairDescriptor(v, a)
	if !ok || one.ID != two.ID {
		t.Fatal("composite descriptor is not stable")
	}
	a.ID = "different"
	three, ok := youtubePairDescriptor(v, a)
	if !ok || three.ID == one.ID {
		t.Fatal("component identity was not bound into public format id")
	}
}
