package identity

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/policies"
	"github.com/naseri-coder/TELMIVO/internal/platform/auth"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
)

// Enter accepts identity only from a server-verified official transport. Entry source never keys identity.
func Enter(ctx context.Context, tx pgx.Tx, u auth.TelegramUser, admins []int64, source, event string) (Result, error) {
	if source != "MAIN_BOT" && source != "GROUP_BOT" && source != "DOWNLOADER_BOT" && source != "MINIAPP" {
		return Result{}, database.ErrInvalid
	}
	if u.ID <= 0 || event == "" {
		return Result{}, auth.ErrInvalid
	}
	if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", u.ID); e != nil {
		return Result{}, e
	}
	var exists bool
	if e := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM telegram_accounts WHERE telegram_id=$1)", u.ID).Scan(&exists); e != nil {
		return Result{}, e
	}
	out, e := Bootstrap(ctx, tx, u, admins)
	if e != nil {
		return out, e
	}
	if e = database.Scope(ctx, tx, out.WorkspaceID); e != nil {
		return out, e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO identity_entries(user_id,source) VALUES($1,$2) ON CONFLICT(user_id,source) DO UPDATE SET last_seen_at=now()", out.UserID, source); e != nil {
		return out, e
	}
	e = policies.Enroll(ctx, tx, policies.Subject{User: out.UserID, Workspace: out.WorkspaceID, Source: source, Event: event, New: !exists}, "DISCOVERY")
	return out, e
}

type GroupAuthority interface {
	CanAdmin(context.Context, int64, int64) (bool, error)
}

func ClaimGroup(ctx context.Context, tx pgx.Tx, user, workspace, correlation string, telegramUser, chat int64, title string, authority GroupAuthority) (string, error) {
	if authority == nil || chat >= 0 || len(title) < 1 || len(title) > 200 {
		return "", database.ErrInvalid
	}
	var actual int64
	if e := tx.QueryRow(ctx, "SELECT telegram_id FROM telegram_accounts WHERE user_id=$1", user).Scan(&actual); e != nil {
		return "", e
	}
	if actual != telegramUser {
		return "", database.ErrForbidden
	}
	allowed, e := authority.CanAdmin(ctx, chat, telegramUser)
	if e != nil {
		return "", e
	}
	if !allowed || !database.Permission(ctx, tx, user, workspace, "groups.moderate") {
		return "", database.ErrForbidden
	}
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "trial-user:"+user); e != nil {
		return "", e
	}
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", chat); e != nil {
		return "", e
	}
	var prior, id string
	e = tx.QueryRow(ctx, "SELECT workspace_id::text,group_id::text FROM group_claims WHERE telegram_chat_id=$1", chat).Scan(&prior, &id)
	if e == nil {
		if prior != workspace {
			return "", database.ErrConflict
		}
		return id, nil
	}
	if e != pgx.ErrNoRows {
		return "", e
	}
	if e = tx.QueryRow(ctx, "INSERT INTO groups(workspace_id,telegram_chat_id,title,status) VALUES($1,$2,$3,'CONNECTED') RETURNING id::text", workspace, chat, title).Scan(&id); e != nil {
		return "", e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO group_claims(telegram_chat_id,workspace_id,group_id,claimed_by) VALUES($1,$2,$3,$4)", chat, workspace, id, user); e != nil {
		return "", e
	}
	if e = policies.Enroll(ctx, tx, policies.Subject{User: user, Workspace: workspace, Group: id, Source: "GROUP_BOT", Event: strconv.FormatInt(chat, 10), Now: time.Now()}, "GROUP_CLAIM"); e != nil {
		return "", e
	}
	return id, database.Audit(ctx, tx, user, workspace, "group.claim", "group", id, correlation, map[string]any{}, map[string]any{"telegram_chat_id": chat})
}
