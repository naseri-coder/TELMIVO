package httpapi

import (
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	platform "github.com/naseri-coder/TELMIVO/internal/domains/pluginplatform"
	"github.com/naseri-coder/TELMIVO/internal/domains/supportplugins"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/testfixtures"
	"github.com/naseri-coder/TELMIVO/internal/plugins/crm"
	"net/http"
	"strconv"
)

func (a *API) support() *supportplugins.Service {
	if a.Support != nil {
		return a.Support
	}
	f := a.factory()
	var provider contentplugins.Provider = contentProvider{f.Credentials, !a.Config.Synthetic}
	if a.Config.ContentTestFixture && a.Config.Environment == "test" && a.Config.Synthetic {
		provider = testfixtures.ContentProvider{}
	}
	return &supportplugins.Service{DB: a.DB, Core: &platform.Service{DB: a.DB, Registry: f.Registry, Policy: f.Commercial, Hooks: f.Hooks}, Commercial: f.Commercial, Provider: provider, Clock: f.Commercial.Clock}
}
func supportFilter(r *http.Request) supportplugins.Filter {
	q := r.URL.Query()
	limit := 25
	if q.Get("limit") != "" {
		limit, _ = strconv.Atoi(q.Get("limit"))
	}
	return supportplugins.Filter{Search: q.Get("search"), Tag: q.Get("tag"), Status: q.Get("status"), Priority: q.Get("priority"), Department: q.Get("department"), Assignee: q.Get("assignee"), Contact: q.Get("contact"), Cursor: q.Get("cursor"), Limit: limit}
}
func supportReply(w http.ResponseWriter, v any, e error) {
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, v)
}
func (a *API) supportContacts(w http.ResponseWriter, r *http.Request) {
	v, e := a.support().Contacts(r.Context(), user(r), contentScope(r, crm.ID), supportFilter(r))
	supportReply(w, v, e)
}
func (a *API) supportContact(w http.ResponseWriter, r *http.Request) {
	v, e := a.support().Contact(r.Context(), user(r), contentScope(r, crm.ID), r.PathValue("contact"))
	supportReply(w, v, e)
}
func (a *API) supportContactCreate(w http.ResponseWriter, r *http.Request) {
	var in crm.ResolveRequest
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	v, e := a.support().Resolve(r.Context(), user(r), contentScope(r, crm.ID), in)
	supportReply(w, v, e)
}
func (a *API) supportContactSave(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Contact  crm.Contact `json:"contact"`
		Expected int64       `json:"expected_revision"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	v, e := a.support().SaveContact(r.Context(), user(r), contentScope(r, crm.ID), r.PathValue("contact"), in.Contact, in.Expected)
	supportReply(w, v, e)
}
func (a *API) supportTickets(w http.ResponseWriter, r *http.Request) {
	v, e := a.support().Tickets(r.Context(), user(r), contentScope(r, "ticketing"), supportFilter(r))
	supportReply(w, v, e)
}
func (a *API) supportTicket(w http.ResponseWriter, r *http.Request) {
	v, e := a.support().Detail(r.Context(), user(r), contentScope(r, "ticketing"), r.PathValue("ticket"), r.URL.Query().Get("cursor"))
	supportReply(w, v, e)
}
func (a *API) supportAction(w http.ResponseWriter, r *http.Request) {
	var in supportplugins.Action
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	v, e := a.support().Act(r.Context(), user(r), contentScope(r, "ticketing"), r.PathValue("ticket"), in)
	supportReply(w, v, e)
}
func (a *API) supportMessage(w http.ResponseWriter, r *http.Request) {
	var in supportplugins.Reply
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	v, e := a.support().Reply(r.Context(), user(r), contentScope(r, "ticketing"), r.PathValue("ticket"), in)
	supportReply(w, v, e)
}
func (a *API) supportDepartments(w http.ResponseWriter, r *http.Request) {
	v, e := a.support().Departments(r.Context(), user(r), contentScope(r, "ticketing"))
	supportReply(w, v, e)
}
func (a *API) supportDepartmentSave(w http.ResponseWriter, r *http.Request) {
	var in supportplugins.DepartmentChange
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	v, e := a.support().SaveDepartment(r.Context(), user(r), contentScope(r, "ticketing"), r.PathValue("department"), in)
	supportReply(w, v, e)
}
func (a *API) supportTemplates(w http.ResponseWriter, r *http.Request) {
	v, e := a.support().Templates(r.Context(), user(r), contentScope(r, "ticketing"))
	supportReply(w, v, e)
}
func (a *API) supportTemplateSave(w http.ResponseWriter, r *http.Request) {
	var in supportplugins.TemplateChange
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	v, e := a.support().SaveTemplate(r.Context(), user(r), contentScope(r, "ticketing"), r.PathValue("template"), in)
	supportReply(w, v, e)
}
func (a *API) supportAgents(w http.ResponseWriter, r *http.Request) {
	v, e := a.support().Agents(r.Context(), user(r), contentScope(r, "ticketing"))
	supportReply(w, v, e)
}
func (a *API) supportBots(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		if !database.Permission(r.Context(), tx, user(r), workspace(r), "tickets.read") && !database.Permission(r.Context(), tx, user(r), workspace(r), "crm.read") {
			return nil, database.ErrForbidden
		}
		rs, e := tx.Query(r.Context(), "SELECT b.id::text,b.name FROM customer_bots b WHERE b.workspace_id=$1 AND EXISTS(SELECT 1 FROM plugin_installations p WHERE p.bot_id=b.id AND p.workspace_id=b.workspace_id AND p.plugin_id IN('crm','ticketing') AND p.state='ACTIVE') ORDER BY b.id LIMIT 100", workspace(r))
		if e != nil {
			return nil, e
		}
		defer rs.Close()
		out := []map[string]string{}
		for rs.Next() {
			var id, name string
			if e = rs.Scan(&id, &name); e != nil {
				return nil, e
			}
			out = append(out, map[string]string{"id": id, "name": name})
		}
		return out, rs.Err()
	})
}
