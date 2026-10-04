package telegram

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

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
