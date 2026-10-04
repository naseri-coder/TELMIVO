//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/identity"
	platform "github.com/naseri-coder/TELMIVO/internal/domains/pluginplatform"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
)

type r3dPolicy struct {
	denied atomic.Bool
	extra  atomic.Bool
}

func (p *r3dPolicy) Authorize(_ context.Context, a sdk.Admission) (sdk.Grant, error) {
	if p.denied.Load() {
		return sdk.Grant{}, sdk.ErrPermission
	}
	caps := append([]string{}, a.Manifest.Permissions...)
	if p.extra.Load() {
		caps = append(caps, "files.write")
	}
	return sdk.Grant{Capabilities: caps, LicenseScope: sdk.PerBot, PolicyRevision: "synthetic-r3d1"}, nil
}

type r3dHandler struct{ calls atomic.Int64 }

func (h *r3dHandler) Invoke(_ context.Context, scope sdk.Scope, capability string, _ json.RawMessage) (json.RawMessage, error) {
	h.calls.Add(1)
	return json.Marshal(map[string]any{"scope": scope, "capability": capability})
}

func r3dManifest(id string, deps ...string) sdk.Manifest {
	m := sdk.Manifest{SchemaVersion: 1, PluginID: id, Name: "Synthetic test plugin", Version: "1.0.0", CoreVersions: sdk.VersionRange{Minimum: "0.1.0", MaximumExclusive: "1.0.0"}, DataVersion: 1, Permissions: []string{"telegram.send"}, Settings: sdk.SettingsSchema{Properties: map[string]sdk.Setting{"title": {Type: "string", MaxLength: 50}}, Required: []string{"title"}}}
	for _, d := range deps {
		m.Dependencies = append(m.Dependencies, sdk.Dependency{PluginID: d, Versions: sdk.VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"}})
	}
	return m
}

func r3dService(t *testing.T, manifests ...sdk.Manifest) (*platform.Service, *r3dPolicy) {
	t.Helper()
	r, e := platform.NewRegistry("0.3.0", manifests)
	must(t, e)
	p := &r3dPolicy{}
	return &platform.Service{DB: db, Registry: r, Policy: p}, p
}

func r3dBot(t *testing.T, u identity.Result, key string) string {
	t.Helper()
	var id string
	tenant(t, u, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO customer_bots(workspace_id,name,username,key_id,encrypted_token,status,idempotency_key) VALUES($1,'Synthetic R3D bot',$2,'synthetic',decode('00','hex'),'DRAFT',$3) RETURNING id::text`, u.WorkspaceID, fmt.Sprintf("r3d_%s_%d", key, sequence.Add(1)), key).Scan(&id)
	})
	return id
}

func r3dChange(t *testing.T, s *platform.Service, actor string, scope sdk.Scope, a sdk.Action, revision int64) sdk.Installation {
	t.Helper()
	var config json.RawMessage
	if a == sdk.Install {
		config = json.RawMessage(`{"title":"سلام"}`)
	}
	i, e := s.Change(ctx, actor, scope, a, revision, config, "r3d1-synthetic")
	must(t, e)
	return i
}

func TestR3D1LifecycleAndRuntime(t *testing.T) {
	u := fixture(t, false)
	bot, otherBot := r3dBot(t, u, "lifecycle"), r3dBot(t, u, "independent")
	s, policy := r3dService(t, r3dManifest("demo-base"), r3dManifest("demo-dependent", "demo-base"))
	scope := sdk.Scope{AccountID: u.WorkspaceID, BotID: bot, PluginID: "demo-base"}
	dependent := scope
	dependent.PluginID = "demo-dependent"
	independent := scope
	independent.BotID = otherBot
	v, e := s.View(ctx, u.UserID, scope)
	must(t, e)
	if v.State != sdk.Available {
		t.Fatal(v)
	}
	base := r3dChange(t, s, u.UserID, scope, sdk.Install, 0)
	dep := r3dChange(t, s, u.UserID, dependent, sdk.Install, 0)
	if _, e = s.Change(ctx, u.UserID, dependent, sdk.Activate, dep.Revision, nil, "r3d1-missing"); !errors.Is(e, sdk.ErrDependency) {
		t.Fatal(e)
	}
	base = r3dChange(t, s, u.UserID, scope, sdk.Activate, base.Revision)
	dep = r3dChange(t, s, u.UserID, dependent, sdk.Activate, dep.Revision)
	if _, e = s.Change(ctx, u.UserID, scope, sdk.Disable, base.Revision, nil, "r3d1-unsafe"); !errors.Is(e, sdk.ErrDependency) {
		t.Fatal(e)
	}
	if _, e = s.Change(ctx, u.UserID, dependent, sdk.Disable, dep.Revision-1, nil, "r3d1-stale"); !errors.Is(e, database.ErrConflict) {
		t.Fatal(e)
	}
	handler := &r3dHandler{}
	runtime := &platform.Runtime{Service: s, Handler: handler}
	bound := runtime.Bind(u.UserID, dependent)
	result, e := bound.Invoke(ctx, "telegram.send", json.RawMessage(`{}`))
	must(t, e)
	var decoded struct {
		Scope sdk.Scope `json:"scope"`
	}
	must(t, json.Unmarshal(result, &decoded))
	if decoded.Scope != dependent {
		t.Fatal("unbound invocation")
	}
	for _, cap := range []string{"files.write", "database.root"} {
		if _, e = bound.Invoke(ctx, cap, json.RawMessage(`{}`)); !errors.Is(e, sdk.ErrPermission) {
			t.Fatal(e)
		}
	}
	policy.denied.Store(true)
	if _, e = bound.Invoke(ctx, "telegram.send", json.RawMessage(`{}`)); !errors.Is(e, sdk.ErrPermission) {
		t.Fatal("revoked admission", e)
	}
	// Revoked commercial access must not prevent disabling the dependent.
	dep = r3dChange(t, s, u.UserID, dependent, sdk.Disable, dep.Revision)
	if _, e = bound.Invoke(ctx, "telegram.send", json.RawMessage(`{}`)); !errors.Is(e, sdk.ErrState) {
		t.Fatal("cached session executed", e)
	}
	policy.denied.Store(false)
	// Persist nonempty synthetic data through the Core's future same-state write seam.
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE plugin_installations SET data='{"retained":"fixture"}',revision=revision+1 WHERE bot_id=$1 AND plugin_id=$2`, bot, scope.PluginID)
		return e
	})
	base, e = s.View(ctx, u.UserID, scope)
	must(t, e)
	base = r3dChange(t, s, u.UserID, scope, sdk.Disable, base.Revision)
	if base.State != sdk.Disabled || !jsonContains(base.Data, "retained", "fixture") || !jsonContains(base.Configuration, "title", "سلام") {
		t.Fatal("disable lost data", base)
	}
	separate := r3dChange(t, s, u.UserID, independent, sdk.Install, 0)
	separate = r3dChange(t, s, u.UserID, independent, sdk.Activate, separate.Revision)
	base = r3dChange(t, s, u.UserID, scope, sdk.RequestUninstall, base.Revision)
	if base.State != sdk.UninstallPending {
		t.Fatal(base)
	}
	base = r3dChange(t, s, u.UserID, scope, sdk.CompleteUninstall, base.Revision)
	if base.State != sdk.Uninstalled || !jsonContains(base.Data, "retained", "fixture") {
		t.Fatal("uninstall retention", base)
	}
	v, e = s.View(ctx, u.UserID, independent)
	must(t, e)
	if v.State != sdk.Active || v.Revision != separate.Revision {
		t.Fatal("per-bot state coupled", v)
	}
	if handler.calls.Load() != 1 {
		t.Fatal("denial reached handler")
	}
	if _, e = s.Change(ctx, u.UserID, scope, sdk.Install, base.Revision, json.RawMessage(`{"title":"new"}`), "r3d1-reinstall"); !errors.Is(e, sdk.ErrState) {
		t.Fatal("implicit reinstall", e)
	}
}

