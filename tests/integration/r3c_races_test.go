//go:build integration

package integration

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/jobs"
	"github.com/naseri-coder/TELMIVO/internal/domains/policies"
	"github.com/naseri-coder/TELMIVO/internal/integrations/media"
	"io"
	"strconv"
	"sync"
	"testing"
	"time"
)

type r3cBlocking struct {
	r3cProvider
	entered, release chan struct{}
	once             sync.Once
}

func (p *r3cBlocking) Fetch(c context.Context, s media.Source, f media.Format, w io.Writer, n int64) error {
	p.once.Do(func() { close(p.entered) })
	select {
	case <-c.Done():
		return c.Err()
	case <-p.release:
		return p.r3cProvider.Fetch(c, s, f, w, n)
	}
}
func TestR3CCancelCompletionRace(t *testing.T) {
	u := fixture(t, false)
	id := r3cSubmit(t, u, "r3c-cancel-race")
	d := &r3cDelivery{result: media.DeliveryResult{Outcome: "DELIVERY_ACKNOWLEDGED", MessageID: 1}}
	p := &r3cBlocking{entered: make(chan struct{}), release: make(chan struct{})}
	w := r3cWorker(t, p, d)
	must(t, w.RunOne(ctx, u.UserID, u.WorkspaceID, id))
	r3cSelect(t, u, id)
	done := make(chan error, 1)
	go func() { done <- w.RunOne(ctx, u.UserID, u.WorkspaceID, id) }()
	select {
	case <-p.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not download")
	}
	tenant(t, u, func(tx pgx.Tx) error { return r3cProduct().Cancel(ctx, tx, u.UserID, u.WorkspaceID, id, "race-cancel") })
	close(p.release)
	must(t, <-done)
	if r3cView(t, u, id).State != "CANCELLED" || d.calls != 0 {
		t.Fatal("cancellation lost or media sent")
	}
}
func TestR3CLeaseRenewReclaimFence(t *testing.T) {
	u := fixture(t, false)
	id := r3cSubmit(t, u, "r3c-lease-chain")
	w := r3cWorker(t, r3cProvider{}, &r3cDelivery{})
	must(t, w.RunOne(ctx, u.UserID, u.WorkspaceID, id))
	r3cSelect(t, u, id)
	var mj string
	var old, current jobs.Lease
	tenant(t, u, func(tx pgx.Tx) error {
		must(t, tx.QueryRow(ctx, "SELECT media_job_id::text FROM download_jobs WHERE id=$1", id).Scan(&mj))
		var e error
		old, e = jobs.Acquire(ctx, tx, u.WorkspaceID, mj, "old-r3c-worker", time.Minute)
		return e
	})
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE job_leases SET expires_at=clock_timestamp() WHERE job_id=$1", mj)
		return e
	})
	tenant(t, u, func(tx pgx.Tx) error {
		var e error
		current, e = jobs.Acquire(ctx, tx, u.WorkspaceID, mj, "new-r3c-worker", time.Minute)
		return e
	})
	if current.Generation <= old.Generation {
		t.Fatal("generation not advanced")
	}
	tenant(t, u, func(tx pgx.Tx) error {
		var e error
		current, e = jobs.Renew(ctx, tx, current, policies.Request{User: u.UserID, Workspace: u.WorkspaceID, Role: "OWNER", Feature: "downloads", Permission: "downloads.create", Requested: 0}, time.Minute)
		return e
	})
	e := db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "SELECT set_config('app.lease_owner',$1,true),set_config('app.lease_generation',$2,true)", old.Owner, strconv.FormatInt(old.Generation, 10))
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, "UPDATE download_jobs SET state='DOWNLOADING' WHERE id=$1", id)
		return e
	})
	if e == nil {
		t.Fatal("stale generation transitioned download")
	}
	e = db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error { return jobs.Finish(ctx, tx, old, "COMPLETED", "stale") })
	if e == nil {
		t.Fatal("stale generation settled quota")
	}
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE job_leases SET expires_at=clock_timestamp() WHERE job_id=$1", mj)
		return e
	})
	tenant(t, u, func(tx pgx.Tx) error {
		return r3cProduct().Cancel(ctx, tx, u.UserID, u.WorkspaceID, id, "cancel-after-reclaim")
	})
	must(t, w.RunOne(ctx, u.UserID, u.WorkspaceID, id))
}
