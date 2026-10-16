package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/naseri-coder/TELMIVO/internal/integrations/media"
	"github.com/naseri-coder/TELMIVO/internal/platform/installationlicense"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"time"
)

type MediaClient struct {
	transport http.RoundTripper
	Token     string
	MaxBytes  int64
	Timeout   time.Duration
}

func (c MediaClient) DeliverMedia(ctx context.Context, chat int64, a media.Asset) (media.DeliveryResult, error) {
	bad := media.DeliveryResult{Outcome: "DELIVERY_FAILED"}
	if chat <= 0 || c.Token == "" || c.MaxBytes <= 0 || a.Bytes > c.MaxBytes || a.Bytes <= 0 || c.Timeout <= 0 {
		return bad, media.TooLarge
	}
	st, e := os.Stat(a.Path)
	if e != nil || st.Size() != a.Bytes {
		return bad, media.Failure("INVALID_MEDIA")
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	reader, writer := io.Pipe()
	mw := multipart.NewWriter(writer)
	field, method := "document", "sendDocument"
	if a.Kind == "video" {
		field, method = "video", "sendVideo"
	} else if a.Kind == "audio" {
		field, method = "audio", "sendAudio"
	}
	done := make(chan error, 1)
	go func() {
		e := mw.WriteField("chat_id", strconv.FormatInt(chat, 10))
		if e == nil {
			var part io.Writer
			part, e = mw.CreateFormFile(field, a.Name)
			if e == nil {
				var f *os.File
				f, e = os.Open(a.Path)
				if e == nil {
					_, e = media.CopyBounded(ctx, part, f, c.MaxBytes)
					f.Close()
				}
			}
		}
		if e == nil {
			e = mw.Close()
		}
		writer.CloseWithError(e)
		done <- e
	}()
	defer func() { reader.Close(); writer.Close(); <-done }()
	req, e := http.NewRequestWithContext(ctx, "POST", "https://api.telegram.org/bot"+c.Token+"/"+method, reader)
	if e != nil {
		return bad, media.Failure("DELIVERY_FAILED")
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	client := &http.Client{Transport: c.transport, Timeout: c.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, e := installationlicense.HTTP(ctx, client, req)
	if e != nil {
		if errors.Is(e, installationlicense.ErrRestricted) {
			return bad, e
		}
		return media.DeliveryResult{Outcome: "DELIVERY_OUTCOME_UNKNOWN"}, nil
	}
	defer resp.Body.Close()
	var body struct {
		OK     bool `json:"ok"`
		Error  int  `json:"error_code"`
		Result struct {
			ID int64 `json:"message_id"`
		}
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&body) != nil {
		return media.DeliveryResult{Outcome: "DELIVERY_OUTCOME_UNKNOWN"}, nil
	}
	if resp.StatusCode == 200 && body.OK && body.Result.ID > 0 {
		return media.DeliveryResult{Outcome: "DELIVERY_ACKNOWLEDGED", MessageID: body.Result.ID}, nil
	}
	if body.Error == 429 || resp.StatusCode == 429 {
		return media.DeliveryResult{Outcome: "DELIVERY_FAILED", Retry: true}, nil
	}
	return bad, nil
}
