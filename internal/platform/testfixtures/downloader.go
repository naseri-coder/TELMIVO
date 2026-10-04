package testfixtures

import (
	"context"
	"embed"
	"github.com/naseri-coder/TELMIVO/internal/integrations/media"
	"io"
)

//go:embed media/compatible.mp4 media/remux.aac
var downloaderData embed.FS

type DownloaderProvider struct{}

func (DownloaderProvider) Analyze(ctx context.Context, s media.Source) (media.Analysis, error) {
	if _, e := media.Detect(s.URL); e != nil {
		return media.Analysis{}, e
	}
	if s.Reference == "restricted" {
		return media.Analysis{}, media.Restricted
	}
	return media.Analysis{Title: "رسانه عمومی آزمایشی", Formats: []media.Format{{ID: "fixture-compatible", Label: "ویدیو MP4", Kind: "video", Container: "mp4", Codec: "h264/aac", Bytes: 2553}, {ID: "fixture-remux", Label: "صوت M4A", Kind: "audio", Container: "m4a", SourceContainer: "aac", Processing: true, Codec: "aac", Bytes: 1221}}}, nil
}
func (DownloaderProvider) Fetch(ctx context.Context, s media.Source, f media.Format, w io.Writer, max int64) error {
	if f.ID != "fixture-compatible" && f.ID != "fixture-remux" {
		return media.Unavailable
	}
	name := "media/compatible.mp4"
	if f.ID == "fixture-remux" {
		name = "media/remux.aac"
	}
	r, e := downloaderData.Open(name)
	if e != nil {
		return e
	}
	defer r.Close()
	_, e = media.CopyBounded(ctx, w, r, max)
	return e
}

type DownloaderDelivery struct{}

func (DownloaderDelivery) DeliverMedia(ctx context.Context, chat int64, a media.Asset) (media.DeliveryResult, error) {
	if chat <= 0 {
		return media.DeliveryResult{}, media.Invalid
	}
	return media.DeliveryResult{Outcome: "DELIVERY_ACKNOWLEDGED", MessageID: 1}, nil
}
