package jobs

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/policies"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"log/slog"
	"strconv"
	"time"
)

type Lease struct {
	Job, Workspace, Owner, Reservation string
	Generation                         int64
	AcquiredAt, ExpiresAt, Deadline    time.Time
}

const MaxLease = time.Minute * 5
const MaxLifetime = time.Hour * 24

// Create reuses media_jobs and the transactional outbox, never a separate queue.
func Create(ctx context.Context, tx pgx.Tx, r policies.Request, key, operation, correlation string) (string, error) {
	if operation == "" || len(operation) > 128 {
		return "", database.ErrInvalid
	}
	if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "job:"+r.Workspace+":"+key); e != nil {
		return "", e
	}
	var id, prior string
	e := tx.QueryRow(ctx, "SELECT id::text,operation FROM media_jobs WHERE workspace_id=$1 AND idempotency_key=$2", r.Workspace, key).Scan(&id, &prior)
	if e == nil {
		if prior != operation {
			return "", database.ErrConflict
		}
		_, e = policies.Reserve(ctx, tx, r, key)
		return id, e
	}
	if e != pgx.ErrNoRows {
		return "", e
	}
	if _, e = policies.Reserve(ctx, tx, r, key); e != nil {
		return "", e
	}
	e = tx.QueryRow(ctx, "INSERT INTO media_jobs(workspace_id,operation,state,idempotency_key) VALUES($1,$2,'QUEUED',$3) RETURNING id::text", r.Workspace, operation, key).Scan(&id)
	if e != nil {
		return "", e
	}
	e = database.Event(ctx, tx, r.Workspace, "telmivo.jobs.media", "media.queued", correlation, map[string]string{"job_id": id})
	return id, e
}
func Acquire(ctx context.Context, tx pgx.Tx, workspace, job, owner string, duration time.Duration) (Lease, error) {
	l := Lease{Job: job, Workspace: workspace, Owner: owner}
	if owner == "" || len(owner) > 128 || duration <= 0 || duration > MaxLease {
		return l, database.ErrInvalid
	}
	var state string
	var created, timeNow time.Time
	if e := tx.QueryRow(ctx, "SELECT state,idempotency_key,created_at,clock_timestamp() FROM media_jobs WHERE id=$1 AND workspace_id=$2 FOR UPDATE", job, workspace).Scan(&state, &l.Reservation, &created, &timeNow); e != nil {
		return l, e
	}
	if state != "QUEUED" && state != "RUNNING" {
		return l, database.ErrConflict
	}
	var oldOwner string
	var oldExpiry time.Time
	e := tx.QueryRow(ctx, "SELECT owner,generation,expires_at,deadline FROM job_leases WHERE job_id=$1", job).Scan(&oldOwner, &l.Generation, &oldExpiry, &l.Deadline)
	if e == nil && oldExpiry.After(timeNow) {
		return l, database.ErrConflict
	}
	if e != nil && e != pgx.ErrNoRows {
		return l, e
	}
	if l.Generation == 0 {
		l.Deadline = created.Add(MaxLifetime)
	}
	if !timeNow.Add(duration).Before(l.Deadline) {
		return l, database.ErrConflict
	}
	l.Generation++
	l.AcquiredAt = timeNow
	l.ExpiresAt = timeNow.Add(duration)
	_, e = tx.Exec(ctx, `INSERT INTO job_leases(workspace_id,job_id,owner,generation,acquired_at,expires_at,deadline,reservation_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(job_id) DO UPDATE SET owner=EXCLUDED.owner,generation=EXCLUDED.generation,acquired_at=EXCLUDED.acquired_at,expires_at=EXCLUDED.expires_at`, workspace, job, owner, l.Generation, l.AcquiredAt, l.ExpiresAt, l.Deadline, l.Reservation)
	if e != nil {
		return l, e
	}
	if e = fence(ctx, tx, l); e != nil {
		return l, e
	}
	if e = policies.BindLease(ctx, tx, workspace, l.Reservation, job, l.ExpiresAt); e != nil {
		return l, e
	}
	_, e = tx.Exec(ctx, "UPDATE media_jobs SET state='RUNNING' WHERE id=$1", job)
	slog.Info("job lease acquired", "state", "RUNNING", "reclaimed", l.Generation > 1)
	return l, e
}
func owned(ctx context.Context, tx pgx.Tx, l Lease) (string, time.Time, error) {
	var state, owner string
	var generation int64
	var expiry, deadline, now time.Time
	e := tx.QueryRow(ctx, "SELECT m.state,j.owner,j.generation,j.expires_at,j.deadline,clock_timestamp() FROM media_jobs m JOIN job_leases j ON j.job_id=m.id WHERE m.id=$1 AND m.workspace_id=$2 FOR UPDATE OF m,j", l.Job, l.Workspace).Scan(&state, &owner, &generation, &expiry, &deadline, &now)
	if e != nil {
		return state, deadline, e
	}
	if owner != l.Owner || generation != l.Generation {
		return state, deadline, database.ErrConflict
	}
	if state == "RUNNING" && !expiry.After(now) {
		return state, deadline, database.ErrConflict
	}
	return state, deadline, nil
}
func Renew(ctx context.Context, tx pgx.Tx, l Lease, r policies.Request, duration time.Duration) (Lease, error) {
	if duration <= 0 || duration > MaxLease || r.Workspace != l.Workspace {
		return l, database.ErrInvalid
	}
	state, deadline, e := owned(ctx, tx, l)
	if e != nil {
		return l, e
	}
	if state != "RUNNING" {
		return l, database.ErrConflict
	}
	var now time.Time
	if e = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); e != nil {
		return l, e
	}
	next := now.Add(duration)
	if next.After(deadline) || next.Before(l.ExpiresAt) {
		return l, database.ErrConflict
	}
	if e = fence(ctx, tx, l); e != nil {
		return l, e
	}
	if e = policies.RenewLease(ctx, tx, r, l.Reservation, l.Job, next); e != nil {
		return l, e
	}
	e = tx.QueryRow(ctx, "UPDATE job_leases SET expires_at=GREATEST(expires_at,$2) WHERE job_id=$1 RETURNING expires_at", l.Job, next).Scan(&l.ExpiresAt)
	slog.Info("job lease renewed", "state", "RUNNING")
	return l, e
}
func Finish(ctx context.Context, tx pgx.Tx, l Lease, state, correlation string) error {
	if state != "COMPLETED" && state != "FAILED" && state != "CANCELLED" {
		return database.ErrInvalid
	}
	previous, _, e := owned(ctx, tx, l)
	if e != nil {
		return e
	}
	if previous == state {
		return nil
	}
	if previous != "RUNNING" {
		return database.ErrConflict
	}
	if e = fence(ctx, tx, l); e != nil {
		return e
	}
	if e = policies.Settle(ctx, tx, l.Workspace, l.Reservation, state == "COMPLETED"); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "SELECT set_config('app.lease_owner',$1,true),set_config('app.lease_generation',$2,true)", l.Owner, fmtGeneration(l.Generation)); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "UPDATE media_jobs SET state=$2 WHERE id=$1", l.Job, state); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "UPDATE job_leases SET terminal_state=$2 WHERE job_id=$1", l.Job, state); e != nil {
		return e
	}
	slog.Info("job terminal transition", "state", state)
	return database.Event(ctx, tx, l.Workspace, "telmivo.events.media", "media.terminal", correlation, map[string]string{"job_id": l.Job, "state": state})
}

func fmtGeneration(v int64) string { return strconv.FormatInt(v, 10) }

func fence(ctx context.Context, tx pgx.Tx, l Lease) error {
	_, e := tx.Exec(ctx, "SELECT set_config('app.lease_owner',$1,true),set_config('app.lease_generation',$2,true)", l.Owner, fmtGeneration(l.Generation))
	return e
}