func jsonContains(raw []byte, key, want string) bool {
	var obj map[string]any
	return json.Unmarshal(raw, &obj) == nil && obj[key] == want
}

func TestR3D1IsolationAndAdmission(t *testing.T) {
	a, b := fixture(t, false), fixture(t, false)
	bot, foreign := r3dBot(t, a, "owned"), r3dBot(t, b, "foreign")
	s, p := r3dService(t, r3dManifest("demo-isolation"))
	scope := sdk.Scope{AccountID: a.WorkspaceID, BotID: bot, PluginID: "demo-isolation"}
	s.Policy = nil
	if _, e := s.Change(ctx, a.UserID, scope, sdk.Install, 0, json.RawMessage(`{"title":"x"}`), "nil-policy"); !errors.Is(e, sdk.ErrPermission) {
		t.Fatal(e)
	}
	s.Policy = p
	p.extra.Store(true)
	if _, e := s.Change(ctx, a.UserID, scope, sdk.Install, 0, json.RawMessage(`{"title":"x"}`), "excess-grant"); !errors.Is(e, sdk.ErrPermission) {
		t.Fatal(e)
	}
	p.extra.Store(false)
	i := r3dChange(t, s, a.UserID, scope, sdk.Install, 0)
	if _, e := s.View(ctx, b.UserID, scope); e == nil {
		t.Fatal("spoofed member")
	}
	forged := scope
	forged.BotID = foreign
	if _, e := s.Change(ctx, a.UserID, forged, sdk.Install, 0, json.RawMessage(`{"title":"x"}`), "foreign-bot"); e == nil {
		t.Fatal("foreign bot admitted")
	}
	if n := number(t, b, "SELECT count(*) FROM plugin_installations WHERE bot_id=$1", bot); n != 0 {
		t.Fatal("RLS read")
	}
	e := db.Tenant(ctx, b.UserID, b.WorkspaceID, false, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `INSERT INTO plugin_installations(workspace_id,bot_id,plugin_id,plugin_version,data_version,state,configuration,license_scope) VALUES($1,$2,'cross-test','1.0.0',1,'INSTALLED','{}','PER_BOT')`, b.WorkspaceID, bot)
		return e
	})
	pgCode(t, e, "23503")
	// Stored version drift must fail activation, never run migration hooks implicitly.
	upgraded := r3dManifest(scope.PluginID)
	upgraded.Version = "1.1.0"
	r, e := platform.NewRegistry("0.3.0", []sdk.Manifest{upgraded})
	must(t, e)
	s.Registry = r
	if _, e = s.Change(ctx, a.UserID, scope, sdk.Activate, i.Revision, nil, "upgrade"); !errors.Is(e, sdk.ErrVersion) {
		t.Fatal(e)
	}
	s.Registry, e = platform.NewRegistry("0.3.0", []sdk.Manifest{r3dManifest(scope.PluginID)})
	must(t, e)
	i = r3dChange(t, s, a.UserID, scope, sdk.Activate, i.Revision)
	for _, q := range []string{`UPDATE plugin_installations SET state='UNINSTALLED',revision=revision+1 WHERE bot_id=$1`, `UPDATE plugin_installations SET state='DISABLED',configuration='{}',revision=revision+1 WHERE bot_id=$1`, `DELETE FROM plugin_installations WHERE bot_id=$1`} {
		e = db.Tenant(ctx, a.UserID, a.WorkspaceID, false, func(tx pgx.Tx) error { _, e := tx.Exec(ctx, q, bot); return e })
		if e == nil {
			t.Fatal("retention/transition guard bypass")
		}
	}
	if v, e := s.View(ctx, a.UserID, scope); e != nil || v.State != sdk.Active || v.Revision != i.Revision {
		t.Fatal("failed operation changed state", v, e)
	}
}

