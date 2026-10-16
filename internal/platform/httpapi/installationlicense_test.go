package httpapi

import (
	"bytes"
	"context"
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	"github.com/naseri-coder/TELMIVO/internal/platform/installationlicense"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRestrictedCustomerWebhookACK(t *testing.T) {
	s := &contentplugins.Service{DigestKey: bytes.Repeat([]byte{7}, 32)}
	a := &API{Content: s}
	for _, valid := range []bool{false, true} {
		r := httptest.NewRequest(http.MethodPost, "/webhooks/customer", bytes.NewBufferString(`{"update_id":1}`))
		r.SetPathValue("account", "test-account")
		r.SetPathValue("bot", "test-bot")
		if valid {
			r.Header.Set("X-Telegram-Bot-Api-Secret-Token", s.WebhookSecret("test-account", "test-bot"))
		}
		r = r.WithContext(installationlicense.RequireContext(context.Background()))
		w := httptest.NewRecorder()
		a.contentWebhook(w, r)
		want := 403
		if valid {
			want = 200
		}
		if w.Code != want {
			t.Fatalf("valid=%v status=%d", valid, w.Code)
		}
	}
}
