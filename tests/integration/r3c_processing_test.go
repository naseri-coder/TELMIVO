//go:build integration

package integration

import (
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/jobs"
	"github.com/naseri-coder/TELMIVO/internal/integrations/media"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestR3CProcessingCleanup(t *testing.T) {
	u := fixture(t, false)
	id := r3cSubmit(t, u, "r3c-remux-success")
	d := &r3cDelivery{result: media.DeliveryResult{Outcome: "DELIVERY_ACKNOWLEDGED", MessageID: 1}}
	w := r3cWorker(t, r3cProvider{}, d)
	must(t, w.RunOne(ctx, u.UserID, u.WorkspaceID, id))
	tenant(t, u, func(tx pgx.Tx) error {
		return r3cProduct().Select(ctx, tx, u.UserID, u.WorkspaceID, id, "fixture-remux")
	})
	must(t, w.RunOne(ctx, u.UserID, u.WorkspaceID, id))
	if r3cView(t, u, id).State != "COMPLETED" || d.calls != 1 {
		t.Fatal("processing journey")
	}
	if number(t, u, "SELECT count(*) FROM audit_logs WHERE workspace_id=$1 AND entity_id=$2 AND action='download.state' AND new_summary->>'state'='PROCESSING'", u.WorkspaceID, id) != 1 {
		t.Fatal("missing durable processing transition")
	}
	must(t, w.Cleanup(ctx))
	if _, e := os.Stat(filepath.Join(w.Root, id)); !os.IsNotExist(e) {
		t.Fatal("terminal artifacts retained")
	}
	active := r3cSubmit(t, u, "r3c-cleanup-live")
	must(t, w.RunOne(ctx, u.UserID, u.WorkspaceID, active))
	var l jobs.Lease
	tenant(t, u, func(tx pgx.Tx) error {
		var mj string
		must(t, tx.QueryRow(ctx, "SELECT media_job_id::text FROM download_jobs WHERE id=$1", active).Scan(&mj))
		var e error
		l, e = jobs.Acquire(ctx, tx, u.WorkspaceID, mj, "cleanup-live-owner", time.Minute)
		return e
	})
	old := filepath.Join(w.Root, active, "1")
	current := filepath.Join(w.Root, active, strconv.FormatInt(l.Generation, 10))
	must(t, os.MkdirAll(old, 0700))
	must(t, os.MkdirAll(current, 0700))
	must(t, os.WriteFile(filepath.Join(current, "asset"), []byte("active owned bytes"), 0600))
	must(t, w.Cleanup(ctx))
	if _, e := os.Stat(old); !os.IsNotExist(e) {
		t.Fatal("stale generation retained")
	}
	if _, e := os.Stat(filepath.Join(current, "asset")); e != nil {
		t.Fatal("active generation deleted", e)
	}
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE job_leases SET expires_at=clock_timestamp() WHERE job_id=$1", l.Job)
		return e
	})
	must(t, w.Cleanup(ctx))
	if _, e := os.Stat(current); !os.IsNotExist(e) {
		t.Fatal("expired generation retained")
	}
	tenant(t, u, func(tx pgx.Tx) error {
		return r3cProduct().Cancel(ctx, tx, u.UserID, u.WorkspaceID, active, "cleanup-cancel")
	})
	must(t, w.RunOne(ctx, u.UserID, u.WorkspaceID, active))
}
