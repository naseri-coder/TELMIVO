package downloads

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Cleanup checks durable ownership under the same download-row lock acquired by
// RunOne. Age never authorizes deletion of a valid generation. Unknown paths are
// retained for operator review rather than risking another tenant's artifact.
func (w *Worker) Cleanup(ctx context.Context) error {
	entries, e := os.ReadDir(w.Root)
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	if e != nil {
		return e
	}
	for i, entry := range entries {
		if i >= 1024 {
			break
		}
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		var u, ws string
		e = w.Product.DB.Pool.QueryRow(ctx, "SELECT payload->>'user_id',workspace_id::text FROM outbox_events WHERE event_type='downloader.queued' AND payload->>'job_id'=$1 ORDER BY created_at LIMIT 1", id).Scan(&u, &ws)
		if errors.Is(e, pgx.ErrNoRows) {
			continue
		}
		if e != nil {
			return e
		}
		e = w.Product.DB.Tenant(ctx, u, ws, false, func(tx pgx.Tx) error {
			var state string
			var generation int64
			var active bool
			e := tx.QueryRow(ctx, `SELECT d.state,COALESCE(j.generation,0),COALESCE(j.expires_at>clock_timestamp() AND j.terminal_state IS NULL,false)
 FROM download_jobs d LEFT JOIN job_leases j ON j.job_id=d.media_job_id WHERE d.id=$1 AND d.creator_id=$2 FOR UPDATE OF d`, id, u).Scan(&state, &generation, &active)
			if e != nil {
				return e
			}
			dir := filepath.Join(w.Root, id)
			children, e := os.ReadDir(dir)
			if e != nil {
				return e
			}
			terminal := state == "COMPLETED" || state == "FAILED" || state == "CANCELLED"
			for _, child := range children {
				n, e := strconv.ParseInt(child.Name(), 10, 64)
				if e != nil || n < 1 || !child.IsDir() {
					continue
				}
				if !terminal && (n > generation || n == generation && active) {
					continue
				}
				if e = os.RemoveAll(filepath.Join(dir, child.Name())); e != nil {
					return e
				}
			}
			if terminal {
				if rest, e := os.ReadDir(dir); e == nil && len(rest) == 0 {
					return os.Remove(dir)
				}
			}
			return nil
		})
		if e != nil {
			return e
		}
	}
	return nil
}

func cleanupDue(last time.Time) bool { return time.Since(last) > time.Minute }
