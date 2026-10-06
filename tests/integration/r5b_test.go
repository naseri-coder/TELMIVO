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

func TestR5BConsumerAssistanceSafetyAndLifecycle(t *testing.T) {
	u := fixture(t, false)
	other := fixture(t, false)
	now := time.Now().UTC().Truncate(time.Second)
	svc := &apple.Service{DB: db, Clock: func() time.Time { return now }}
	r5aWorkspaceGrant(t, u.UserID, u.WorkspaceID, 3)

	if _, err := svc.CreateOperation(ctx, u.UserID, u.WorkspaceID, "r5b-generic-consumer-blocked", apple.CreateRequest{
		Kind: apple.ConsumerAccount, SubjectRef: "telmivo-user:"+u.UserID, Key: "generic-consumer-blocked",
	}); !errors.Is(err, database.ErrInvalid) {
		t.Fatal("generic foundation API still creates CONSUMER_ACCOUNT after R5B", err)
	}

	start := apple.ConsumerStart{
		Goal: apple.ConsumerCreateAccount,
		Channel: apple.ConsumerWeb,
		Locale: "fa-IR",
		Region: "DE",
		Key: "r5b-create",
	}
	flow, err := svc.StartConsumer(ctx, u.UserID, u.WorkspaceID, "r5b-start", start)
	must(t, err)
	if flow.Operation.Kind != apple.ConsumerAccount || flow.Operation.State != apple.Requested ||
		flow.Operation.SubjectRef != "telmivo-user:"+u.UserID || flow.CompletionSource != "NONE" {
		t.Fatal("unexpected R5B start", flow)
	}
	if flow.Guidance.SensitiveInputAllowed || flow.Guidance.IdentityVerified || flow.Guidance.CompletionMeaning != "USER_ATTESTED_ONLY" {
		t.Fatal("unsafe R5B guidance", flow.Guidance)
	}
	earlyAttestationErr := db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE apple_consumer_assistance SET completion_source='USER_ATTESTED' WHERE operation_id=$1", flow.Operation.ID)
		return err
	})
	pgCode(t, earlyAttestationErr, "23514")

	replay, err := svc.StartConsumer(ctx, u.UserID, u.WorkspaceID, "r5b-replay", start)
	must(t, err)
	if replay.Operation.ID != flow.Operation.ID || replay.Operation.Revision != flow.Operation.Revision {
		t.Fatal("idempotent R5B replay changed operation", replay, flow)
	}
	conflict := start
	conflict.Goal = apple.ConsumerRecoverAccess
	if _, err = svc.StartConsumer(ctx, u.UserID, u.WorkspaceID, "r5b-conflict", conflict); !errors.Is(err, database.ErrConflict) {
		t.Fatal("same R5B key accepted different goal", err)
	}

	if _, err = svc.Transition(ctx, u.UserID, u.WorkspaceID, flow.Operation.ID, "r5b-bypass", apple.TransitionRequest{
		State: apple.Precheck, Expected: flow.Operation.Revision, Reason: "attempt generic bypass",
	}); !errors.Is(err, database.ErrInvalid) {
		t.Fatal("generic R5A transition bypassed R5B guided flow", err)
	}
	if _, err = svc.ConsumerAction(ctx, u.UserID, u.WorkspaceID, flow.Operation.ID, "r5b-skip", apple.ConsumerActionRequest{
		Action: apple.ConsumerConfirmCompletion, Expected: flow.Operation.Revision,
	}); !errors.Is(err, database.ErrInvalid) {
		t.Fatal("R5B allowed skipped lifecycle", err)
	}

	for _, action := range []apple.ConsumerAction{
		apple.ConsumerConfirmPrecheck,
		apple.ConsumerContinueAtApple,
		apple.ConsumerAppleFlowCompleted,
		apple.ConsumerVerificationStarted,
		apple.ConsumerConfirmCompletion,
	} {
		flow, err = svc.ConsumerAction(ctx, u.UserID, u.WorkspaceID, flow.Operation.ID, "r5b-"+string(action), apple.ConsumerActionRequest{
			Action: action, Expected: flow.Operation.Revision,
		})
		must(t, err)
		if action == apple.ConsumerContinueAtApple && flow.Guidance.OfficialURL != apple.AppleAccountURL {
			t.Fatal("R5B create flow did not point to Apple-controlled account site", flow.Guidance)
		}
	}
	if flow.Operation.State != apple.Ready || flow.CompletionSource != "USER_ATTESTED" || flow.Guidance.IdentityVerified {
		t.Fatal("R5B completion semantics drift", flow)
	}

	events, err := svc.Events(ctx, u.UserID, u.WorkspaceID, flow.Operation.ID)
	must(t, err)
	if len(events) != 6 || events[0].To != apple.Requested || events[len(events)-1].To != apple.Ready {
		t.Fatal("R5B event history drift", events)
	}

	recovery, err := svc.StartConsumer(ctx, u.UserID, u.WorkspaceID, "r5b-recovery-start", apple.ConsumerStart{
		Goal: apple.ConsumerRecoverAccess, Channel: apple.ConsumerMac, Locale: "en-US", Region: "US", Key: "r5b-recover",
	})
	must(t, err)
	recovery, err = svc.ConsumerAction(ctx, u.UserID, u.WorkspaceID, recovery.Operation.ID, "r5b-recovery-precheck", apple.ConsumerActionRequest{
		Action: apple.ConsumerConfirmPrecheck, Expected: recovery.Operation.Revision,
	})
	must(t, err)
	recovery, err = svc.ConsumerAction(ctx, u.UserID, u.WorkspaceID, recovery.Operation.ID, "r5b-recovery-apple", apple.ConsumerActionRequest{
		Action: apple.ConsumerContinueAtApple, Expected: recovery.Operation.Revision,
	})
	must(t, err)
	if recovery.Guidance.OfficialURL != apple.AppleRecoveryURL || recovery.Guidance.IdentityVerified {
		t.Fatal("R5B recovery guidance drift", recovery.Guidance)
	}

	var used int64
	tenant(t, u, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT COALESCE(sum(used),0) FROM usage_counters
			WHERE workspace_id=$1 AND feature_key='apple_services'`, u.WorkspaceID).Scan(&used)
	})
	if used != 2 {
		t.Fatal("R5B replay or denied paths changed quota", used)
	}

	var leaked int64
	must(t, db.Tenant(ctx, other.UserID, other.WorkspaceID, false, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT count(*) FROM apple_consumer_assistance WHERE operation_id=$1", flow.Operation.ID).Scan(&leaked)
	}))
	if leaked != 0 {
		t.Fatal("R5B tenant isolation failed")
	}

	peer := fixture(t, false)
	_, err = owner.Pool.Exec(ctx, "INSERT INTO workspace_members(workspace_id,user_id,role_key) VALUES($1,$2,'ADMIN')", u.WorkspaceID, peer.UserID)
	must(t, err)
	peer.WorkspaceID = u.WorkspaceID
	if _, err = svc.Consumer(ctx, peer.UserID, peer.WorkspaceID, flow.Operation.ID); !errors.Is(err, database.ErrForbidden) {
		t.Fatal("same-workspace administrator read another user's consumer assistance", err)
	}
	if _, err = svc.ConsumerAction(ctx, peer.UserID, peer.WorkspaceID, flow.Operation.ID, "r5b-peer-action", apple.ConsumerActionRequest{
		Action: apple.ConsumerConfirmCompletion, Expected: flow.Operation.Revision,
	}); !errors.Is(err, database.ErrForbidden) {
		t.Fatal("same-workspace administrator mutated another user's consumer assistance", err)
	}

	var forcedRLS int
	must(t, owner.Pool.QueryRow(ctx, `SELECT count(*) FROM pg_class
		WHERE relname='apple_consumer_assistance' AND relrowsecurity AND relforcerowsecurity`).Scan(&forcedRLS))
	if forcedRLS != 1 {
		t.Fatal("R5B FORCE RLS missing", forcedRLS)
	}

	var sensitiveColumns int
	must(t, owner.Pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name='apple_consumer_assistance'
		  AND lower(column_name) ~ '(password|otp|token|secret|credential|verification_code)'`).Scan(&sensitiveColumns))
	if sensitiveColumns != 0 {
		t.Fatal("R5B schema contains prohibited secret-bearing columns", sensitiveColumns)
	}

	if n := number(t, u, "SELECT count(*) FROM audit_logs WHERE workspace_id=$1 AND action='apple.consumer.started'", u.WorkspaceID); n != 2 {
		t.Fatal("R5B consumer start audit count", n)
	}
	if n := number(t, u, "SELECT count(*) FROM audit_logs WHERE workspace_id=$1 AND action='apple.consumer.action'", u.WorkspaceID); n != 7 {
		t.Fatal("R5B consumer action audit count", n)
	}

	mutationErr := db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE apple_consumer_assistance SET completion_source='NONE' WHERE operation_id=$1", flow.Operation.ID)
		return err
	})
	pgCode(t, mutationErr, "23514")
}

