//go:build integration

package integration

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/identity"
	"github.com/naseri-coder/TELMIVO/internal/domains/jobs"
	"github.com/naseri-coder/TELMIVO/internal/domains/policies"
	"github.com/naseri-coder/TELMIVO/internal/domains/wallets"
	"github.com/naseri-coder/TELMIVO/internal/platform/auth"
	"github.com/naseri-coder/TELMIVO/internal/platform/config"
	cred "github.com/naseri-coder/TELMIVO/internal/platform/crypto"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/httpapi"
	"github.com/naseri-coder/TELMIVO/internal/platform/storage"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func installation(t *testing.T, u identity.Result) string {
	var id string
	tenant(t, u, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "INSERT INTO customer_bots(workspace_id,name,username,key_id,encrypted_token,status,idempotency_key) VALUES($1,'synthetic',$2,'v1',$3,'DRAFT',$2) RETURNING id::text", u.WorkspaceID, fmt.Sprintf("r2b_install_%d", sequence.Add(1)), []byte("synthetic-pending")).Scan(&id)
	})
	return id
}
func TestR2BTrialScopes(t *testing.T) {
	for _, scope := range []string{"PER_WORKSPACE", "PER_INSTALLATION"} {
		t.Run(scope, func(t *testing.T) {
			u := fixture(t, true)
			other := fixture(t, false)
			bot := installation(t, u)
			subject := u.WorkspaceID
			if scope == "PER_INSTALLATION" {
				subject = bot
			}
			v := savePolicy(t, u, policies.Version{Key: "r2b_trial_" + scope, Kind: "TRIAL", Workspace: u.WorkspaceID, Enabled: true, EffectiveFrom: time.Now().Add(-time.Second), Reason: "scope acceptance", Definition: policies.Definition{Scope: scope, Duration: 2, Eligibility: "ANY", Trigger: "INSTALLATION", Once: true, Stacking: "KEEP_FIRST", Fallback: "FREE_LIMITED", GrantKind: "PRODUCT_TRIAL", Features: []string{"downloads"}}})
			enroll := func(source string) error {
				return db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
					return policies.Enroll(ctx, tx, policies.Subject{User: u.UserID, Workspace: u.WorkspaceID, Installation: bot, Source: source, Event: "stable-entry"}, "INSTALLATION")
				})
			}
			var wg sync.WaitGroup
			errs := make(chan error, 8)
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func() { defer wg.Done(); errs <- enroll("MAIN_BOT") }()
			}
			wg.Wait()
			close(errs)
			for e := range errs {
				must(t, e)
			}
			must(t, enroll("DOWNLOADER_BOT"))
			if n := number(t, u, "SELECT count(*) FROM trial_enrollments WHERE policy_key=$1 AND subject_id=$2", v.Key, subject); n != 1 {
				t.Fatal("duplicate enrollment", n)
			}
			v.Definition.Duration = 86400
			v = savePolicy(t, u, v)
			must(t, enroll("MINIAPP"))
			if n := number(t, u, "SELECT policy_version FROM trial_enrollments WHERE policy_key=$1 AND subject_id=$2", v.Key, subject); n != 1 {
				t.Fatal("snapshot rewritten")
			}
			tenant(t, u, func(tx pgx.Tx) error {
				r := policyRequest(u)
				r.Installation = bot
				var end time.Time
				if e := tx.QueryRow(ctx, "SELECT ends_at FROM trial_enrollments WHERE policy_key=$1", v.Key).Scan(&end); e != nil {
					return e
				}
				r.Now = end.Add(time.Second)
				d, e := policies.Resolve(ctx, tx, r)
				if e == nil && (len(d.Trials) != 1 || d.Trials[0].State != "EXPIRED" || d.Trials[0].Fallback != "FREE_LIMITED" || !d.Allowed) {
					t.Fatalf("expiry/fallback %+v", d)
				}
				return e
			})
			if n := number(t, other, "SELECT count(*) FROM trial_enrollments WHERE policy_key=$1", v.Key); n != 0 {
				t.Fatal("workspace leaked")
			}
			if scope == "PER_INSTALLATION" {
				e := db.Tenant(ctx, other.UserID, other.WorkspaceID, false, func(tx pgx.Tx) error {
					return policies.Enroll(ctx, tx, policies.Subject{User: other.UserID, Workspace: other.WorkspaceID, Installation: bot, Source: "MINIAPP", Event: "foreign"}, "INSTALLATION")
				})
				if !errors.Is(e, database.ErrForbidden) {
					t.Fatal("installation ownership", e)
				}
			}
			time.Sleep(2100 * time.Millisecond)
			must(t, enroll("GROUP_BOT"))
			if n := number(t, u, "SELECT count(*) FROM trial_enrollments WHERE policy_key=$1", v.Key); n != 1 {
				t.Fatal("expiry reset")
			}
		})
	}
}
func TestR2BJobLease(t *testing.T) {
	u := fixture(t, true)
	savePolicy(t, u, policies.Version{Key: "r2b_job_quota", Kind: "QUOTA", Feature: "tickets", Workspace: u.WorkspaceID, Enabled: true, EffectiveFrom: time.Now().Add(-time.Second), Reason: "synthetic long jobs", Definition: policies.Definition{Metric: "r2b.job_count", Unit: "JOBS", Window: "ROLLING_WINDOW", WindowSeconds: 3600, Timezone: "UTC", Limit: 5, Mode: "USAGE", ReservationSeconds: 2}})
	r := policies.Request{User: u.UserID, Workspace: u.WorkspaceID, Role: "OWNER", Feature: "tickets", Permission: "tickets.create", Requested: 1}
	var id string
	tenant(t, u, func(tx pgx.Tx) error {
		var e error
		id, e = jobs.Create(ctx, tx, r, "r2b-long-job", "synthetic", "r2b")
		return e
	})
	var won atomic.Int64
	var winner jobs.Lease
	var mu sync.Mutex
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, worker := range []string{"worker-a", "worker-b"} {
		wg.Add(1)
		go func(worker string) {
			defer wg.Done()
			var l jobs.Lease
			e := db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
				var e error
				l, e = jobs.Acquire(ctx, tx, u.WorkspaceID, id, worker, 200*time.Millisecond)
				return e
			})
			if e == nil {
				won.Add(1)
				mu.Lock()
				winner = l
				mu.Unlock()
			} else if !errors.Is(e, database.ErrConflict) {
				errs <- e
			}
		}(worker)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		must(t, e)
	}
	if won.Load() != 1 {
		t.Fatal("owners", won.Load())
	}
	stale := winner
	tenant(t, u, func(tx pgx.Tx) error { var e error; winner, e = jobs.Renew(ctx, tx, winner, r, time.Second); return e })
	time.Sleep(1100 * time.Millisecond)
	tenant(t, u, func(tx pgx.Tx) error {
		var e error
		winner, e = jobs.Acquire(ctx, tx, u.WorkspaceID, id, "reclaimer", 500*time.Millisecond)
		return e
	})
	for _, operation := range []string{"renew", "finish"} {
		e := db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
			if operation == "renew" {
				_, e := jobs.Renew(ctx, tx, stale, r, time.Second)
				return e
			}
			return jobs.Finish(ctx, tx, stale, "COMPLETED", "stale")
		})
		if !errors.Is(e, database.ErrConflict) {
			t.Fatal("stale worker", operation, e)
		}
	}
	tenant(t, u, func(tx pgx.Tx) error {
		var e error
		winner, e = jobs.Renew(ctx, tx, winner, r, 5*time.Second)
		return e
	})
	// Outlive the immutable reservation's original two-second TTL.
	time.Sleep(1100 * time.Millisecond)
	tenant(t, u, func(tx pgx.Tx) error { return jobs.Finish(ctx, tx, winner, "COMPLETED", "finish") })
	tenant(t, u, func(tx pgx.Tx) error { return jobs.Finish(ctx, tx, winner, "COMPLETED", "replay") })
	if n := number(t, u, "SELECT COALESCE(sum(used),0)::bigint FROM quota_usage WHERE workspace_id=$1 AND metric='r2b.job_count'", u.WorkspaceID); n != 1 {
		t.Fatal("usage charged twice", n)
	}
	if n := number(t, u, "SELECT count(*) FROM outbox_events WHERE workspace_id=$1 AND event_type='media.terminal'", u.WorkspaceID); n != 1 {
		t.Fatal("terminal replay", n)
	}
	for _, state := range []string{"FAILED", "CANCELLED"} {
		var job string
		var l jobs.Lease
		key := "r2b-" + state
		tenant(t, u, func(tx pgx.Tx) error {
			var e error
			job, e = jobs.Create(ctx, tx, r, key, "synthetic", "fixture")
			return e
		})
		tenant(t, u, func(tx pgx.Tx) error {
			var e error
			l, e = jobs.Acquire(ctx, tx, u.WorkspaceID, job, "worker", time.Second)
			return e
		})
		tenant(t, u, func(tx pgx.Tx) error { return jobs.Finish(ctx, tx, l, state, "failure") })
		if n := number(t, u, "SELECT count(*) FROM quota_reservations WHERE workspace_id=$1 AND operation_key=$2 AND state='RELEASED'", u.WorkspaceID, key); n != 1 {
			t.Fatal("failure release", n)
		}
	}
	other := fixture(t, false)
	if n := number(t, other, "SELECT count(*) FROM job_leases WHERE job_id=$1", id); n != 0 {
		t.Fatal("lease isolation")
	}
}
func TestR2BCredentialRotation(t *testing.T) {
	u := fixture(t, true)
	bot := installation(t, u)
	old, e := cred.NewKeyring("v1", map[string][]byte{"v1": bytes.Repeat([]byte{1}, 32)})
	must(t, e)
	encrypted, e := (cred.RotatingStore{Keys: old}).Seal(ctx, u.WorkspaceID+":"+bot, []byte("synthetic-credential"))
	must(t, e)
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE customer_bots SET key_id=$2,encrypted_token=$3 WHERE id=$1", bot, encrypted.KeyID, encrypted.Ciphertext)
		return e
	})
	keys, e := cred.NewKeyring("v2", map[string][]byte{"v1": bytes.Repeat([]byte{1}, 32), "v2": bytes.Repeat([]byte{2}, 32)})
	must(t, e)
	store := cred.RotatingStore{Keys: keys}
	for i := 0; i < 2; i++ {
		tenant(t, u, func(tx pgx.Tx) error {
			return identity.RotateCredential(ctx, tx, u.UserID, u.WorkspaceID, bot, "test rotation", "rotation", store)
		})
	}
	if n := number(t, u, "SELECT count(*) FROM audit_logs WHERE entity_id=$1 AND action='credential.rotate'", bot); n != 1 {
		t.Fatal("rotation repeated")
	}
	tenant(t, u, func(tx pgx.Tx) error {
		var v cred.EncryptedSecret
		if e := tx.QueryRow(ctx, "SELECT key_id,encrypted_token FROM customer_bots WHERE id=$1", bot).Scan(&v.KeyID, &v.Ciphertext); e != nil {
			return e
		}
		if v.KeyID != "v2" {
			t.Fatal("active key unused")
		}
		_, e := store.Open(ctx, u.WorkspaceID+":"+bot, v)
		return e
	})
}
func TestR2BPluginContracts(t *testing.T) {
	u := fixture(t, true)
	_, e := owner.Pool.Exec(ctx, "UPDATE feature_flags SET enabled=true WHERE feature_key='uploader'; UPDATE plugins SET status='ENABLED' WHERE key='uploader'; UPDATE plan_plugin_entitlements SET enabled=true WHERE plan_key='PRO' AND plugin_key='uploader'")
	must(t, e)
	defer func() {
		_, e := owner.Pool.Exec(ctx, "UPDATE feature_flags SET enabled=false WHERE feature_key='uploader'; UPDATE plugins SET status='DISABLED' WHERE key='uploader'; UPDATE plan_plugin_entitlements SET enabled=false WHERE plan_key='PRO' AND plugin_key='uploader'; DELETE FROM plugin_dependencies WHERE plugin_key='uploader'")
		must(t, e)
	}()
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE subscriptions SET plan_key='PRO' WHERE workspace_id=$1", u.WorkspaceID)
		return e
	})
	r := policies.Request{User: u.UserID, Workspace: u.WorkspaceID, Role: "OWNER", Feature: "uploader", Plugin: "uploader", Permission: "uploads.create", Capability: "uploader.use"}
	check := func(t *testing.T, want bool) {
		t.Helper()
		tenant(t, u, func(tx pgx.Tx) error {
			d, e := policies.Resolve(ctx, tx, r)
			if e == nil && d.Allowed != want {
				t.Fatalf("decision %+v", d)
			}
			again, e2 := policies.Resolve(ctx, tx, r)
			if e2 != nil {
				return e2
			}
			if d.Reason != again.Reason || d.Allowed != again.Allowed {
				t.Fatal("nondeterministic")
			}
			return e
		})
	}
	t.Run("plan_enabled", func(t *testing.T) { check(t, true) })
	t.Run("disabled_plugin", func(t *testing.T) {
		_, e := owner.Pool.Exec(ctx, "UPDATE plugins SET status='DISABLED' WHERE key='uploader'")
		must(t, e)
		check(t, false)
		_, e = owner.Pool.Exec(ctx, "UPDATE plugins SET status='ENABLED' WHERE key='uploader'")
		must(t, e)
	})
	t.Run("dependency_missing_and_satisfied", func(t *testing.T) {
		_, e := owner.Pool.Exec(ctx, "INSERT INTO plugin_dependencies VALUES('uploader','faq')")
		must(t, e)
		check(t, false)
		_, e = owner.Pool.Exec(ctx, "UPDATE plugins SET status='ENABLED' WHERE key='faq';UPDATE plan_plugin_entitlements SET enabled=true WHERE plan_key='PRO' AND plugin_key='faq'")
		must(t, e)
		check(t, true)
		_, e = owner.Pool.Exec(ctx, "DELETE FROM plugin_dependencies WHERE plugin_key='uploader';UPDATE plugins SET status='DISABLED' WHERE key='faq';UPDATE plan_plugin_entitlements SET enabled=false WHERE plan_key='PRO' AND plugin_key='faq'")
		must(t, e)
	})
	t.Run("capability_permission", func(t *testing.T) {
		r.Role = "MEMBER"
		check(t, false)
		r.Role = "OWNER"
		r.Capability = "uploader.unknown"
		check(t, false)
		r.Capability = "uploader.use"
	})

	t.Run("trial_entitlement_independent_of_plan", func(t *testing.T) {
		tenant(t, u, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, "UPDATE subscriptions SET plan_key='FREE' WHERE workspace_id=$1", u.WorkspaceID)
			return e
		})
		check(t, false)
		savePolicy(t, u, policies.Version{Key: "r2b_plugin_trial", Kind: "TRIAL", Workspace: u.WorkspaceID, Enabled: true, EffectiveFrom: time.Now().Add(-time.Second), Reason: "trial entitlement fixture", Definition: policies.Definition{Scope: "PER_WORKSPACE", Duration: 1, Eligibility: "ANY", Trigger: "INSTALLATION", Once: true, Stacking: "PARALLEL", Fallback: "FREE_LIMITED", GrantKind: "PRODUCT_TRIAL", Features: []string{"uploader"}, Plugins: []string{"uploader"}, Capabilities: []string{"uploader.use"}}})
		tenant(t, u, func(tx pgx.Tx) error {
			return policies.Enroll(ctx, tx, policies.Subject{User: u.UserID, Workspace: u.WorkspaceID, Source: "MINIAPP", Event: "plugin-trial"}, "INSTALLATION")
		})
		check(t, true)
		r.Role = "MEMBER"
		check(t, false)
		r.Role = "OWNER"
		time.Sleep(1100 * time.Millisecond)
		check(t, false)
	})
	t.Run("commercial_grants_cannot_bypass_security", func(t *testing.T) {
		v := savePolicy(t, u, policies.Version{Key: "r2b_addon", Kind: "GRANT", Workspace: u.WorkspaceID, Enabled: true, EffectiveFrom: time.Now().Add(-time.Second), EffectiveUntil: ptrTime(time.Now().Add(time.Hour)), Reason: "test addon", Definition: policies.Definition{GrantKind: "ADDON", Features: []string{"uploader"}, Plugins: []string{"uploader"}, Capabilities: []string{"uploader.use"}}})
		check(t, true)
		r.Role = "MEMBER"
		check(t, false)
		r.Role = "OWNER"
		v.Definition.GrantKind = "TEMPORARY_GRANT"
		savePolicy(t, u, v)
		check(t, true)
	})
	t.Run("ADD_MAX_MIN_REPLACE_and_cap", func(t *testing.T) {
		for _, test := range []struct {
			strategy     string
			amount, want int64
		}{{"ADD", 10, 20000000010}, {"MAX", 30000000000, 30000000000}, {"MIN", 10, 10}, {"REPLACE", 42, 42}} {
			t.Run(test.strategy, func(t *testing.T) {
				v := activePolicy(t, u, "r2b_addon")
				v.Definition.Limits = map[string]policies.LimitGrant{"uploader.storage_bytes": {Amount: test.amount, Strategy: test.strategy}, "uploader.max_file_bytes": {Amount: 10000000000, Strategy: "MAX"}}
				savePolicy(t, u, v)
				tenant(t, u, func(tx pgx.Tx) error {
					d, e := policies.Resolve(ctx, tx, r)
					if e == nil && (d.Resources["uploader.storage_bytes"].Limit != test.want || d.Resources["uploader.max_file_bytes"].Limit != 5000000000) {
						t.Fatal("merge", d.Resources)
					}
					return e
				})
			})
		}
		r.Amounts = map[string]int64{"uploader.max_file_bytes": 6000000000}
		check(t, false)
		r.Amounts = nil
	})
	t.Run("workspace_override", func(t *testing.T) {
		v := activePolicy(t, u, "uploader_PRO_uploader.storage_bytes")
		v.Key = "r2b_storage_override"
		v.Version = 0
		v.Workspace = u.WorkspaceID
		v.Plan = ""
		v.Definition.Source = "WORKSPACE_OVERRIDE"
		v.Definition.Strategy = "REPLACE"
		v.Definition.Limit = 77
		savePolicy(t, u, v)
		tenant(t, u, func(tx pgx.Tx) error {
			d, e := policies.Resolve(ctx, tx, r)
			if e == nil && d.Resources["uploader.storage_bytes"].Limit != 77 {
				t.Fatal("override", d.Resources)
			}
			return e
		})
	})
	t.Run("same_priority_conflict", func(t *testing.T) {
		v := activePolicy(t, u, "r2b_storage_override")
		v.Key = "r2b_conflicting_override"
		v.Version = 0
		e := db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
			_, e := policies.Save(ctx, tx, u.UserID, u.WorkspaceID, "conflict", policies.Change{Version: v})
			return e
		})
		if !errors.Is(e, database.ErrConflict) {
			t.Fatal("ambiguous resource", e)
		}
	})
}
func TestR2BRecoveryPreparation(t *testing.T) {
	u := fixture(t, true)
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := wallets.Adjust(ctx, tx, u.UserID, u.WorkspaceID, "synthetic-recovery-ledger", wallets.Adjustment{Asset: "USD", AmountAtomic: 100, Reason: "synthetic restore fixture", Key: "r2b-restore-ledger"})
		return e
	})
	savePolicy(t, u, policies.Version{Key: "r2b_recovery_quota", Kind: "QUOTA", Feature: "tickets", Workspace: u.WorkspaceID, Enabled: true, EffectiveFrom: time.Now().Add(-time.Second), Reason: "recovery fixture", Definition: policies.Definition{Metric: "r2b.recovery_count", Unit: "JOBS", Window: "NONE", Timezone: "UTC", Limit: 3, Mode: "USAGE", ReservationSeconds: 3600, CountFailure: true}})
	r := policies.Request{User: u.UserID, Workspace: u.WorkspaceID, Role: "OWNER", Feature: "tickets", Permission: "tickets.create", Requested: 1}
	var id string
	tenant(t, u, func(tx pgx.Tx) error {
		var e error
		id, e = jobs.Create(ctx, tx, r, "r2b-recovery-job", "synthetic-recovery", "recover")
		return e
	})
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := jobs.Acquire(ctx, tx, u.WorkspaceID, id, "crashed-worker", 100*time.Millisecond)
		return e
	})
	// Persist dedup before the dump. The queued job itself remains unapplied.
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "INSERT INTO inbox_events(consumer,event_id) SELECT 'r2b-restored-dedup',id FROM outbox_events WHERE workspace_id=$1 AND event_type='media.queued'", u.WorkspaceID)
		return e
	})
	var failed string
	var l jobs.Lease
	tenant(t, u, func(tx pgx.Tx) error {
		var e error
		failed, e = jobs.Create(ctx, tx, r, "r2b-charged-failure", "synthetic-recovery", "failure")
		return e
	})
	tenant(t, u, func(tx pgx.Tx) error {
		var e error
		l, e = jobs.Acquire(ctx, tx, u.WorkspaceID, failed, "failed-worker", time.Second)
		return e
	})
	tenant(t, u, func(tx pgx.Tx) error { return jobs.Finish(ctx, tx, l, "FAILED", "failure") })
	if n := number(t, u, "SELECT used FROM quota_usage WHERE workspace_id=$1 AND metric='r2b.recovery_count'", u.WorkspaceID); n != 1 {
		t.Fatal("recorded count_failure ignored", n)
	}
}

