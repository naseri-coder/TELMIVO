//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/commerce"
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	platform "github.com/naseri-coder/TELMIVO/internal/domains/pluginplatform"
	"github.com/naseri-coder/TELMIVO/internal/platform/config"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/httpapi"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	gate "github.com/naseri-coder/TELMIVO/internal/plugins/accessgate"
	files "github.com/naseri-coder/TELMIVO/internal/plugins/filemanager"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type r3d4Transport struct {
	states                 map[int64]string
	state, code            string
	sends, checks, prompts atomic.Int64
	outcome                string
	lastKind, lastFile     string
}

func (t *r3d4Transport) Check(_ context.Context, channel int64, _ int64) (string, string) {
	t.checks.Add(1)
	if t.states != nil {
		return t.states[channel], t.code
	}
	return t.state, t.code
}
func (t *r3d4Transport) SendFile(_ context.Context, _ int64, kind, id, title string) (int64, string) {
	t.sends.Add(1)
	t.lastKind, t.lastFile = kind, id
	if t.outcome != "" {
		return 0, t.outcome
	}
	return 101, "DELIVERY_ACKNOWLEDGED"
}
func (t *r3d4Transport) Prompt(context.Context, int64, string, any) error {
	t.prompts.Add(1)
	return nil
}
func (t *r3d4Transport) Connect(context.Context, string, string) (string, error) {
	return "r3d4_connected_test", nil
}

type r3d4Provider struct{ transport *r3d4Transport }

func (p r3d4Provider) ForBot(context.Context, pgx.Tx, sdk.Scope) (contentplugins.Transport, error) {
	return p.transport, nil
}

type r3d4FixtureData struct {
	commercialFixture
	content   *contentplugins.Service
	core      *platform.Service
	transport *r3d4Transport
}