func TestR5BRecoveryPreparation(t *testing.T) {
	u := fixture(t, false)
	r5aWorkspaceGrant(t, u.UserID, u.WorkspaceID, 5)
	svc := &apple.Service{DB: db}
	flow, err := svc.StartConsumer(ctx, u.UserID, u.WorkspaceID, "r5b-restore-start", apple.ConsumerStart{
		Goal: apple.ConsumerCheckExisting, Channel: apple.ConsumerAndroidWindows, Locale: "de-DE", Region: "DE", Key: "r5b-recovery",
	})
	must(t, err)
	for _, action := range []apple.ConsumerAction{
		apple.ConsumerConfirmPrecheck,
		apple.ConsumerContinueAtApple,
		apple.ConsumerAppleFlowCompleted,
		apple.ConsumerVerificationStarted,
		apple.ConsumerConfirmCompletion,
	} {
		flow, err = svc.ConsumerAction(ctx, u.UserID, u.WorkspaceID, flow.Operation.ID, "r5b-restore-"+string(action), apple.ConsumerActionRequest{
			Action: action, Expected: flow.Operation.Revision,
		})
		must(t, err)
	}
}

func TestR5BRecovered(t *testing.T) {
	store, err := database.Open(ctx, os.Getenv("TEST_RESTORE_MIGRATION_DATABASE_URL"), false)
	must(t, err)
	defer store.Pool.Close()

	var state, subject, goal, channel, locale, region, completion string
	var revision int64
	must(t, store.Pool.QueryRow(ctx, `SELECT o.state,o.subject_ref,o.revision,c.goal,c.channel,c.locale,c.region,c.completion_source
		FROM apple_service_operations o
		JOIN apple_consumer_assistance c ON c.operation_id=o.id AND c.workspace_id=o.workspace_id
		WHERE o.operation_key='r5b-recovery'`).
		Scan(&state, &subject, &revision, &goal, &channel, &locale, &region, &completion))
	if state != "READY" || revision != 6 || goal != "CHECK_EXISTING" || channel != "ANDROID_WINDOWS" ||
		locale != "de-DE" || region != "DE" || completion != "USER_ATTESTED" || subject == "" {
		t.Fatal("R5B restore drift", state, subject, revision, goal, channel, locale, region, completion)
	}
	var events int
	must(t, store.Pool.QueryRow(ctx, `SELECT count(*) FROM apple_service_operation_events e
		JOIN apple_service_operations o ON o.id=e.operation_id AND o.workspace_id=e.workspace_id
		WHERE o.operation_key='r5b-recovery'`).Scan(&events))
	if events != 6 {
		t.Fatal("R5B event history lost after restore", events)
	}
}
