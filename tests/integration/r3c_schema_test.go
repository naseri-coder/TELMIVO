//go:build integration

package integration

import (
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/jobs"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestR3CSchema(t *testing.T) {
	a, b := fixture(t, false), fixture(t, false)
	id := r3cSubmit(t, a, "r3c-schema-owned")
	foreign := r3cSubmit(t, b, "r3c-schema-foreign")
	own, e := r3cProduct().Dispatch(ctx, a.UserID, a.WorkspaceID)
	must(t, e)
	if len(own) != 1 || own[0] != id {
		t.Fatal("own dispatch", own)
	}
	other, e := r3cProduct().Dispatch(ctx, b.UserID, b.WorkspaceID)
	must(t, e)
	for _, j := range other {
		if j == id {
			t.Fatal("foreign coordinates exposed")
		}
	}
	if _, e = r3cProduct().Dispatch(ctx, b.UserID, a.WorkspaceID); e == nil {
		t.Fatal("spoofed workspace admitted")
	}
	var ma, mb string
	tenant(t, a, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT media_job_id::text FROM download_jobs WHERE id=$1", id).Scan(&ma)
	})
	tenant(t, b, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT media_job_id::text FROM download_jobs WHERE id=$1", foreign).Scan(&mb)
	})
	e = db.Tenant(ctx, b.UserID, b.WorkspaceID, false, func(tx pgx.Tx) error {
		_, e := jobs.Acquire(ctx, tx, a.WorkspaceID, ma, "foreign-runtime", time.Minute)
		return e
	})
	if e == nil {
		t.Fatal("cross-workspace lease")
	}
	e = db.Tenant(ctx, a.UserID, a.WorkspaceID, false, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `INSERT INTO download_jobs(workspace_id,creator_id,provider,source_url,format_id,formats,state,synthetic,idempotency_key,media_job_id) VALUES($1,$2,'youtube','https://youtu.be/dQw4w9WgXcQ','','[]','QUEUED',true,'r3c-schema-cross',$3)`, a.WorkspaceID, a.UserID, mb)
		return e
	})
	if e == nil {
		t.Fatal("cross-workspace binding")
	}
	tenant(t, a, func(tx pgx.Tx) error {
		var dangerous, definer bool
		must(t, tx.QueryRow(ctx, "SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname=current_user").Scan(&dangerous))
		must(t, tx.QueryRow(ctx, "SELECT prosecdef FROM pg_proc WHERE proname='guard_downloader_product'").Scan(&definer))
		if dangerous || definer {
			t.Fatal("privileged runtime/schema")
		}
		var n int
		must(t, tx.QueryRow(ctx, "SELECT count(*) FROM download_jobs WHERE id=$1", foreign).Scan(&n))
		if n != 0 {
			t.Fatal("RLS read")
		}
		return nil
	})
	w := r3cWorker(t, r3cProvider{}, &r3cDelivery{})
	must(t, w.RunOne(ctx, a.UserID, a.WorkspaceID, id))
	tenant(t, a, func(tx pgx.Tx) error {
		l, e := jobs.Acquire(ctx, tx, a.WorkspaceID, ma, "schema-expiry", time.Minute)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "SELECT set_config('app.lease_owner',$1,true),set_config('app.lease_generation',$2,true)", l.Owner, strconv.FormatInt(l.Generation, 10)); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "UPDATE download_jobs SET format_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1", id); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, "UPDATE job_leases SET expires_at=clock_timestamp() WHERE job_id=$1", ma)
		return e
	})
	e = db.Tenant(ctx, a.UserID, a.WorkspaceID, false, func(tx pgx.Tx) error {
		return r3cProduct().Select(ctx, tx, a.UserID, a.WorkspaceID, id, "fixture-compatible")
	})
	if e == nil {
		t.Fatal("expired selection")
	}
	tenant(t, a, func(tx pgx.Tx) error {
		return r3cProduct().Cancel(ctx, tx, a.UserID, a.WorkspaceID, id, "schema-cancel")
	})
	reconnected, e := database.Open(ctx, os.Getenv("TEST_DATABASE_URL"), true)
	must(t, e)
	defer reconnected.Pool.Close()
	must(t, reconnected.Tenant(ctx, a.UserID, a.WorkspaceID, false, func(tx pgx.Tx) error {
		var cancel bool
		e := tx.QueryRow(ctx, "SELECT cancel_requested FROM download_jobs WHERE id=$1", id).Scan(&cancel)
		if !cancel {
			t.Fatal("lost cancellation")
		}
		return e
	}))
	must(t, w.RunOne(ctx, a.UserID, a.WorkspaceID, id))
	if r3cView(t, a, id).State != "CANCELLED" {
		t.Fatal("cancel reclaim")
	}
	if e = w.RunOne(ctx, a.UserID, a.WorkspaceID, id); e == nil {
		t.Fatal("terminal replay")
	}
}
