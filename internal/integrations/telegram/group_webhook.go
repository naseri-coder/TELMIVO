package telegram

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"github.com/naseri-coder/TELMIVO/internal/domains/groupbot"
	"github.com/naseri-coder/TELMIVO/internal/platform/auth"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/installationlicense"
	"io"
	"log/slog"
	"net/http"
)

func GroupBotWebhook(secret string, s *groupbot.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		if secret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(r.Header.Get("X-Telegram-Bot-Api-Secret-Token"))) != 1 {
			w.WriteHeader(403)
			return
		}
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		var u groupbot.Update
		var rest any
		if d.Decode(&u) != nil || d.Decode(&rest) != io.EOF || u.ID == nil {
			w.WriteHeader(400)
			return
		}
		if installationlicense.CheckRuntime(r.Context()) != nil {
			w.Header().Set("X-Telmivo-License", "restricted")
			w.WriteHeader(200)
			return
		}
		id, err := auth.Token()
		if err != nil {
			w.WriteHeader(500)
			return
		}
		id = id[:24]
		w.Header().Set("X-Request-ID", id)
		e := s.Handle(r.Context(), u, id)
		if e != nil {
			code := 503
			category := "UNAVAILABLE"
			if errors.Is(e, database.ErrInvalid) || errors.Is(e, auth.ErrInvalid) {
				code = 400
				category = "INVALID"
			}
			if errors.Is(e, database.ErrForbidden) || errors.Is(e, groupbot.ErrRights) || errors.Is(e, groupbot.ErrProtected) {
				code = 403
				category = "DENIED"
			}
			slog.Warn("group update denied", "outcome", category, "correlation_id", id)
			w.WriteHeader(code)
			return
		}
		slog.Info("group update completed", "outcome", "ACCEPTED", "correlation_id", id)
		w.WriteHeader(200)
	})
}
