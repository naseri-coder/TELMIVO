package httpapi

import (
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/downloads"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"net/http"
)

func (a *API) downloaderProduct() *downloads.Product {
	return &downloads.Product{DB: a.DB, Synthetic: a.Config.Synthetic, MaxBytes: a.Config.MaxMediaBytes}
}
func (a *API) downloaderSubmit(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL string `json:"url"`
		Key string `json:"idempotency_key"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		id, e := a.downloaderProduct().Submit(r.Context(), tx, user(r), workspace(r), correlation(r), in.URL, in.Key)
		return map[string]string{"id": id}, e
	})
}
func (a *API) downloaderDetail(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		return a.downloaderProduct().Detail(r.Context(), tx, user(r), workspace(r), r.PathValue("job"), false)
	})
}
func (a *API) downloaderSelect(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID string `json:"format_id"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		e := a.downloaderProduct().Select(r.Context(), tx, user(r), workspace(r), r.PathValue("job"), in.ID)
		return map[string]bool{"accepted": e == nil}, e
	})
}
func (a *API) downloaderCancel(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		e := a.downloaderProduct().Cancel(r.Context(), tx, user(r), workspace(r), r.PathValue("job"), correlation(r))
		return map[string]bool{"requested": e == nil}, e
	})
}
func (a *API) downloaderHistory(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		if !database.Permission(r.Context(), tx, user(r), workspace(r), "downloads.read") {
			return nil, database.ErrForbidden
		}
		jobs, next, e := a.downloaderProduct().History(r.Context(), tx, user(r), workspace(r), r.URL.Query().Get("cursor"))
		return map[string]any{"jobs": jobs, "next_cursor": next}, e
	})
}
