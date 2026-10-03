package identity

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/naseri-coder/TELMIVO/internal/platform/auth"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"strconv"
	"time"
)

type Result struct {
	UserID      string `json:"user_id"`
	WorkspaceID string `json:"workspace_id"`
	Admin       bool   `json:"is_admin"`
}

func Bootstrap(ctx context.Context, tx pgx.Tx, u auth.TelegramUser, admins []int64) (Result, error) {
	var out Result
	if u.ID <= 0 {
		return out, auth.ErrInvalid
	}
	if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", u.ID); e != nil {
		return out, e
	}
	e := tx.QueryRow(ctx, "SELECT user_id::text FROM telegram_accounts WHERE telegram_id=$1", u.ID).Scan(&out.UserID)
	if errors.Is(e, pgx.ErrNoRows) {
		e = tx.QueryRow(ctx, "INSERT INTO users(display_name) VALUES($1) RETURNING id::text", u.FirstName).Scan(&out.UserID)
		if e != nil {
			return out, e
		}
		_, e = tx.Exec(ctx, "INSERT INTO telegram_accounts(telegram_id,user_id,username) VALUES($1,$2,$3)", u.ID, out.UserID, u.Username)
	}
	if e != nil {
		return out, e
	}
	var blocked bool
	if e = tx.QueryRow(ctx, "SELECT blocked FROM users WHERE id=$1", out.UserID).Scan(&blocked); e != nil {
		return out, e
	}
	if blocked {
		return out, database.ErrForbidden
	}
	if _, e = tx.Exec(ctx, "UPDATE users SET last_active_at=now() WHERE id=$1", out.UserID); e != nil {
		return out, e
	}
	for _, id := range admins {
		if id == u.ID {
			_, e = tx.Exec(ctx, "INSERT INTO platform_admins(user_id) VALUES($1) ON CONFLICT DO NOTHING", out.UserID)
			if e != nil {
				return out, e
			}
			if _, e = tx.Exec(ctx, "INSERT INTO platform_admin_permissions SELECT $1,key FROM platform_permissions ON CONFLICT DO NOTHING", out.UserID); e != nil {
				return out, e
			}
		}
	}
	if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM platform_admins WHERE user_id=$1)", out.UserID).Scan(&out.Admin); e != nil {
		return out, e
	}
	e = tx.QueryRow(ctx, "SELECT id::text FROM workspaces WHERE owner_id=$1 ORDER BY created_at LIMIT 1", out.UserID).Scan(&out.WorkspaceID)
	if errors.Is(e, pgx.ErrNoRows) {
		e = tx.QueryRow(ctx, "INSERT INTO workspaces(name,owner_id) VALUES($1,$2) RETURNING id::text", u.FirstName, out.UserID).Scan(&out.WorkspaceID)
		if e != nil {
			return out, e
		}
		_, e = tx.Exec(ctx, "INSERT INTO workspace_members(workspace_id,user_id,role_key) VALUES($1,$2,'OWNER')", out.WorkspaceID, out.UserID)
		if e != nil {
			return out, e
		}
		if e = database.Scope(ctx, tx, out.WorkspaceID); e != nil {
			return out, e
		}
		_, e = tx.Exec(ctx, "INSERT INTO subscriptions(workspace_id,plan_key) VALUES($1,'FREE')", out.WorkspaceID)
		if e != nil {
			return out, e
		}
		_, e = tx.Exec(ctx, "INSERT INTO wallet_accounts(workspace_id,asset,kind) SELECT $1,a,k FROM unnest(ARRAY['IRR','USD']) a CROSS JOIN unnest(ARRAY['WALLET','CLEARING']) k", out.WorkspaceID)
		if e != nil {
			return out, e
		}
	}
	return out, e
}
func (s Result) String() string { return s.UserID + ":" + s.WorkspaceID }

type Service struct {
	DB         *database.Store
	Admins     []int64
	SessionTTL time.Duration
}

func (s *Service) Login(ctx context.Context, u auth.TelegramUser, raw string) (Result, string, error) {
	var out Result
	token, e := auth.Token()
	if e != nil {
		return out, "", e
	}
	tx, e := s.DB.Pool.Begin(ctx)
	if e != nil {
		return out, "", e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "DELETE FROM auth_replays WHERE expires_at<now()"); e != nil {
		return out, "", e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO auth_replays(digest,expires_at) VALUES($1,$2)", auth.Digest(raw), time.Now().Add(10*time.Minute)); e != nil {
		var pe *pgconn.PgError
		if errors.As(e, &pe) && pe.Code == "23505" {
			return out, "", auth.ErrInvalid
		}
		return out, "", e
	}
	out, e = Bootstrap(ctx, tx, u, s.Admins)
	if e != nil {
		return out, "", e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,$3)", auth.Digest(token), out.UserID, time.Now().Add(s.SessionTTL)); e != nil {
		return out, "", e
	}
	if e = tx.Commit(ctx); e != nil {
		return out, "", e
	}
	return out, token, nil
}
func (s *Service) Session(ctx context.Context, token string) (string, error) {
	if len(token) != 64 {
		return "", auth.ErrInvalid
	}
	var id string
	e := s.DB.Pool.QueryRow(ctx, "SELECT s.user_id::text FROM sessions s JOIN users u ON u.id=s.user_id WHERE token_hash=$1 AND expires_at>now() AND NOT u.blocked", auth.Digest(token)).Scan(&id)
	if e != nil {
		return "", auth.ErrInvalid
	}
	return id, nil
}
func TelegramID(ctx context.Context, tx pgx.Tx, user string) (int64, error) {
	var id int64
	e := tx.QueryRow(ctx, "SELECT telegram_id FROM telegram_accounts WHERE user_id=$1", user).Scan(&id)
	return id, e
}
func UpdateKey(id int64) string { return strconv.FormatInt(id, 10) }
