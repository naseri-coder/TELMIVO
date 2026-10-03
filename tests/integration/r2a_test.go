//go:build integration

package integration

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"context"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/identity"
	"github.com/naseri-coder/TELMIVO/internal/domains/policies"
	"github.com/naseri-coder/TELMIVO/internal/platform/auth"
	"github.com/naseri-coder/TELMIVO/internal/platform/config"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/httpapi"
)

func policyRequest(u identity.Result) policies.Request {
	return policies.Request{User: u.UserID, Workspace: u.WorkspaceID, Role: "OWNER", Feature: "downloads", Permission: "downloads.create", Requested: 1}
}
func activePolicy(t *testing.T, u identity.Result, key string) policies.Version {
	var v policies.Version
	var raw []byte
	tenant(t, u, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT key,version,kind,COALESCE(feature_key,''),COALESCE(plugin_key,''),COALESCE(plan_key,''),COALESCE(workspace_id::text,''),enabled,definition,effective_from,effective_until FROM policy_versions WHERE key=$1 ORDER BY version DESC LIMIT 1`, key).Scan(&v.Key, &v.Version, &v.Kind, &v.Feature, &v.Plugin, &v.Plan, &v.Workspace, &v.Enabled, &raw, &v.EffectiveFrom, &v.EffectiveUntil)
	})
	must(t, json.Unmarshal(raw, &v.Definition))
	return v
}
func savePolicy(t *testing.T, u identity.Result, v policies.Version) policies.Version {
	t.Helper()
	var saved policies.Version
	tenant(t, u, func(tx pgx.Tx) error {
		var e error
		saved, e = policies.Save(ctx, tx, u.UserID, u.WorkspaceID, "r2a-policy", policies.Change{Version: v, ExpectedVersion: v.Version})
		return e
	})
	return saved
}
func discover(t *testing.T, id int64, source string) identity.Result {
	t.Helper()
	var out identity.Result
	tx, e := db.Pool.Begin(ctx)
	must(t, e)
	defer tx.Rollback(ctx)
	out, e = identity.Enter(ctx, tx, auth.TelegramUser{ID: id, FirstName: "کاربر R2A"}, []int64{id}, source, "entry-replay-key")
	must(t, e)
	must(t, tx.Commit(ctx))
	return out
}
func TestR2AMultiEntryAndTrials(t *testing.T) {
	id := int64(91000000) + sequence.Add(1)
	first := discover(t, id, "DOWNLOADER_BOT")
	second := discover(t, id, "MAIN_BOT")
	if first.UserID != second.UserID || first.WorkspaceID != second.WorkspaceID {
		t.Fatal("entry split identity")
	}
	t.Run("one_identity_one_default_once_trial", func(t *testing.T) {
		var n int
		must(t, db.Pool.QueryRow(ctx, "SELECT count(*) FROM workspaces WHERE owner_id=$1", first.UserID).Scan(&n))
		if n != 1 {
			t.Fatal("duplicate workspace")
		}
		if n := number(t, first, "SELECT count(*) FROM trial_enrollments WHERE user_id=$1", first.UserID); n != 1 {
			t.Fatal("trial restarted")
		}
		if n := number(t, first, "SELECT count(*) FROM identity_entries WHERE user_id=$1", first.UserID); n != 2 {
			t.Fatal("entry source")
		}
	})
	t.Run("snapshot_and_new_version", func(t *testing.T) {
		v := activePolicy(t, first, "global_customer_trial")
		oldVersion := v.Version
		oldDuration := v.Definition.Duration
		v.Definition.Duration = 3 * 86400
		v = savePolicy(t, first, v)
		third := discover(t, int64(92000000)+sequence.Add(1), "GROUP_BOT")
		for _, x := range []struct {
			u                 identity.Result
			version, duration int64
		}{{first, oldVersion, oldDuration}, {third, v.Version, 3 * 86400}} {
			tenant(t, x.u, func(tx pgx.Tx) error {
				var version, duration int64
				if e := tx.QueryRow(ctx, "SELECT policy_version,extract(epoch FROM ends_at-started_at)::bigint FROM trial_enrollments WHERE user_id=$1 AND policy_key='global_customer_trial'", x.u.UserID).Scan(&version, &duration); e != nil {
					return e
				}
				if version != x.version || duration != x.duration {
					t.Fatalf("snapshot changed %d %d", version, duration)
				}
				return nil
			})
		}
		discover(t, id, "MINIAPP")
		if number(t, first, "SELECT count(*) FROM trial_enrollments WHERE user_id=$1", first.UserID) != 1 {
			t.Fatal("version restarted enrollment")
		}
	})
	t.Run("trial_expiry_returns_free", func(t *testing.T) {
		tenant(t, first, func(tx pgx.Tx) error {
			var end time.Time
			if e := tx.QueryRow(ctx, "SELECT ends_at FROM trial_enrollments WHERE user_id=$1", first.UserID).Scan(&end); e != nil {
				return e
			}
			r := policyRequest(first)
			r.Now = end.Add(time.Second)
			d, e := policies.Resolve(ctx, tx, r)
			if e == nil && (len(d.Trials) != 1 || d.Trials[0].State != "EXPIRED" || !d.Allowed || d.Limit == nil || *d.Limit != 5) {
				t.Fatalf("FREE fallback %+v", d)
			}
			return e
		})
	})
}

type groupAuthority bool

func (g groupAuthority) CanAdmin(context.Context, int64, int64) (bool, error) { return bool(g), nil }
func TestR2AGroupTrial(t *testing.T) {
	u := fixture(t, true)
	var telegram int64
	must(t, db.Pool.QueryRow(ctx, "SELECT telegram_id FROM telegram_accounts WHERE user_id=$1", u.UserID).Scan(&telegram))
	chat := int64(-20000000) - sequence.Add(1)
	t.Run("authority_required", func(t *testing.T) {
		e := db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
			_, e := identity.ClaimGroup(ctx, tx, u.UserID, u.WorkspaceID, "group-denied", telegram, chat, "گروه", groupAuthority(false))
			return e
		})
		if !errors.Is(e, database.ErrForbidden) {
			t.Fatal("unverified claim", e)
		}
	})
	var group string
	tenant(t, u, func(tx pgx.Tx) error {
		var e error
		group, e = identity.ClaimGroup(ctx, tx, u.UserID, u.WorkspaceID, "group", telegram, chat, "گروه", groupAuthority(true))
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, "INSERT INTO group_settings(workspace_id,group_id,moderation) VALUES($1,$2,'{\"anti_spam\":true}')", u.WorkspaceID, group)
		return e
	})
	t.Run("no_silent_transfer", func(t *testing.T) {
		other := fixture(t, true)
		var id int64
		must(t, db.Pool.QueryRow(ctx, "SELECT telegram_id FROM telegram_accounts WHERE user_id=$1", other.UserID).Scan(&id))
		e := db.Tenant(ctx, other.UserID, other.WorkspaceID, false, func(tx pgx.Tx) error {
			_, e := identity.ClaimGroup(ctx, tx, other.UserID, other.WorkspaceID, "steal", id, chat, "دزدیده", groupAuthority(true))
			return e
		})
		if e == nil {
			t.Fatal("group stolen")
		}
		if number(t, other, "SELECT count(*) FROM groups WHERE workspace_id=$1", other.WorkspaceID) != 0 {
			t.Fatal("failed claim left resource")
		}
	})
	t.Run("config_survives_expiry_policy_editable", func(t *testing.T) {
		v := activePolicy(t, u, "group_product_trial")
		v.Definition.Duration = 30 * 86400
		savePolicy(t, u, v)
		tenant(t, u, func(tx pgx.Tx) error {
			var end time.Time
			if e := tx.QueryRow(ctx, "SELECT ends_at FROM trial_enrollments WHERE subject_id=$1", group).Scan(&end); e != nil {
				return e
			}
			r := policies.Request{User: u.UserID, Workspace: u.WorkspaceID, Role: "OWNER", Feature: "groups", Permission: "groups.read", Group: group, Now: end.Add(time.Second)}
			d, e := policies.Resolve(ctx, tx, r)
			if e != nil {
				return e
			}
			if d.Allowed {
				t.Fatal("expired/disabled group allowed")
			}
			return nil
		})
		if number(t, u, "SELECT count(*) FROM group_settings WHERE group_id=$1", group) != 1 {
			t.Fatal("settings deleted")
		}
	})
}
func TestR2ADynamicQuotaAndReservations(t *testing.T) {
	u := fixture(t, true)
	r := policyRequest(u)
	for i := 0; i < 5; i++ {
		tenant(t, u, func(tx pgx.Tx) error {
			_, e := policies.Reserve(ctx, tx, r, fmt.Sprintf("day-job-%d", i))
			if e != nil {
				return e
			}
			return policies.Settle(ctx, tx, u.WorkspaceID, fmt.Sprintf("day-job-%d", i), true)
		})
	}
	t.Run("configured_paywall", func(t *testing.T) {
		tenant(t, u, func(tx pgx.Tx) error {
			d, e := policies.Resolve(ctx, tx, r)
			if e == nil && (d.Allowed || d.Reason != "QUOTA_EXCEEDED" || d.Paywall != "downloader_free_limit" || len(d.Offers) != 1 || d.Usage != 5) {
				t.Fatalf("paywall %+v", d)
			}
			return e
		})
	})
	t.Run("daily_to_weekly_then_three_daily", func(t *testing.T) {
		v := activePolicy(t, u, "downloader_free")
		v.Definition.Window = "CALENDAR_WEEK"
		v = savePolicy(t, u, v)
		tenant(t, u, func(tx pgx.Tx) error {
			d, e := policies.Resolve(ctx, tx, r)
			x := d.Resources["downloader.successes"]
			if e == nil && (x.Window != "CALENDAR_WEEK" || x.Limit != 5 || x.Used != 5 || d.Allowed) {
				t.Fatal("stored weekly policy ignored")
			}
			return e
		})
		v.Definition.Window = "CALENDAR_DAY"
		v.Definition.Limit = 3
		savePolicy(t, u, v)
		tenant(t, u, func(tx pgx.Tx) error {
			d, e := policies.Resolve(ctx, tx, r)
			if e == nil && (d.Limit == nil || *d.Limit != 3 || d.Allowed) {
				t.Fatal("stored daily policy ignored")
			}
			return e
		})
	})
	t.Run("parallel_reserve_and_retry_release", func(t *testing.T) {
		target := fixture(t, false)
		request := policyRequest(target)
		var passed atomic.Int64
		var wg sync.WaitGroup
		errs := make(chan error, 12)
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				e := db.Tenant(ctx, target.UserID, target.WorkspaceID, false, func(tx pgx.Tx) error {
					_, e := policies.Reserve(ctx, tx, request, fmt.Sprintf("parallel-job-%02d", i))
					return e
				})
				if e == nil {
					passed.Add(1)
				} else {
					var denied *policies.Denied
					if !errors.As(e, &denied) {
						errs <- e
					}
				}
			}(i)
		}
		wg.Wait()
		close(errs)
		for e := range errs {
			t.Fatal(e)
		}
		if passed.Load() != 3 {
			t.Fatalf("concurrency escaped %d", passed.Load())
		}
		var key string
		tenant(t, target, func(tx pgx.Tx) error {
			if e := tx.QueryRow(ctx, "SELECT operation_key FROM quota_reservations WHERE workspace_id=$1 LIMIT 1", target.WorkspaceID).Scan(&key); e != nil {
				return e
			}
			_, e := policies.Reserve(ctx, tx, request, key)
			if e != nil {
				return e
			}
			return policies.Settle(ctx, tx, target.WorkspaceID, key, false)
		})
		tenant(t, target, func(tx pgx.Tx) error { _, e := policies.Reserve(ctx, tx, request, "replacement-job"); return e })
		if number(t, target, "SELECT count(*) FROM quota_reservations WHERE workspace_id=$1 AND state='RESERVED'", target.WorkspaceID) != 3 {
			t.Fatal("retry/release count")
		}
		if number(t, target, "SELECT COALESCE(sum(used),0)::bigint FROM quota_usage WHERE workspace_id=$1", target.WorkspaceID) != 0 {
			t.Fatal("failed job billed")
		}
	})
}
func TestR2ARollingAndFailurePolicy(t *testing.T) {
	u := fixture(t, true)
	v := activePolicy(t, u, "downloader_free")
	v.Definition.Window = "ROLLING_WINDOW"
	v.Definition.WindowSeconds = 3600
	v.Definition.Limit = 2
	v.Definition.CountFailure = true
	v = savePolicy(t, u, v)
	r := policyRequest(u)
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := policies.Reserve(ctx, tx, r, "rolling-failure")
		if e != nil {
			return e
		}
		return policies.Settle(ctx, tx, u.WorkspaceID, "rolling-failure", false)
	})
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := policies.Reserve(ctx, tx, r, "rolling-success")
		if e != nil {
			return e
		}
		return policies.Settle(ctx, tx, u.WorkspaceID, "rolling-success", true)
	})
	tenant(t, u, func(tx pgx.Tx) error {
		d, e := policies.Resolve(ctx, tx, r)
		if e == nil && (d.Allowed || d.Usage != 2) {
			t.Fatalf("rolling failed %+v", d)
		}
		return e
	})
	// Reservations disappear from capacity at expiry, but their terminal/usage history remains.
	v.Definition.ReservationSeconds = 1
	v.Definition.CountFailure = false
	v.Definition.Limit = 1
	savePolicy(t, u, v)
	target := fixture(t, false)
	r = policyRequest(target)
	tenant(t, target, func(tx pgx.Tx) error { _, e := policies.Reserve(ctx, tx, r, "expiry-reservation"); return e })
	time.Sleep(1100 * time.Millisecond)
	tenant(t, target, func(tx pgx.Tx) error { _, e := policies.Reserve(ctx, tx, r, "after-expiry-job"); return e })
	e := db.Tenant(ctx, target.UserID, target.WorkspaceID, false, func(tx pgx.Tx) error { return policies.Settle(ctx, tx, target.WorkspaceID, "expiry-reservation", true) })
	if !errors.Is(e, database.ErrConflict) {
		t.Fatal("late completion bypassed quota", e)
	}
}

func TestR2APluginGovernanceAndCommercial(t *testing.T) {
	u := fixture(t, true)
	_, e := owner.Pool.Exec(ctx, "UPDATE feature_flags SET enabled=true,rollout_status='ON' WHERE feature_key='uploader';UPDATE plugins SET status='ENABLED' WHERE key='uploader'")
	must(t, e)
	defer func() {
		_, e := owner.Pool.Exec(ctx, "UPDATE feature_flags SET enabled=false,rollout_status='OFF' WHERE feature_key='uploader';UPDATE plugins SET status='DISABLED' WHERE key='uploader'")
		must(t, e)
	}()
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE subscriptions SET plan_key='PRO' WHERE workspace_id=$1;", u.WorkspaceID)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, "UPDATE plan_plugin_entitlements SET enabled=true WHERE plan_key='PRO' AND plugin_key='uploader'")
		return e
	})
	defer func() {
		_, e := owner.Pool.Exec(ctx, "UPDATE plan_plugin_entitlements SET enabled=false WHERE plan_key='PRO' AND plugin_key='uploader'")
		must(t, e)
	}()
	grant := policies.Version{Key: "storage_pack_r2a", Kind: "GRANT", Workspace: u.WorkspaceID, Enabled: true, EffectiveFrom: time.Now().Add(-time.Second), EffectiveUntil: ptrTime(time.Now().Add(time.Hour)), Definition: policies.Definition{GrantKind: "ADDON", Limits: map[string]policies.LimitGrant{"uploader.storage_bytes": {Amount: 50000000000, Strategy: "ADD"}, "uploader.max_file_bytes": {Amount: 10000000000, Strategy: "MAX"}}}}
	savePolicy(t, u, grant)
	request := policies.Request{User: u.UserID, Workspace: u.WorkspaceID, Role: "OWNER", Feature: "uploader", Plugin: "uploader", Permission: "uploads.create", Requested: 1, Amounts: map[string]int64{"uploader.max_file_bytes": 4000000000, "uploader.concurrent_uploads": 1, "uploader.storage_bytes": 100}}
	t.Run("resources_capabilities_and_safety", func(t *testing.T) {
		tenant(t, u, func(tx pgx.Tx) error {
			d, e := policies.Resolve(ctx, tx, request)
			if e == nil && (!d.Allowed || len(d.Capabilities) != 7 || d.Resources["uploader.storage_bytes"].Limit != 70000000000 || d.Resources["uploader.max_file_bytes"].Limit != 5000000000 || d.Resources["uploader.bytes_uploaded"].Unit == d.Resources["uploader.storage_bytes"].Unit) {
				t.Fatalf("plugin result %+v", d)
			}
			return e
		})
		large := request
		large.Amounts = map[string]int64{"uploader.max_file_bytes": 6000000000}
		tenant(t, u, func(tx pgx.Tx) error {
			d, e := policies.Resolve(ctx, tx, large)
			if e == nil && d.Allowed {
				t.Fatal("safety exceeded")
			}
			return e
		})
		_, e := db.Pool.Exec(ctx, "UPDATE infrastructure_safety_caps SET limit_value=9999999999 WHERE metric='uploader.max_file_bytes'")
		pgCode(t, e, "42501")
	})
	t.Run("runtime_equals_admin_preview", func(t *testing.T) {
		runtime := request
		runtime.User = ""
		runtime.Workspace = ""
		runtime.Role = ""
		b, e := json.Marshal(runtime)
		must(t, e)
		x := apiJSON(t, u, "/api/v1/access/decision", string(b))
		b, e = json.Marshal(request)
		must(t, e)
		y := apiJSON(t, u, "/api/v1/access-policies/preview", string(b))
		if string(x) != string(y) {
			t.Fatalf("preview differs\n%s\n%s", x, y)
		}
	})
	t.Run("immutable_audit_and_granular_permissions", func(t *testing.T) {
		if number(t, u, "SELECT count(*) FROM audit_logs WHERE entity_id='storage_pack_r2a' AND action='policy.version.create'") != 1 {
			t.Fatal("missing policy audit")
		}
		e := db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, "DELETE FROM audit_logs WHERE entity_id='storage_pack_r2a'")
			return e
		})
		if e == nil {
			t.Fatal("audit mutable")
		}
		limited := fixture(t, true)
		_, e = owner.Pool.Exec(ctx, "DELETE FROM platform_admin_permissions WHERE user_id=$1 AND permission_key<>'platform.access_policies.manage'", limited.UserID)
		must(t, e)
		price := policies.Version{Key: "denied_price", Kind: "PRICE", Enabled: true, EffectiveFrom: time.Now(), Definition: policies.Definition{Asset: "USD", AmountAtomic: "100", Scale: 2}}
		b, e := json.Marshal(policies.Change{Version: price})
		must(t, e)
		if code := apiRequest(t, limited, "/api/v1/access-policies", "POST", string(b)); code != 403 {
			t.Fatal("commercial authority collapsed", code)
		}
	})
	t.Run("historical_price_and_offer_reference", func(t *testing.T) {
		v := activePolicy(t, u, "downloader_upgrade_price")
		oldVersion := v.Version
		v.Definition.AmountAtomic = "900"
		savePolicy(t, u, v)
		var amount string
		must(t, db.Pool.QueryRow(ctx, "SELECT definition->>'amount_atomic' FROM policy_versions WHERE key=$1 AND version=$2", v.Key, oldVersion).Scan(&amount))
		if amount != "500" {
			t.Fatal("historical price rewritten")
		}
		o := activePolicy(t, u, "downloader_upgrade")
		if o.Definition.PriceVersion != oldVersion {
			t.Fatal("offer floats to current price")
		}
	})
}
func ptrTime(t time.Time) *time.Time { return &t }
func apiJSON(t *testing.T, u identity.Result, path, body string) []byte {
	t.Helper()
	token, e := auth.Token()
	must(t, e)
	_, e = db.Pool.Exec(ctx, "INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '1 hour')", auth.Digest(token), u.UserID)
	must(t, e)
	a := httpapi.API{DB: db, Identity: &identity.Service{DB: db}, Config: config.Config{MiniAppURL: "http://localhost:5173", Synthetic: true}}
	r := httptest.NewRequest("POST", path, strings.NewReader(body))
	r.Header.Set("Origin", "http://localhost:5173")
	r.Header.Set("X-Workspace-ID", u.WorkspaceID)
	r.AddCookie(&http.Cookie{Name: "telmivo_session", Value: token})
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
	}
	return w.Body.Bytes()
}
