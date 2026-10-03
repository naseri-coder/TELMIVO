package telegram

import (
	"crypto/subtle"
	"encoding/json"
	"github.com/naseri-coder/TELMIVO/internal/domains/groupbot"
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
		e := s.Handle(r.Context(), u, "group-update")
		if e != nil {
			slog.Warn("group update denied", "outcome", "DENIED")
			w.WriteHeader(503)
			return
		}
		slog.Info("group update completed", "outcome", "ACCEPTED")
		w.WriteHeader(200)
	})
}
