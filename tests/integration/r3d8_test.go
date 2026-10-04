//go:build integration

package integration

import (
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/botfactory"
	"github.com/naseri-coder/TELMIVO/internal/domains/commerce"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"testing"
)

func TestR3D8StoreAdminSecurityAndLifecycle(t *testing.T) {
	f := r3d7Fixture(t)
	factory := &botfactory.Service{DB: db, Registry: f.core.Registry, Commercial: f.s}
	page, e := factory.Store(ctx, f.u.UserID, f.u.WorkspaceID, f.bot, botfactory.StoreFilter{Search: "forms"})
	must(t, e)
	if len(page.Items) != 1 {
		t.Fatal(page)
	}
	p, e := factory.StoreDetail(ctx, f.u.UserID, f.u.WorkspaceID, f.bot, "forms")
	must(t, e)
	if !p.Entitled || !p.Included || p.State != sdk.Active {
		t.Fatal(p)
	}
	outsider := fixture(t, false)
	_, e = factory.Store(ctx, outsider.UserID, f.u.WorkspaceID, f.bot, botfactory.StoreFilter{})
	if !errors.Is(e, database.ErrForbidden) {
		t.Fatal("tenant isolation", e)
	}
	_, e = factory.Store(ctx, f.u.UserID, f.u.WorkspaceID, r3dBot(t, outsider, "other"), botfactory.StoreFilter{})
	if e == nil {
		t.Fatal("bot isolation")
	}
	c, e := f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
	must(t, e)
	v := c[commerce.PluginKind]["forms"]
	_, e = f.s.SaveCatalog(ctx, f.u.UserID, f.u.WorkspaceID, "r3d8", commerce.CatalogChange{Version: v, Expected: v.Version, Reason: "not admin"})
	if !errors.Is(e, database.ErrForbidden) {
		t.Fatal("admin isolation", e)
	}
	v.Definition.Metadata = map[string]string{"store.visible": "true", "store.category": "business", "store.operational": "MAINTENANCE"}
	v = f.save(t, v, v.Version)
	_, e = f.s.SaveCatalog(ctx, f.admin.UserID, f.u.WorkspaceID, "stale", commerce.CatalogChange{Version: v, Expected: v.Version - 1, Reason: "stale"})
	if !errors.Is(e, database.ErrConflict) {
		t.Fatal("stale revision", e)
	}
	p, e = factory.StoreDetail(ctx, f.u.UserID, f.u.WorkspaceID, f.bot, "forms")
	must(t, e)
	if !p.Entitled || p.Installable || p.Store.Metadata.Operational != "MAINTENANCE" {
		t.Fatal("states collapsed", p)
	}
	disabled, e := factory.Change(ctx, f.u.UserID, f.u.WorkspaceID, f.bot, "forms", "disable", botfactory.Change{Action: sdk.Disable, Expected: p.Revision})
	must(t, e)
	_, e = factory.Change(ctx, f.u.UserID, f.u.WorkspaceID, f.bot, "forms", "activate", botfactory.Change{Action: sdk.Activate, Expected: disabled.Revision})
	if e == nil {
		t.Fatal("maintenance bypass")
	}
	if number(t, f.u, "SELECT count(*) FROM plugin_installations WHERE bot_id=$1 AND plugin_id='forms' AND state='DISABLED'", f.bot) != 1 {
		t.Fatal("disable destroyed installation")
	}
	v.Definition.Metadata["store.operational"] = "OPERATIONAL"
	v.Definition.Metadata["store.visible"] = "false"
	v = f.save(t, v, v.Version)
	page, e = factory.Store(ctx, f.u.UserID, f.u.WorkspaceID, f.bot, botfactory.StoreFilter{Search: "forms"})
	must(t, e)
	if len(page.Items) != 0 {
		t.Fatal("hidden exposed")
	}
	policy, e := f.s.Effective(ctx, f.admin.UserID, f.u.WorkspaceID, f.bot, "forms")
	must(t, e)
	if !policy.Decision.Allowed || policy.Inclusion != commerce.Included {
		t.Fatal(policy)
	}
	_, e = f.s.Effective(ctx, f.u.UserID, f.u.WorkspaceID, f.bot, "forms")
	if !errors.Is(e, database.ErrForbidden) {
		t.Fatal("effective admin leak", e)
	}
	if number(t, f.u, "SELECT count(*) FROM audit_logs WHERE action='commercial.catalog.version' AND entity_id='PLUGIN:forms'") == 0 {
		t.Fatal("missing audit")
	}
	v.Definition.Metadata["store.visible"] = "true"
	f.save(t, v, v.Version)
}
func TestR3D8CommercialPolicyValidationAndHistory(t *testing.T) {
	f := r3d2Fixture(t)
	c, e := f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
	must(t, e)
	v := c[commerce.PluginKind][f.p1]
	v.Definition.Plugin.Prices.Daily.Amount = -1
	_, e = f.s.SaveCatalog(ctx, f.admin.UserID, f.u.WorkspaceID, "invalid", commerce.CatalogChange{Version: v, Expected: v.Version, Reason: "invalid"})
	if !errors.Is(e, database.ErrInvalid) {
		t.Fatal(e)
	}
	v.Definition.Plugin.Prices = r3d2Prices(900)
	v.Definition.Plugin.Trial.DurationSeconds = 120
	v.Definition.Plugin.GraceSeconds = 60
	f.save(t, v, v.Version)
	if f.decision(t, f.p1, f.bot).Allowed {
		t.Fatal("price silently grants access")
	}
	h, e := f.s.History(ctx, f.admin.UserID, f.u.WorkspaceID, commerce.PluginKind, f.p1)
	must(t, e)
	if len(h) != 2 {
		t.Fatal("history", h)
	}
	plan := c[commerce.PlanKind][f.plan]
	plan.Definition.Plan.Plugins[f.p1] = commerce.Included
	f.save(t, plan, plan.Version)
	if !f.decision(t, f.p1, f.bot).Allowed {
		t.Fatal("plan inclusion")
	}
	q := c[commerce.QuotaKind][f.metric]
	q.Definition.Quota.Default.Value = 1
	f.save(t, q, q.Version)
	f.save(t, commerce.Version{Kind: commerce.BundleKind, Key: f.p1 + "-bundle", Definition: commerce.Definition{Product: &commerce.Product{Name: "Bundle", Scope: sdk.PerBot, SaleEnabled: false, Prices: r3d2Prices(0), Plugins: []string{f.p1}, Quotas: map[string]int64{f.metric: 1}}}}, 0)
	f.save(t, commerce.Version{Kind: commerce.AddonKind, Key: f.p1 + "-addon", Definition: commerce.Definition{Product: &commerce.Product{Name: "Addon", Scope: sdk.PerBot, SaleEnabled: false, Prices: r3d2Prices(0), Quotas: map[string]int64{f.metric: 1}}}}, 0)
}
func TestR3D8BrowserFixture(t *testing.T) {
	f := r3d7Fixture(t)
	u, _ := r3aLogin(t, 98008001, false)
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE subscriptions SET plan_key=$1,expires_at=NULL WHERE workspace_id=$2", f.plan, u.WorkspaceID)
		return e
	})
	bot := r3dBot(t, u, "R3D8 Store browser bot")
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE customer_bots SET name='R3D8 Store browser bot' WHERE id=$1", bot)
		return e
	})
	scope := sdk.Scope{AccountID: u.WorkspaceID, BotID: bot}
	for _, id := range []string{"crm", "forms"} {
		scope.PluginID = id
		i, e := f.core.Change(ctx, u.UserID, scope, sdk.Install, 0, json.RawMessage(`{}`), "r3d8")
		must(t, e)
		_, e = f.core.Change(ctx, u.UserID, scope, sdk.Activate, i.Revision, nil, "r3d8")
		must(t, e)
	}
}
