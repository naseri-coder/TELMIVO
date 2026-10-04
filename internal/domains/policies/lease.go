package policies

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"time"
)

// The immutable reservation snapshot is preserved. A companion allocation holds
// approved effective expiry; its identity cannot move to another job/window.
func BindLease(ctx context.Context, tx pgx.Tx, workspace, key, job string, expiry time.Time) error {
	if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "quota:"+workspace); e != nil {
		return e
	}
	rs, e := tx.Query(ctx, `SELECT q.metric,COALESCE(a.expires_at,q.expires_at),q.created_at,p.definition FROM quota_reservations q JOIN policy_versions p ON p.key=q.policy_key AND p.version=q.policy_version LEFT JOIN quota_lease_allocations a ON a.workspace_id=q.workspace_id AND a.operation_key=q.operation_key AND a.metric=q.metric WHERE q.workspace_id=$1 AND q.operation_key=$2 AND q.state='RESERVED'`, workspace, key)
	if e != nil {
		return e
	}
	type allocation struct {
		metric  string
		expires time.Time
		created time.Time
		def     Definition
	}
	var all []allocation
	for rs.Next() {
		var a allocation
		var raw []byte
		if e = rs.Scan(&a.metric, &a.expires, &a.created, &raw); e != nil {
			rs.Close()
			return e
		}
		a.def, e = decodeDefinition(raw)
		if e != nil {
			rs.Close()
			return e
		}
		all = append(all, a)
	}
	e = rs.Err()
	rs.Close()
	if e != nil {
		return e
	}
	if len(all) == 0 {
		return database.ErrConflict
	}
	for _, a := range all {
		var end *time.Time
		if one(a.def.Window, "CALENDAR_DAY", "CALENDAR_WEEK", "CALENDAR_MONTH") {
			_, end, e = Bucket(a.def, a.created)
			if e != nil {
				return e
			}
		}
		if expiry.After(a.expires) {
			return database.ErrConflict
		}
		var prior string
		e = tx.QueryRow(ctx, "SELECT job_id::text FROM quota_lease_allocations WHERE workspace_id=$1 AND operation_key=$2 AND metric=$3", workspace, key, a.metric).Scan(&prior)
		if e == nil && prior != job {
			return database.ErrConflict
		}
		if e != nil && e != pgx.ErrNoRows {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO quota_lease_allocations(workspace_id,operation_key,metric,job_id,expires_at,window_end) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(workspace_id,operation_key,metric) DO NOTHING`, workspace, key, a.metric, job, a.expires, end)
		if e != nil {
			return e
		}
	}
	return nil
}
func RenewLease(ctx context.Context, tx pgx.Tx, r Request, key, job string, expiry time.Time) error {
	if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "quota:"+r.Workspace); e != nil {
		return e
	}
	var owned bool
	if e := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM job_leases WHERE job_id=$1 AND workspace_id=$2 AND owner=current_setting('app.lease_owner',true) AND generation::text=current_setting('app.lease_generation',true) AND expires_at>clock_timestamp() AND terminal_state IS NULL)", job, r.Workspace).Scan(&owned); e != nil {
		return e
	}
	if !owned {
		return database.ErrConflict
	}
	r.Requested = 0
	r.Amounts = nil
	r.RequireReservation = false
	r.Plan = ""
	d, e := Resolve(ctx, tx, r)
	if e != nil {
		return e
	}
	if !d.Allowed {
		return &Denied{d}
	}
	rs, e := tx.Query(ctx, `SELECT q.metric,q.amount,a.expires_at,a.window_end FROM quota_reservations q JOIN quota_lease_allocations a USING(workspace_id,operation_key,metric) WHERE q.workspace_id=$1 AND q.operation_key=$2 AND a.job_id=$3 AND q.state='RESERVED' FOR UPDATE OF a`, r.Workspace, key, job)
	if e != nil {
		return e
	}
	type item struct {
		metric   string
		amount   int64
		previous time.Time
		end      *time.Time
	}
	var all []item
	for rs.Next() {
		var x item
		if e = rs.Scan(&x.metric, &x.amount, &x.previous, &x.end); e != nil {
			rs.Close()
			return e
		}
		all = append(all, x)
	}
	e = rs.Err()
	rs.Close()
	if e != nil {
		return e
	}
	if len(all) == 0 {
		return database.ErrConflict
	}
	now := time.Now()
	for _, x := range all {
		v, ok := d.Resources[x.metric]
		if !ok || !x.previous.After(now) || (x.end != nil && expiry.After(*x.end)) {
			return database.ErrConflict
		}
		if x.amount > v.Limit || v.Used+v.Reserved > v.Limit {
			return database.ErrForbidden
		}
	}
	for _, x := range all {
		if expiry.After(x.previous) {
			if _, e = tx.Exec(ctx, "UPDATE quota_lease_allocations SET expires_at=$4 WHERE workspace_id=$1 AND operation_key=$2 AND metric=$3", r.Workspace, key, x.metric, expiry); e != nil {
				return e
			}
		}
	}
	return nil
}
