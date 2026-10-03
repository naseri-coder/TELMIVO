//go:build integration

package integration

import (
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/downloads"
	"github.com/naseri-coder/TELMIVO/internal/domains/identity"
	"github.com/naseri-coder/TELMIVO/internal/integrations/media"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"os"
	"testing"
)

func TestR3CRecovered(t *testing.T) {
	restored, e := database.Open(ctx, os.Getenv("TEST_RESTORE_DATABASE_URL"), true)
	must(t, e)
	defer restored.Pool.Close()
	fixtureOwner, e := database.Open(ctx, os.Getenv("TEST_RESTORE_MIGRATION_DATABASE_URL"), false)
	must(t, e)
	defer fixtureOwner.Pool.Close()
	var u identity.Result
	var completed, active, cancelled, unknown string
	must(t, fixtureOwner.Pool.QueryRow(ctx, "SELECT user_id::text FROM telegram_accounts WHERE telegram_id=98002001").Scan(&u.UserID))
	must(t, fixtureOwner.Pool.QueryRow(ctx, "SELECT workspace_id::text,id::text FROM download_jobs WHERE idempotency_key='r3c-recovery-completed'").Scan(&u.WorkspaceID, &completed))
	must(t, fixtureOwner.Pool.QueryRow(ctx, "SELECT id::text FROM download_jobs WHERE idempotency_key='r3c-recovery-active'").Scan(&active))
	must(t, fixtureOwner.Pool.QueryRow(ctx, "SELECT id::text FROM download_jobs WHERE idempotency_key='r3c-recovery-cancel'").Scan(&cancelled))
	must(t, fixtureOwner.Pool.QueryRow(ctx, "SELECT id::text FROM download_jobs WHERE idempotency_key='r3c-recovery-unknown'").Scan(&unknown))
	p := &downloads.Product{DB: restored, Synthetic: true, MaxBytes: 1 << 20}
	must(t, restored.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
		v, e := p.Detail(ctx, tx, u.UserID, u.WorkspaceID, completed, false)
		if e != nil {
			return e
		}
		if v.State != "COMPLETED" || v.Delivery != "DELIVERY_ACKNOWLEDGED" {
			t.Fatal(v)
		}
		var n int
		if e = tx.QueryRow(ctx, "SELECT count(*) FROM quota_reservations WHERE workspace_id=$1 AND operation_key='r3c-recovery-active' AND state='RESERVED'", u.WorkspaceID).Scan(&n); e != nil {
			return e
		}
		if n != 1 {
			t.Fatal("reservation snapshot", n)
		}
		return nil
	}))
	d := &r3cDelivery{result: media.DeliveryResult{Outcome: "DELIVERY_ACKNOWLEDGED", MessageID: 2}}
	w := r3cWorker(t, r3cProvider{}, d)
	w.Product = p
	w.Owner = "restored-worker"
	must(t, w.RunOne(ctx, u.UserID, u.WorkspaceID, active))
	if e = w.RunOne(ctx, u.UserID, u.WorkspaceID, completed); e == nil {
		t.Fatal("terminal restored replay")
	}
	must(t, w.RunOne(ctx, u.UserID, u.WorkspaceID, cancelled))
	must(t, restored.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
		v, e := p.Detail(ctx, tx, u.UserID, u.WorkspaceID, cancelled, false)
		if e != nil {
			return e
		}
		if v.State != "CANCELLED" {
			t.Fatal("lost cancel intent", v)
		}
		v, e = p.Detail(ctx, tx, u.UserID, u.WorkspaceID, unknown, false)
		if e != nil {
			return e
		}
		if v.State != "FAILED" || v.Delivery != "DELIVERY_OUTCOME_UNKNOWN" {
			t.Fatal("unknown collapsed", v)
		}
		var n int
		must(t, tx.QueryRow(ctx, "SELECT count(*) FROM download_jobs d JOIN media_jobs m ON (m.id,m.workspace_id)=(d.media_job_id,d.workspace_id) WHERE d.id=$1", active).Scan(&n))
		if n != 1 {
			t.Fatal("lost execution relation")
		}
		return nil
	}))
	if e = w.RunOne(ctx, u.UserID, u.WorkspaceID, unknown); e == nil {
		t.Fatal("unknown delivery replay")
	}
	if d.calls != 1 {
		t.Fatal("double delivery")
	}
}
