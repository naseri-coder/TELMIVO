//go:build integration

package integration

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	apple "github.com/naseri-coder/TELMIVO/internal/domains/appleservices"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
)

func r5aWorkspaceGrant(t *testing.T, userID, workspaceID string, limit int64) {
	t.Helper()
	must(t, db.Tenant(ctx, userID, workspaceID, false, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO temporary_grants(workspace_id,feature_key,enabled,limit_value,expires_at,reason)
			VALUES($1,'apple_services',true,$2,now()+interval '1 hour','R5A integration grant')
			ON CONFLICT(workspace_id,feature_key) DO UPDATE
			SET enabled=EXCLUDED.enabled,limit_value=EXCLUDED.limit_value,expires_at=EXCLUDED.expires_at,reason=EXCLUDED.reason`,
			workspaceID, limit)
		return err
	}))
}

func TestR5AFoundationPermissionsQuotaAuditIsolation(t *testing.T) {
	u := fixture(t, false)
	other := fixture(t, false)
	now := time.Now().UTC().Truncate(time.Second)
	svc := &apple.Service{DB: db, Clock: func() time.Time { return now }}

	if _, err := svc.EnsureProfile(ctx, u.UserID, u.WorkspaceID, "r5a-profile-denied"); err == nil {
		t.Fatal("Apple Services foundation unexpectedly available without entitlement")
	}

	r5aWorkspaceGrant(t, u.UserID, u.WorkspaceID, 2)
	profile, err := svc.EnsureProfile(ctx, u.UserID, u.WorkspaceID, "r5a-profile")
	must(t, err)
	if profile.State != "FOUNDATION_READY" || profile.Revision != 1 {
		t.Fatal("unexpected foundation profile", profile)
	}

	var ownerPermissions, supportPermissions int
	must(t, owner.Pool.QueryRow(ctx, `SELECT count(*) FROM role_permissions
		WHERE role_key='OWNER' AND permission_key LIKE 'apple.%'`).Scan(&ownerPermissions))
	must(t, owner.Pool.QueryRow(ctx, `SELECT count(*) FROM role_permissions
		WHERE role_key='SUPPORT_AGENT' AND permission_key LIKE 'apple.%'`).Scan(&supportPermissions))
	if ownerPermissions != 9 || supportPermissions != 0 {
		t.Fatal("Apple Services role permissions drift", ownerPermissions, supportPermissions)
	}

	expiry := now.Add(time.Hour)
	op, err := svc.CreateOperation(ctx, u.UserID, u.WorkspaceID, "r5a-create-consumer", apple.CreateRequest{
		Kind: apple.ConsumerAccount, SubjectRef: "telmivo-user:consumer-1", Key: "consumer-1", ExpiresAt: &expiry,
	})
	must(t, err)
	replay, err := svc.CreateOperation(ctx, u.UserID, u.WorkspaceID, "r5a-create-consumer-replay", apple.CreateRequest{
		Kind: apple.ConsumerAccount, SubjectRef: "telmivo-user:consumer-1", Key: "consumer-1", ExpiresAt: &expiry,
	})
	must(t, err)
	if replay.ID != op.ID || replay.Revision != op.Revision {
		t.Fatal("idempotent Apple operation replay changed identity", op, replay)
	}

	for _, step := range []apple.State{apple.Precheck, apple.AppleActionRequired, apple.AppleCompleted, apple.VerificationPending, apple.Ready} {
		op, err = svc.Transition(ctx, u.UserID, u.WorkspaceID, op.ID, "r5a-transition-"+string(step), apple.TransitionRequest{
			State: step, Expected: op.Revision, Reason: "integration state transition",
		})
		must(t, err)
	}
	if _, err = svc.Transition(ctx, u.UserID, u.WorkspaceID, op.ID, "r5a-invalid-transition", apple.TransitionRequest{
		State: apple.Precheck, Expected: op.Revision, Reason: "invalid reverse transition",
	}); !errors.Is(err, database.ErrInvalid) {
		t.Fatal("invalid state transition accepted", err)
	}

	events, err := svc.Events(ctx, u.UserID, u.WorkspaceID, op.ID)
	must(t, err)
	if len(events) != 6 || events[0].To != apple.Requested || events[len(events)-1].To != apple.Ready {
		t.Fatal("operation event history drift", events)
	}

	profile, err = svc.SetProfileState(ctx, u.UserID, u.WorkspaceID, "r5a-suspend", apple.ProfileAction{
		State: "SUSPENDED", Expected: profile.Revision, Reason: "integration suspension",
	})
	must(t, err)
	if _, err = svc.CreateOperation(ctx, u.UserID, u.WorkspaceID, "r5a-suspended-create", apple.CreateRequest{
		Kind: apple.BusinessConnection, SubjectRef: "workspace-business", Key: "business-while-suspended",
	}); !errors.Is(err, database.ErrConflict) {
		t.Fatal("suspended Apple Services profile allowed new operation", err)
	}
	profile, err = svc.SetProfileState(ctx, u.UserID, u.WorkspaceID, "r5a-resume", apple.ProfileAction{
		State: "FOUNDATION_READY", Expected: profile.Revision, Reason: "integration resume",
	})
	must(t, err)

	second, err := svc.CreateOperation(ctx, u.UserID, u.WorkspaceID, "r5a-create-business", apple.CreateRequest{
		Kind: apple.BusinessConnection, SubjectRef: "workspace-business", Key: "business-1",
	})
	must(t, err)
	if second.State != apple.Requested {
		t.Fatal("second Apple operation state", second)
	}
	if _, err = svc.CreateOperation(ctx, u.UserID, u.WorkspaceID, "r5a-over-quota", apple.CreateRequest{
		Kind: apple.IdentityVerification, SubjectRef: "telmivo-user:identity-1", Key: "identity-1",
	}); err == nil {
		t.Fatal("Apple Services quota was not enforced")
	}

	var used int64
	tenant(t, u, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT COALESCE(sum(used),0) FROM usage_counters
			WHERE workspace_id=$1 AND feature_key='apple_services'`, u.WorkspaceID).Scan(&used)
	})
	if used != 2 {
		t.Fatal("idempotent replay or denied request changed quota", used)
	}

	if number(t, u, "SELECT count(*) FROM audit_logs WHERE workspace_id=$1 AND action LIKE 'apple.%'", u.WorkspaceID) < 8 {
		t.Fatal("Apple Services audit coverage incomplete")
	}

	var leaked int64
	must(t, db.Tenant(ctx, other.UserID, other.WorkspaceID, false, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT count(*) FROM apple_service_operations WHERE id=$1", op.ID).Scan(&leaked)
	}))
	if leaked != 0 {
		t.Fatal("Apple Services tenant isolation failed")
	}

	var forcedRLS int
	must(t, owner.Pool.QueryRow(ctx, `SELECT count(*)
		FROM pg_class
		WHERE relname=ANY($1::text[])
		  AND relrowsecurity
		  AND relforcerowsecurity`,
		[]string{"apple_service_profiles", "apple_service_operations", "apple_service_operation_events"}).Scan(&forcedRLS))
	if forcedRLS != 3 {
		t.Fatal("Apple Services FORCE RLS coverage incomplete", forcedRLS)
	}

	var sensitiveColumns int
	must(t, owner.Pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public'
		  AND table_name LIKE 'apple_service_%'
		  AND lower(column_name) ~ '(password|otp|token|secret|credential)'`).Scan(&sensitiveColumns))
	if sensitiveColumns != 0 {
		t.Fatal("R5A schema contains prohibited secret-bearing columns", sensitiveColumns)
	}

	var pluginState string
	must(t, owner.Pool.QueryRow(ctx, "SELECT status FROM plugins WHERE key='apple-services'").Scan(&pluginState))
	if pluginState != "BETA" {
		t.Fatal("Apple Services must remain BETA in R5A", pluginState)
	}
	var enabledPlans int
	must(t, owner.Pool.QueryRow(ctx, "SELECT count(*) FROM plan_plugin_entitlements WHERE plugin_key='apple-services' AND enabled").Scan(&enabledPlans))
	if enabledPlans != 0 {
		t.Fatal("R5A unexpectedly commercialized Apple Services", enabledPlans)
	}
}

func TestR5ARecoveryPreparation(t *testing.T) {
	u := fixture(t, false)
	r5aWorkspaceGrant(t, u.UserID, u.WorkspaceID, 5)
	svc := &apple.Service{DB: db}
	_, err := svc.EnsureProfile(ctx, u.UserID, u.WorkspaceID, "r5a-recovery-profile")
	must(t, err)
	op, err := svc.CreateOperation(ctx, u.UserID, u.WorkspaceID, "r5a-recovery-create", apple.CreateRequest{
		Kind: apple.ManagedAccount, SubjectRef: "managed:recovery", Key: "r5a-recovery",
	})
	must(t, err)
	op, err = svc.Transition(ctx, u.UserID, u.WorkspaceID, op.ID, "r5a-recovery-precheck", apple.TransitionRequest{
		State: apple.Precheck, Expected: op.Revision, Reason: "prepare restore fixture",
	})
	must(t, err)
	_, err = svc.Transition(ctx, u.UserID, u.WorkspaceID, op.ID, "r5a-recovery-sync", apple.TransitionRequest{
		State: apple.SyncPending, Expected: op.Revision, Reason: "prepare restore fixture",
	})
	must(t, err)
}

func TestR5ARecovered(t *testing.T) {
	store, err := database.Open(ctx, os.Getenv("TEST_RESTORE_MIGRATION_DATABASE_URL"), false)
	must(t, err)
	defer store.Pool.Close()

	var state string
	var revision int64
	var workspace string
	must(t, store.Pool.QueryRow(ctx, `SELECT workspace_id::text,state,revision
		FROM apple_service_operations WHERE operation_key='r5a-recovery'`).Scan(&workspace, &state, &revision))
	if state != "SYNC_PENDING" || revision != 3 {
		t.Fatal("R5A operation recovery drift", state, revision)
	}
	var profileState string
	must(t, store.Pool.QueryRow(ctx, "SELECT state FROM apple_service_profiles WHERE workspace_id=$1", workspace).Scan(&profileState))
	if profileState != "FOUNDATION_READY" {
		t.Fatal("R5A profile recovery drift", profileState)
	}
	var events int
	must(t, store.Pool.QueryRow(ctx, "SELECT count(*) FROM apple_service_operation_events WHERE workspace_id=$1", workspace).Scan(&events))
	if events != 3 {
		t.Fatal("R5A event history recovery drift", events)
	}
}
