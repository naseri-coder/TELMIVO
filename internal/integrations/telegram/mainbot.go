package telegram

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"github.com/naseri-coder/TELMIVO/internal/domains/identity"
	"github.com/naseri-coder/TELMIVO/internal/domains/mainbot"
	"github.com/naseri-coder/TELMIVO/internal/platform/auth"
	"github.com/naseri-coder/TELMIVO/internal/platform/installationlicense"
	"io"
	"log/slog"
	"net/http"
)

// MainBotWebhook validates transport identity before dispatching an application command.
// The application commits its response to the existing durable outbox, never sends in the webhook.
func MainBotWebhook(secret string, service *mainbot.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		if secret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(r.Header.Get("X-Telegram-Bot-Api-Secret-Token"))) != 1 {
			w.WriteHeader(401)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		var raw json.RawMessage
		decoder := json.NewDecoder(r.Body)
		if decoder.Decode(&raw) != nil {
			w.WriteHeader(400)
			return
		}
		var rest any
		if decoder.Decode(&rest) != io.EOF {
			w.WriteHeader(400)
			return
		}
		var metadata struct {
			ID *int64 `json:"update_id"`
		}
		var u identity.Update
		if json.Unmarshal(raw, &metadata) != nil || metadata.ID == nil || json.Unmarshal(raw, &u) != nil {
			w.WriteHeader(400)
			return
		}
		if installationlicense.CheckRuntime(r.Context()) != nil {
			w.Header().Set("X-Telmivo-License", "restricted")
			w.WriteHeader(200)
			return
		}
		correlation, e := auth.Token()
		if e != nil {
			w.WriteHeader(503)
			return
		}
		correlation = correlation[:24]
		w.Header().Set("X-Request-ID", correlation)
		_, e = service.Handle(r.Context(), u, correlation)
		status := 200
		outcome := "accepted"
		if errors.Is(e, mainbot.ErrUnsupported) {
			outcome = "unsupported"
		} else if errors.Is(e, auth.ErrInvalid) {
			status = 400
			outcome = "invalid"
		} else if e != nil {
			status = 503
			outcome = "retry"
		}
		slog.Info("main bot update completed", "correlation_id", correlation, "outcome", outcome, "status", status)
		w.WriteHeader(status)
	})
}
