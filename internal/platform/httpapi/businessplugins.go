package httpapi

import (
	"github.com/naseri-coder/TELMIVO/internal/domains/businessplugins"
	"github.com/naseri-coder/TELMIVO/internal/plugins/broadcast"
	"github.com/naseri-coder/TELMIVO/internal/plugins/forms"
	"github.com/naseri-coder/TELMIVO/internal/plugins/shop"
	"net/http"
)

func (a *API) BusinessService() *businessplugins.Service {
	if a.Business != nil {
		return a.Business
	}
	i := a.IntelligenceService()
	return &businessplugins.Service{DB: a.DB, Core: i.Core, Commercial: i.Commercial, Support: i.Support, Content: i.Content, Clock: i.Clock, Fallback: i}
}
func (a *API) businessDefinitions(w http.ResponseWriter, r *http.Request) {
	v, e := a.BusinessService().Definitions(r.Context(), user(r), contentScope(r, r.PathValue("plugin")), r.URL.Query().Get("cursor"), r.URL.Query().Get("search"))
	supportReply(w, v, e)
}
func (a *API) businessSave(w http.ResponseWriter, r *http.Request) {
	s := a.BusinessService()
	scope := contentScope(r, r.PathValue("plugin"))
	id := r.PathValue("definition")
	switch scope.PluginID {
	case broadcast.ID:
		var in businessplugins.Change[broadcast.Campaign]
		if decode(w, r, &in) != nil {
			fail(w, 400, "invalid")
			return
		}
		v, e := s.SaveCampaign(r.Context(), user(r), scope, id, in)
		supportReply(w, v, e)
	case forms.ID:
		var in businessplugins.Change[forms.Form]
		if decode(w, r, &in) != nil {
			fail(w, 400, "invalid")
			return
		}
		v, e := s.SaveForm(r.Context(), user(r), scope, id, in)
		supportReply(w, v, e)
	case shop.ID:
		var in businessplugins.Change[shop.Product]
		if decode(w, r, &in) != nil {
			fail(w, 400, "invalid")
			return
		}
		v, e := s.SaveProduct(r.Context(), user(r), scope, id, in)
		supportReply(w, v, e)
	default:
		fail(w, 404, "not_found")
	}
}
func (a *API) businessPreview(w http.ResponseWriter, r *http.Request) {
	var in broadcast.Audience
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	v, e := a.BusinessService().PreviewAudience(r.Context(), user(r), contentScope(r, broadcast.ID), in)
	supportReply(w, v, e)
}
func (a *API) businessCampaign(w http.ResponseWriter, r *http.Request) {
	v, e := a.BusinessService().Campaign(r.Context(), user(r), contentScope(r, broadcast.ID), r.PathValue("campaign"))
	supportReply(w, v, e)
}
func (a *API) businessAction(w http.ResponseWriter, r *http.Request) {
	var in businessplugins.Action
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	s := a.BusinessService()
	switch r.PathValue("plugin") {
	case broadcast.ID:
		v, e := s.CampaignAction(r.Context(), user(r), contentScope(r, broadcast.ID), r.PathValue("resource"), in)
		supportReply(w, v, e)
	case shop.ID:
		v, e := s.OrderAction(r.Context(), user(r), contentScope(r, shop.ID), r.PathValue("resource"), in)
		supportReply(w, v, e)
	default:
		fail(w, 404, "not_found")
	}
}
func (a *API) businessRecipients(w http.ResponseWriter, r *http.Request) {
	v, e := a.BusinessService().Recipients(r.Context(), user(r), contentScope(r, broadcast.ID), r.PathValue("campaign"), r.URL.Query().Get("cursor"))
	supportReply(w, v, e)
}
func (a *API) businessSubmissions(w http.ResponseWriter, r *http.Request) {
	v, e := a.BusinessService().Submissions(r.Context(), user(r), contentScope(r, forms.ID), r.PathValue("form"), r.URL.Query().Get("cursor"))
	supportReply(w, v, e)
}
func (a *API) businessOrders(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	v, e := a.BusinessService().Orders(r.Context(), user(r), contentScope(r, shop.ID), q.Get("cursor"), q.Get("status"), q.Get("contact"))
	supportReply(w, v, e)
}
func (a *API) businessOrderHistory(w http.ResponseWriter, r *http.Request) {
	v, e := a.BusinessService().OrderHistory(r.Context(), user(r), contentScope(r, shop.ID), r.PathValue("order"), r.URL.Query().Get("cursor"))
	supportReply(w, v, e)
}

func (a *API) businessCampaignHistory(w http.ResponseWriter, r *http.Request) {
	v, e := a.BusinessService().CampaignHistory(r.Context(), user(r), contentScope(r, broadcast.ID), r.PathValue("campaign"), r.URL.Query().Get("cursor"))
	supportReply(w, v, e)
}


func (a *API) businessCheckout(w http.ResponseWriter, r *http.Request) {
	var in businessplugins.CheckoutRequest
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	v, e := a.BusinessService().Checkout(r.Context(), user(r), contentScope(r, shop.ID), r.PathValue("order"), correlation(r), in)
	supportReply(w, v, e)
}

func (a *API) businessPayments(w http.ResponseWriter, r *http.Request) {
	v, e := a.BusinessService().Payments(r.Context(), user(r), contentScope(r, shop.ID), r.URL.Query().Get("order"))
	supportReply(w, v, e)
}

func (a *API) businessPaymentHistory(w http.ResponseWriter, r *http.Request) {
	v, e := a.BusinessService().PaymentHistory(r.Context(), user(r), contentScope(r, shop.ID), r.PathValue("payment"))
	supportReply(w, v, e)
}

func (a *API) businessRefund(w http.ResponseWriter, r *http.Request) {
	var in businessplugins.RefundRequest
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	v, e := a.BusinessService().Refund(r.Context(), user(r), contentScope(r, shop.ID), r.PathValue("payment"), correlation(r), in)
	supportReply(w, v, e)
}

func (a *API) businessReconcile(w http.ResponseWriter, r *http.Request) {
	var in struct{}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	v, e := a.BusinessService().Reconcile(r.Context(), user(r), contentScope(r, shop.ID), r.PathValue("payment"), correlation(r))
	supportReply(w, v, e)
}
