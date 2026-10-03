//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/identity"
	"github.com/naseri-coder/TELMIVO/internal/domains/jobs"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/queue"
	"os"
	"testing"
	"time"
)

func TestR2BRecovered(t *testing.T) {
	if os.Getenv("TEST_RESTORE_DATABASE_URL") == "" {
		t.Fatal("fresh restore URL required")
	}
	restored, e := database.Open(ctx, os.Getenv("TEST_RESTORE_DATABASE_URL"), true)
	must(t, e)
	defer restored.Pool.Close()
	var dangerous bool
	must(t, restored.Pool.QueryRow(ctx, "SELECT rolsuper OR rolbypassrls OR rolinherit FROM pg_roles WHERE rolname=current_user").Scan(&dangerous))
	if dangerous {
		t.Fatal("restored runtime must be NOSUPERUSER NOBYPASSRLS NOINHERIT")
	}
	// Bootstrap synthetic fixture coordinates with the dedicated restore migration
	// role. All behavior/isolation assertions below use the restricted runtime.
	restoredOwner, e := database.Open(ctx, os.Getenv("TEST_RESTORE_MIGRATION_DATABASE_URL"), false)
	must(t, e)
	defer restoredOwner.Pool.Close()
	original := db
	db = restored
	defer func() { db = original }()
	var u identity.Result
	var id string
	must(t, restoredOwner.Pool.QueryRow(ctx, "SELECT w.owner_id::text,m.workspace_id::text,m.id::text FROM media_jobs m JOIN workspaces w ON w.id=m.workspace_id WHERE m.idempotency_key='r2b-recovery-job'").Scan(&u.UserID, &u.WorkspaceID, &id))
	var trial identity.Result
	must(t, restoredOwner.Pool.QueryRow(ctx, "SELECT w.owner_id::text,t.workspace_id::text FROM trial_enrollments t JOIN workspaces w ON w.id=t.workspace_id WHERE t.policy_key='r2b_trial_PER_WORKSPACE'").Scan(&trial.UserID, &trial.WorkspaceID))
	if n := number(t, trial, `SELECT count(*) FROM trial_enrollments t JOIN policy_versions p ON p.key=t.policy_key WHERE t.policy_key='r2b_trial_PER_WORKSPACE' AND t.policy_version=1 AND t.snapshot->>'duration_seconds'='2' AND t.ends_at<clock_timestamp() AND p.version=2 AND p.definition->>'duration_seconds'='86400'`); n != 1 {
		t.Fatal("restored immutable trial/policy snapshot", n)
	}
	if n := number(t, u, "SELECT count(*) FROM ledger_transactions WHERE workspace_id=$1", u.WorkspaceID); n != 1 {
		t.Fatal("restored representative ledger transaction", n)
	}
	if n := number(t, u, "SELECT count(*) FROM (SELECT transaction_id FROM ledger_entries GROUP BY transaction_id HAVING sum(amount_atomic)<>0) unbalanced"); n != 0 {
		t.Fatal("restored ledger imbalance", n)
	}
	tenant(t, u, func(tx pgx.Tx) error {
		n, e := jobs.Reconcile(ctx, tx, u.WorkspaceID, "restore-reconcile")
		if e == nil && n != 1 {
			t.Fatal("recovery wakeup", n)
		}
		return e
	})
	tenant(t, u, func(tx pgx.Tx) error {
		n, e := jobs.Reconcile(ctx, tx, u.WorkspaceID, "restore-replay")
		if e == nil && n != 0 {
			t.Fatal("duplicate recovery wakeup")
		}
		return e
	})
	var stale, current jobs.Lease
	tenant(t, u, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT job_id::text,workspace_id::text,owner,generation,reservation_key FROM job_leases WHERE job_id=$1", id).Scan(&stale.Job, &stale.Workspace, &stale.Owner, &stale.Generation, &stale.Reservation)
	})
	tenant(t, u, func(tx pgx.Tx) error {
		var e error
		current, e = jobs.Acquire(ctx, tx, u.WorkspaceID, id, "restored-worker", time.Minute)
		return e
	})
	e = db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error { return jobs.Finish(ctx, tx, stale, "COMPLETED", "stale-after-restore") })
	if !errors.Is(e, database.ErrConflict) {
		t.Fatal("restored fencing", e)
	}
	other := fixture(t, false)
	if n := number(t, other, "SELECT count(*) FROM job_leases WHERE job_id=$1", id); n != 0 {
		t.Fatal("restored isolation")
	}
	q, e := queue.Open(os.Getenv("TEST_NATS_URL"))
	must(t, e)
	defer q.Conn.Close()
	must(t, q.Ensure())
	var event queue.Envelope
	must(t, db.Pool.QueryRow(ctx, "SELECT id::text,workspace_id::text,event_type,correlation_id,payload FROM outbox_events WHERE workspace_id=$1 AND event_type='media.queued' AND payload->>'job_id'=$2", u.WorkspaceID, id).Scan(&event.ID, &event.WorkspaceID, &event.Type, &event.CorrelationID, &event.Payload))
	var calls int
	cancel, done := consumer(t, q, "telmivo.jobs.media", "r2b-restored-dedup", func(context.Context, pgx.Tx, queue.Envelope) error {
		calls++
		return errors.New("dedup must skip handler")
	})
	raw, e := json.Marshal(event)
	must(t, e)
	_, e = q.JS.Publish("telmivo.jobs.media", raw)
	must(t, e)
	wait(t, func() bool {
		info, e := q.JS.ConsumerInfo("TELMIVO", "r2b-restored-dedup")
		return e == nil && info.NumPending == 0 && info.NumAckPending == 0
	})
	stop(t, cancel, done)
	if calls != 0 {
		t.Fatal("inbox restore dedup failed")
	}
	// Use a distinct durable for the still-pending committed event. Handler effects
	// and inbox acknowledgement are atomic; duplicate transport replay is harmless.
	cancel, done = consumer(t, q, "telmivo.jobs.media", "r2b-restored-work", func(c context.Context, tx pgx.Tx, en queue.Envelope) error {
		var payload struct {
			Job string `json:"job_id"`
		}
		if e := json.Unmarshal(en.Payload, &payload); e != nil {
			return e
		}
		if payload.Job != id {
			return nil
		}
		return jobs.Finish(c, tx, current, "COMPLETED", en.CorrelationID)
	})
	wait(t, func() bool {
		must(t, q.Relay(ctx, db))
		return number(t, u, "SELECT count(*) FROM media_jobs WHERE id=$1 AND state='COMPLETED'", id) == 1
	})
	wait(t, func() bool {
		info, e := q.JS.ConsumerInfo("TELMIVO", "r2b-restored-work")
		return e == nil && info.NumPending == 0 && info.NumAckPending == 0
	})
	stop(t, cancel, done)
	_, e = q.JS.Publish("telmivo.jobs.media", raw)
	must(t, e)
	var replayCalls int
	cancel, done = consumer(t, q, "telmivo.jobs.media", "r2b-restored-work", func(context.Context, pgx.Tx, queue.Envelope) error {
		replayCalls++
		return errors.New("terminal replay handler must be skipped")
	})
	wait(t, func() bool {
		info, e := q.JS.ConsumerInfo("TELMIVO", "r2b-restored-work")
		return e == nil && info.NumPending == 0 && info.NumAckPending == 0
	})
	stop(t, cancel, done)
	if replayCalls != 0 {
		t.Fatal("terminal replay executed handler", replayCalls)
	}
	if n := number(t, u, "SELECT count(*) FROM outbox_events WHERE workspace_id=$1 AND event_type='media.terminal' AND payload->>'job_id'=$2", u.WorkspaceID, id); n != 1 {
		t.Fatal("restored terminal duplicated")
	}
	if n := number(t, u, "SELECT used FROM quota_usage WHERE workspace_id=$1 AND metric='r2b.recovery_count'", u.WorkspaceID); n != 2 {
		t.Fatal("restored quota posting", n)
	}
	if n := number(t, u, "SELECT count(*) FROM quota_reservations WHERE workspace_id=$1 AND operation_key IN('r2b-recovery-job','r2b-charged-failure') AND state='COMMITTED' AND policy_version=1 AND amount=1 AND count_failure", u.WorkspaceID); n != 2 {
		t.Fatal("restored reservation snapshot/terminal history", n)
	}
	t.Log("restricted restored runtime, snapshots, ledger, inbox dedup, replay, isolation, reclaim, fencing and quota history PASS")
}
