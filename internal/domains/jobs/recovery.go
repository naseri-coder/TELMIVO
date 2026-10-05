package jobs

import (
	"context"
	"github.com/jackc/pgx/v5"
	"log/slog"
)

// Reconcile emits wakeups from authoritative job state after broker loss or a
// crashed worker. Call in an authorized workspace transaction; no queue owns state.
func Reconcile(ctx context.Context, tx pgx.Tx, workspace, correlation string) (int, error) {
	rows, e := tx.Query(ctx, `SELECT m.id::text,COALESCE(j.generation,0) FROM media_jobs m LEFT JOIN job_leases j ON j.job_id=m.id WHERE m.workspace_id=$1 AND m.state IN('QUEUED','RUNNING') AND (j.job_id IS NULL OR (j.expires_at<=clock_timestamp() AND j.deadline>clock_timestamp())) ORDER BY m.created_at LIMIT 100 FOR UPDATE OF m SKIP LOCKED`, workspace)
	if e != nil {
		return 0, e
	}
	type abandoned struct {
		id         string
		generation int64
	}
	var batch []abandoned
	for rows.Next() {
		var x abandoned
		if e = rows.Scan(&x.id, &x.generation); e != nil {
			rows.Close()
			return 0, e
		}
		batch = append(batch, x)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return 0, e
	}
	n := 0
	for _, x := range batch {
		var exists bool
		if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM job_recovery_events WHERE job_id=$1 AND generation=$2)", x.id, x.generation).Scan(&exists); e != nil {
			return n, e
		}
		if exists {
			continue
		}
		var event string
		e = tx.QueryRow(ctx, "INSERT INTO outbox_events(workspace_id,subject,event_type,correlation_id,payload) VALUES($1,'telmivo.jobs.media','media.recover',$2,jsonb_build_object('job_id',$3::text)) RETURNING id::text", workspace, correlation, x.id).Scan(&event)
		if e != nil {
			return n, e
		}
		if _, e = tx.Exec(ctx, "INSERT INTO job_recovery_events(workspace_id,job_id,generation,event_id) VALUES($1,$2,$3,$4)", workspace, x.id, x.generation, event); e != nil {
			return n, e
		}
		n++
	}
	slog.Info("job recovery reconciliation", "scheduled", n)
	return n, nil
}
