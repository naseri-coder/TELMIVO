package telegram

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	transport "github.com/naseri-coder/TELMIVO/internal/platform/telegram"
)

func TestClassifiedSendAcceptsGroupChatAndMarkup(t *testing.T) {
	c := New("synthetic-only", true)
	c.http = &http.Client{Transport: groupRoundTrip(func(r *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		raw := string(body)
		if !strings.Contains(raw, `"chat_id":-100123`) || !strings.Contains(raw, `"reply_markup"`) {
			t.Fatalf("group target/markup missing from payload: %s", raw)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":{"message_id":9}}`))}, nil
	})}
	id, code, delay := c.SendClassified(context.Background(), transport.Message{
		ChatID: -100123,
		Text:   "Synthetic",
		Markup: map[string]any{"inline_keyboard": []any{}},
	})
	if id != 9 || code != "DELIVERY_ACKNOWLEDGED" || delay != 0 {
		t.Fatalf("unexpected result id=%d code=%s delay=%s", id, code, delay)
	}
}

func TestBroadcastRejectionClassification(t *testing.T) {
	for _, tc := range []struct {
		body  string
		code  string
		delay time.Duration
	}{{`{"ok":true,"result":{"message_id":3}}`, "DELIVERY_ACKNOWLEDGED", 0}, {`{"ok":false,"error_code":429,"parameters":{"retry_after":120}}`, "RATE_LIMITED", 120 * time.Second}, {`{"ok":false,"error_code":403}`, "DELIVERY_FAILED", 0}, {`{"ok":false,"error_code":500}`, "DELIVERY_OUTCOME_UNKNOWN", 0}, {`broken`, "DELIVERY_OUTCOME_UNKNOWN", 0}, {`{"ok":true,"result":{"message_id":0}}`, "DELIVERY_OUTCOME_UNKNOWN", 0}} {
		c := New("synthetic-only", true)
		c.http = &http.Client{Transport: groupRoundTrip(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
		})}
		_, code, delay := c.BroadcastReply(context.Background(), 10001, "Synthetic")
		if code != tc.code || delay != tc.delay {
			t.Fatal(tc, code, delay)
		}
	}
}
