package policies

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
)

type Denied struct{ Decision Decision }

func (e *Denied) Error() string { return e.Decision.Reason }
func Reserve(ctx context.Context, tx pgx.Tx, r Request, key string) (Decision, error) {
	if len(key) < 8 || len(key) > 200 || r.Plan != "" {
		return Decision{}, database.ErrInvalid
	}
	if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "quota:"+r.Workspace); e != nil {
		return Decision{}, e
	}
	fp := fingerprint(struct {
		User, Feature, Plugin, Capability string
		Requested                         int64
		Amounts                           map[string]int64
	}{r.User, r.Feature, r.Plugin, r.Capability, r.Requested, r.Amounts})
	var previous string
	var expired bool
	e := tx.QueryRow(ctx, "SELECT fingerprint, state='RELEASED' OR (state='RESERVED' AND expires_at<=now()) FROM quota_reservations WHERE workspace_id=$1 AND operation_key=$2 LIMIT 1", r.Workspace, key).Scan(&previous, &expired)
	if e == nil {
		if previous != fp || expired {
			return Decision{}, database.ErrConflict
		}
		return Decision{Allowed: true, Reason: "REPLAY"}, nil
	}
	if e != pgx.ErrNoRows {
		return Decision{}, e
	}
	r.RequireReservation = true
	d, e := Resolve(ctx, tx, r)
	if e != nil {
		return d, e
	}
	if !d.Allowed {
		return d, &Denied{d}
	}
	if r.Now.IsZero() {
		r.Now = time.Now()
	}
	for _, metric := range sortedKeys(d.Resources) {
		x := d.Resources[metric]
		amount := r.Amounts[metric]
		if x.Mode == "CONCURRENT" && r.Requested > 0 {
			amount = max(amount, 1)
		}
		if x.Primary && r.Requested > 0 {
			amount = max(amount, r.Requested)
		}
		if x.Mode == "ITEM" || amount == 0 {
			continue
		}
		expiry := r.Now.Add(time.Duration(x.TTL) * time.Second)
		if x.ResetAt != nil && x.ResetAt.Before(expiry) {
			expiry = *x.ResetAt
		}
		_, e = tx.Exec(ctx, `INSERT INTO quota_reservations(workspace_id,operation_key,metric,fingerprint,amount,bucket,policy_key,policy_version,state,concurrent,count_failure,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'RESERVED',$9,$10,$11)`, r.Workspace, key, metric, fp, amount, x.Bucket, x.PolicyKey, x.PolicyVersion, x.Mode == "CONCURRENT", x.CountFailure, expiry)
		if e != nil {
			return d, e
		}
	}
	return d, nil
}

// Terminal settlement is replay-safe. Failures count only when the recorded policy explicitly requests it.
func Settle(ctx context.Context, tx pgx.Tx, workspace, key string, success bool) error {
	if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "quota:"+workspace); e != nil {
		return e
	}
	rs, e := tx.Query(ctx, "SELECT metric,amount,bucket,concurrent,count_failure,expires_at FROM quota_reservations WHERE workspace_id=$1 AND operation_key=$2 AND state='RESERVED' FOR UPDATE", workspace, key)
	if e != nil {
		return e
	}
	type posting struct {
		metric, bucket    string
		amount            int64
		concurrent, count bool
		expires           time.Time
	}
	ps := []posting{}
	for rs.Next() {
		var p posting
		if e = rs.Scan(&p.metric, &p.amount, &p.bucket, &p.concurrent, &p.count, &p.expires); e != nil {
			rs.Close()
			return e
		}
		ps = append(ps, p)
	}
	e = rs.Err()
	rs.Close()
	if e != nil {
		return e
	}
	for _, p := range ps {
		if success && !time.Now().Before(p.expires) {
			return database.ErrConflict
		}
		committed := (success || p.count) && time.Now().Before(p.expires) && !p.concurrent
		state := "RELEASED"
		if committed {
			state = "COMMITTED"
			_, e = tx.Exec(ctx, "INSERT INTO quota_usage(workspace_id,metric,bucket,used) VALUES($1,$2,$3,$4) ON CONFLICT(workspace_id,metric,bucket) DO UPDATE SET used=quota_usage.used+EXCLUDED.used,updated_at=now()", workspace, p.metric, p.bucket, p.amount)
			if e != nil {
				return e
			}
		}
		if _, e = tx.Exec(ctx, "UPDATE quota_reservations SET state=$4,settled_at=now() WHERE workspace_id=$1 AND operation_key=$2 AND metric=$3", workspace, key, p.metric, state); e != nil {
			return e
		}
	}
	return nil
}
