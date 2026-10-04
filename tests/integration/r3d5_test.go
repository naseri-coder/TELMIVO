//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/commerce"
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	"github.com/naseri-coder/TELMIVO/internal/domains/identity"
	platform "github.com/naseri-coder/TELMIVO/internal/domains/pluginplatform"
	"github.com/naseri-coder/TELMIVO/internal/domains/supportplugins"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/plugins/crm"
	tickets "github.com/naseri-coder/TELMIVO/internal/plugins/ticketing"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type r3d5Transport struct {
	r3d4Transport
	replies atomic.Int64
	outcome string
	last    string
}

func (t *r3d5Transport) SupportReply(_ context.Context, _ int64, text string, _ []tickets.Attachment) (int64, string) {
	t.replies.Add(1)
	t.last = text
	if t.outcome != "" {
		return 0, t.outcome
	}
	return 101, "DELIVERY_ACKNOWLEDGED"
}

type r3d5Provider struct{ tr *r3d5Transport }

func (p r3d5Provider) ForBot(context.Context, pgx.Tx, sdk.Scope) (contentplugins.Transport, error) {
	return p.tr, nil
}

type r3d5FixtureData struct {
	commercialFixture
	srv        *supportplugins.Service
	core       *platform.Service
	transport  *r3d5Transport
	agent      identity.Result
	department tickets.Department
}