func TestR2BStorageAPI(t *testing.T) {
	u := fixture(t, false)
	other := fixture(t, false)
	local, e := storage.NewLocal(t.TempDir(), 100)
	must(t, e)
	defer local.Close()
	scoped := storage.Scoped{Provider: local}
	key, e := storage.ObjectKey(u.WorkspaceID, "synthetic-object")
	must(t, e)
	must(t, scoped.Put(ctx, u.WorkspaceID, key, strings.NewReader("fixture"), storage.Metadata{Size: 7, ContentType: "text/plain"}))
	for _, tc := range []struct {
		user   identity.Result
		status int
	}{{u, 200}, {other, 403}} {
		token, e := auth.Token()
		must(t, e)
		_, e = db.Pool.Exec(ctx, "INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '1 hour')", auth.Digest(token), tc.user.UserID)
		must(t, e)
		api := httpapi.API{DB: db, Identity: &identity.Service{DB: db}, Storage: &scoped, Config: config.Config{MiniAppURL: "http://localhost:5173"}}
		req := httptest.NewRequest("GET", "/api/v1/storage/"+key, nil)
		req.Header.Set("X-Workspace-ID", tc.user.WorkspaceID)
		req.AddCookie(&http.Cookie{Name: "telmivo_session", Value: token})
		w := httptest.NewRecorder()
		api.Handler().ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("storage API %d", w.Code)
		}
	}
}

