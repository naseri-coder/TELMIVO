package httpapi

import (
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/storage"
	"io"
	"net/http"
)

// Read-only foundation endpoint. Product ingestion/Put flows remain deferred.
func (a *API) storageOpen(w http.ResponseWriter, r *http.Request) {
	if a.Storage == nil {
		fail(w, 501, "not_implemented")
		return
	}
	var body io.ReadCloser
	var m storage.Metadata
	e := a.DB.Tenant(r.Context(), user(r), workspace(r), false, func(tx pgx.Tx) error {
		if !database.Permission(r.Context(), tx, user(r), workspace(r), "uploads.read") {
			return database.ErrForbidden
		}
		key := r.PathValue("owner") + "/" + r.PathValue("object")
		var e error
		m, e = a.Storage.Metadata(r.Context(), workspace(r), key)
		if e != nil {
			return e
		}
		body, e = a.Storage.Open(r.Context(), workspace(r), key)
		return e
	})
	if errors.Is(e, storage.ErrDenied) {
		fail(w, 403, "forbidden")
		return
	}
	if e != nil {
		handleError(w, e)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", m.ContentType)
	w.Header().Set("Content-Disposition", "attachment")
	w.WriteHeader(200)
	_, _ = io.Copy(w, body)
}