type r3dHook struct{}

func (r3dHook) Validate(context.Context, sdk.Action, sdk.Installation, sdk.Manifest) error {
	return sdk.ErrState
}

func TestR3D1HookAndConcurrentConflicts(t *testing.T) {
	u := fixture(t, false)
	bot := r3dBot(t, u, "conflicts")
	a, b := r3dManifest("demo-a"), r3dManifest("demo-b")
	a.Conflicts = []string{b.PluginID}
	hooked := r3dManifest("demo-hook")
	hooked.LifecycleHooks = []sdk.Action{sdk.Install}
	s, _ := r3dService(t, a, b, hooked)
	scope := sdk.Scope{AccountID: u.WorkspaceID, BotID: bot, PluginID: hooked.PluginID}
	for _, h := range []sdk.LifecycleValidator{nil, r3dHook{}} {
		s.Hooks = h
		if _, e := s.Change(ctx, u.UserID, scope, sdk.Install, 0, json.RawMessage(`{"title":"x"}`), "hook-veto"); !errors.Is(e, sdk.ErrState) {
			t.Fatal(e)
		}
	}
	if v, e := s.View(ctx, u.UserID, scope); e != nil || v.State != sdk.Available {
		t.Fatal("hook veto persisted", v, e)
	}
	scopes := []sdk.Scope{}
	for _, id := range []string{a.PluginID, b.PluginID} {
		scope.PluginID = id
		r3dChange(t, s, u.UserID, scope, sdk.Install, 0)
		scopes = append(scopes, scope)
	}
	var wg sync.WaitGroup
	var successes atomic.Int64
	errs := make(chan error, 2)
	for _, sc := range scopes {
		wg.Add(1)
		go func(sc sdk.Scope) {
			defer wg.Done()
			_, e := s.Change(ctx, u.UserID, sc, sdk.Activate, 1, nil, "race-conflict")
			if e == nil {
				successes.Add(1)
			} else {
				errs <- e
			}
		}(sc)
	}
	wg.Wait()
	close(errs)
	if successes.Load() != 1 {
		t.Fatal("conflicting activation race", successes.Load())
	}
	for e := range errs {
		if !errors.Is(e, sdk.ErrConflict) {
			t.Fatal(e)
		}
	}
}

