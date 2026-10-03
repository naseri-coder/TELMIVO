package telegram

import (
	"github.com/naseri-coder/TELMIVO/internal/domains/mainbot"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebhookBoundary(t *testing.T) {
	handler := MainBotWebhook("synthetic-webhook-fixture", &mainbot.Service{})
	for _, fixture := range []struct {
		body, secret string
		status       int
	}{{"{}", "wrong", 401}, {"{", "synthetic-webhook-fixture", 400}, {"{} {}", "synthetic-webhook-fixture", 400}, {"{}", "synthetic-webhook-fixture", 400}, {`{"update_id":1,"message":{"from":{"id":1},"chat":{"id":1,"type":"private"},"text":"unknown"}}`, "synthetic-webhook-fixture", 200}} {
		request := httptest.NewRequest("POST", "/webhook", strings.NewReader(fixture.body))
		request.Header.Set("X-Telegram-Bot-Api-Secret-Token", fixture.secret)
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, request)
		if out.Code != fixture.status {
			t.Fatalf("status %d expected %d", out.Code, fixture.status)
		}
	}
}