func r3d5Fixture(t *testing.T) r3d5FixtureData {
	t.Helper()
	f := r3d2Fixture(t)
	f.clock.at = time.Now().UTC().Truncate(time.Microsecond)
	r, e := contentplugins.Registry()
	must(t, e)
	for _, id := range []string{crm.ID, tickets.ID} {
		m, _ := r.Manifest(id)
		f.save(t, commerce.Version{Kind: commerce.PluginKind, Key: id, Definition: commerce.Definition{Plugin: &commerce.Plugin{Scope: sdk.PerBot, SaleEnabled: true, Capabilities: m.Permissions, Prices: r3d2Prices(20)}}}, factoryVersion(t, f.s, f.admin, f.u, commerce.PluginKind, id))
	}
	c, e := f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
	must(t, e)
	p, _ := c.Get(commerce.PlanKind, f.plan)
	p.Definition.Plan.Plugins = map[string]commerce.Inclusion{crm.ID: commerce.Included, tickets.ID: commerce.Included}
	f.save(t, p, p.Version)
	core := &platform.Service{DB: db, Registry: r, Policy: f.s}
	tr := &r3d5Transport{}
	out := r3d5FixtureData{commercialFixture: f, core: core, transport: tr}
	out.srv = &supportplugins.Service{DB: db, Core: core, Commercial: f.s, Provider: r3d5Provider{tr}, Clock: f.clock}
	for _, id := range []string{crm.ID, tickets.ID} {
		scope := out.scope(id)
		i, e := core.Change(ctx, f.u.UserID, scope, sdk.Install, 0, json.RawMessage(`{}`), "r3d5")
		must(t, e)
		_, e = core.Change(ctx, f.u.UserID, scope, sdk.Activate, i.Revision, nil, "r3d5")
		must(t, e)
	}
	out.agent = fixture(t, false)
	tenant(t, f.u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "INSERT INTO workspace_members(workspace_id,user_id,role_key) VALUES($1,$2,'SUPPORT_AGENT')", f.u.WorkspaceID, out.agent.UserID)
		return e
	})
	out.department, e = out.srv.SaveDepartment(ctx, out.agent.UserID, out.scope(tickets.ID), "", supportplugins.DepartmentChange{Department: tickets.Department{Name: "General", Enabled: true, ResponseSeconds: 60, ResolutionSeconds: 3600}, Key: "general"})
	must(t, e)
	return out
}
func (f r3d5FixtureData) scope(id string) sdk.Scope {
	return sdk.Scope{AccountID: f.u.WorkspaceID, BotID: f.bot, PluginID: id}
}
func (f r3d5FixtureData) inbound(t *testing.T, key, body string) tickets.Ticket {
	t.Helper()
	v, e := f.srv.Inbound(ctx, f.scope(tickets.ID), tickets.Inbound{User: 10001, Name: "Synthetic customer", Username: "synthetic", Locale: "fa", Body: body, Key: key})
	must(t, e)
	return v
}
func TestR3D5CRM(t *testing.T) {
	f := r3d5Fixture(t)
	scope := f.scope(crm.ID)
	r := crm.ResolveRequest{Identity: crm.Identity{Provider: "telegram", Key: "10001"}, Name: "Customer"}
	a, e := f.srv.Resolve(ctx, f.agent.UserID, scope, r)
	must(t, e)
	b, e := f.srv.Resolve(ctx, f.agent.UserID, scope, r)
	must(t, e)
	if a.ID != b.ID {
		t.Fatal("duplicate contact")
	}
	b.Tags = []string{"vip", "customer"}
	b.Notes = []string{"Internal CRM note"}
	b.Fields = map[string]string{"company": "Synthetic"}
	b, e = f.srv.SaveContact(ctx, f.agent.UserID, scope, b.ID, b, b.Revision)
	must(t, e)
	if b.Fields["company"] != "Synthetic" || len(b.Tags) != 2 {
		t.Fatal(b)
	}
	b.Tags = []string{"customer"}
	b, e = f.srv.SaveContact(ctx, f.agent.UserID, scope, b.ID, b, b.Revision)
	must(t, e)
	if _, e = f.srv.SaveContact(ctx, f.agent.UserID, scope, b.ID, b, 1); !errors.Is(e, database.ErrConflict) {
		t.Fatal("stale", e)
	}
	r.Identity.Key = "10002"
	r.Name = "Customer two"
	_, e = f.srv.Resolve(ctx, f.agent.UserID, scope, r)
	must(t, e)
	p, e := f.srv.Contacts(ctx, f.agent.UserID, scope, supportplugins.Filter{Search: "Customer", Limit: 1})
	must(t, e)
	if len(p.Items) != 1 || p.Next == "" {
		t.Fatal(p)
	}
	p2, e := f.srv.Contacts(ctx, f.agent.UserID, scope, supportplugins.Filter{Search: "Customer", Limit: 1, Cursor: p.Next})
	must(t, e)
	if len(p2.Items) != 1 || p2.Items[0].ID == p.Items[0].ID {
		t.Fatal("pagination")
	}
	p, e = f.srv.Contacts(ctx, f.agent.UserID, scope, supportplugins.Filter{Tag: "customer", Limit: 25})
	must(t, e)
	if len(p.Items) != 1 {
		t.Fatal(p)
	}
	other := fixture(t, false)
	if _, e = f.srv.Contact(ctx, other.UserID, scope, b.ID); !errors.Is(e, database.ErrForbidden) {
		t.Fatal(e)
	}
}
func TestR3D5Journey(t *testing.T) {
	f := r3d5Fixture(t)
	update := contentplugins.Update{ID: 555, Message: &contentplugins.Message{Text: "I need help"}}
	update.Message.From.ID = 10001
	update.Message.From.Name = "Synthetic customer"
	update.Message.From.Username = "synthetic"
	update.Message.From.Locale = "fa"
	update.Message.Chat.ID = 10001
	update.Message.Chat.Type = "private"
	broker := &contentplugins.Service{DB: db, Core: f.core, Commercial: f.s, Support: f.srv}
	must(t, broker.HandleUpdate(ctx, f.scope(tickets.ID), "r3d5", update))
	x := f.inbound(t, "support-tg:555", "I need help")
	same := f.inbound(t, "support-tg:555", "I need help")
	if x.Reference != same.Reference {
		t.Fatal("update duplicate")
	}
	detail, e := f.srv.Detail(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, "")
	must(t, e)
	if len(detail.Messages.Items) != 1 || detail.Contact.ID != x.ContactID {
		t.Fatal(detail)
	}
	x, e = f.srv.Act(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, supportplugins.Action{Kind: "assign", Value: f.agent.UserID, Expected: x.Revision})
	must(t, e)
	x, e = f.srv.Act(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, supportplugins.Action{Kind: "priority", Value: "URGENT", Expected: x.Revision})
	must(t, e)
	if x.Priority != "URGENT" || x.Assignee != f.agent.UserID {
		t.Fatal(x)
	}
	r, e := f.srv.Reply(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, supportplugins.Reply{Body: "Never sent externally", Internal: true, Key: "internal-one", Expected: x.Revision})
	must(t, e)
	if r.Delivery != "NOT_APPLICABLE" || f.transport.replies.Load() != 0 {
		t.Fatal("internal note leaked")
	}
	detail, e = f.srv.Detail(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, "")
	must(t, e)
	x = detail.Ticket
	reply := supportplugins.Reply{Body: "How can we help?", Key: "agent-one", Expected: x.Revision}
	r, e = f.srv.Reply(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, reply)
	must(t, e)
	if r.Delivery != "SENT" || f.transport.last != reply.Body {
		t.Fatal(r)
	}
	r, e = f.srv.Reply(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, reply)
	must(t, e)
	if !r.Replayed || f.transport.replies.Load() != 1 {
		t.Fatal("duplicate reply")
	}
	x = f.inbound(t, "inbound-two", "More information")
	if x.Reference != same.Reference || x.FirstResponse == nil {
		t.Fatal("active ticket or SLA", x)
	}
	x, e = f.srv.Act(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, supportplugins.Action{Kind: "status", Value: "RESOLVED", Expected: x.Revision})
	must(t, e)
	x, e = f.srv.Act(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, supportplugins.Action{Kind: "status", Value: "CLOSED", Expected: x.Revision})
	must(t, e)
	if x.Closed == nil {
		t.Fatal("closed timestamp")
	}
	if _, e = f.srv.Act(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, supportplugins.Action{Kind: "status", Value: "OPEN", Expected: x.Revision}); !errors.Is(e, database.ErrInvalid) {
		t.Fatal("implicit reopen")
	}
	x, e = f.srv.Act(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, supportplugins.Action{Kind: "reopen", Value: "OPEN", Expected: x.Revision})
	must(t, e)
	if x.Closed != nil {
		t.Fatal("reopen")
	}
	f.clock.at = f.clock.at.Add(2 * time.Hour)
	detail, e = f.srv.Detail(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, "")
	must(t, e)
	if !detail.Ticket.ResolutionBreached {
		t.Fatal("SLA clock")
	}
}
func TestR3D5FailuresAndSecurity(t *testing.T) {
	f := r3d5Fixture(t)
	x := f.inbound(t, "first", "Attachments")
	other := fixture(t, false)
	if _, e := f.srv.Act(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, supportplugins.Action{Kind: "assign", Value: other.UserID, Expected: x.Revision}); !errors.Is(e, database.ErrForbidden) {
		t.Fatal("cross agent", e)
	}
	if _, e := f.srv.Act(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, supportplugins.Action{Kind: "assign", Value: "00000000-0000-0000-0000-000000000001", Expected: x.Revision}); !errors.Is(e, database.ErrForbidden) {
		t.Fatal("unknown agent", e)
	}
	if _, e := f.srv.Act(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, supportplugins.Action{Kind: "status", Value: "CLOSED", Expected: 1}); !errors.Is(e, database.ErrConflict) {
		t.Fatal("stale", e)
	}
	if _, e := f.srv.Inbound(ctx, f.scope(tickets.ID), tickets.Inbound{User: 10001, Name: "Synthetic", Body: "Changed", Key: "first"}); !errors.Is(e, database.ErrConflict) {
		t.Fatal("changed retry", e)
	}
	if _, e := f.srv.Inbound(ctx, f.scope(tickets.ID), tickets.Inbound{User: 10001, Name: "Synthetic", Body: "bad", Key: "bad", Attachments: []tickets.Attachment{{FileID: "bad url", Kind: "document"}}}); !errors.Is(e, database.ErrInvalid) {
		t.Fatal("invalid attachment", e)
	}
	f.transport.outcome = "DELIVERY_FAILED"
	r, e := f.srv.Reply(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, supportplugins.Reply{Body: "Failed delivery", Key: "failed", Expected: x.Revision})
	must(t, e)
	if r.Delivery != "FAILED" {
		t.Fatal(r)
	}
	d, e := f.srv.Detail(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, "")
	must(t, e)
	f.transport.outcome = "DELIVERY_OUTCOME_UNKNOWN"
	reply := supportplugins.Reply{Body: "Unknown", Key: "unknown", Expected: d.Ticket.Revision}
	_, e = f.srv.Reply(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, reply)
	must(t, e)
	r, e = f.srv.Reply(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, reply)
	must(t, e)
	if !r.Replayed || f.transport.replies.Load() != 2 {
		t.Fatal("unknown resent")
	}
	scope := f.scope(tickets.ID)
	scope.BotID = f.other
	if _, e = f.srv.Detail(ctx, f.agent.UserID, scope, x.Reference, ""); e == nil {
		t.Fatal("bot isolation")
	}
	if _, e = f.srv.Tickets(ctx, other.UserID, f.scope(tickets.ID), supportplugins.Filter{Limit: 25}); !errors.Is(e, database.ErrForbidden) {
		t.Fatal("tenant search")
	}
	if _, e = f.core.Change(ctx, f.u.UserID, f.scope(crm.ID), sdk.Disable, 2, nil, "r3d5"); !errors.Is(e, sdk.ErrDependency) {
		t.Fatal("disabled CRM dependency", e)
	}
}
func TestR3D5QuotaAndRetention(t *testing.T) {
	f := r3d5Fixture(t)
	c, e := f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
	must(t, e)
	v, _ := c.Get(commerce.QuotaKind, "ticketing.messages.monthly")
	v.Definition.Quota.Default = commerce.Limit{Value: 0}
	f.save(t, v, v.Version)
	_, e = f.srv.Inbound(ctx, f.scope(tickets.ID), tickets.Inbound{User: 10001, Name: "Quota", Body: "Blocked", Key: "quota"})
	var q *contentplugins.QuotaDenied
	if !errors.As(e, &q) {
		t.Fatal(e)
	}
	if number(t, f.u, "SELECT count(*) FROM support_tickets WHERE bot_id=$1", f.bot) != 0 || number(t, f.u, "SELECT count(*) FROM crm_contacts WHERE bot_id=$1", f.bot) != 0 {
		t.Fatal("quota partial state")
	}
	v.Definition.Quota.Default = commerce.Limit{Unlimited: true}
	f.save(t, v, v.Version+1)
	x := f.inbound(t, "valid", "Retained")
	c, e = f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
	must(t, e)
	plan, _ := c.Get(commerce.PlanKind, f.plan)
	plan.Definition.Plan.Plugins[tickets.ID] = commerce.PaidAddon
	f.save(t, plan, plan.Version)
	_, e = f.srv.Inbound(ctx, f.scope(tickets.ID), tickets.Inbound{User: 10001, Name: "Quota", Body: "No entitlement", Key: "expired"})
	if !errors.Is(e, sdk.ErrPermission) {
		t.Fatal(e)
	}
	_, e = f.core.Change(ctx, f.u.UserID, f.scope(tickets.ID), sdk.Disable, 2, nil, "r3d5")
	must(t, e)
	_, e = f.core.Change(ctx, f.u.UserID, f.scope(crm.ID), sdk.Disable, 2, nil, "r3d5")
	must(t, e)
	if number(t, f.u, "SELECT count(*) FROM support_tickets WHERE reference=$1", x.Reference) != 1 || number(t, f.u, "SELECT count(*) FROM support_messages WHERE ticket_reference=$1", x.Reference) != 1 || number(t, f.u, "SELECT count(*) FROM crm_contacts WHERE bot_id=$1", f.bot) != 1 {
		t.Fatal("retention")
	}
}
func TestR3D5ConfigurationAndAttachments(t *testing.T) {
	f := r3d5Fixture(t)
	a := tickets.Attachment{FileID: "synthetic-document", UniqueID: "unique", Kind: "document", Title: "Attachment"}
	x, e := f.srv.Inbound(ctx, f.scope(tickets.ID), tickets.Inbound{User: 10001, Name: "Synthetic", Body: "Attached", Key: "attachment", Attachments: []tickets.Attachment{a}})
	must(t, e)
	d, e := f.srv.Detail(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, "")
	must(t, e)
	if len(d.Messages.Items[0].Attachments) != 1 {
		t.Fatal("attachment lost")
	}
	template, e := f.srv.SaveTemplate(ctx, f.agent.UserID, f.scope(tickets.ID), "", supportplugins.TemplateChange{Template: tickets.Template{Name: "Welcome", Body: "Prepared reply", Enabled: true}, Key: "template"})
	must(t, e)
	r, e := f.srv.Reply(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, supportplugins.Reply{Template: template.ID, Key: "canned", Expected: x.Revision})
	must(t, e)
	if r.Delivery != "SENT" || f.transport.last != "Prepared reply" {
		t.Fatal(r)
	}
	deps, e := f.srv.Departments(ctx, f.agent.UserID, f.scope(tickets.ID))
	must(t, e)
	if len(deps) != 1 {
		t.Fatal(deps)
	}
	f.department.Enabled = false
	_, e = f.srv.SaveDepartment(ctx, f.agent.UserID, f.scope(tickets.ID), f.department.ID, supportplugins.DepartmentChange{Department: f.department, Key: "disable-department", Expected: 1})
	must(t, e)
	if _, e = f.srv.Inbound(ctx, f.scope(tickets.ID), tickets.Inbound{User: 10002, Name: "No department", Body: "Help", Key: "disabled-dep"}); !errors.Is(e, database.ErrInvalid) {
		t.Fatal(e)
	}
}
func TestR3D5BrowserFixture(t *testing.T) {
	f := r3d5Fixture(t)
	u, _ := r3aLogin(t, 98005001, false)
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE subscriptions SET plan_key=$1,expires_at=NULL WHERE workspace_id=$2", f.plan, u.WorkspaceID)
		return e
	})
	bot := r3dBot(t, u, "r3d5-browser")
	scope := sdk.Scope{AccountID: u.WorkspaceID, BotID: bot}
	for _, id := range []string{crm.ID, tickets.ID} {
		scope.PluginID = id
		i, e := f.core.Change(ctx, u.UserID, scope, sdk.Install, 0, json.RawMessage(`{}`), "r3d5")
		must(t, e)
		_, e = f.core.Change(ctx, u.UserID, scope, sdk.Activate, i.Revision, nil, "r3d5")
		must(t, e)
	}
	_, e := f.srv.SaveDepartment(ctx, u.UserID, scope, "", supportplugins.DepartmentChange{Department: tickets.Department{Name: "Browser support", Enabled: true}, Key: "browser-dep"})
	must(t, e)
	for index, key := range []string{"390", "1280"} {
		_, e = f.srv.Inbound(ctx, scope, tickets.Inbound{User: 98005100 + int64(index), Name: "Browser customer " + key, Body: "Browser help " + key, Key: "browser-" + key, Department: ""})
		must(t, e)
	}
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE customer_bots SET name='R3D5 browser bot' WHERE id=$1", bot)
		return e
	})
}
func TestR3D5RecoveryPreparation(t *testing.T) {
	f := r3d5Fixture(t)
	f.inbound(t, "r3d5-recovery", "R3D5 retained history")
}
func TestR3D5Recovered(t *testing.T) {
	old := db
	var e error
	db, e = database.Open(ctx, os.Getenv("TEST_RESTORE_DATABASE_URL"), true)
	must(t, e)
	defer func() { db.Pool.Close(); db = old }()
	var account, actor, bot, ref string
	must(t, owner.Pool.QueryRow(ctx, `SELECT m.workspace_id::text,w.user_id::text,m.bot_id::text,m.ticket_reference FROM support_messages m JOIN workspace_members w ON w.workspace_id=m.workspace_id AND w.role_key='OWNER' WHERE m.operation_key='r3d5-recovery' LIMIT 1`).Scan(&account, &actor, &bot, &ref))
	r, e := contentplugins.Registry()
	must(t, e)
	s := &commerce.Service{DB: db, Clock: &r3d2Clock{at: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)}}
	srv := &supportplugins.Service{DB: db, Core: &platform.Service{DB: db, Registry: r, Policy: s}, Commercial: s}
	d, e := srv.Detail(ctx, actor, sdk.Scope{AccountID: account, BotID: bot}, ref, "")
	must(t, e)
	if len(d.Messages.Items) != 1 || d.Contact.ID == "" || !strings.Contains(d.Messages.Items[0].Body, "retained") {
		t.Fatal("restore")
	}
}
func TestR3D5AgentPrivacyAndExpiry(t *testing.T) {
	f := r3d5Fixture(t)
	x := f.inbound(t, "privacy", "Privacy test")
	note, e := f.srv.Reply(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, supportplugins.Reply{Body: "Private note", Internal: true, Key: "privacy-note", Expected: x.Revision})
	must(t, e)
	if note.Delivery != "NOT_APPLICABLE" {
		t.Fatal(note)
	}
	sales := fixture(t, false)
	tenant(t, f.u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "INSERT INTO workspace_members(workspace_id,user_id,role_key) VALUES($1,$2,'SALES_AGENT')", f.u.WorkspaceID, sales.UserID)
		return e
	})
	d, e := f.srv.Detail(ctx, sales.UserID, f.scope(tickets.ID), x.Reference, "")
	must(t, e)
	for _, m := range d.Messages.Items {
		if m.Kind == "INTERNAL" || m.Body == "Private note" {
			t.Fatal("internal note leaked to sales")
		}
	}
	_, e = f.srv.Reply(ctx, sales.UserID, f.scope(tickets.ID), x.Reference, supportplugins.Reply{Body: "Forbidden", Internal: true, Key: "forbidden-note", Expected: d.Ticket.Revision})
	if !errors.Is(e, database.ErrForbidden) {
		t.Fatal(e)
	}
	second := fixture(t, false)
	tenant(t, f.u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "INSERT INTO workspace_members(workspace_id,user_id,role_key) VALUES($1,$2,'SUPPORT_AGENT')", f.u.WorkspaceID, second.UserID)
		return e
	})
	x, e = f.srv.Act(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, supportplugins.Action{Kind: "assign", Value: f.agent.UserID, Expected: d.Ticket.Revision})
	must(t, e)
	x, e = f.srv.Act(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, supportplugins.Action{Kind: "assign", Value: second.UserID, Expected: x.Revision})
	must(t, e)
	if x.Assignee != second.UserID {
		t.Fatal("reassign")
	}
	x, e = f.srv.Act(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, supportplugins.Action{Kind: "assign", Value: "", Expected: x.Revision})
	must(t, e)
	if x.Assignee != "" {
		t.Fatal("unassign")
	}
	c, e := f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
	must(t, e)
	plan, _ := c.Get(commerce.PlanKind, f.plan)
	plan.Definition.Plan.Plugins[crm.ID] = commerce.PaidAddon
	f.save(t, plan, plan.Version)
	expiry := f.clock.at.Add(time.Hour)
	_, e = f.s.Issue(ctx, f.admin.UserID, f.u.WorkspaceID, "r3d5", commerce.Issue{Key: "crm-temp", Kind: commerce.PluginKind, CatalogKey: crm.ID, Bot: f.bot, AdminGrant: true, ExpiresAt: &expiry, Reason: "synthetic"})
	must(t, e)
	_, e = f.srv.Detail(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, "")
	must(t, e)
	f.clock.at = expiry
	if _, e = f.srv.Detail(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, ""); !errors.Is(e, sdk.ErrPermission) {
		t.Fatal("expiry", e)
	}
	if number(t, f.u, "SELECT count(*) FROM support_messages WHERE ticket_reference=$1", x.Reference) != 2 {
		t.Fatal("expiry lost history")
	}
}
func TestR3D5OccupancyAndReplyQuotas(t *testing.T) {
	f := r3d5Fixture(t)
	set := func(key string, limit commerce.Limit) {
		c, e := f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
		must(t, e)
		v, _ := c.Get(commerce.QuotaKind, key)
		v.Definition.Quota.Default = limit
		f.save(t, v, v.Version)
	}
	set("crm.contacts.count", commerce.Limit{Value: 0})
	_, e := f.srv.Inbound(ctx, f.scope(tickets.ID), tickets.Inbound{User: 10001, Name: "Quota", Body: "Help", Key: "contact-quota"})
	var q *contentplugins.QuotaDenied
	if !errors.As(e, &q) {
		t.Fatal("contact quota", e)
	}
	set("crm.contacts.count", commerce.Limit{Unlimited: true})
	set("ticketing.active_tickets.count", commerce.Limit{Value: 1})
	x := f.inbound(t, "one-active", "First")
	_, e = f.srv.Inbound(ctx, f.scope(tickets.ID), tickets.Inbound{User: 10002, Name: "Other", Body: "Help", Key: "active-quota"})
	if !errors.As(e, &q) {
		t.Fatal("active quota", e)
	}
	if number(t, f.u, "SELECT count(*) FROM crm_contacts WHERE bot_id=$1", f.bot) != 1 {
		t.Fatal("contact rollback")
	}
	set("ticketing.agents.count", commerce.Limit{Value: 0})
	_, e = f.srv.Act(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, supportplugins.Action{Kind: "assign", Value: f.agent.UserID, Expected: x.Revision})
	if !errors.As(e, &q) {
		t.Fatal("agent quota", e)
	}
	d, e := f.srv.Detail(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, "")
	must(t, e)
	if d.Ticket.Assignee != "" || d.Ticket.Revision != x.Revision {
		t.Fatal("assignment rollback")
	}
	set("ticketing.messages.monthly", commerce.Limit{Value: 1})
	_, e = f.srv.Reply(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, supportplugins.Reply{Body: "Blocked reply", Key: "reply-quota", Expected: x.Revision})
	if !errors.As(e, &q) || f.transport.replies.Load() != 0 {
		t.Fatal("reply quota external effect", e)
	}
	if number(t, f.u, "SELECT count(*) FROM support_messages WHERE ticket_reference=$1", x.Reference) != 1 {
		t.Fatal("reply partial state")
	}
	set("ticketing.messages.monthly", commerce.Limit{Unlimited: true})
	set("ticketing.agents.count", commerce.Limit{Unlimited: true})
	x, e = f.srv.Act(ctx, f.agent.UserID, f.scope(tickets.ID), x.Reference, supportplugins.Action{Kind: "status", Value: "CLOSED", Expected: x.Revision})
	must(t, e)
	_, e = f.srv.Inbound(ctx, f.scope(tickets.ID), tickets.Inbound{User: 10002, Name: "Other", Body: "Now allowed", Key: "after-close"})
	must(t, e)
	set("ticketing.active_tickets.count", commerce.Limit{Unlimited: true})
}
