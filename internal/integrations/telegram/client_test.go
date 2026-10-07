package telegram

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	transport "github.com/naseri-coder/TELMIVO/internal/platform/telegram"
)

func TestMessagePayloadOmitsNilMarkup(t *testing.T) {
	p := messagePayload(transport.Message{ChatID: -1001, Text: "Synthetic"})
	if _, ok := p["reply_markup"]; ok {
		t.Fatal("nil reply_markup must be omitted")
	}
	p = messagePayload(transport.Message{ChatID: -1001, Text: "Synthetic", Markup: map[string]any{"inline_keyboard": []any{}}})
	if _, ok := p["reply_markup"]; !ok {
		t.Fatal("non-nil reply_markup must be preserved")
	}
}

func TestMessageAcknowledgementMustBePositiveAndKnown(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		unknown    bool
	}{
		{"ack", `{"ok":true,"result":{"message_id":3}}`, 200, false},
		{"missing ID", `{"ok":true,"result":{}}`, 200, true},
		{"zero ID", `{"ok":true,"result":{"message_id":0}}`, 200, true},
		{"negative ID", `{"ok":true,"result":{"message_id":-3}}`, 200, true},
		{"invalid result", `{"ok":true,"result":true}`, 200, true},
		{"server ambiguity", `{"ok":true,"result":{"message_id":3}}`, 502, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := New("synthetic-only", true)
			c.http = &http.Client{Transport: groupRoundTrip(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			for _, send := range []func() (int64, error){
				func() (int64, error) {
					return c.Send(context.Background(), transport.Message{ChatID: 10001, Text: "Synthetic"})
				},
				func() (int64, error) { return c.Copy(context.Background(), 10001, 10002, 1) },
				func() (int64, error) { return c.Forward(context.Background(), 10001, 10002, 1) },
			} {
				id, e := send()
				if tc.unknown {
					if e == nil || e.Error() != "TELEGRAM_DELIVERY_UNKNOWN" {
						t.Fatal("false acknowledgement", id, e)
					}
				} else if e != nil || id != 3 {
					t.Fatal(id, e)
				}
			}
		})
	}
}
