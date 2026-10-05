package httpapi

import (
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/naseri-coder/TELMIVO/internal/domains/groupbot"
	"github.com/naseri-coder/TELMIVO/internal/domains/identity"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"net/http"
	"strconv"
	"time"
)

func (a *API) groupService() *groupbot.Service {
	if a.Groups != nil {
		return a.Groups
	}
	return &groupbot.Service{DB: a.DB, Identity: a.Identity}
}
func (a *API) groupList(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		if !database.Permission(r.Context(), tx, user(r), workspace(r), "groups.moderate") {
			return nil, database.ErrForbidden
		}
		return rows(r.Context(), tx, "SELECT g.id::text,g.title,g.status,s.revision,s.moderation,s.security FROM groups g JOIN group_settings s ON s.group_id=g.id AND s.workspace_id=g.workspace_id WHERE g.workspace_id=$1 ORDER BY g.created_at DESC,g.id DESC LIMIT 100", workspace(r))
	})
}
func (a *API) groupDetail(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		if !database.Permission(r.Context(), tx, user(r), workspace(r), "groups.moderate") {
			return nil, database.ErrForbidden
		}
		return a.groupService().View(r.Context(), tx, user(r), workspace(r), r.PathValue("group"))
	})
}
func (a *API) groupClaim(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Chat  int64  `json:"chat_id"`
		Title string `json:"title"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}

	var id string
	e := a.DB.Tenant(r.Context(), user(r), workspace(r), false, func(tx pgx.Tx) error {
		tg, e := identity.TelegramID(r.Context(), tx, user(r))
		if e != nil {
			return e
		}
		id, e = a.groupService().Claim(r.Context(), tx, user(r), workspace(r), correlation(r), tg, in.Chat, in.Title)
		return e
	})
	if e != nil {
		var pe *pgconn.PgError
		if errors.Is(e, database.ErrConflict) || errors.As(e, &pe) && pe.Code == "23505" {
			auditErr := a.DB.Tenant(r.Context(), user(r), workspace(r), false, func(tx pgx.Tx) error {
				return database.Audit(r.Context(), tx, user(r), workspace(r), "group.claim.conflict", "telegram_chat", strconv.FormatInt(in.Chat, 10), correlation(r), map[string]any{}, map[string]any{"reason": "CHAT_ALREADY_CLAIMED"})
			})
			if auditErr != nil {
				handleError(w, auditErr)
				return
			}
		}
		handleError(w, e)
		return
	}
	respond(w, 200, map[string]string{"id": id})
}
func (a *API) groupConfigure(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision int64           `json:"revision"`
		Policy   groupbot.Policy `json:"policy"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		return a.groupService().Configure(r.Context(), tx, user(r), workspace(r), r.PathValue("group"), correlation(r), in.Revision, in.Policy)
	})
}
func (a *API) groupModerate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Target  int64           `json:"target_id"`
		Message int64           `json:"message_id"`
		Action  groupbot.Action `json:"action"`
		Reason  string          `json:"reason"`
		Key     string          `json:"idempotency_key"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		e := a.groupService().Moderate(r.Context(), tx, user(r), workspace(r), r.PathValue("group"), in.Key, correlation(r), in.Target, in.Message, in.Action, in.Reason)
		return map[string]bool{"recorded": e == nil}, e
	})
}
func (a *API) groupHistory(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		if !database.Permission(r.Context(), tx, user(r), workspace(r), "groups.moderate") {
			return nil, database.ErrForbidden
		}
		if _, e := a.groupService().View(r.Context(), tx, user(r), workspace(r), r.PathValue("group")); e != nil {
			return nil, e
		}
		before := r.URL.Query().Get("before")
		ts := time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
		if before == "" {
			before = "ffffffff-ffff-ffff-ffff-ffffffffffff"
		} else {
			if len(before) != 36 {
				return nil, database.ErrInvalid
			}
			if e := tx.QueryRow(r.Context(), "SELECT created_at FROM group_moderation_events WHERE workspace_id=$1 AND group_id::text=$2 AND id::text=$3", workspace(r), r.PathValue("group"), before).Scan(&ts); e != nil {
				return nil, e
			}
		}
		return rows(r.Context(), tx, "SELECT id::text,target_id::text,kind,action,reason,outcome,created_at FROM group_moderation_events WHERE workspace_id=$1 AND group_id::text=$2 AND (created_at,id)<($3,$4::uuid) ORDER BY created_at DESC,id DESC LIMIT 50", workspace(r), r.PathValue("group"), ts, before)
	})
}
