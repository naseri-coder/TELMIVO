package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/botmodules"
	"github.com/naseri-coder/TELMIVO/internal/domains/downloads"
	"github.com/naseri-coder/TELMIVO/internal/domains/entitlements"
	"github.com/naseri-coder/TELMIVO/internal/domains/identity"
	"github.com/naseri-coder/TELMIVO/internal/domains/subscriptions"
	"github.com/naseri-coder/TELMIVO/internal/domains/tickets"
	"github.com/naseri-coder/TELMIVO/internal/domains/wallets"
	"github.com/naseri-coder/TELMIVO/internal/platform/auth"
	"github.com/naseri-coder/TELMIVO/internal/platform/cache"
	"github.com/naseri-coder/TELMIVO/internal/platform/config"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/entitlement"
	"github.com/naseri-coder/TELMIVO/internal/platform/localization"
	"github.com/naseri-coder/TELMIVO/internal/platform/queue"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type API struct {
	DB       *database.Store
	Cache    *cache.Client
	Queue    *queue.Client
	Config   config.Config
	Identity *identity.Service
	Requests atomic.Uint64
}
type ctxKey string

const userKey ctxKey = "user"
const requestKey ctxKey = "request"

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, map[string]string{"status": "live"}) })
	mux.HandleFunc("GET /health/ready", a.ready)
	mux.HandleFunc("POST /api/v1/auth/telegram", a.login)
	protected := http.NewServeMux()
	protected.HandleFunc("GET /api/v1/auth/me", a.me)
	protected.HandleFunc("POST /api/v1/auth/logout", a.logout)
	protected.HandleFunc("GET /api/v1/workspaces", a.workspaces)
	protected.HandleFunc("GET /api/v1/dashboard", a.dashboard)
	protected.HandleFunc("GET /api/v1/entitlements", a.access)
	protected.HandleFunc("POST /api/v1/entitlements/preview", a.preview)
	protected.HandleFunc("GET /api/v1/plans", a.plans)
	protected.HandleFunc("POST /api/v1/plans", a.createPlan)
	protected.HandleFunc("PATCH /api/v1/plans/{plan}", a.editPlan)
	protected.HandleFunc("PUT /api/v1/plans/{plan}/entitlements/{feature}", a.editEntitlement)
	protected.HandleFunc("PUT /api/v1/plans/{plan}/plugins/{plugin}", a.editPluginEntitlement)
	protected.HandleFunc("GET /api/v1/features", a.features)
	protected.HandleFunc("PUT /api/v1/features/{feature}/flag", a.editFlag)
	protected.HandleFunc("GET /api/v1/plugins", a.plugins)
	protected.HandleFunc("PUT /api/v1/plugins/{plugin}/status", a.editPluginStatus)
	protected.HandleFunc("PUT /api/v1/entitlements/overrides/{feature}", a.override)
	protected.HandleFunc("DELETE /api/v1/entitlements/overrides/{feature}", a.removeOverride)
	protected.HandleFunc("PUT /api/v1/entitlements/grants/{feature}", a.grant)
	protected.HandleFunc("DELETE /api/v1/entitlements/grants/{feature}", a.removeGrant)
	protected.HandleFunc("PUT /api/v1/subscriptions", a.changePlan)
	protected.HandleFunc("GET /api/v1/wallets", a.wallet)
	protected.HandleFunc("POST /api/v1/wallets/adjustments", a.adjust)
	protected.HandleFunc("GET /api/v1/ledger", a.ledger)
	protected.HandleFunc("GET /api/v1/tickets", a.ticketList)
	protected.HandleFunc("POST /api/v1/tickets", a.ticketCreate)
	protected.HandleFunc("GET /api/v1/tickets/{ticket}", a.ticketDetail)
	protected.HandleFunc("POST /api/v1/tickets/{ticket}/replies", a.ticketReply)
	protected.HandleFunc("POST /api/v1/tickets/{ticket}/actions", a.ticketAction)
	protected.HandleFunc("GET /api/v1/downloads", a.downloadList)
	protected.HandleFunc("GET /api/v1/downloads/formats", a.formats)
	protected.HandleFunc("POST /api/v1/downloads", a.downloadCreate)
	protected.HandleFunc("GET /api/v1/audit", a.audit)
	protected.HandleFunc("GET /api/v1/team", a.team)
	protected.HandleFunc("/api/v1/", func(w http.ResponseWriter, r *http.Request) { fail(w, 501, "not_implemented") })
	mux.Handle("/api/v1/", a.authenticated(protected))
	return a.secure(mux)
}
func respond(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
func fail(w http.ResponseWriter, status int, key string) {
	respond(w, status, map[string]any{"error": map[string]string{"code": key, "message": localization.Text(key)}})
}
func decode(w http.ResponseWriter, r *http.Request, out any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(out); e != nil {
		return e
	}
	var rest any
	if e := d.Decode(&rest); e != io.EOF {
		return errors.New("extra request data")
	}
	return nil
}
func (a *API) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.Requests.Add(1)
		id, _ := auth.Token()
		id = id[:24]
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors https://web.telegram.org https://telegram.org")
		if a.Config.Environment == "production" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			origin := r.Header.Get("Origin")
			if origin != "" {
				u, e := url.Parse(origin)
				configured, _ := url.Parse(a.Config.MiniAppURL)
				if e != nil || (u.Host != r.Host && (configured == nil || u.Host != configured.Host || u.Scheme != configured.Scheme)) {
					fail(w, 403, "forbidden")
					return
				}
			} else if _, e := r.Cookie("telmivo_session"); e == nil {
				fail(w, 403, "forbidden")
				return
			}
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && a.Cache != nil {
			host, _, _ := net.SplitHostPort(r.RemoteAddr)
			allowed, e := a.Cache.Allow(r.Context(), "ip:"+host, 120, time.Minute)
			if e != nil {
				fail(w, 503, "unavailable")
				return
			}
			if !allowed {
				fail(w, 429, "rate_limit")
				return
			}
		}
		defer func() {
			if recover() != nil {
				fail(w, 500, "internal")
			}
		}()
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestKey, id)))
	})
}
func (a *API) authenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, e := r.Cookie("telmivo_session")
		if e != nil {
			fail(w, 401, "unauthorized")
			return
		}
		id, e := a.Identity.Session(r.Context(), c.Value)
		if e != nil {
			fail(w, 401, "unauthorized")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, id)))
	})
}
func user(r *http.Request) string        { v, _ := r.Context().Value(userKey).(string); return v }
func correlation(r *http.Request) string { v, _ := r.Context().Value(requestKey).(string); return v }
func workspace(r *http.Request) string   { return r.Header.Get("X-Workspace-ID") }
func handleError(w http.ResponseWriter, e error) {
	if errors.Is(e, database.ErrForbidden) {
		fail(w, 403, "forbidden")
		return
	}
	var denied *entitlements.Denied
	if errors.As(e, &denied) {
		fail(w, 403, "forbidden")
		return
	}
	if errors.Is(e, database.ErrConflict) {
		fail(w, 409, "conflict")
		return
	}
	if errors.Is(e, pgx.ErrNoRows) {
		fail(w, 404, "invalid")
		return
	}
	fail(w, 400, "invalid")
}
func (a *API) tenant(w http.ResponseWriter, r *http.Request, admin bool, fn func(pgx.Tx) (any, error)) {
	if admin && !a.DB.IsAdmin(r.Context(), user(r)) {
		fail(w, 403, "forbidden")
		return
	}
	var out any
	e := a.DB.Tenant(r.Context(), user(r), workspace(r), admin, func(tx pgx.Tx) error { var e error; out, e = fn(tx); return e })
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
func rows(ctx context.Context, tx pgx.Tx, query string, args ...any) ([]map[string]any, error) {
	rs, e := tx.Query(ctx, query, args...)
	if e != nil {
		return nil, e
	}
	defer rs.Close()
	out := []map[string]any{}
	for rs.Next() {
		vs, e := rs.Values()
		if e != nil {
			return nil, e
		}
		m := map[string]any{}
		for i, f := range rs.FieldDescriptions() {
			m[f.Name] = vs[i]
		}
		out = append(out, m)
	}
	return out, rs.Err()
}
func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if a.DB.Pool.Ping(ctx) != nil || a.Cache.Redis.Ping(ctx).Err() != nil || !a.Queue.Conn.IsConnected() {
		fail(w, 503, "unavailable")
		return
	}
	respond(w, 200, map[string]string{"status": "ready"})
}
func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		InitData string `json:"init_data"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	u, e := auth.ValidateInitData(in.InitData, a.Config.BotToken, time.Now(), 10*time.Minute)
	if e != nil {
		fail(w, 401, "unauthorized")
		return
	}
	out, token, e := a.Identity.Login(r.Context(), u, in.InitData)
	if e != nil {
		fail(w, 401, "unauthorized")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "telmivo_session", Value: token, Path: "/api/v1", HttpOnly: true, Secure: a.Config.Environment == "production", SameSite: http.SameSiteStrictMode, MaxAge: int(a.Config.SessionTTL.Seconds())})
	respond(w, 200, out)
}
func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	cookie, e := r.Cookie("telmivo_session")
	if e != nil {
		fail(w, 401, "unauthorized")
		return
	}
	_, e = a.DB.Pool.Exec(r.Context(), "DELETE FROM sessions WHERE token_hash=$1", auth.Digest(cookie.Value))
	if e != nil {
		fail(w, 503, "unavailable")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "telmivo_session", Path: "/api/v1", MaxAge: -1, HttpOnly: true, Secure: a.Config.Environment == "production", SameSite: http.SameSiteStrictMode})
	respond(w, 200, map[string]bool{"logged_out": true})
}
func (a *API) me(w http.ResponseWriter, r *http.Request) {
	var name string
	e := a.DB.Pool.QueryRow(r.Context(), "SELECT display_name FROM users WHERE id=$1", user(r)).Scan(&name)
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, map[string]any{"user_id": user(r), "name": name, "is_admin": a.DB.IsAdmin(r.Context(), user(r)), "synthetic_mode": a.Config.Synthetic})
}
func (a *API) workspaces(w http.ResponseWriter, r *http.Request) {
	tx, e := a.DB.Pool.Begin(r.Context())
	if e != nil {
		handleError(w, e)
		return
	}
	defer tx.Rollback(r.Context())
	out, e := rows(r.Context(), tx, "SELECT w.id::text,w.name,m.role_key FROM workspaces w JOIN workspace_members m ON m.workspace_id=w.id WHERE m.user_id=$1 ORDER BY w.created_at", user(r))
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
func (a *API) dashboard(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		plan, e := entitlements.EffectivePlan(r.Context(), tx, workspace(r))
		if e != nil {
			return nil, e
		}
		counts, e := rows(r.Context(), tx, "SELECT (SELECT count(*) FROM channels WHERE workspace_id=$1) AS channels,(SELECT count(*) FROM groups WHERE workspace_id=$1) AS groups,(SELECT count(*) FROM customer_bots WHERE workspace_id=$1) AS bots,(SELECT count(*) FROM tickets WHERE workspace_id=$1 AND status NOT IN('CLOSED','RESOLVED')) AS open_tickets,(SELECT count(*) FROM tickets WHERE workspace_id=$1 AND priority='URGENT' AND status NOT IN('CLOSED','RESOLVED')) AS urgent_tickets,(SELECT count(*) FROM workspace_members WHERE workspace_id=$1) AS members", workspace(r))
		return map[string]any{"plan": plan, "counts": counts}, e
	})
}
func (a *API) access(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		role, e := entitlements.Role(r.Context(), tx, user(r), workspace(r))
		if e != nil {
			return nil, e
		}
		return accessMap(r.Context(), tx, workspace(r), role)
	})
}
func accessMap(ctx context.Context, tx pgx.Tx, workspace, role string) (any, error) {
	fs, e := rows(ctx, tx, "SELECT key,permission_key FROM features ORDER BY key")
	if e != nil {
		return nil, e
	}
	out := map[string]entitlement.Decision{}
	for _, f := range fs {
		k := f["key"].(string)
		d, e := entitlements.Resolve(ctx, tx, workspace, role, k, "", f["permission_key"].(string), 0)
		if e != nil {
			return nil, e
		}
		out[k] = d
	}
	return out, nil
}
func (a *API) preview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Role string `json:"role"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, true, func(tx pgx.Tx) (any, error) { return accessMap(r.Context(), tx, workspace(r), in.Role) })
}
func (a *API) plans(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		ps, e := rows(r.Context(), tx, "SELECT key,name,active,version FROM plans ORDER BY key")
		if e != nil {
			return nil, e
		}
		es, e := rows(r.Context(), tx, "SELECT plan_key,feature_key,enabled,limit_value::text FROM plan_entitlements ORDER BY plan_key,feature_key")
		if e != nil {
			return nil, e
		}
		plugins, e := rows(r.Context(), tx, "SELECT plan_key,plugin_key,enabled FROM plan_plugin_entitlements ORDER BY plan_key,plugin_key")
		return map[string]any{"plans": ps, "entitlements": es, "plugins": plugins}, e
	})
}
func (a *API) features(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		return rows(r.Context(), tx, "SELECT f.key,f.name_key,ff.enabled,ff.rollout_status FROM features f JOIN feature_flags ff ON ff.feature_key=f.key ORDER BY f.key")
	})
}
func (a *API) plugins(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		catalog, e := rows(r.Context(), tx, "SELECT key,name_key,version,status,required_feature,configuration_schema FROM plugins ORDER BY key")
		return map[string]any{"catalog": catalog, "modules": botmodules.Defaults().List()}, e
	})
}
func (a *API) changePlan(w http.ResponseWriter, r *http.Request) {
	var in subscriptions.Change
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, true, func(tx pgx.Tx) (any, error) {
		e := subscriptions.ChangePlan(r.Context(), tx, user(r), workspace(r), correlation(r), in)
		return map[string]bool{"saved": e == nil}, e
	})
}
func (a *API) wallet(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		if !database.Permission(r.Context(), tx, user(r), workspace(r), "wallet.read") {
			return nil, database.ErrForbidden
		}
		return rows(r.Context(), tx, "SELECT asset,balance_atomic FROM wallet_balances WHERE workspace_id=$1 ORDER BY asset", workspace(r))
	})
}
func (a *API) adjust(w http.ResponseWriter, r *http.Request) {
	if !a.Config.Synthetic {
		fail(w, 501, "not_implemented")
		return
	}
	var in wallets.Adjustment
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, true, func(tx pgx.Tx) (any, error) {
		id, e := wallets.Adjust(r.Context(), tx, user(r), workspace(r), correlation(r), in)
		return map[string]string{"ledger_transaction_id": id}, e
	})
}
func (a *API) ledger(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		if !database.Permission(r.Context(), tx, user(r), workspace(r), "wallet.read") {
			return nil, database.ErrForbidden
		}
		return rows(r.Context(), tx, "SELECT t.id::text,t.asset,t.transaction_type,t.reason,t.created_at,e.amount_atomic::text FROM ledger_transactions t JOIN ledger_entries e ON e.transaction_id=t.id JOIN wallet_accounts a ON a.id=e.account_id WHERE t.workspace_id=$1 AND a.kind='WALLET' ORDER BY t.created_at DESC LIMIT 100", workspace(r))
	})
}
func (a *API) ticketList(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		if e := entitlements.Require(r.Context(), tx, user(r), workspace(r), "tickets", "tickets.read", 0); e != nil {
			return nil, e
		}
		return rows(r.Context(), tx, "SELECT id::text,subject,status,priority,creator_id::text,created_at FROM tickets WHERE workspace_id=$1 ORDER BY updated_at DESC LIMIT 100", workspace(r))
	})
}
func (a *API) ticketCreate(w http.ResponseWriter, r *http.Request) {
	var in tickets.CreateInput
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		id, e := tickets.Create(r.Context(), tx, user(r), workspace(r), correlation(r), in)
		return map[string]string{"id": id}, e
	})
}
func (a *API) ticketDetail(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		if e := entitlements.Require(r.Context(), tx, user(r), workspace(r), "tickets", "tickets.read", 0); e != nil {
			return nil, e
		}
		ts, e := rows(r.Context(), tx, "SELECT id::text,subject,status,priority,creator_id::text FROM tickets WHERE workspace_id=$1 AND id=$2", workspace(r), r.PathValue("ticket"))
		if e != nil {
			return nil, e
		}
		if len(ts) == 0 {
			return nil, pgx.ErrNoRows
		}
		ms, e := rows(r.Context(), tx, "SELECT id::text,body,actor_id::text,created_at FROM ticket_messages WHERE workspace_id=$1 AND ticket_id=$2 ORDER BY created_at", workspace(r), r.PathValue("ticket"))
		return map[string]any{"ticket": ts[0], "messages": ms}, e
	})
}
func (a *API) ticketReply(w http.ResponseWriter, r *http.Request) {
	var in tickets.ReplyInput
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		e := tickets.Reply(r.Context(), tx, user(r), workspace(r), r.PathValue("ticket"), correlation(r), in)
		return map[string]bool{"saved": e == nil}, e
	})
}
func (a *API) ticketAction(w http.ResponseWriter, r *http.Request) {
	var in tickets.ActionInput
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		e := tickets.Act(r.Context(), tx, user(r), workspace(r), r.PathValue("ticket"), correlation(r), in)
		return map[string]bool{"saved": e == nil}, e
	})
}
func (a *API) formats(w http.ResponseWriter, r *http.Request) {
	if !a.Config.Synthetic {
		fail(w, 501, "not_implemented")
		return
	}
	if _, e := downloads.Provider(r.URL.Query().Get("url")); e != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		if e := entitlements.Require(r.Context(), tx, user(r), workspace(r), "downloads", "downloads.create", 0); e != nil {
			return nil, e
		}
		return map[string]any{"synthetic": true, "formats": downloads.FixtureFormats()}, nil
	})
}
func (a *API) downloadList(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		if !database.Permission(r.Context(), tx, user(r), workspace(r), "downloads.read") {
			return nil, database.ErrForbidden
		}
		return rows(r.Context(), tx, "SELECT id::text,provider,state,synthetic,created_at FROM download_jobs WHERE workspace_id=$1 ORDER BY created_at DESC LIMIT 100", workspace(r))
	})
}
func (a *API) downloadCreate(w http.ResponseWriter, r *http.Request) {
	if !a.Config.Synthetic {
		fail(w, 501, "not_implemented")
		return
	}
	var in downloads.CreateInput
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		id, e := downloads.CreateSynthetic(r.Context(), tx, user(r), workspace(r), correlation(r), in)
		return map[string]any{"id": id, "synthetic": true}, e
	})
}
func (a *API) audit(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		if !database.Permission(r.Context(), tx, user(r), workspace(r), "audit.read") {
			return nil, database.ErrForbidden
		}
		return rows(r.Context(), tx, "SELECT id::text,action,entity,entity_id,actor_id::text,correlation_id,created_at FROM audit_logs WHERE workspace_id=$1 ORDER BY created_at DESC LIMIT 100", workspace(r))
	})
}
func (a *API) team(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		if !database.Permission(r.Context(), tx, user(r), workspace(r), "team.read") {
			return nil, database.ErrForbidden
		}
		return rows(r.Context(), tx, "SELECT m.user_id::text,u.display_name,m.role_key FROM workspace_members m JOIN users u ON u.id=m.user_id WHERE m.workspace_id=$1 ORDER BY m.created_at", workspace(r))
	})
}
func parseLimit(s *string) (*int64, error) {
	if s == nil {
		return nil, nil
	}
	n, e := strconv.ParseInt(*s, 10, 64)
	if e != nil || n < 0 {
		return nil, errors.New("invalid limit")
	}
	return &n, nil
}