func TestR2BCalendarBoundary(t *testing.T) {
	u := fixture(t, true)
	savePolicy(t, u, policies.Version{Key: "r2b_calendar_quota", Kind: "QUOTA", Feature: "tickets", Workspace: u.WorkspaceID, Enabled: true, EffectiveFrom: time.Now().Add(-time.Second), Reason: "window boundary", Definition: policies.Definition{Metric: "r2b.calendar", Unit: "JOBS", Window: "CALENDAR_DAY", Timezone: "UTC", Limit: 1, Mode: "USAGE", ReservationSeconds: 900}})
	r := policies.Request{User: u.UserID, Workspace: u.WorkspaceID, Role: "OWNER", Feature: "tickets", Permission: "tickets.create", Requested: 1}
	var id string
	var l jobs.Lease
	tenant(t, u, func(tx pgx.Tx) error {
		var e error
		id, e = jobs.Create(ctx, tx, r, "r2b-calendar-job", "synthetic", "boundary")
		return e
	})
	tenant(t, u, func(tx pgx.Tx) error {
		var e error
		l, e = jobs.Acquire(ctx, tx, u.WorkspaceID, id, "calendar-worker", time.Minute)
		return e
	})
	e := db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
		var end time.Time
		if e := tx.QueryRow(ctx, "SELECT window_end FROM quota_lease_allocations WHERE job_id=$1", id).Scan(&end); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, "SELECT set_config('app.lease_owner',$1,true),set_config('app.lease_generation',$2,true)", l.Owner, fmt.Sprint(l.Generation)); e != nil {
			return e
		}
		return policies.RenewLease(ctx, tx, r, l.Reservation, l.Job, end.Add(time.Second))
	})
	if !errors.Is(e, database.ErrConflict) {
		t.Fatal("cross-window renewal", e)
	}
	if n := number(t, u, "SELECT count(*) FROM quota_reservations WHERE workspace_id=$1 AND operation_key='r2b-calendar-job'", u.WorkspaceID); n != 1 {
		t.Fatal("double allocation")
	}
}
