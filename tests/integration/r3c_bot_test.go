//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/downloads"
	"github.com/naseri-coder/TELMIVO/internal/integrations/media"
	"testing"
)

func TestR3CBotCallbacks(t *testing.T) {
	u := fixture(t, false)
	var telegram int64
	tenant(t, u, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT telegram_id FROM telegram_accounts WHERE user_id=$1", u.UserID).Scan(&telegram)
	})
	b := downloads.Bot{Identity: r3aService(), Product: r3cProduct()}
	send := func(update int64, data string, callback bool) {
		from := map[string]any{"id": telegram, "first_name": "مشتری"}
		chat := map[string]any{"id": telegram, "type": "private"}
		raw := map[string]any{"update_id": update}
		if callback {
			raw["callback_query"] = map[string]any{"from": from, "data": data, "message": map[string]any{"chat": chat}}
		} else {
			raw["message"] = map[string]any{"from": from, "chat": chat, "text": data}
		}
		buf, e := json.Marshal(raw)
		must(t, e)
		var input downloads.BotUpdate
		must(t, json.Unmarshal(buf, &input))
		must(t, b.Handle(ctx, "r3c-bot-fixture", input))
	}
	send(98003100, "/start", false)
	send(98003101, "https://youtu.be/dQw4w9WgXcQ", false)
	send(98003101, "https://youtu.be/dQw4w9WgXcQ", false)
	var id string
	tenant(t, u, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT id::text FROM download_jobs WHERE idempotency_key='downloader-bot:98003101'").Scan(&id)
	})
	w := r3cWorker(t, r3cProvider{}, &r3cDelivery{result: media.DeliveryResult{Outcome: "DELIVERY_ACKNOWLEDGED", MessageID: 1}})
	must(t, w.RunOne(ctx, u.UserID, u.WorkspaceID, id))
	send(98003102, "f:"+id+":tampered", true)
	if r3cView(t, u, id).Selected != "" {
		t.Fatal("tampered callback mutated job")
	}
	send(98003103, "f:"+id+":fixture-compatible", true)
	if r3cView(t, u, id).Selected != "fixture-compatible" {
		t.Fatal("valid callback failed")
	}
	send(98003104, fmt.Sprint("/cancel ", id), false)
	must(t, w.RunOne(ctx, u.UserID, u.WorkspaceID, id))
	if r3cView(t, u, id).State != "CANCELLED" {
		t.Fatal("bot cancellation")
	}
	send(98003105, "/status", false)
	send(98003106, "/history", false)
}