func r3d4Fixture(t *testing.T) r3d4FixtureData {
	t.Helper()
	f := r3d2Fixture(t)
	f.clock.at = time.Now().UTC().Truncate(time.Microsecond)
	registry, e := contentplugins.Registry()
	must(t, e)
	for _, id := range registry.IDs() {
		m, _ := registry.Manifest(id)
		v := commerce.Version{Kind: commerce.PluginKind, Key: id, Definition: commerce.Definition{Plugin: &commerce.Plugin{Scope: sdk.PerBot, SaleEnabled: true, Capabilities: m.Permissions, Prices: r3d2Prices(20)}}}
		f.save(t, v, factoryVersion(t, f.s, f.admin, f.u, commerce.PluginKind, id))
	}
	c, e := f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
	must(t, e)
	p, _ := c.Get(commerce.PlanKind, f.plan)
	p.Definition.Plan.Plugins = map[string]commerce.Inclusion{gate.ID: commerce.Included, files.ID: commerce.Included}
	f.save(t, p, p.Version)
	core := &platform.Service{DB: db, Registry: registry, Policy: f.s}
	tr := &r3d4Transport{state: "NOT_MEMBER"}
	s := &contentplugins.Service{DB: db, Core: core, Commercial: f.s, Provider: r3d4Provider{tr}, DigestKey: bytes.Repeat([]byte{7}, 32)}
	out := r3d4FixtureData{f, s, core, tr}
	for _, id := range []string{gate.ID, files.ID} {
		out.change(t, id, sdk.Install, 0)
		out.change(t, id, sdk.Activate, 1)
	}
	return out
}
func (f r3d4FixtureData) scope(id string) sdk.Scope {
	return sdk.Scope{AccountID: f.u.WorkspaceID, BotID: f.bot, PluginID: id}
}
func (f r3d4FixtureData) change(t *testing.T, id string, a sdk.Action, rev int64) sdk.Installation {
	t.Helper()
	var cfg json.RawMessage
	if a == sdk.Install {
		cfg = json.RawMessage(`{}`)
	}
	i, e := f.core.Change(ctx, f.u.UserID, f.scope(id), a, rev, cfg, "r3d4")
	must(t, e)
	return i
}
func (f r3d4FixtureData) policy(t *testing.T) gate.Policy {
	t.Helper()
	p, e := f.content.SavePolicy(ctx, f.u.UserID, f.scope(gate.ID), "", "r3d4", contentplugins.PolicyChange{Policy: gate.Policy{Name: "Premium", Enabled: true, Mode: "ALL", Requirements: []gate.Requirement{{Channel: -1000000000001, JoinURL: "https://t.me/channel_one"}}}, Key: "policy"})
	must(t, e)
	return p
}
func (f r3d4FixtureData) file(t *testing.T, policy, key string) files.File {
	t.Helper()
	p, e := f.content.SaveFile(ctx, f.u.UserID, f.scope(files.ID), "", "r3d4", contentplugins.FileChange{File: files.File{FileID: "synthetic-file-" + key, UniqueID: "unique-" + key, Kind: "document", Title: "File " + key, Category: "Course", Policy: policy, Enabled: true}, Key: key})
	must(t, e)
	return p
}
func (f r3d4FixtureData) deliver(t *testing.T, ref, key string) contentplugins.Result {
	t.Helper()
	r, e := f.content.Deliver(ctx, f.u.UserID, f.scope(files.ID), files.Request{Reference: ref, User: 123, Key: key})
	must(t, e)
	return r
}
func TestR3D4Policies(t *testing.T) {
	f := r3d4Fixture(t)
	p := f.policy(t)
	if f.policy(t).ID != p.ID {
		t.Fatal("policy retry duplicated")
	}
	ps, e := f.content.Policies(ctx, f.u.UserID, f.scope(gate.ID))
	must(t, e)
	if len(ps) != 1 || ps[0].ID != p.ID {
		t.Fatal(ps)
	}
	runtime := &platform.Runtime{Service: f.core, Handler: f.content}
	plugin := gate.Plugin{Runtime: runtime.Bind(f.u.UserID, f.scope(gate.ID))}
	d, e := plugin.Authorize(ctx, p.ID, 123)
	must(t, e)
	if d.Allowed || d.Code != "DENIED_MISSING_MEMBERSHIP" {
		t.Fatal(d)
	}
	f.transport.state = "MEMBER"
	d, e = plugin.Authorize(ctx, p.ID, 123)
	must(t, e)
	if !d.Allowed {
		t.Fatal(d)
	}
	p.Mode = "ANY"
	p.Requirements = append(p.Requirements, gate.Requirement{Channel: -1000000000002})
	p, e = f.content.SavePolicy(ctx, f.u.UserID, f.scope(gate.ID), p.ID, "r3d4", contentplugins.PolicyChange{Policy: p, Key: "any", Expected: p.Revision})
	must(t, e)
	f.transport.states = map[int64]string{-1000000000001: "MEMBER", -1000000000002: "NOT_MEMBER"}
	d, e = plugin.Authorize(ctx, p.ID, 123)
	must(t, e)
	if !d.Allowed {
		t.Fatal("ANY", d)
	}
	p.Mode = "ALL"
	p, e = f.content.SavePolicy(ctx, f.u.UserID, f.scope(gate.ID), p.ID, "r3d4", contentplugins.PolicyChange{Policy: p, Key: "all", Expected: p.Revision})
	must(t, e)
	d, e = plugin.Authorize(ctx, p.ID, 123)
	must(t, e)
	if d.Allowed {
		t.Fatal("ALL missing accepted")
	}
	f.transport.states = nil
	f.transport.state = "CHECK_FAILED"
	f.transport.code = "CHANNEL_NOT_ACCESSIBLE"
	d, e = plugin.Authorize(ctx, p.ID, 123)
	must(t, e)
	if d.Allowed || d.Checks[0].Code != "CHANNEL_NOT_ACCESSIBLE" {
		t.Fatal(d)
	}
	p.Enabled = false
	p, e = f.content.SavePolicy(ctx, f.u.UserID, f.scope(gate.ID), p.ID, "r3d4", contentplugins.PolicyChange{Policy: p, Key: "update", Expected: p.Revision})
	must(t, e)
	d, e = plugin.Authorize(ctx, p.ID, 123)
	must(t, e)
	if d.Allowed || d.Code != "POLICY_DISABLED" {
		t.Fatal(d)
	}
	if _, e = f.content.SavePolicy(ctx, f.u.UserID, f.scope(gate.ID), p.ID, "r3d4", contentplugins.PolicyChange{Policy: p, Key: "stale", Expected: 1}); !errors.Is(e, database.ErrConflict) {
		t.Fatal("stale", e)
	}
	f.change(t, gate.ID, sdk.Disable, 2)
	if _, e = plugin.Authorize(ctx, p.ID, 123); !errors.Is(e, sdk.ErrState) {
		t.Fatal("disabled gate", e)
	}
	if number(t, f.u, "SELECT count(*) FROM content_access_policies WHERE bot_id=$1", f.bot) != 1 {
		t.Fatal("lost policy")
	}
}
func TestR3D4FilesAndDelivery(t *testing.T) {
	f := r3d4Fixture(t)
	p := f.policy(t)
	protected := f.file(t, p.ID, "protected")
	public := f.file(t, "", "public")
	if !files.ReferencePattern.MatchString(protected.Reference) {
		t.Fatal("reference")
	}
	dup := f.file(t, p.ID, "protected")
	if dup.Reference != protected.Reference {
		t.Fatal("duplicate")
	}
	r := f.deliver(t, protected.Reference, "denied")
	if r.Allowed || f.transport.sends.Load() != 0 {
		t.Fatal("protected sent")
	}
	f.transport.state = "MEMBER"
	r = f.deliver(t, protected.Reference, "retry-new-update")
	if !r.Allowed || f.transport.sends.Load() != 1 {
		t.Fatal(r)
	}
	r = f.deliver(t, protected.Reference, "retry-new-update")
	if !r.Replayed || f.transport.sends.Load() != 1 {
		t.Fatal("duplicate send")
	}
	if f.transport.checks.Load() != 2 {
		t.Fatal("access not rechecked")
	}
	r = f.deliver(t, public.Reference, "public")
	if !r.Allowed {
		t.Fatal(r)
	}
	f.change(t, gate.ID, sdk.Disable, 2)
	r = f.deliver(t, protected.Reference, "gate-disabled")
	if r.Allowed || r.Code != "ACCESS_GATE_UNAVAILABLE" || f.transport.sends.Load() != 2 {
		t.Fatal("public fallback", r)
	}
	r = f.deliver(t, public.Reference, "public-without-gate")
	if !r.Allowed {
		t.Fatal("public needs gate", r)
	}
	list, e := f.content.Files(ctx, f.u.UserID, f.scope(files.ID), "protected")
	must(t, e)
	if len(list) != 1 || list[0].Delivered != 1 || list[0].Denied != 2 || list[0].Unique != 1 {
		t.Fatal(list)
	}
	public.Title = "Edited"
	public.Category = "Changed"
	public.Enabled = false
	public, e = f.content.SaveFile(ctx, f.u.UserID, f.scope(files.ID), public.Reference, "r3d4", contentplugins.FileChange{File: public, Key: "edit", Expected: 1})
	must(t, e)
	r = f.deliver(t, public.Reference, "disabled-file")
	if r.Allowed || r.Code != "FILE_DISABLED" {
		t.Fatal(r)
	}
	r = f.deliver(t, strings.Repeat("x", 32), "unknown")
	if r.Allowed || r.Code != "FILE_NOT_FOUND" {
		t.Fatal(r)
	}
	f.change(t, files.ID, sdk.Disable, 2)
	if _, e = f.content.Deliver(ctx, f.u.UserID, f.scope(files.ID), files.Request{Reference: protected.Reference, User: 123, Key: "plugin-disabled"}); !errors.Is(e, sdk.ErrState) {
		t.Fatal(e)
	}
	if number(t, f.u, "SELECT count(*) FROM content_files WHERE bot_id=$1", f.bot) != 2 {
		t.Fatal("data lost")
	}
}
func TestR3D4IsolationSecurity(t *testing.T) {
	f := r3d4Fixture(t)
	other := fixture(t, false)
	if _, e := f.content.Policies(ctx, other.UserID, f.scope(gate.ID)); !errors.Is(e, database.ErrForbidden) {
		t.Fatal(e)
	}
	member := fixture(t, false)
	tenant(t, f.u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "INSERT INTO workspace_members(workspace_id,user_id,role_key)VALUES($1,$2,'ANALYST')", f.u.WorkspaceID, member.UserID)
		return e
	})
	if _, e := f.content.Files(ctx, member.UserID, f.scope(files.ID), ""); !errors.Is(e, database.ErrForbidden) {
		t.Fatal(e)
	}
	p := f.file(t, "", "retained")
	c, e := f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
	must(t, e)
	v, _ := c.Get(commerce.PlanKind, f.plan)
	v.Definition.Plan.Plugins[files.ID] = commerce.PaidAddon
	f.save(t, v, v.Version)
	_, e = f.s.Issue(ctx, f.admin.UserID, f.u.WorkspaceID, "r3d4", commerce.Issue{Key: "temporary", Kind: commerce.PluginKind, CatalogKey: files.ID, Bot: f.bot, Period: commerce.Daily, Reason: "synthetic", AdminGrant: true, ExpiresAt: func() *time.Time { at := f.clock.at.Add(time.Hour); return &at }()})
	must(t, e)
	if !f.deliver(t, p.Reference, "before-expiry").Allowed {
		t.Fatal("grant not active before expiry")
	}
	f.clock.at = f.clock.at.Add(time.Hour)
	if _, e = f.content.Deliver(ctx, f.u.UserID, f.scope(files.ID), files.Request{Reference: p.Reference, User: 123, Key: "expired"}); !errors.Is(e, sdk.ErrPermission) {
		t.Fatal("expiry", e)
	}
	f.change(t, files.ID, sdk.Disable, 2)
	if number(t, f.u, "SELECT count(*) FROM content_files WHERE bot_id=$1", f.bot) != 1 {
		t.Fatal("expiry removed files")
	}
}
func TestR3D4IngestionAndConnection(t *testing.T) {
	f := r3d4Fixture(t)
	must(t, f.content.Connect(ctx, f.u.UserID, f.scope(files.ID), "https://mini.test", "r3d4"))
	var id int64
	tenant(t, f.u, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT telegram_id FROM telegram_accounts WHERE user_id=$1", f.u.UserID).Scan(&id)
	})
	u := contentplugins.Update{ID: 111, Message: &contentplugins.Message{Document: &contentplugins.Media{FileID: "captured", UniqueID: "captured-unique", Name: "Captured"}}}
	u.Message.From.ID = id
	u.Message.Chat.ID = id
	u.Message.Chat.Type = "private"
	must(t, f.content.HandleUpdate(ctx, f.scope(files.ID), "r3d4", u))
	must(t, f.content.HandleUpdate(ctx, f.scope(files.ID), "r3d4", u))
	list, e := f.content.Files(ctx, f.u.UserID, f.scope(files.ID), "Captured")
	must(t, e)
	if len(list) != 1 || list[0].Enabled || list[0].FileID != "captured" {
		t.Fatal(list)
	}
	u.ID++
	u.Message.From.ID = 123
	u.Message.Chat.ID = 123
	if e = f.content.HandleUpdate(ctx, f.scope(files.ID), "r3d4", u); !errors.Is(e, database.ErrForbidden) {
		t.Fatal("enduser manager", e)
	}
	api := (&httpapi.API{DB: db, Identity: r3aService(), Content: f.content, Config: config.Config{Environment: "test", Synthetic: true, MiniAppURL: "https://mini.test"}}).Handler()
	r := httptest.NewRequest("POST", "/api/v1/customer-bot-updates/"+f.u.WorkspaceID+"/"+f.bot, strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	api.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("unsigned webhook", w.Code)
	}
}
func TestR3D4BrowserFixture(t *testing.T) {
	f := r3d4Fixture(t)
	u, _ := r3aLogin(t, 98004001, false)
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE subscriptions SET plan_key=$1,expires_at=NULL WHERE workspace_id=$2", f.plan, u.WorkspaceID)
		return e
	})
	bot := r3dBot(t, u, "r3d4-browser")
	for _, id := range []string{gate.ID, files.ID} {
		scope := sdk.Scope{AccountID: u.WorkspaceID, BotID: bot, PluginID: id}
		i, e := f.core.Change(ctx, u.UserID, scope, sdk.Install, 0, json.RawMessage(`{}`), "r3d4")
		must(t, e)
		_, e = f.core.Change(ctx, u.UserID, scope, sdk.Activate, i.Revision, nil, "r3d4")
		must(t, e)
	}
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE customer_bots SET name='R3D4 browser bot' WHERE id=$1", bot)
		return e
	})
}
func TestR3D4RecoveryPreparation(t *testing.T) {
	f := r3d4Fixture(t)
	f.file(t, f.policy(t).ID, "r3d4-recovery")
}
func TestR3D4Recovered(t *testing.T) {
	old := db
	var e error
	db, e = database.Open(ctx, os.Getenv("TEST_RESTORE_DATABASE_URL"), true)
	must(t, e)
	defer func() { db.Pool.Close(); db = old }()
	var account, actor, bot, reference string
	must(t, owner.Pool.QueryRow(ctx, `SELECT f.workspace_id::text,m.user_id::text,f.bot_id::text,f.reference FROM content_files f JOIN workspace_members m ON m.workspace_id=f.workspace_id AND m.role_key='OWNER' WHERE f.operation_key='r3d4-recovery' LIMIT 1`).Scan(&account, &actor, &bot, &reference))
	s := &commerce.Service{DB: db, Clock: &r3d2Clock{at: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)}}
	r, e := contentplugins.Registry()
	must(t, e)
	core := &platform.Service{DB: db, Registry: r, Policy: s}
	service := &contentplugins.Service{DB: db, Core: core, Commercial: s}
	list, e := service.Files(ctx, actor, sdk.Scope{AccountID: account, BotID: bot, PluginID: files.ID}, "r3d4-recovery")
	must(t, e)
	if len(list) != 1 || list[0].Reference != reference || list[0].Policy == "" {
		t.Fatal("restore lost plugin data")
	}
}
func TestR3D4QuotaAndFailures(t *testing.T) {
	f := r3d4Fixture(t)
	p := f.policy(t)
	protected := f.file(t, p.ID, "quota-protected")
	public := f.file(t, "", "quota-public")
	c, e := f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
	must(t, e)
	v, _ := c.Get(commerce.QuotaKind, "membership_checks.monthly")
	v.Definition.Quota.Default = commerce.Limit{Value: 0}
	f.save(t, v, v.Version)
	r := f.deliver(t, protected.Reference, "quota-denied")
	if r.Allowed || r.Code != "QUOTA_EXCEEDED" || f.transport.checks.Load() != 0 || f.transport.sends.Load() != 0 {
		t.Fatal("quota bypass", r)
	}
	f.deliver(t, protected.Reference, "quota-denied")
	if number(t, f.u, "SELECT COALESCE(sum(used),0) FROM commercial_usage WHERE workspace_id=$1 AND metric='downloads.monthly'", f.u.WorkspaceID) != 1 {
		t.Fatal("replay double consumed quota")
	}
	v.Definition.Quota.Default = commerce.Limit{Unlimited: true}
	f.save(t, v, v.Version+1)
	_, e = owner.Pool.Exec(ctx, `CREATE FUNCTION r3d4_reject_quota() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.workspace_id::text='`+f.u.WorkspaceID+`' AND NEW.metric='membership_checks.monthly' THEN RAISE EXCEPTION 'synthetic quota persistence failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER r3d4_reject_quota BEFORE INSERT OR UPDATE ON commercial_usage FOR EACH ROW EXECUTE FUNCTION r3d4_reject_quota()`)
	must(t, e)
	defer func() {
		_, err := owner.Pool.Exec(ctx, "DROP TRIGGER IF EXISTS r3d4_reject_quota ON commercial_usage; DROP FUNCTION IF EXISTS r3d4_reject_quota()")
		must(t, err)
	}()
	_, e = f.content.Deliver(ctx, f.u.UserID, f.scope(files.ID), files.Request{Reference: protected.Reference, User: 123, Key: "quota-storage-failed"})
	if e == nil || f.transport.sends.Load() != 0 || f.transport.checks.Load() != 0 {
		t.Fatal("quota storage failure did not fail closed", e)
	}
	r = f.deliver(t, protected.Reference, "quota-storage-failed")
	if !r.Replayed || r.Allowed || f.transport.sends.Load() != 0 {
		t.Fatal("failed quota operation retried external effect", r)
	}
	_, e = owner.Pool.Exec(ctx, "DROP TRIGGER r3d4_reject_quota ON commercial_usage; DROP FUNCTION r3d4_reject_quota()")
	must(t, e)
	f.transport.state = "CHECK_FAILED"
	f.transport.code = "BOT_NOT_AUTHORIZED"
	r = f.deliver(t, protected.Reference, "check-failed")
	if r.Allowed || r.Access.Checks[0].Code != "BOT_NOT_AUTHORIZED" {
		t.Fatal(r)
	}
	f.transport.outcome = "DELIVERY_OUTCOME_UNKNOWN"
	r = f.deliver(t, public.Reference, "send-unknown")
	if r.Allowed || r.Code != "DELIVERY_OUTCOME_UNKNOWN" {
		t.Fatal(r)
	}
	f.deliver(t, public.Reference, "send-unknown")
	if f.transport.sends.Load() != 1 {
		t.Fatal("unknown resent")
	}
	f.transport.outcome = "DELIVERY_FAILED"
	r = f.deliver(t, public.Reference, "send-failed")
	if r.Allowed {
		t.Fatal("failed succeeded")
	}
	list, e := f.content.Files(ctx, f.u.UserID, f.scope(files.ID), "quota-public")
	must(t, e)
	if list[0].Delivered != 0 {
		t.Fatal("false analytics")
	}
	c, e = f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
	must(t, e)
	v, _ = c.Get(commerce.QuotaKind, "files.count")
	v.Definition.Quota.Default = commerce.Limit{Value: 2}
	f.save(t, v, v.Version)
	_, e = f.content.SaveFile(ctx, f.u.UserID, f.scope(files.ID), "", "r3d4", contentplugins.FileChange{File: files.File{FileID: "over-quota", UniqueID: "over-quota", Kind: "photo", Title: "Over", Enabled: true}, Key: "over"})
	var denied *contentplugins.QuotaDenied
	if !errors.As(e, &denied) {
		t.Fatal("files quota", e)
	}
	v.Definition.Quota.Default = commerce.Limit{Unlimited: true}
	f.save(t, v, v.Version+1)
	c, e = f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
	must(t, e)
	plan, _ := c.Get(commerce.PlanKind, f.plan)
	plan.Definition.Plan.Plugins[gate.ID] = commerce.PaidAddon
	f.save(t, plan, plan.Version)
	if _, e = f.content.Deliver(ctx, f.u.UserID, f.scope(files.ID), files.Request{Reference: protected.Reference, User: 123, Key: "gate-no-entitlement"}); !errors.Is(e, sdk.ErrPermission) {
		t.Fatal("gate entitlement", e)
	}
	if number(t, f.u, "SELECT count(*) FROM content_access_policies WHERE bot_id=$1", f.bot) != 1 || number(t, f.u, "SELECT count(*) FROM content_files WHERE bot_id=$1", f.bot) != 2 {
		t.Fatal("commercial loss deleted data")
	}
}
