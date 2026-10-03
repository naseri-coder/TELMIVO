package identity

import (
	"context"
	"encoding/json"
	"github.com/naseri-coder/TELMIVO/internal/domains/accessgates"
	"github.com/naseri-coder/TELMIVO/internal/domains/entitlements"
	"github.com/naseri-coder/TELMIVO/internal/platform/auth"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/localization"
)

type Update struct {
	ID      int64 `json:"update_id"`
	Message *struct {
		From auth.TelegramUser `json:"from"`
		Chat struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		} `json:"chat"`
		Text string `json:"text"`
	} `json:"message"`
}

func MainMenu(admin bool, url string) any {
	keys := []string{"dashboard", "channels", "groups", "bots", "tickets", "wallet", "plans", "settings"}
	rows := [][]map[string]any{}
	for i := 0; i < len(keys); i += 2 {
		rows = append(rows, []map[string]any{{"text": localization.Text("menu." + keys[i])}, {"text": localization.Text("menu." + keys[i+1])}})
	}
	if admin {
		rows = append(rows, []map[string]any{{"text": localization.Text("menu.admin")}})
	}
	if url != "" {
		rows = append(rows, []map[string]any{{"text": localization.Text("menu.open"), "web_app": map[string]string{"url": url}}})
	}
	return map[string]any{"keyboard": rows, "resize_keyboard": true}
}
func (s *Service) Start(ctx context.Context, bot, url, correlation string, u Update, membership accessgates.MembershipService) (Result, error) {
	var out Result
	if u.Message == nil || u.Message.Chat.Type != "private" || u.Message.From.ID != u.Message.Chat.ID || u.ID < 0 {
		return out, auth.ErrInvalid
	}
	tx, e := s.DB.Pool.Begin(ctx)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	tag, e := tx.Exec(ctx, "INSERT INTO telegram_updates(bot_key,update_id) VALUES($1,$2) ON CONFLICT DO NOTHING", bot, u.ID)
	if e != nil {
		return out, e
	}
	if tag.RowsAffected() == 0 {
		return out, nil
	}
	source := map[string]string{"main-bot": "MAIN_BOT", "group-bot": "GROUP_BOT", "downloader-bot": "DOWNLOADER_BOT", "main": "MAIN_BOT", "group": "GROUP_BOT", "downloader": "DOWNLOADER_BOT"}[bot]
	out, e = Enter(ctx, tx, u.Message.From, s.Admins, source, bot+":"+UpdateKey(u.ID))
	if e != nil {
		return out, e
	}
	if e = database.Scope(ctx, tx, out.WorkspaceID); e != nil {
		return out, e
	}
	role, e := entitlements.Role(ctx, tx, out.UserID, out.WorkspaceID)
	if e != nil {
		return out, e
	}
	decision, e := entitlements.Resolve(ctx, tx, out.WorkspaceID, role, "wallet", "", "wallet.read", 0)
	if e != nil {
		return out, e
	}
	_ = decision
	allowed, e := accessgates.Check(ctx, tx, out.WorkspaceID, u.Message.From.ID, membership)
	if e != nil {
		return out, e
	}
	message := localization.Text("welcome")
	var markup any
	if allowed {
		markup = MainMenu(out.Admin, url)
	} else {
		message = localization.Text("gate.required")
	}
	payload := map[string]any{"user_id": out.UserID, "body": message, "markup": markup, "idempotency_key": bot + ":" + UpdateKey(u.ID)}
	if e = database.Event(ctx, tx, out.WorkspaceID, "telmivo.jobs.notification", "bot.menu", correlation, payload); e != nil {
		return out, e
	}
	e = tx.Commit(ctx)
	return out, e
}
func DecodeUpdate(raw []byte) (Update, error) {
	var u Update
	e := json.Unmarshal(raw, &u)
	return u, e
}
