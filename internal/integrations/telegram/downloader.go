package telegram

import (
	"crypto/subtle"
	"encoding/json"
	"github.com/naseri-coder/TELMIVO/internal/domains/downloads"
	"github.com/naseri-coder/TELMIVO/internal/platform/installationlicense"
	"io"
	"net/http"
)

func DownloaderWebhook(secret string, b *downloads.Bot) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if secret == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Telegram-Bot-Api-Secret-Token")), []byte(secret)) != 1 {
			w.WriteHeader(403)
			return
		}
		var u downloads.BotUpdate
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
		if dec.Decode(&u) != nil {
			w.WriteHeader(400)
			return
		}
		var extra any
		if dec.Decode(&extra) != io.EOF {
			w.WriteHeader(400)
			return
		}
		if installationlicense.CheckRuntime(r.Context()) != nil {
			w.Header().Set("X-Telmivo-License", "restricted")
			w.WriteHeader(200)
			return
		}
		if b.Handle(r.Context(), "downloader-webhook", u) != nil {
			w.WriteHeader(400)
			return
		}
		w.WriteHeader(200)
	})
}
