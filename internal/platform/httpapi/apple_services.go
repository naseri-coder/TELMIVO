package httpapi

import (
	"net/http"

	"github.com/naseri-coder/TELMIVO/internal/domains/appleservices"
)

func (a *API) AppleService() *appleservices.Service {
	if a.Apple != nil {
		return a.Apple
	}
	return &appleservices.Service{DB: a.DB}
}

func (a *API) appleProfile(w http.ResponseWriter, r *http.Request) {
	v, e := a.AppleService().Profile(r.Context(), user(r), workspace(r))
	supportReply(w, v, e)
}

func (a *API) appleProfileEnsure(w http.ResponseWriter, r *http.Request) {
	var in struct{}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	v, e := a.AppleService().EnsureProfile(r.Context(), user(r), workspace(r), correlation(r))
	supportReply(w, v, e)
}

func (a *API) appleProfileAction(w http.ResponseWriter, r *http.Request) {
	var in appleservices.ProfileAction
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	v, e := a.AppleService().SetProfileState(r.Context(), user(r), workspace(r), correlation(r), in)
	supportReply(w, v, e)
}

func (a *API) appleOperations(w http.ResponseWriter, r *http.Request) {
	v, e := a.AppleService().Operations(r.Context(), user(r), workspace(r))
	supportReply(w, v, e)
}

func (a *API) appleOperationCreate(w http.ResponseWriter, r *http.Request) {
	var in appleservices.CreateRequest
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	v, e := a.AppleService().CreateOperation(r.Context(), user(r), workspace(r), correlation(r), in)
	supportReply(w, v, e)
}

func (a *API) appleOperationAction(w http.ResponseWriter, r *http.Request) {
	var in appleservices.TransitionRequest
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	v, e := a.AppleService().Transition(r.Context(), user(r), workspace(r), r.PathValue("operation"), correlation(r), in)
	supportReply(w, v, e)
}

func (a *API) appleOperationEvents(w http.ResponseWriter, r *http.Request) {
	v, e := a.AppleService().Events(r.Context(), user(r), workspace(r), r.PathValue("operation"))
	supportReply(w, v, e)
}
