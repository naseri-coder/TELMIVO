package httpapi

import (
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/botfactory"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"net/http"
)

func (a *API) storeList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	out, e := a.factory().Store(r.Context(), user(r), workspace(r), r.PathValue("bot"), botfactory.StoreFilter{Search: q.Get("search"), Category: q.Get("category"), Status: q.Get("status"), After: q.Get("cursor")})
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
func (a *API) storeDetail(w http.ResponseWriter, r *http.Request) {
	out, e := a.factory().StoreDetail(r.Context(), user(r), workspace(r), r.PathValue("bot"), r.PathValue("plugin"))
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
func (a *API) storeEffective(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Bot    string `json:"bot"`
		Plugin string `json:"plugin"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	if !a.factory().Registered(in.Plugin) {
		fail(w, 400, "invalid")
		return
	}
	out, e := a.commercial().Effective(r.Context(), user(r), workspace(r), in.Bot, in.Plugin)
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}

func (a *API) commercialAudit(w http.ResponseWriter, r *http.Request) {
	var out any
	e := a.DB.Tenant(r.Context(), user(r), workspace(r), true, func(tx pgx.Tx) error {
		if !database.PlatformPermission(r.Context(), tx, user(r), "platform.commercial.read") {
			return database.ErrForbidden
		}
		var e error
		out, e = rows(r.Context(), tx, "SELECT id::text,actor_id::text,action,entity,entity_id,correlation_id,created_at FROM audit_logs WHERE workspace_id=$1 AND action LIKE 'commercial.%' ORDER BY created_at DESC,id DESC LIMIT 100", workspace(r))
		return e
	})
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
