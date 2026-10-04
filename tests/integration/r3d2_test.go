//go:build integration

package integration

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/commerce"
	"github.com/naseri-coder/TELMIVO/internal/domains/identity"
	platform "github.com/naseri-coder/TELMIVO/internal/domains/pluginplatform"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
)

type r3d2Clock struct{ at time.Time }

func (c *r3d2Clock) Now() time.Time { return c.at }

type commercialFixture struct {
	s                                *commerce.Service
	clock                            *r3d2Clock
	admin, u                         identity.Result
	bot, other, p1, p2, plan, metric string
}

func r3d2Prices(n int64) commerce.Prices {
	return commerce.Prices{Daily: commerce.Money{Unit: "USD", Scale: 2, Amount: n}, Monthly: commerce.Money{Unit: "USD", Scale: 2, Amount: n * 20}}
}
func (f commercialFixture) save(t *testing.T, v commerce.Version, expected int64) commerce.Version {
	t.Helper()
	v.Enabled = true
	v.EffectiveAt = f.clock.at
	out, e := f.s.SaveCatalog(ctx, f.admin.UserID, f.u.WorkspaceID, "r3d2", commerce.CatalogChange{Version: v, Expected: expected, Reason: "isolated synthetic policy"})
	must(t, e)
	return out
}
func r3d2Fixture(t *testing.T) commercialFixture {
	t.Helper()
	n := sequence.Add(1)
	f := commercialFixture{admin: fixture(t, true), u: fixture(t, false), p1: fmt.Sprintf("r3d2-bot-%d", n), p2: fmt.Sprintf("r3d2-account-%d", n), plan: fmt.Sprintf("R3D2_%d", n), metric: fmt.Sprintf("r3d2-requests-%d", n)}
	f.clock = &r3d2Clock{at: time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)}
	f.s = &commerce.Service{DB: db, Clock: f.clock}
	f.bot = r3dBot(t, f.u, fmt.Sprintf("commercial-%d", n))
	f.other = r3dBot(t, f.u, fmt.Sprintf("commercial-other-%d", n))
	for _, p := range []struct {
		id    string
		scope sdk.LicenseScope
	}{{f.p1, sdk.PerBot}, {f.p2, sdk.PerAccount}} {
		f.save(t, commerce.Version{Kind: commerce.PluginKind, Key: p.id, Definition: commerce.Definition{Plugin: &commerce.Plugin{Scope: p.scope, SaleEnabled: true, Prices: r3d2Prices(50), Capabilities: []string{"telegram.send"}, Trial: commerce.TrialPolicy{Enabled: true, DurationSeconds: 3600}, GraceSeconds: 3600}}}, 0)
	}
	f.save(t, commerce.Version{Kind: commerce.QuotaKind, Key: f.metric, Definition: commerce.Definition{Quota: &commerce.Quota{Scope: sdk.PerBot, Reset: commerce.Daily, Default: commerce.Limit{Value: 2}}}}, 0)
	f.save(t, commerce.Version{Kind: commerce.PlanKind, Key: f.plan, Definition: commerce.Definition{Plan: &commerce.Plan{Name: "Synthetic flexible plan", Order: 30, Plugins: map[string]commerce.Inclusion{f.p1: commerce.PaidAddon, f.p2: commerce.Included}, Limits: map[string]commerce.Limit{f.metric: {Value: 10}}}}}, 0)
	tenant(t, f.u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE subscriptions SET plan_key=$1,expires_at=NULL,scheduled_plan_key=NULL,scheduled_at=NULL WHERE workspace_id=$2", f.plan, f.u.WorkspaceID)
		return e
	})
	return f
}
func (f commercialFixture) issue(t *testing.T, in commerce.Issue) commerce.Agreement {
	t.Helper()
	in.Reason = "synthetic commercial agreement"
	a, e := f.s.Issue(ctx, f.admin.UserID, f.u.WorkspaceID, "r3d2", in)
	must(t, e)
	return a
}
func (f commercialFixture) decision(t *testing.T, plugin, bot string) commerce.Decision {
	t.Helper()
	d, e := f.s.Decide(ctx, f.u.UserID, sdk.Scope{AccountID: f.u.WorkspaceID, BotID: bot, PluginID: plugin})
	must(t, e)
	return d
}
func TestR3D2PricingAndCatalog(t *testing.T) {
	f := r3d2Fixture(t)
	t.Run("plans_and_policies", func(t *testing.T) {
		c, e := f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
		must(t, e)
		for _, p := range []string{"FREE", "PRO", "BUSINESS", f.plan} {
			if _, ok := c.Get(commerce.PlanKind, p); !ok {
				t.Fatal("missing plan", p)
			}
		}
		if !f.decision(t, f.p2, f.bot).Allowed || f.decision(t, f.p1, f.bot).Code != "NO_ENTITLEMENT" {
			t.Fatal("plan policy")
		}
	})

	t.Run("plan_configuration_and_disable_safety", func(t *testing.T) {
		v := commerce.Version{Kind: commerce.PlanKind, Key: "UNUSED_" + f.plan, Enabled: true, Definition: commerce.Definition{Plan: &commerce.Plan{Name: "Future plan", Order: 40, Plugins: map[string]commerce.Inclusion{f.p1: commerce.Unavailable}}}}
		v = f.save(t, v, 0)
		v.Enabled = false
		_, err := f.s.SaveCatalog(ctx, f.admin.UserID, f.u.WorkspaceID, "test", commerce.CatalogChange{Version: v, Expected: 1, Reason: "disable unused"})
		must(t, err)
		catalog, err := f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
		must(t, err)
		if _, ok := catalog.Get(commerce.PlanKind, v.Key); ok {
			t.Fatal("disabled plan available")
		}
		used, _ := catalog.Get(commerce.PlanKind, f.plan)
		used.Enabled = false
		_, err = f.s.SaveCatalog(ctx, f.admin.UserID, f.u.WorkspaceID, "test", commerce.CatalogChange{Version: used, Expected: 1, Reason: "unsafe disable"})
		if !errors.Is(err, database.ErrConflict) {
			t.Fatal("used plan safeguard", err)
		}
	})
	_, e := f.s.Issue(ctx, f.admin.UserID, f.u.WorkspaceID, "invalid-period", commerce.Issue{Key: "weekly", Kind: commerce.PluginKind, CatalogKey: f.p1, Bot: f.bot, Period: "WEEKLY", Reason: "test"})
	if !errors.Is(e, database.ErrInvalid) {
		t.Fatal("invalid period accepted", e)
	}
	daily := f.issue(t, commerce.Issue{Key: "daily", Kind: commerce.PluginKind, CatalogKey: f.p1, Bot: f.bot, Period: commerce.Daily})
	monthly := f.issue(t, commerce.Issue{Key: "monthly", Kind: commerce.PluginKind, CatalogKey: f.p1, Bot: f.bot, Period: commerce.Monthly})
	if daily.Terms.Price.Amount != 50 || monthly.Terms.Price.Amount != 1000 || daily.EndsAt.Sub(daily.StartsAt) != 24*time.Hour || monthly.EndsAt.Day() != 28 {
		t.Fatal(daily, monthly)
	}
	c, e := f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
	must(t, e)
	v, _ := c.Get(commerce.PluginKind, f.p1)
	v.Definition.Plugin.Prices = r3d2Prices(99)
	f.save(t, v, 1)
	t.Run("immutable_snapshot_and_future_price", func(t *testing.T) {
		next := f.issue(t, commerce.Issue{Key: "new-price", Kind: commerce.PluginKind, CatalogKey: f.p1, Bot: f.bot, Period: commerce.Daily})
		as, e := f.s.Agreements(ctx, f.admin.UserID, f.u.WorkspaceID)
		must(t, e)
		for _, a := range as {
			if a.ID == daily.ID && a.Terms.Price.Amount != 50 {
				t.Fatal("historical price changed")
			}
		}
		history, err := f.s.History(ctx, f.admin.UserID, f.u.WorkspaceID, commerce.PluginKind, f.p1)
		must(t, err)
		if len(history) != 2 || history[0].Definition.Plugin.Prices.Daily.Amount != 50 || history[1].Definition.Plugin.Prices.Daily.Amount != 99 {
			t.Fatal("price history", history)
		}
		if next.Terms.Price.Amount != 99 || next.Terms.Catalog.Version != 2 {
			t.Fatal(next)
		}
		for _, q := range []string{"UPDATE commercial_agreements SET terms='{}',revision=revision+1 WHERE id=$1", "DELETE FROM commercial_agreements WHERE id=$1"} {
			e := db.Tenant(ctx, f.u.UserID, f.u.WorkspaceID, false, func(tx pgx.Tx) error { _, e := tx.Exec(ctx, q, daily.ID); return e })
			pgCode(t, e, "23514")
		}
		_, e = db.Pool.Exec(ctx, "UPDATE commercial_catalog SET definition='{}' WHERE kind='PLUGIN' AND key=$1", f.p1)
		pgCode(t, e, "P0001")
	})
	t.Run("free_sale_disabled_invalid_and_idempotency", func(t *testing.T) {
		v.Definition.Plugin.Prices = r3d2Prices(0)
		f.save(t, v, 2)
		free := f.issue(t, commerce.Issue{Key: "free", Kind: commerce.PluginKind, CatalogKey: f.p1, Bot: f.bot, Period: commerce.Daily})
		if free.Terms.Price.Amount != 0 {
			t.Fatal(free)
		}
		v.Definition.Plugin.SaleEnabled = false
		f.save(t, v, 3)
		_, e = f.s.Issue(ctx, f.admin.UserID, f.u.WorkspaceID, "test", commerce.Issue{Key: "closed", Kind: commerce.PluginKind, CatalogKey: f.p1, Bot: f.bot, Period: commerce.Daily, Reason: "test"})
		var d *commerce.Denied
		if !errors.As(e, &d) || d.Decision.Code != "SALE_DISABLED" {
			t.Fatal(e)
		}
		if !f.decision(t, f.p1, f.bot).Allowed {
			t.Fatal("existing sale entitlement removed")
		}
		v.Definition.Plugin.Prices.Daily.Amount = -1
		if _, e = f.s.SaveCatalog(ctx, f.admin.UserID, f.u.WorkspaceID, "test", commerce.CatalogChange{Version: v, Expected: 4, Reason: "test"}); !errors.Is(e, database.ErrInvalid) {
			t.Fatal(e)
		}
		replay := f.issue(t, commerce.Issue{Key: "daily", Kind: commerce.PluginKind, CatalogKey: f.p1, Bot: f.bot, Period: commerce.Daily})
		if replay.ID != daily.ID || replay.Terms.Price.Amount != 50 {
			t.Fatal("idempotency")
		}
		_, e = f.s.Issue(ctx, f.admin.UserID, f.u.WorkspaceID, "test", commerce.Issue{Key: "daily", Kind: commerce.PluginKind, CatalogKey: f.p1, Bot: f.bot, Period: commerce.Monthly, Reason: "synthetic commercial agreement"})
		if !errors.Is(e, database.ErrConflict) {
			t.Fatal(e)
		}
	})
}
func TestR3D2EntitlementLicensingAndActivation(t *testing.T) {
	f := r3d2Fixture(t)
	s, _ := r3dService(t, r3dManifest(f.p1))
	s.Policy = f.s
	scope := sdk.Scope{AccountID: f.u.WorkspaceID, BotID: f.bot, PluginID: f.p1}
	// Preinstall using an administrative grant, revoke before activation.
	a := f.issue(t, commerce.Issue{Key: "install-grant", Kind: commerce.PluginKind, CatalogKey: f.p1, Bot: f.bot, AdminGrant: true})
	i := r3dChange(t, s, f.u.UserID, scope, sdk.Install, 0)
	_, e := f.s.SetState(ctx, f.admin.UserID, f.u.WorkspaceID, "test", a.ID, 1, commerce.Cancelled, "revoked")
	must(t, e)
	if _, e = s.Change(ctx, f.u.UserID, scope, sdk.Activate, i.Revision, nil, "denied"); !errors.Is(e, sdk.ErrPermission) {
		t.Fatal("unentitled activation", e)
	}
	purchase := f.issue(t, commerce.Issue{Key: "purchase", Kind: commerce.PluginKind, CatalogKey: f.p1, Bot: f.bot, Period: commerce.Daily})
	if !f.decision(t, f.p1, f.bot).Allowed || f.decision(t, f.p1, f.other).Allowed {
		t.Fatal("per-bot leakage")
	}
	catalog, err := f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
	must(t, err)
	plan, _ := catalog.Get(commerce.PlanKind, f.plan)
	plan.Definition.Plan.Plugins[f.p2] = commerce.PaidAddon
	f.save(t, plan, 1)
	if f.decision(t, f.p2, f.other).Allowed {
		t.Fatal("paid add-on granted without agreement")
	}
	account := f.issue(t, commerce.Issue{Key: "account", Kind: commerce.PluginKind, CatalogKey: f.p2, Period: commerce.Daily})
	if account.Scope != sdk.PerAccount || !f.decision(t, f.p2, f.other).Allowed {
		t.Fatal("per-account license")
	}
	i = r3dChange(t, s, f.u.UserID, scope, sdk.Activate, i.Revision)
	tenant(t, f.u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE plugin_installations SET data='{"retained":"commercial"}',revision=revision+1 WHERE bot_id=$1`, f.bot)
		return e
	})
	i, e = s.View(ctx, f.u.UserID, scope)
	must(t, e)
	h := &r3dHandler{}
	runtime := &platform.Runtime{Service: s, Handler: h}
	_, e = runtime.Bind(f.u.UserID, scope).Invoke(ctx, "telegram.send", json.RawMessage(`{}`))
	must(t, e)
	f.clock.at = *purchase.EndsAt
	if state := commerce.CommercialState(purchase, f.clock.at); state != commerce.Grace {
		t.Fatal(state)
	}
	_, e = runtime.Bind(f.u.UserID, scope).Invoke(ctx, "telegram.send", json.RawMessage(`{}`))
	must(t, e)
	f.clock.at = *purchase.GraceUntil
	if _, e = runtime.Bind(f.u.UserID, scope).Invoke(ctx, "telegram.send", json.RawMessage(`{}`)); !errors.Is(e, sdk.ErrPermission) {
		t.Fatal("expired execution", e)
	}
	if d := f.decision(t, f.p1, f.bot); d.Code != "ENTITLEMENT_EXPIRED" {
		t.Fatal(d)
	}
	view, e := s.View(ctx, f.u.UserID, scope)
	must(t, e)
	if view.State != sdk.Active || !jsonContains(view.Data, "retained", "commercial") {
		t.Fatal("expiration mutated lifecycle", view)
	}
	i = r3dChange(t, s, f.u.UserID, scope, sdk.Disable, i.Revision)
	f.issue(t, commerce.Issue{Key: "restoration", Kind: commerce.PluginKind, CatalogKey: f.p1, Bot: f.bot, AdminGrant: true})
	i = r3dChange(t, s, f.u.UserID, scope, sdk.Activate, i.Revision)
	if !jsonContains(i.Data, "retained", "commercial") {
		t.Fatal(i)
	}
	c, e := f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
	must(t, e)
	v, _ := c.Get(commerce.PluginKind, f.p1)
	v.Definition.Plugin.Capabilities = []string{}
	f.save(t, v, 1)
	if _, e = runtime.Bind(f.u.UserID, scope).Invoke(ctx, "telegram.send", json.RawMessage(`{}`)); !errors.Is(e, sdk.ErrPermission) {
		t.Fatal("unauthorized capability", e)
	}
	other := fixture(t, false)
	if _, e = f.s.Decide(ctx, other.UserID, scope); !errors.Is(e, database.ErrForbidden) {
		t.Fatal("cross-account read", e)
	}
}
func TestR3D2Trials(t *testing.T) {
	f := r3d2Fixture(t)
	a, e := f.s.StartTrial(ctx, f.u.UserID, f.u.WorkspaceID, "test", "trial", f.p1, f.bot)
	must(t, e)
	if a.Source != commerce.TrialSource || a.EndsAt.Sub(a.StartsAt) != time.Hour || !f.decision(t, f.p1, f.bot).Allowed {
		t.Fatal(a)
	}
	f.clock.at = *a.EndsAt
	if f.decision(t, f.p1, f.bot).Allowed {
		t.Fatal("expired trial")
	}
	_, e = f.s.StartTrial(ctx, f.u.UserID, f.u.WorkspaceID, "test", "repeat", f.p1, f.bot)
	var d *commerce.Denied
	if !errors.As(e, &d) || d.Decision.Code != "TRIAL_CONSUMED" {
		t.Fatal(e)
	}
	replay, e := f.s.StartTrial(ctx, f.u.UserID, f.u.WorkspaceID, "test", "trial", f.p1, f.bot)
	must(t, e)
	if replay.ID != a.ID {
		t.Fatal("trial replay")
	}
	if n := number(t, f.u, "SELECT count(*) FROM commercial_agreements WHERE workspace_id=$1", f.u.WorkspaceID); n != 1 {
		t.Fatal("failed trial persisted", n)
	}
	renewed := f.issue(t, commerce.Issue{Key: "admin-repeat", Kind: commerce.PluginKind, CatalogKey: f.p1, Bot: f.bot, AdminGrant: true, RepeatTrial: true})
	if renewed.Source != commerce.TrialSource || !f.decision(t, f.p1, f.bot).Allowed {
		t.Fatal(renewed)
	}
	_, e = f.s.StartTrial(ctx, f.u.UserID, f.u.WorkspaceID, "test", "other-trial", f.p1, f.other)
	must(t, e)
}
func TestR3D2BundlesAddonsAndQuota(t *testing.T) {
	f := r3d2Fixture(t)
	bundle := "bundle-" + f.p1
	addon := "addon-" + f.p1
	// A per-bot bundle may contain only per-bot plugins: no implicit license escalation.
	p3 := "extra-" + f.p1
	f.save(t, commerce.Version{Kind: commerce.PluginKind, Key: p3, Definition: commerce.Definition{Plugin: &commerce.Plugin{Scope: sdk.PerBot, SaleEnabled: true, Prices: r3d2Prices(1)}}}, 0)
	c, e := f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
	must(t, e)
	plan, _ := c.Get(commerce.PlanKind, f.plan)
	plan.Definition.Plan.Plugins[p3] = commerce.PaidAddon
	f.save(t, plan, 1)
	f.save(t, commerce.Version{Kind: commerce.BundleKind, Key: bundle, Definition: commerce.Definition{Product: &commerce.Product{Name: "Synthetic pack", Scope: sdk.PerBot, SaleEnabled: true, Prices: r3d2Prices(5), Plugins: []string{f.p1, p3}}}}, 0)
	b := f.issue(t, commerce.Issue{Key: "bundle", Kind: commerce.BundleKind, CatalogKey: bundle, Bot: f.bot, Period: commerce.Monthly})
	if !f.decision(t, f.p1, f.bot).Allowed || !f.decision(t, p3, f.bot).Allowed || f.decision(t, p3, f.other).Allowed {
		t.Fatal("bundle sources")
	}
	f.issue(t, commerce.Issue{Key: "independent", Kind: commerce.PluginKind, CatalogKey: f.p1, Bot: f.bot, AdminGrant: true})
	_, e = f.s.SetState(ctx, f.admin.UserID, f.u.WorkspaceID, "test", b.ID, 1, commerce.Cancelled, "bundle revoked")
	must(t, e)
	if !f.decision(t, f.p1, f.bot).Allowed || f.decision(t, p3, f.bot).Allowed {
		t.Fatal("bundle revocation erased independent grant")
	}
	f.save(t, commerce.Version{Kind: commerce.AddonKind, Key: addon, Definition: commerce.Definition{Product: &commerce.Product{Name: "Additional units", Scope: sdk.PerBot, SaleEnabled: true, Prices: r3d2Prices(2), Quotas: map[string]int64{f.metric: 5}}}}, 0)
	q, e := f.s.Consume(ctx, f.u.UserID, f.u.WorkspaceID, f.bot, f.metric, 10)
	must(t, e)
	if !q.Allowed || q.Limit.Value != 10 {
		t.Fatal(q)
	}
	q, e = f.s.Check(ctx, f.u.UserID, f.u.WorkspaceID, f.bot, f.metric, 1)
	must(t, e)
	if q.Allowed || q.Code != "QUOTA_EXCEEDED" {
		t.Fatal(q)
	}
	f.issue(t, commerce.Issue{Key: "addon", Kind: commerce.AddonKind, CatalogKey: addon, Bot: f.bot, Period: commerce.Monthly})
	q, e = f.s.Check(ctx, f.u.UserID, f.u.WorkspaceID, f.bot, f.metric, 5)
	must(t, e)
	if !q.Allowed || q.Limit.Value != 15 {
		t.Fatal(q)
	}
	q, e = f.s.Check(ctx, f.u.UserID, f.u.WorkspaceID, f.other, f.metric, 11)
	must(t, e)
	if q.Allowed {
		t.Fatal("addon leaked")
	}
	var wg sync.WaitGroup
	var successes atomic.Int64
	errs := make(chan error, 8)
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q, e := f.s.Consume(ctx, f.u.UserID, f.u.WorkspaceID, f.bot, f.metric, 1)
			if e != nil {
				errs <- e
			} else if q.Allowed {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		must(t, e)
	}
	if successes.Load() != 5 {
		t.Fatal("quota race", successes.Load())
	}
	f.clock.at = f.clock.at.Add(24 * time.Hour)
	q, e = f.s.Check(ctx, f.u.UserID, f.u.WorkspaceID, f.bot, f.metric, 15)
	must(t, e)
	if !q.Allowed || q.Usage != 0 {
		t.Fatal("UTC reset", q)
	}
}
func TestR3D2CommercialSecurityAPI(t *testing.T) {
	f := r3d2Fixture(t)
	for _, op := range []func() error{func() error {
		_, e := f.s.Issue(ctx, f.u.UserID, f.u.WorkspaceID, "test", commerce.Issue{Key: "self-grant", Kind: commerce.PluginKind, CatalogKey: f.p1, Bot: f.bot, AdminGrant: true, Reason: "attempt"})
		return e
	}, func() error {
		_, e := f.s.SaveCatalog(ctx, f.u.UserID, f.u.WorkspaceID, "test", commerce.CatalogChange{Version: commerce.Version{Kind: commerce.PluginKind, Key: f.p1, Enabled: true, Definition: commerce.Definition{Plugin: &commerce.Plugin{Scope: sdk.PerBot, Prices: r3d2Prices(1)}}}, Expected: 1, Reason: "attempt"})
		return e
	}} {
		if e := op(); !errors.Is(e, database.ErrForbidden) {
			t.Fatal("non-admin commercial mutation", e)
		}
	}
	u, token := r3aLogin(t, 98002001, false)
	admin, adminToken := r3aLogin(t, 98002099, true)
	handler := r3aAPI()
	for _, path := range []string{"/api/v1/commercial/catalog", "/api/v1/commercial/agreements"} {
		r := r3aRequest(handler, token, u.WorkspaceID, "POST", path, map[string]any{})
		if r.Code != 400 && r.Code != 403 {
			t.Fatal(r.Code, r.Body.String())
		}
	}
	for _, method := range []string{"GET", "POST"} {
		var body any
		if method == "POST" {
			body = commerce.CatalogChange{Version: commerce.Version{Kind: commerce.PluginKind, Key: "r3d2-api", Enabled: true, Definition: commerce.Definition{Plugin: &commerce.Plugin{Scope: sdk.PerAccount, Prices: r3d2Prices(1)}}}, Reason: "test"}
		}
		r := r3aRequest(handler, token, u.WorkspaceID, method, "/api/v1/commercial/catalog", body)
		if r.Code != 403 {
			t.Fatal(r.Code, r.Body.String())
		}
	}
	r := r3aRequest(handler, adminToken, admin.WorkspaceID, "POST", "/api/v1/commercial/catalog", commerce.CatalogChange{Version: commerce.Version{Kind: commerce.PluginKind, Key: "r3d2-api", Enabled: true, Definition: commerce.Definition{Plugin: &commerce.Plugin{Scope: sdk.PerAccount, SaleEnabled: true, Prices: r3d2Prices(1)}}}, Reason: "synthetic API"})
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	for _, tc := range []struct {
		token    string
		expected int
	}{{token, 403}, {adminToken, 200}} {
		history := r3aRequest(handler, tc.token, u.WorkspaceID, "GET", "/api/v1/commercial/catalog/PLUGIN/r3d2-api/history", nil)
		if history.Code != tc.expected {
			t.Fatal("history RBAC", history.Code, history.Body.String())
		}
	}
	badTrial := r3aRequest(handler, token, u.WorkspaceID, "POST", "/api/v1/commercial/trials", map[string]any{"key": "bad-trial", "plugin": "r3d2-api", "duration_seconds": 9999})
	if badTrial.Code != 400 {
		t.Fatal("trial override", badTrial.Code)
	}
	r = r3aRequest(handler, token, u.WorkspaceID, "POST", "/api/v1/commercial/agreements", commerce.Issue{Key: "api-self-grant", Kind: commerce.PluginKind, CatalogKey: "r3d2-api", AdminGrant: true, Reason: "attempt"})
	if r.Code != 403 {
		t.Fatal(r.Code, r.Body.String())
	}
	if n := number(t, f.u, "SELECT count(*) FROM audit_logs WHERE workspace_id=$1 AND action LIKE 'commercial.%'", f.u.WorkspaceID); n < 4 {
		t.Fatal("audit absent", n)
	}
}
func TestR3D2RecoveryPreparation(t *testing.T) {
	f := r3d2Fixture(t)
	a := f.issue(t, commerce.Issue{Key: "r3d2-recovery", Kind: commerce.PluginKind, CatalogKey: f.p1, Bot: f.bot, Period: commerce.Monthly})
	if a.Terms.Price.Amount != 1000 {
		t.Fatal(a)
	}
	q, e := f.s.Consume(ctx, f.u.UserID, f.u.WorkspaceID, f.bot, f.metric, 4)
	must(t, e)
	if !q.Allowed {
		t.Fatal(q)
	}
}
func TestR3D2Recovered(t *testing.T) {
	migration, e := database.Open(ctx, os.Getenv("TEST_RESTORE_MIGRATION_DATABASE_URL"), false)
	must(t, e)
	defer migration.Pool.Close()
	var actor, account, bot, plugin, metric string
	must(t, migration.Pool.QueryRow(ctx, `SELECT w.owner_id::text,a.workspace_id::text,a.bot_id::text,a.catalog_key,u.metric FROM commercial_agreements a JOIN workspaces w ON w.id=a.workspace_id JOIN commercial_usage u ON u.workspace_id=a.workspace_id WHERE a.operation_key='r3d2-recovery'`).Scan(&actor, &account, &bot, &plugin, &metric))
	restored, e := database.Open(ctx, os.Getenv("TEST_RESTORE_DATABASE_URL"), true)
	must(t, e)
	defer restored.Pool.Close()
	s := &commerce.Service{DB: restored, Clock: &r3d2Clock{at: time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)}}
	d, e := s.Decide(ctx, actor, sdk.Scope{AccountID: account, BotID: bot, PluginID: plugin})
	must(t, e)
	if !d.Allowed {
		t.Fatal(d)
	}
	s.Clock = &r3d2Clock{at: time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)}
	q, e := s.Check(ctx, actor, account, bot, metric, 7)
	must(t, e)
	if q.Allowed || q.Usage != 4 {
		t.Fatal("restored quota", q)
	}
}
