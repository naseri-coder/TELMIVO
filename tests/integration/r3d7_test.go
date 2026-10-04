//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/businessplugins"
	"github.com/naseri-coder/TELMIVO/internal/domains/commerce"
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/ledger"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/plugins/broadcast"
	"github.com/naseri-coder/TELMIVO/internal/plugins/crm"
	"github.com/naseri-coder/TELMIVO/internal/plugins/forms"
	"github.com/naseri-coder/TELMIVO/internal/plugins/shop"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type r3d7Transport struct {
	r3d4Transport
	sends atomic.Int64
	code  string
	delay time.Duration
}

func (t *r3d7Transport) BroadcastReply(context.Context, int64, string) (int64, string, time.Duration) {
	t.sends.Add(1)
	if t.code != "" {
		return 0, t.code, t.delay
	}
	return 101, "DELIVERY_ACKNOWLEDGED", 0
}

type r3d7Provider struct{ tr *r3d7Transport }

func (p r3d7Provider) ForBot(context.Context, pgx.Tx, sdk.Scope) (contentplugins.Transport, error) {
	return p.tr, nil
}

type r3d7Data struct {
	r3d5FixtureData
	biz *businessplugins.Service
	tr  *r3d7Transport
}

func r3d7Fixture(t *testing.T) r3d7Data {
	t.Helper()
	f := r3d5Fixture(t)
	for _, id := range []string{broadcast.ID, forms.ID, shop.ID} {
		m, e := f.core.Registry.Manifest(id)
		must(t, e)
		f.save(t, commerce.Version{Kind: commerce.PluginKind, Key: id, Definition: commerce.Definition{Plugin: &commerce.Plugin{Scope: sdk.PerBot, SaleEnabled: false, Capabilities: m.Permissions, Prices: r3d2Prices(0)}}}, factoryVersion(t, f.s, f.admin, f.u, commerce.PluginKind, id))
	}
	c, e := f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
	must(t, e)
	plan, _ := c.Get(commerce.PlanKind, f.plan)
	for _, id := range []string{broadcast.ID, forms.ID, shop.ID} {
		plan.Definition.Plan.Plugins[id] = commerce.Included
	}
	f.save(t, plan, plan.Version)
	for _, id := range []string{broadcast.ID, forms.ID, shop.ID} {
		i, e := f.core.Change(ctx, f.u.UserID, f.scope(id), sdk.Install, 0, json.RawMessage(`{}`), "r3d7")
		must(t, e)
		_, e = f.core.Change(ctx, f.u.UserID, f.scope(id), sdk.Activate, i.Revision, nil, "r3d7")
		must(t, e)
	}
	tr := &r3d7Transport{}
	biz := &businessplugins.Service{DB: db, Core: f.core, Commercial: f.s, Support: f.srv, Content: &contentplugins.Service{DB: db, Core: f.core, Commercial: f.s, Provider: r3d7Provider{tr}}, Clock: f.clock}
	return r3d7Data{f, biz, tr}
}
func (f r3d7Data) contact(t *testing.T, user string, tags []string) crm.Contact {
	t.Helper()
	c, e := f.srv.Resolve(ctx, f.u.UserID, f.scope(crm.ID), crm.ResolveRequest{Identity: crm.Identity{Provider: "telegram", Key: user}, Name: "Synthetic", Locale: "fa"})
	must(t, e)
	c.Tags = tags
	c, e = f.srv.SaveContact(ctx, f.u.UserID, f.scope(crm.ID), c.ID, c, c.Revision)
	must(t, e)
	return c
}
func (f r3d7Data) campaign(t *testing.T) broadcast.Campaign {
	t.Helper()
	d, e := f.biz.SaveCampaign(ctx, f.u.UserID, f.scope(broadcast.ID), "", businessplugins.Change[broadcast.Campaign]{Definition: broadcast.Campaign{Name: "Campaign", Text: "Synthetic message", Audience: broadcast.Audience{Tags: []string{"target"}}}, Key: "campaign"})
	must(t, e)
	c, e := f.biz.CampaignAction(ctx, f.u.UserID, f.scope(broadcast.ID), d.ID, businessplugins.Action{Action: "START", Expected: d.Revision, Key: "start"})
	must(t, e)
	return c
}
func TestR3D7BroadcastSnapshotReplayAndRecovery(t *testing.T) {
	f := r3d7Fixture(t)
	a := f.contact(t, "10001", []string{"target"})
	f.contact(t, "10002", []string{"target"})
	f.contact(t, "10003", nil)
	c := f.campaign(t)
	if c.Recipients != 2 {
		t.Fatal(c)
	}
	f.contact(t, "10004", []string{"target"})
	a.Tags = []string{}
	_, e := f.srv.SaveContact(ctx, f.u.UserID, f.scope(crm.ID), a.ID, a, a.Revision)
	must(t, e)
	page, e := f.biz.Recipients(ctx, f.u.UserID, f.scope(broadcast.ID), c.ID, "")
	must(t, e)
	if len(page.Items) != 2 {
		t.Fatal("snapshot drift")
	}
	again, e := f.biz.CampaignAction(ctx, f.u.UserID, f.scope(broadcast.ID), c.ID, businessplugins.Action{Action: "START", Expected: 1, Key: "start"})
	must(t, e)
	if again.ID != c.ID {
		t.Fatal("replayed start")
	}
	must(t, f.biz.DeliverOne(ctx, f.u.UserID, f.scope(broadcast.ID), c.ID))
	if f.tr.sends.Load() != 1 {
		t.Fatal("send not made")
	}
	must(t, f.biz.DeliverOne(ctx, f.u.UserID, f.scope(broadcast.ID), c.ID))
	if f.tr.sends.Load() != 1 {
		t.Fatal("rate limit bypass")
	}
	f.clock.at = f.clock.at.Add(2 * time.Second)
	f.tr.code = "DELIVERY_OUTCOME_UNKNOWN"
	must(t, f.biz.DeliverOne(ctx, f.u.UserID, f.scope(broadcast.ID), c.ID))
	must(t, f.biz.DeliverOne(ctx, f.u.UserID, f.scope(broadcast.ID), c.ID))
	c, e = f.biz.Campaign(ctx, f.u.UserID, f.scope(broadcast.ID), c.ID)
	must(t, e)
	if c.State != "FAILED" || c.Sent != 1 || c.Unknown != 1 || f.tr.sends.Load() != 2 {
		t.Fatal(c, f.tr.sends.Load())
	}
	e = db.Tenant(ctx, f.u.UserID, f.u.WorkspaceID, false, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE plugin_broadcast_recipients SET state='QUEUED' WHERE campaign_id=$1", c.ID)
		return e
	})
	if e == nil {
		t.Fatal("terminal recipient changed")
	}
}
func TestR3D7BroadcastPauseCancelRetryAndConcurrentClaim(t *testing.T) {
	f := r3d7Fixture(t)
	f.contact(t, "10001", []string{"target"})
	c := f.campaign(t)
	c, e := f.biz.CampaignAction(ctx, f.u.UserID, f.scope(broadcast.ID), c.ID, businessplugins.Action{Action: "PAUSE", Expected: c.Revision, Key: "pause"})
	must(t, e)
	must(t, f.biz.DeliverOne(ctx, f.u.UserID, f.scope(broadcast.ID), c.ID))
	if f.tr.sends.Load() != 0 {
		t.Fatal("paused send")
	}
	c, e = f.biz.CampaignAction(ctx, f.u.UserID, f.scope(broadcast.ID), c.ID, businessplugins.Action{Action: "RESUME", Expected: c.Revision, Key: "resume"})
	must(t, e)
	f.tr.code = "RATE_LIMITED"
	f.tr.delay = 60 * time.Second
	must(t, f.biz.DeliverOne(ctx, f.u.UserID, f.scope(broadcast.ID), c.ID))
	f.clock.at = f.clock.at.Add(2 * time.Second)
	must(t, f.biz.DeliverOne(ctx, f.u.UserID, f.scope(broadcast.ID), c.ID))
	if f.tr.sends.Load() != 1 {
		t.Fatal("retry-after ignored")
	}
	f.clock.at = f.clock.at.Add(60 * time.Second)
	f.tr.code = ""
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- f.biz.DeliverOne(ctx, f.u.UserID, f.scope(broadcast.ID), c.ID) }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		must(t, e)
	}
	if f.tr.sends.Load() != 2 {
		t.Fatal("duplicate delivery", f.tr.sends.Load())
	}
}
func TestR3D7FormsAtomicMappingsAndIdempotency(t *testing.T) {
	f := r3d7Fixture(t)
	form := forms.Form{Name: "Form", Enabled: true, Access: "PUBLIC_TO_BOT_USER", Fields: []forms.Field{{Key: "choice", Label: "Choice", Type: "SINGLE_CHOICE", Required: true, MaxLength: 100, Options: []string{"yes", "no"}, CRMField: "answer", Tags: map[string]string{"yes": "interested"}}}}
	d, e := f.biz.SaveForm(ctx, f.u.UserID, f.scope(forms.ID), "", businessplugins.Change[forms.Form]{Definition: form, Key: "form"})
	must(t, e)
	customer := businessplugins.Customer{User: 10001, Name: "Synthetic"}
	answers := map[string]json.RawMessage{"choice": json.RawMessage(`"yes"`)}
	sub, e := f.biz.SubmitTrusted(ctx, f.u.UserID, f.scope(forms.ID), d.ID, "answer", customer, answers)
	must(t, e)
	again, e := f.biz.SubmitTrusted(ctx, f.u.UserID, f.scope(forms.ID), d.ID, "answer", customer, answers)
	must(t, e)
	if sub.ID != again.ID {
		t.Fatal("duplicate submission")
	}
	answers["choice"] = json.RawMessage(`"no"`)
	_, e = f.biz.SubmitTrusted(ctx, f.u.UserID, f.scope(forms.ID), d.ID, "answer", customer, answers)
	if !errors.Is(e, database.ErrConflict) {
		t.Fatal("changed replay", e)
	}
	contact, e := f.srv.Contact(ctx, f.u.UserID, f.scope(crm.ID), sub.Contact)
	must(t, e)
	if contact.Fields["answer"] != "yes" || len(contact.Tags) != 1 {
		t.Fatal(contact)
	}
	if n := number(t, f.u, "SELECT count(*) FROM form_submissions WHERE bot_id=$1", f.bot); n != 1 {
		t.Fatal(n)
	}
	if n := number(t, f.u, "SELECT count(*) FROM outbox_events WHERE workspace_id=$1 AND event_type='form.submitted'", f.u.WorkspaceID); n != 1 {
		t.Fatal("missing event", n)
	}
	form.Access = "ACCESS_POLICY_PROTECTED"
	form.Policy = "00000000-0000-0000-0000-000000000000"
	d, e = f.biz.SaveForm(ctx, f.u.UserID, f.scope(forms.ID), "", businessplugins.Change[forms.Form]{Definition: form, Key: "protected"})
	must(t, e)
	_, e = f.biz.SubmitTrusted(ctx, f.u.UserID, f.scope(forms.ID), d.ID, "protected-answer", customer, answers)
	if e == nil {
		t.Fatal("missing gate allowed")
	}
}
func TestR3D7CommerceHistoryAndIsolation(t *testing.T) {
	f := r3d7Fixture(t)
	p := shop.Product{Name: "Original", Enabled: true, Price: ledger.Money{Asset: ledger.USD, AmountAtomic: 9007199254740993}}
	d, e := f.biz.SaveProduct(ctx, f.u.UserID, f.scope(shop.ID), "", businessplugins.Change[shop.Product]{Definition: p, Key: "product"})
	must(t, e)
	c := businessplugins.Customer{User: 10001, Name: "Synthetic"}
	items := []shop.Item{{Product: d.ID, Quantity: 2}}
	o, e := f.biz.PlaceTrusted(ctx, f.u.UserID, f.scope(shop.ID), "order", c, items)
	must(t, e)
	again, e := f.biz.PlaceTrusted(ctx, f.u.UserID, f.scope(shop.ID), "order", c, items)
	must(t, e)
	if again.ID != o.ID || o.Total.AmountAtomic != 18014398509481986 {
		t.Fatal(o)
	}
	p.Name = "Updated"
	p.Price.AmountAtomic = 1
	_, e = f.biz.SaveProduct(ctx, f.u.UserID, f.scope(shop.ID), d.ID, businessplugins.Change[shop.Product]{Definition: p, Key: "price", Expected: d.Revision})
	must(t, e)
	orders, e := f.biz.Orders(ctx, f.u.UserID, f.scope(shop.ID), "", "", "")
	must(t, e)
	if orders.Items[0].Lines[0].Name != "Original" || orders.Items[0].Total != o.Total {
		t.Fatal("historical drift")
	}
	_, e = f.biz.OrderAction(ctx, f.u.UserID, f.scope(shop.ID), o.ID, businessplugins.Action{Action: "FULFILLED", Expected: o.Revision, Key: "invalid"})
	if e == nil {
		t.Fatal("illegal state")
	}
	o, e = f.biz.OrderAction(ctx, f.u.UserID, f.scope(shop.ID), o.ID, businessplugins.Action{Action: "CONFIRMED", Expected: o.Revision, Key: "confirm"})
	must(t, e)
	o, e = f.biz.OrderAction(ctx, f.u.UserID, f.scope(shop.ID), o.ID, businessplugins.Action{Action: "CANCELLED", Expected: o.Revision, Key: "cancel"})
	must(t, e)
	history, e := f.biz.OrderHistory(ctx, f.u.UserID, f.scope(shop.ID), o.ID, "")
	must(t, e)
	if len(history) != 3 {
		t.Fatal(history)
	}
	other := fixture(t, false)
	_, e = f.biz.Orders(ctx, other.UserID, f.scope(shop.ID), "", "", "")
	if !errors.Is(e, database.ErrForbidden) {
		t.Fatal("tenant isolation", e)
	}
	_, e = f.biz.Definitions(ctx, f.agent.UserID, f.scope(shop.ID), "", "")
	if !errors.Is(e, database.ErrForbidden) {
		t.Fatal("RBAC", e)
	}
	scope := f.scope(shop.ID)
	scope.BotID = f.other
	_, e = f.biz.PlaceTrusted(ctx, f.u.UserID, scope, "cross-bot", c, items)
	if e == nil {
		t.Fatal("bot isolation")
	}
}
func TestR3D7QuotaRollbackRetentionAndInterruptedSend(t *testing.T) {
	f := r3d7Fixture(t)
	catalog, e := f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
	must(t, e)
	v, _ := catalog.Get(commerce.QuotaKind, "commerce.orders.monthly")
	v.Definition.Quota.Default = commerce.Limit{Value: 0}
	f.save(t, v, v.Version)
	defer func() {
		catalog, e := f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
		must(t, e)
		current, _ := catalog.Get(commerce.QuotaKind, "commerce.orders.monthly")
		current.Definition.Quota.Default = commerce.Limit{Unlimited: true}
		f.save(t, current, current.Version)
	}()
	p := shop.Product{Name: "Product", Enabled: true, Price: ledger.Money{Asset: ledger.USD, AmountAtomic: 1}}
	d, e := f.biz.SaveProduct(ctx, f.u.UserID, f.scope(shop.ID), "", businessplugins.Change[shop.Product]{Definition: p, Key: "p"})
	must(t, e)
	_, e = f.biz.PlaceTrusted(ctx, f.u.UserID, f.scope(shop.ID), "denied", businessplugins.Customer{User: 10001, Name: "Synthetic"}, []shop.Item{{Product: d.ID, Quantity: 1}})
	var q *contentplugins.QuotaDenied
	if !errors.As(e, &q) {
		t.Fatal(e)
	}
	if number(t, f.u, "SELECT count(*) FROM crm_contacts WHERE bot_id=$1", f.bot) != 0 || number(t, f.u, "SELECT count(*) FROM commerce_orders WHERE bot_id=$1", f.bot) != 0 {
		t.Fatal("quota partial state")
	}
	f.contact(t, "10001", []string{"target"})
	c := f.campaign(t)
	tenant(t, f.u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE plugin_broadcast_recipients SET state='SENDING',attempt=1,lease_until=$2 WHERE campaign_id=$1", c.ID, f.clock.Now().Add(-time.Second))
		return e
	})
	must(t, f.biz.DeliverOne(ctx, f.u.UserID, f.scope(broadcast.ID), c.ID))
	if f.tr.sends.Load() != 0 {
		t.Fatal("interrupted reservation replayed")
	}
	c, e = f.biz.Campaign(ctx, f.u.UserID, f.scope(broadcast.ID), c.ID)
	must(t, e)
	if c.Unknown != 1 || c.State != "FAILED" {
		t.Fatal(c)
	}
	for _, id := range []string{broadcast.ID, forms.ID, shop.ID} {
		i, e := f.core.View(ctx, f.u.UserID, f.scope(id))
		must(t, e)
		_, e = f.core.Change(ctx, f.u.UserID, f.scope(id), sdk.Disable, i.Revision, nil, "retain")
		must(t, e)
	}
	if number(t, f.u, "SELECT count(*) FROM commerce_products WHERE bot_id=$1", f.bot) != 1 || number(t, f.u, "SELECT count(*) FROM plugin_broadcast_recipients WHERE bot_id=$1", f.bot) != 1 {
		t.Fatal("disable discarded data")
	}
}
func TestR3D7ConcurrentOrdersAndCustomerBoundary(t *testing.T) {
	f := r3d7Fixture(t)
	d, e := f.biz.SaveProduct(ctx, f.u.UserID, f.scope(shop.ID), "", businessplugins.Change[shop.Product]{Definition: shop.Product{Name: "Product", Enabled: true, Price: ledger.Money{Asset: ledger.USD, AmountAtomic: 10}}, Key: "product"})
	must(t, e)
	c := businessplugins.Customer{User: 10001, Name: "Synthetic"}
	items := []shop.Item{{Product: d.ID, Quantity: 1}}
	var wg sync.WaitGroup
	results := make(chan shop.Order, 4)
	errs := make(chan error, 4)
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			o, e := f.biz.PlaceTrusted(ctx, f.u.UserID, f.scope(shop.ID), "concurrent", c, items)
			results <- o
			errs <- e
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		must(t, e)
	}
	var id string
	for o := range results {
		if id != "" && o.ID != id {
			t.Fatal("duplicate order")
		}
		id = o.ID
	}
	if number(t, f.u, "SELECT count(*) FROM commerce_orders WHERE bot_id=$1", f.bot) != 1 {
		t.Fatal("non atomic replay")
	}
	_, e = f.biz.BindCustomer(f.u.UserID, f.scope(shop.ID), c).Invoke(ctx, "commerce.order", json.RawMessage(`{"idempotency_key":"spoof","items":[],"user":20001}`))
	if e == nil {
		t.Fatal("principal injected")
	}
	_, e = f.biz.BindCustomer(f.u.UserID, f.scope(forms.ID), c).Invoke(ctx, "commerce.order", json.RawMessage(`{}`))
	if !errors.Is(e, sdk.ErrPermission) {
		t.Fatal("cross plugin capability")
	}
	preview, e := f.biz.PreviewAudience(ctx, f.u.UserID, f.scope(broadcast.ID), broadcast.Audience{})
	must(t, e)
	if preview["eligible"] != 1 {
		t.Fatal("empty tags should select every eligible contact", preview)
	}
}
