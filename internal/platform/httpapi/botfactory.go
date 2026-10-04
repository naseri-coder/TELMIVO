package httpapi

import (
	"encoding/base64"
	"github.com/naseri-coder/TELMIVO/internal/domains/botfactory"
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	platform "github.com/naseri-coder/TELMIVO/internal/domains/pluginplatform"
	cred "github.com/naseri-coder/TELMIVO/internal/platform/crypto"
	"net/http"
)

func (a *API) factory() *botfactory.Service {
	if a.Factory != nil {
		return a.Factory
	}
	registry, _ := contentplugins.Registry()
	s := &botfactory.Service{DB: a.DB, Registry: registry, Commercial: a.commercial()}
	if key, e := base64.StdEncoding.DecodeString(a.Config.CredentialKey); e == nil {
		s.Credentials, _ = cred.NewAESStore("primary-v1", key)
	}
	return s
}
func (a *API) factoryList(w http.ResponseWriter, r *http.Request) {
	out, e := a.factory().List(r.Context(), user(r), workspace(r))
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
func (a *API) factoryCatalog(w http.ResponseWriter, r *http.Request) {
	out, e := a.factory().Catalog(r.Context(), user(r), workspace(r), r.PathValue("bot"))
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
func (a *API) factoryPreview(w http.ResponseWriter, r *http.Request) {
	var in botfactory.Selection
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	out, e := a.factory().Preview(r.Context(), user(r), workspace(r), r.PathValue("bot"), in)
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
func (a *API) factoryCreate(w http.ResponseWriter, r *http.Request) {
	var in botfactory.Create
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	if a.factory().Credentials == nil {
		fail(w, 503, "unavailable")
		return
	}
	out, e := a.factory().Create(r.Context(), user(r), workspace(r), correlation(r), in)
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
func (a *API) factoryChange(w http.ResponseWriter, r *http.Request) {
	var in botfactory.Change
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	out, e := a.factory().Change(r.Context(), user(r), workspace(r), r.PathValue("bot"), r.PathValue("plugin"), correlation(r), in)
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
func (a *API) factoryImpact(w http.ResponseWriter, r *http.Request) {
	var in botfactory.Change
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	out, e := a.factory().Impact(r.Context(), user(r), workspace(r), r.PathValue("bot"), r.PathValue("plugin"), in)
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}

// Customers cannot supply duration/grant overrides. Reuse the controlled R3D-2 path.
func (a *API) factoryTrial(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Key string `json:"idempotency_key"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	s := a.factory()
	items, e := s.Catalog(r.Context(), user(r), workspace(r), r.PathValue("bot"))
	if e != nil {
		handleError(w, e)
		return
	}
	for _, item := range items {
		if item.ID == r.PathValue("plugin") {
			bot := r.PathValue("bot")
			if item.Scope == "PER_ACCOUNT" {
				bot = ""
			}
			_, e = s.Commercial.StartTrial(r.Context(), user(r), workspace(r), correlation(r), in.Key, item.ID, bot)
			if e != nil {
				handleError(w, e)
				return
			}
			respond(w, 200, map[string]bool{"started": true})
			return
		}
	}
	fail(w, 400, "invalid")
}

// SetFactoryRegistry is trusted startup injection; customer APIs cannot register code.
func (a *API) SetFactoryRegistry(registry *platform.Registry) {
	s := a.factory()
	s.Registry = registry
	a.Factory = s
}