func TestR3D1RecoveryPreparation(t *testing.T) {
	u := fixture(t, false)
	bot := r3dBot(t, u, "r3d1-recovery")
	s, _ := r3dService(t, r3dManifest("demo-recovery"))
	scope := sdk.Scope{AccountID: u.WorkspaceID, BotID: bot, PluginID: "demo-recovery"}
	i := r3dChange(t, s, u.UserID, scope, sdk.Install, 0)
	i = r3dChange(t, s, u.UserID, scope, sdk.Activate, i.Revision)
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE plugin_installations SET data='{"retained":"recovery"}',revision=revision+1 WHERE bot_id=$1`, bot)
		return e
	})
	i, e := s.View(ctx, u.UserID, scope)
	must(t, e)
	i = r3dChange(t, s, u.UserID, scope, sdk.Disable, i.Revision)
	r3dChange(t, s, u.UserID, scope, sdk.RequestUninstall, i.Revision)
}

func TestR3D1Recovered(t *testing.T) {
	// Locate synthetic coordinates using the restore migration role; all behavior is restricted.
	migration, e := database.Open(ctx, os.Getenv("TEST_RESTORE_MIGRATION_DATABASE_URL"), false)
	must(t, e)
	defer migration.Pool.Close()
	var actor string
	scope := sdk.Scope{PluginID: "demo-recovery"}
	must(t, migration.Pool.QueryRow(ctx, `SELECT w.owner_id::text,b.workspace_id::text,b.id::text FROM customer_bots b JOIN workspaces w ON w.id=b.workspace_id WHERE b.idempotency_key='r3d1-recovery'`).Scan(&actor, &scope.AccountID, &scope.BotID))
	restored, e := database.Open(ctx, os.Getenv("TEST_RESTORE_DATABASE_URL"), true)
	must(t, e)
	defer restored.Pool.Close()
	r, e := platform.NewRegistry("0.3.0", []sdk.Manifest{r3dManifest(scope.PluginID)})
	must(t, e)
	s := &platform.Service{DB: restored, Registry: r} // No commercial admission needed to complete retained uninstall.
	i, e := s.View(ctx, actor, scope)
	must(t, e)
	if i.State != sdk.UninstallPending || i.Revision != 5 || !jsonContains(i.Configuration, "title", "سلام") || !jsonContains(i.Data, "retained", "recovery") {
		t.Fatal("restored installation", i)
	}
	i = r3dChange(t, s, actor, scope, sdk.CompleteUninstall, i.Revision)
	if i.State != sdk.Uninstalled || !jsonContains(i.Data, "retained", "recovery") {
		t.Fatal("restored uninstall", i)
	}
}
