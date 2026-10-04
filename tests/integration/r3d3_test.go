//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/botfactory"
	"github.com/naseri-coder/TELMIVO/internal/domains/commerce"
	"github.com/naseri-coder/TELMIVO/internal/domains/identity"
	platform "github.com/naseri-coder/TELMIVO/internal/domains/pluginplatform"
	"github.com/naseri-coder/TELMIVO/internal/platform/config"
	cred "github.com/naseri-coder/TELMIVO/internal/platform/crypto"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/httpapi"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/platform/testfixtures"
)

type factoryFixture struct {
	commercialFixture
	factory  *botfactory.Service
	registry *platform.Registry
}

func r3d3Fixture(t *testing.T) factoryFixture {
	t.Helper()
	f := r3d2Fixture(t)
	f.clock.at = time.Now().UTC().Truncate(time.Microsecond)
	registry, e := testfixtures.FactoryRegistry()
	must(t, e)
	keys, e := cred.NewAESStore("primary-v1", bytes.Repeat([]byte{1}, 32))
	must(t, e)
	for _, id := range registry.IDs() {
		scope := sdk.PerBot
		if id == "factory-account" {
			scope = sdk.PerAccount
		}
		f.save(t, commerce.Version{Kind: commerce.PluginKind, Key: id, Definition: commerce.Definition{Plugin: &commerce.Plugin{Scope: scope, Capabilities: []string{"telegram.send"}, SaleEnabled: true, Prices: r3d2Prices(20), Trial: commerce.TrialPolicy{Enabled: true, DurationSeconds: 3600}}}}, factoryVersion(t, f.s, f.admin, f.u, commerce.PluginKind, id))
	}
	c, e := f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
	must(t, e)
	plan, _ := c.Get(commerce.PlanKind, f.plan)
	plan.Definition.Plan.Plugins = map[string]commerce.Inclusion{"factory-basic": commerce.Included, "factory-dependent": commerce.Included, "factory-account": commerce.Included, "factory-paid": commerce.PaidAddon, "factory-conflict": commerce.Included}
	f.save(t, plan, 1)
	return factoryFixture{f, &botfactory.Service{DB: db, Registry: registry, Commercial: f.s, Credentials: keys}, registry}
}
func factoryVersion(t *testing.T, s *commerce.Service, admin, u identity.Result, k commerce.Kind, id string) int64 {
	t.Helper()
	history, e := s.History(ctx, admin.UserID, u.WorkspaceID, k, id)
	must(t, e)
	if len(history) == 0 {
		return 0
	}
	return history[len(history)-1].Version
}
func factoryRequest(t *testing.T, f factoryFixture, ids []string, key string, activate bool) botfactory.Create {
	t.Helper()
	selection := botfactory.Selection{Requested: ids, Configurations: map[string]json.RawMessage{}, Activate: activate}
	r, e := f.factory.Preview(ctx, f.u.UserID, f.u.WorkspaceID, "", selection)
	must(t, e)
	return botfactory.Create{Name: "ربات آزمایشی", Token: fmt.Sprint(990000+sequence.Add(1)) + ":" + strings.Repeat("x", 35), Key: key, Selection: selection, Accepted: r.Order, ReviewDigest: r.Digest}
}
func factoryCreate(t *testing.T, f factoryFixture, ids []string, key string, activate bool) botfactory.Bot {
	t.Helper()
	bot, e := f.factory.Create(ctx, f.u.UserID, f.u.WorkspaceID, "r3d3", factoryRequest(t, f, ids, key, activate))
	must(t, e)
	return bot
}
func TestR3D3PreviewAndCommercialCatalog(t *testing.T) {
	f := r3d3Fixture(t)
	before := number(t, f.u, "SELECT count(*) FROM customer_bots WHERE workspace_id=$1", f.u.WorkspaceID)
	agreements := number(t, f.u, "SELECT count(*) FROM commercial_agreements WHERE workspace_id=$1", f.u.WorkspaceID)
	items, e := f.factory.Catalog(ctx, f.u.UserID, f.u.WorkspaceID, "")
	must(t, e)
	statuses := map[string]botfactory.Item{}
	for _, p := range items {
		statuses[p.ID] = p
	}
	if statuses["factory-basic"].Status != "INCLUDED" || statuses["factory-paid"].Status != "TRIAL_AVAILABLE" || statuses["factory-account"].Scope != sdk.PerAccount || statuses["factory-basic"].Scope != sdk.PerBot || statuses["factory-paid"].Prices.Daily.Amount != 20 {
		t.Fatal("commercial presentation")
	}
	for _, tc := range []struct {
		ids      []string
		ready    bool
		required int
	}{{nil, true, 0}, {[]string{"factory-basic"}, true, 0}, {[]string{"factory-dependent"}, true, 1}, {[]string{"factory-paid"}, false, 0}, {[]string{"factory-basic", "factory-conflict"}, false, 0}, {[]string{"unknown"}, false, 0}} {
		r, e := f.factory.Preview(ctx, f.u.UserID, f.u.WorkspaceID, "", botfactory.Selection{Requested: tc.ids})
		must(t, e)
		if r.Ready != tc.ready || len(r.Required) != tc.required {
			t.Fatal(r)
		}
		again, e := f.factory.Preview(ctx, f.u.UserID, f.u.WorkspaceID, "", botfactory.Selection{Requested: tc.ids})
		must(t, e)
		if r.Digest != again.Digest {
			t.Fatal("nondeterministic preview")
		}
	}
	// A paid required dependency must appear explicitly, never grant access silently.
	manifests := []sdk.Manifest{r3dManifest("factory-paid"), r3dManifest("factory-needs-paid", "factory-paid")}
	for i := range manifests {
		manifests[i].Settings = sdk.SettingsSchema{Properties: map[string]sdk.Setting{}}
	}
	r, e := platform.NewRegistry("0.3.0", manifests)
	must(t, e)
	copyService := *f.factory
	copyService.Registry = r
	review, e := copyService.Preview(ctx, f.u.UserID, f.u.WorkspaceID, "", botfactory.Selection{Requested: []string{"factory-needs-paid"}})
	must(t, e)
	if review.Ready || len(review.Required) != 1 || review.Required[0] != "factory-paid" {
		t.Fatal(review)
	}
	if number(t, f.u, "SELECT count(*) FROM customer_bots WHERE workspace_id=$1", f.u.WorkspaceID) != before || number(t, f.u, "SELECT count(*) FROM commercial_agreements WHERE workspace_id=$1", f.u.WorkspaceID) != agreements {
		t.Fatal("preview mutated")
	}
	// Unavailable and paid-only labels are driven by catalog/plan data.
	conflict, e := f.factory.Preview(ctx, f.u.UserID, f.u.WorkspaceID, "", botfactory.Selection{Requested: []string{"factory-basic", "factory-conflict"}})
	must(t, e)
	if len(conflict.Issues) != 1 || conflict.Issues[0].Code != "PLUGIN_CONFLICT" || conflict.Issues[0].Plugin != "factory-conflict" || conflict.Issues[0].Other != "factory-basic" {
		t.Fatal("conflict identities missing", conflict)
	}
	c, e := f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
	must(t, e)
	v, _ := c.Get(commerce.PluginKind, "factory-paid")
	v.Definition.Plugin.Trial.Enabled = false
	f.save(t, v, v.Version)
	items, e = f.factory.Catalog(ctx, f.u.UserID, f.u.WorkspaceID, "")
	must(t, e)
	for _, p := range items {
		if p.ID == "factory-paid" && p.Status != "PAID" {
			t.Fatal(p)
		}
	}
	v.Definition.Plugin.SaleEnabled = false
	f.save(t, v, v.Version+1)
	items, e = f.factory.Catalog(ctx, f.u.UserID, f.u.WorkspaceID, "")
	must(t, e)
	for _, p := range items {
		if p.ID == "factory-paid" && p.Status != "SALE_DISABLED" {
			t.Fatal(p)
		}
	}
	plan, _ := c.Get(commerce.PlanKind, f.plan)
	plan.Definition.Plan.Plugins["factory-paid"] = commerce.Unavailable
	f.save(t, plan, plan.Version)
	items, e = f.factory.Catalog(ctx, f.u.UserID, f.u.WorkspaceID, "")
	must(t, e)
	for _, p := range items {
		if p.ID == "factory-paid" && (p.Status != "UNAVAILABLE" || p.TrialEligible) {
			t.Fatal(p)
		}
	}
}
func TestR3D3AtomicCreationAndReplay(t *testing.T) {
	f := r3d3Fixture(t)
	in := factoryRequest(t, f, []string{"factory-dependent"}, "factory-create", true)
	var wg sync.WaitGroup
	bots := make(chan botfactory.Bot, 2)
	errs := make(chan error, 2)
	for n := 0; n < 2; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, e := f.factory.Create(ctx, f.u.UserID, f.u.WorkspaceID, "r3d3", in)
			if e != nil {
				errs <- e
			} else {
				bots <- b
			}
		}()
	}
	wg.Wait()
	close(bots)
	close(errs)
	for e := range errs {
		must(t, e)
	}
	var bot botfactory.Bot
	for b := range bots {
		if bot.ID != "" && bot.ID != b.ID {
			t.Fatal("double creation")
		}
		bot = b
	}
	if bot.Status != "DRAFT" {
		t.Fatal("fake verification")
	}
	if number(t, f.u, "SELECT count(*) FROM plugin_installations WHERE bot_id=$1 AND state='ACTIVE'", bot.ID) != 2 {
		t.Fatal("dependency installation")
	}
	in.Name = "changed"
	if _, e := f.factory.Create(ctx, f.u.UserID, f.u.WorkspaceID, "r3d3", in); !errors.Is(e, database.ErrConflict) {
		t.Fatal(e)
	}
	in.Name = "ربات آزمایشی"
	in.Key = "duplicate-credential"
	if _, e := f.factory.Create(ctx, f.u.UserID, f.u.WorkspaceID, "r3d3", in); e == nil {
		t.Fatal("duplicate token accepted")
	}
	before := number(t, f.u, "SELECT count(*) FROM customer_bots WHERE workspace_id=$1", f.u.WorkspaceID)
	for _, ids := range [][]string{{"factory-paid"}, {"factory-basic", "factory-conflict"}} {
		bad := factoryRequest(t, f, ids, "rejected-"+ids[0], true)
		if _, e := f.factory.Create(ctx, f.u.UserID, f.u.WorkspaceID, "r3d3", bad); e == nil {
			t.Fatal("unsafe selection")
		}
	}
	if number(t, f.u, "SELECT count(*) FROM customer_bots WHERE workspace_id=$1", f.u.WorkspaceID) != before {
		t.Fatal("partial bot persisted")
	}
	// A veto after the first dependency is persisted must roll back everything.
	basic, _ := f.registry.Manifest("factory-basic")
	dependent, _ := f.registry.Manifest("factory-dependent")
	dependent.LifecycleHooks = []sdk.Action{sdk.Install}
	hooked, e := platform.NewRegistry("0.3.0", []sdk.Manifest{basic, dependent})
	must(t, e)
	copyService := *f.factory
	copyService.Registry, copyService.Hooks = hooked, r3dHook{}
	input := factoryRequest(t, f, []string{"factory-dependent"}, "hook-veto", false)
	if _, e = copyService.Create(ctx, f.u.UserID, f.u.WorkspaceID, "r3d3", input); e == nil {
		t.Fatal("hook veto ignored")
	}
	if number(t, f.u, "SELECT count(*) FROM customer_bots WHERE workspace_id=$1", f.u.WorkspaceID) != before || number(t, f.u, "SELECT count(*) FROM plugin_installations WHERE workspace_id=$1", f.u.WorkspaceID) != 2 {
		t.Fatal("partial provisioning survived hook veto")
	}
	zero := factoryCreate(t, f, nil, "empty", false)
	if number(t, f.u, "SELECT count(*) FROM plugin_installations WHERE bot_id=$1", zero.ID) != 0 {
		t.Fatal("zero plugin creation")
	}
	var secret cred.EncryptedSecret
	tenant(t, f.u, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT key_id,encrypted_token FROM customer_bots WHERE id=$1", bot.ID).Scan(&secret.KeyID, &secret.Ciphertext)
	})
	plain, e := f.factory.Credentials.Open(ctx, f.u.WorkspaceID+":"+bot.ID, secret)
	must(t, e)
	if string(plain) != in.Token {
		t.Fatal("credential round-trip mismatch")
	}
	if _, e = f.factory.Credentials.Open(ctx, "other:"+bot.ID, secret); e == nil {
		t.Fatal("unbound credential")
	}
	body, _ := json.Marshal(bot)
	if strings.Contains(string(body), in.Token) || strings.Contains(fmt.Sprintf("%+v %#v", in, in), in.Token) {
		t.Fatal("token leaked")
	}
	for _, change := range []func(*botfactory.Create){func(i *botfactory.Create) { i.Name = "" }, func(i *botfactory.Create) { i.Token = "invalid" }, func(i *botfactory.Create) { i.Key = "" }} {
		bad := in
		change(&bad)
		if _, e = f.factory.Create(ctx, f.u.UserID, f.u.WorkspaceID, "r3d3", bad); !errors.Is(e, database.ErrInvalid) {
			t.Fatal("invalid input", e)
		}
	}
}
func TestR3D3LifecycleIsolationAndAccessLoss(t *testing.T) {
	f := r3d3Fixture(t)
	a := factoryCreate(t, f, []string{"factory-basic", "factory-account"}, "first", false)
	b := factoryCreate(t, f, []string{"factory-basic"}, "second", false)
	change := func(bot, id string, action sdk.Action, rev int64) botfactory.Item {
		t.Helper()
		item, e := f.factory.Change(ctx, f.u.UserID, f.u.WorkspaceID, bot, id, "r3d3", botfactory.Change{Action: action, Expected: rev, Configuration: func() json.RawMessage {
			if action == sdk.Install {
				return json.RawMessage(`{}`)
			}
			return nil
		}()})
		must(t, e)
		return item
	}
	accountA := change(a.ID, "factory-account", sdk.Activate, 1)
	accountB := change(b.ID, "factory-account", sdk.Install, 0)
	accountB = change(b.ID, accountB.ID, sdk.Activate, accountB.Revision)
	change(a.ID, accountA.ID, sdk.Disable, accountA.Revision)
	accountItems, e := f.factory.Catalog(ctx, f.u.UserID, f.u.WorkspaceID, b.ID)
	must(t, e)
	for _, item := range accountItems {
		if item.ID == "factory-account" && (!item.Entitled || item.State != sdk.Active) {
			t.Fatal("account license changed another bot lifecycle", item)
		}
	}
	p := change(a.ID, "factory-basic", sdk.Activate, 1)
	p = change(a.ID, "factory-basic", sdk.Disable, p.Revision)
	items, e := f.factory.Catalog(ctx, f.u.UserID, f.u.WorkspaceID, b.ID)
	must(t, e)
	for _, item := range items {
		if item.ID == "factory-basic" && item.State != sdk.Installed {
			t.Fatal("bot state leaked")
		}
	}
	if _, e = f.factory.Change(ctx, f.u.UserID, f.u.WorkspaceID, a.ID, p.ID, "r3d3", botfactory.Change{Action: sdk.Activate, Expected: 1}); !errors.Is(e, database.ErrConflict) {
		t.Fatal("stale revision")
	}
	p = change(a.ID, p.ID, sdk.Activate, p.Revision)
	tenant(t, f.u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE plugin_installations SET data='{"retained":"factory"}',revision=revision+1 WHERE bot_id=$1 AND plugin_id=$2`, a.ID, p.ID)
		return e
	})
	p.Revision++
	_, e = f.s.StartTrial(ctx, f.u.UserID, f.u.WorkspaceID, "r3d3", "trial-on-a", "factory-paid", a.ID)
	must(t, e)
	paid, e := f.factory.Change(ctx, f.u.UserID, f.u.WorkspaceID, a.ID, "factory-paid", "r3d3", botfactory.Change{Action: sdk.Install, Expected: 0, Configuration: json.RawMessage(`{}`)})
	must(t, e)
	paid = change(a.ID, paid.ID, sdk.Activate, paid.Revision)
	if _, e = f.factory.Change(ctx, f.u.UserID, f.u.WorkspaceID, b.ID, "factory-paid", "r3d3", botfactory.Change{Action: sdk.Install, Configuration: json.RawMessage(`{}`)}); !errors.Is(e, sdk.ErrPermission) {
		t.Fatal("per-bot entitlement leaked", e)
	}
	// Fixture-only verification simulates the future connection stage; production creation stays DRAFT.
	tenant(t, f.u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE customer_bots SET status='RUNNING' WHERE id=$1", a.ID)
		return e
	})
	coreBefore := &platform.Service{DB: db, Registry: f.registry, Policy: f.s}
	handlerBefore := &r3dHandler{}
	_, e = (&platform.Runtime{Service: coreBefore, Handler: handlerBefore}).Bind(f.u.UserID, sdk.Scope{AccountID: f.u.WorkspaceID, BotID: a.ID, PluginID: paid.ID}).Invoke(ctx, "telegram.send", json.RawMessage(`{}`))
	must(t, e)
	tenant(t, f.u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE plugin_installations SET data='{"retained":"expired-trial"}',revision=revision+1 WHERE bot_id=$1 AND plugin_id=$2`, a.ID, paid.ID)
		return e
	})
	paid.Revision++
	f.clock.at = f.clock.at.Add(time.Hour)
	core := &platform.Service{DB: db, Registry: f.registry, Policy: f.s}
	handler := &r3dHandler{}
	if _, e = (&platform.Runtime{Service: core, Handler: handler}).Bind(f.u.UserID, sdk.Scope{AccountID: f.u.WorkspaceID, BotID: a.ID, PluginID: paid.ID}).Invoke(ctx, "telegram.send", json.RawMessage(`{}`)); !errors.Is(e, sdk.ErrPermission) {
		t.Fatal("expired factory trial executed", e)
	}
	paid = change(a.ID, paid.ID, sdk.Disable, paid.Revision)
	if number(t, f.u, "SELECT count(*) FROM plugin_installations WHERE bot_id=$1 AND plugin_id=$2 AND data->>'retained'='expired-trial'", a.ID, paid.ID) != 1 {
		t.Fatal("expiry/disable deleted retained trial data")
	}
	paid = change(a.ID, paid.ID, sdk.RequestUninstall, paid.Revision)
	if paid.State != sdk.UninstallPending {
		t.Fatal(paid)
	}
	paid = change(a.ID, paid.ID, sdk.CompleteUninstall, paid.Revision)
	if paid.State != sdk.Uninstalled {
		t.Fatal(paid)
	}
	p = change(a.ID, p.ID, sdk.Disable, p.Revision)
	if number(t, f.u, "SELECT count(*) FROM plugin_installations WHERE bot_id=$1 AND plugin_id=$2 AND data->>'retained'='factory'", a.ID, p.ID) != 1 {
		t.Fatal("disable deleted data")
	}
	// Active dependents require explicit safe ordering.
	d := factoryCreate(t, f, []string{"factory-dependent"}, "dependent", true)
	impact, e := f.factory.Impact(ctx, f.u.UserID, f.u.WorkspaceID, d.ID, "factory-basic", botfactory.Change{Action: sdk.Disable, Expected: 2})
	must(t, e)
	if impact.Ready || len(impact.Issues) == 0 {
		t.Fatal(impact)
	}
	if _, e = f.factory.Change(ctx, f.u.UserID, f.u.WorkspaceID, d.ID, "factory-basic", "r3d3", botfactory.Change{Action: sdk.Disable, Expected: 2}); !errors.Is(e, sdk.ErrDependency) {
		t.Fatal(e)
	}
}
func TestR3D3SecurityAndAPI(t *testing.T) {
	f := r3d3Fixture(t)
	other := fixture(t, false)
	bot := factoryCreate(t, f, nil, "security", false)
	if _, e := f.factory.Catalog(ctx, other.UserID, f.u.WorkspaceID, bot.ID); !errors.Is(e, database.ErrForbidden) {
		t.Fatal("cross account")
	}
	member := fixture(t, false)
	tenant(t, f.u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "INSERT INTO workspace_members(workspace_id,user_id,role_key) VALUES($1,$2,'ANALYST')", f.u.WorkspaceID, member.UserID)
		return e
	})
	if _, e := f.factory.Catalog(ctx, member.UserID, f.u.WorkspaceID, bot.ID); !errors.Is(e, database.ErrForbidden) {
		t.Fatal("member without management")
	}
	u, token := r3aLogin(t, 98003091, false)
	api := (&httpapi.API{DB: db, Identity: r3aService(), Factory: f.factory, Config: config.Config{Environment: "test", Synthetic: true, MiniAppURL: "https://mini.test"}}).Handler()
	r := r3aRequest(api, token, u.WorkspaceID, "GET", "/api/v1/customer-bots/catalog", nil)
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	if strings.Contains(r.Body.String(), "capabilities") || strings.Contains(r.Body.String(), "encrypted_token") {
		t.Fatal("internal metadata exposed")
	}
	var captured bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&captured, nil)))
	defer slog.SetDefault(previous)
	in := factoryRequest(t, f, nil, "api-secret", false)
	r = r3aRequest(api, token, u.WorkspaceID, "POST", "/api/v1/customer-bots", in)
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	if strings.Contains(r.Body.String(), in.Token) || strings.Contains(captured.String(), in.Token) {
		t.Fatal("token leaked to API/log")
	}
	var created botfactory.Bot
	must(t, json.Unmarshal(r.Body.Bytes(), &created))
	r = r3aRequest(api, token, u.WorkspaceID, "POST", "/api/v1/customer-bots/"+created.ID+"/plugins/factory-paid/actions", botfactory.Change{Action: sdk.Install, Configuration: json.RawMessage(`{}`)})
	if r.Code != 403 || !strings.Contains(r.Body.String(), `"decision"`) || !strings.Contains(r.Body.String(), "PLAN_UNAVAILABLE") {
		t.Fatal("structured commercial denial lost", r.Code, r.Body.String())
	}
	r = r3aRequest(api, token, u.WorkspaceID, "POST", "/api/v1/customer-bots", map[string]any{"name": "invalid", "token": "bad", "idempotency_key": "invalid", "price": 0})
	if r.Code != 400 {
		t.Fatal("commercial field accepted")
	}
}
func TestR3D3BrowserFixture(t *testing.T) {
	f := r3d3Fixture(t)
	u, _ := r3aLogin(t, 98003001, false)
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE subscriptions SET plan_key=$1,expires_at=NULL WHERE workspace_id=$2", f.plan, u.WorkspaceID)
		return e
	})
	// Browser API uses wall clock, so catalog fixtures must be effective now.
	f.clock.at = time.Now().UTC().Truncate(time.Microsecond)
	for _, id := range f.registry.IDs() {
		c, e := f.s.Catalog(ctx, f.admin.UserID, u.WorkspaceID)
		must(t, e)
		v, _ := c.Get(commerce.PluginKind, id)
		f.u = u
		f.save(t, v, v.Version)
	}
}
func TestR3D3RecoveryPreparation(t *testing.T) {
	f := r3d3Fixture(t)
	in := factoryRequest(t, f, []string{"factory-basic"}, "r3d3-recovery", false)
	bot, e := f.factory.Create(ctx, f.u.UserID, f.u.WorkspaceID, "r3d3", in)
	must(t, e)
	if bot.ID == "" {
		t.Fatal("missing recovery bot")
	}
}
func TestR3D3Recovered(t *testing.T) {
	migration, e := database.Open(ctx, os.Getenv("TEST_RESTORE_MIGRATION_DATABASE_URL"), false)
	must(t, e)
	defer migration.Pool.Close()
	var actor, account, bot string
	must(t, migration.Pool.QueryRow(ctx, `SELECT w.owner_id::text,b.workspace_id::text,b.id::text FROM customer_bots b JOIN workspaces w ON w.id=b.workspace_id WHERE b.idempotency_key='r3d3-recovery'`).Scan(&actor, &account, &bot))
	restored, e := database.Open(ctx, os.Getenv("TEST_RESTORE_DATABASE_URL"), true)
	must(t, e)
	defer restored.Pool.Close()
	registry, e := testfixtures.FactoryRegistry()
	must(t, e)
	keys, e := cred.NewAESStore("primary-v1", bytes.Repeat([]byte{1}, 32))
	must(t, e)
	s := &botfactory.Service{DB: restored, Registry: registry, Commercial: &commerce.Service{DB: restored, Clock: &r3d2Clock{at: time.Date(2027, 1, 1, 12, 0, 0, 0, time.UTC)}}, Credentials: keys}
	items, e := s.Catalog(ctx, actor, account, bot)
	must(t, e)
	for _, item := range items {
		if item.ID == "factory-basic" && item.State != sdk.Installed {
			t.Fatal("restored installation")
		}
	}
}
