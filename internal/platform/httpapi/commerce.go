package httpapi

import (
	"net/http"

	"github.com/naseri-coder/TELMIVO/internal/domains/commerce"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
)

func (a *API) commercial() *commerce.Service { return &commerce.Service{DB: a.DB} }
func (a *API) commercialCatalog(w http.ResponseWriter, r *http.Request) {
	out, e := a.commercial().Catalog(r.Context(), user(r), workspace(r))
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
func (a *API) commercialSave(w http.ResponseWriter, r *http.Request) {
	var in commerce.CatalogChange
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	out, e := a.commercial().SaveCatalog(r.Context(), user(r), workspace(r), correlation(r), in)
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
func (a *API) commercialIssue(w http.ResponseWriter, r *http.Request) {
	var in commerce.Issue
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	out, e := a.commercial().Issue(r.Context(), user(r), workspace(r), correlation(r), in)
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
func (a *API) commercialAgreements(w http.ResponseWriter, r *http.Request) {
	out, e := a.commercial().Agreements(r.Context(), user(r), workspace(r))
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
func (a *API) commercialState(w http.ResponseWriter, r *http.Request) {
	var in struct {
		State    commerce.State `json:"state"`
		Expected int64          `json:"expected_revision"`
		Reason   string         `json:"reason"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	out, e := a.commercial().SetState(r.Context(), user(r), workspace(r), correlation(r), r.PathValue("agreement"), in.Expected, in.State, in.Reason)
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
func (a *API) commercialDecision(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Bot    string `json:"bot"`
		Plugin string `json:"plugin"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	out, e := a.commercial().Decide(r.Context(), user(r), sdk.Scope{AccountID: workspace(r), BotID: in.Bot, PluginID: in.Plugin})
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
func (a *API) commercialTrial(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Key    string `json:"key"`
		Plugin string `json:"plugin"`
		Bot    string `json:"bot"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	out, e := a.commercial().StartTrial(r.Context(), user(r), workspace(r), correlation(r), in.Key, in.Plugin, in.Bot)
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
func (a *API) commercialQuota(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Bot    string `json:"bot"`
		Metric string `json:"metric"`
		Amount int64  `json:"amount,string"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	out, e := a.commercial().Check(r.Context(), user(r), workspace(r), in.Bot, in.Metric, in.Amount)
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}

func (a *API) commercialHistory(w http.ResponseWriter, r *http.Request) {
	out, e := a.commercial().History(r.Context(), user(r), workspace(r), commerce.Kind(r.PathValue("kind")), r.PathValue("key"))
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
