package downloads

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/jobs"
	"github.com/naseri-coder/TELMIVO/internal/integrations/media"

	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/storage"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

type Worker struct {
	Product        *Product
	Adapter        media.Adapter
	Delivery       media.Delivery
	Storage        storage.Scoped
	Root, Owner    string
	Timeout        time.Duration
	MaxBytes       int64
	workspaceAfter string
}
type work struct {
	User, Workspace, ID, MediaID, State, Key, Selected, URL string
	Cancel                                                  bool
	Formats                                                 []media.Format
	Lease                                                   jobs.Lease
}

func (w *Worker) tx(ctx context.Context, x work, fn func(pgx.Tx) error) error {
	return w.Product.DB.Tenant(ctx, x.User, x.Workspace, false, fn)
}
func fence(ctx context.Context, tx pgx.Tx, l jobs.Lease) error {
	var owned bool
	e := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM job_leases WHERE job_id=$1 AND workspace_id=$2 AND owner=$3 AND generation=$4 AND expires_at>clock_timestamp() AND terminal_state IS NULL)", l.Job, l.Workspace, l.Owner, l.Generation).Scan(&owned)
	if e != nil {
		return e
	}
	if !owned {
		return database.ErrConflict
	}
	_, e = tx.Exec(ctx, "SELECT set_config('app.lease_owner',$1,true),set_config('app.lease_generation',$2,true)", l.Owner, strconv.FormatInt(l.Generation, 10))
	return e
}
func (w *Worker) finish(ctx context.Context, x work, state, code, outcome string, messageID int64) error {
	return w.tx(ctx, x, func(tx pgx.Tx) error {
		var current string
		var cancel bool
		if e := tx.QueryRow(ctx, "SELECT state,cancel_requested FROM download_jobs WHERE id=$1 FOR UPDATE", x.ID).Scan(&current, &cancel); e != nil {
			return e
		}
		if e := fence(ctx, tx, x.Lease); e != nil {
			return e
		}
		if cancel && current != "UPLOADING" {
			state, code = "CANCELLED", ""
		}
		if _, e := tx.Exec(ctx, "UPDATE download_jobs SET state=$2,error_code=NULLIF($3,''),delivery_outcome=$4,delivery_message_id=NULLIF($5,0),updated_at=now() WHERE id=$1", x.ID, state, code, outcome, messageID); e != nil {
			return e
		}
		body := "وضعیت دانلود: " + state
		if code != "" {
			body += "\n" + PersianError(code)
		}
		x.State = state
		if e := w.notify(ctx, tx, x, body, nil); e != nil {
			return e
		}
		return jobs.Finish(ctx, tx, x.Lease, state, "downloader-worker")
	})
}
func (w *Worker) step(ctx context.Context, x *work, next string) error {
	return w.tx(ctx, *x, func(tx pgx.Tx) error {
		var cancel bool
		if e := tx.QueryRow(ctx, "SELECT cancel_requested FROM download_jobs WHERE id=$1 FOR UPDATE", x.ID).Scan(&cancel); e != nil {
			return e
		}
		if cancel {
			return context.Canceled
		}
		if e := fence(ctx, tx, x.Lease); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, "UPDATE download_jobs SET state=$2,updated_at=now() WHERE id=$1", x.ID, next)
		if e == nil {
			e = database.Audit(ctx, tx, x.User, x.Workspace, "download.state", "download_job", x.ID, "downloader-worker", map[string]any{"state": x.State}, map[string]any{"state": next})
			x.State = next
		}
		return e
	})
}
func (w *Worker) RunOne(parent context.Context, u, workspace, id string) error {
	x := work{User: u, Workspace: workspace, ID: id}
	if w.Timeout <= 0 || w.Timeout > 10*time.Minute || w.MaxBytes <= 0 || w.Owner == "" {
		return database.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, w.Timeout)
	defer cancel()
	e := w.tx(ctx, x, func(tx pgx.Tx) error {
		var raw []byte
		if e := tx.QueryRow(ctx, "SELECT media_job_id::text,state,idempotency_key,format_id,source_url,cancel_requested,formats FROM download_jobs WHERE id=$1 AND creator_id=$2 FOR UPDATE", id, u).Scan(&x.MediaID, &x.State, &x.Key, &x.Selected, &x.URL, &x.Cancel, &raw); e != nil {
			return e
		}
		if x.MediaID == "" {
			return database.ErrInvalid
		}
		if json.Unmarshal(raw, &x.Formats) != nil {
			return database.ErrInvalid
		}
		var e error
		var expired bool
		if e = tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM quota_reservations q LEFT JOIN quota_lease_allocations a USING(workspace_id,operation_key,metric) WHERE q.workspace_id=$1 AND q.operation_key=$2 AND q.state='RESERVED' AND COALESCE(a.expires_at,q.expires_at)>clock_timestamp())`, workspace, x.Key).Scan(&expired); e != nil {
			return e
		}
		if expired {
			x.Lease, e = jobs.AcquireExpiredSettlement(ctx, tx, workspace, x.MediaID, w.Owner, time.Minute)
			x.State = "EXPIRED_RESERVATION"
			return e
		}
		x.Lease, e = jobs.Acquire(ctx, tx, workspace, x.MediaID, w.Owner, time.Minute)
		return e
	})
	if e != nil {
		return e
	}
	// Heartbeat and durable cancel observation are independent of external work.
	heartbeatWork := x
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				e := w.tx(ctx, heartbeatWork, func(tx pgx.Tx) error {
					var cancelled bool
					if e := tx.QueryRow(ctx, "SELECT cancel_requested FROM download_jobs WHERE id=$1", id).Scan(&cancelled); e != nil {
						return e
					}
					if cancelled {
						return context.Canceled
					}
					r, e := request(ctx, tx, u, workspace)
					if e != nil {
						return e
					}
					_, e = jobs.Renew(ctx, tx, heartbeatWork.Lease, r, time.Minute)
					return e
				})
				if e != nil {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-heartbeatDone }()
	finalize := func(state, code, outcome string, mid int64) error {
		final, stop := context.WithTimeout(context.WithoutCancel(parent), 10*time.Second)
		defer stop()
		return w.finish(final, x, state, code, outcome, mid)
	}
	if x.State == "EXPIRED_RESERVATION" {
		return finalize("FAILED", "QUOTA_RESERVATION_EXPIRED", "DELIVERY_FAILED", 0)
	}
	if x.State == "UPLOADING" {
		return finalize("FAILED", "DELIVERY_OUTCOME_UNKNOWN", "DELIVERY_OUTCOME_UNKNOWN", 0)
	}
	if x.Cancel {
		return finalize("CANCELLED", "", "DELIVERY_PENDING", 0)
	}
	s, e := media.Detect(x.URL)
	if e != nil {
		return finalize("FAILED", "INVALID_URL", "DELIVERY_FAILED", 0)
	}
	if x.State == "QUEUED" || x.State == "ANALYZING" {
		if x.State == "QUEUED" {
			if e = w.step(ctx, &x, "ANALYZING"); e != nil {
				return finalize("FAILED", "ANALYSIS_FAILED", "DELIVERY_FAILED", 0)
			}
		}
		a, err := w.Adapter.Analyze(ctx, s)
		for attempt := 0; attempt < 2 && retryableProvider(err); attempt++ {
			timer := time.NewTimer(time.Duration(attempt+1) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				err = ctx.Err()
			case <-timer.C:
				a, err = w.Adapter.Analyze(ctx, s)
			}
		}

		if err != nil {
			return finalize("FAILED", category(err, "ANALYSIS_FAILED"), "DELIVERY_FAILED", 0)
		}
		if len(a.Formats) == 0 || len(a.Formats) > 12 {
			return finalize("FAILED", "FORMAT_UNAVAILABLE", "DELIVERY_FAILED", 0)
		}
		e = w.tx(ctx, x, func(tx pgx.Tx) error {
			var cancelled bool
			if e := tx.QueryRow(ctx, "SELECT cancel_requested FROM download_jobs WHERE id=$1 FOR UPDATE", id).Scan(&cancelled); e != nil {
				return e
			}
			if cancelled {
				return context.Canceled
			}
			if e := fence(ctx, tx, x.Lease); e != nil {
				return e
			}
			raw, e := json.Marshal(a.Formats)
			if e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, "UPDATE download_jobs SET state='AWAITING_FORMAT',formats=$2,title=$3,format_expires_at=now()+interval '5 minutes',updated_at=now() WHERE id=$1", id, raw, media.SafeTitle(a.Title)); e != nil {
				return e
			}
			buttons := [][]map[string]any{}
			for _, f := range a.Formats {
				data := "f:" + id + ":" + f.ID
				if len(data) > 64 {
					return database.ErrInvalid
				}
				buttons = append(buttons, []map[string]any{{"text": f.Label, "callback_data": data}})
			}
			if e := w.notify(ctx, tx, x, "فرمت را انتخاب کنید", map[string]any{"inline_keyboard": buttons}); e != nil {
				return e
			}
			_, e = tx.Exec(ctx, "UPDATE job_leases SET expires_at=clock_timestamp() WHERE job_id=$1", x.MediaID)
			return e
		})
		if errors.Is(e, context.Canceled) {
			return finalize("CANCELLED", "", "DELIVERY_PENDING", 0)
		}
		return e
	}
	if x.State == "AWAITING_FORMAT" && x.Selected == "" {
		return finalize("FAILED", "FORMAT_UNAVAILABLE", "DELIVERY_FAILED", 0)
	}
	var selected media.Format
	for _, f := range x.Formats {
		if f.ID == x.Selected {
			selected = f
		}
	}
	if selected.ID == "" {
		return finalize("FAILED", "FORMAT_UNAVAILABLE", "DELIVERY_FAILED", 0)
	}
	if x.State == "AWAITING_FORMAT" {
		if e = w.step(ctx, &x, "DOWNLOADING"); e != nil {
			return finalize("FAILED", "DOWNLOAD_FAILED", "DELIVERY_FAILED", 0)
		}
	}
	// Only trusted UUID and numeric generation derive paths; human metadata never does.
	dir := filepath.Join(w.Root, x.ID, strconv.FormatInt(x.Lease.Generation, 10))
	if e = os.MkdirAll(dir, 0700); e != nil {
		return finalize("FAILED", "TEMP_STORAGE_FAILED", "DELIVERY_FAILED", 0)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "asset")
	file, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return finalize("FAILED", "TEMP_STORAGE_FAILED", "DELIVERY_FAILED", 0)
	}
	e = w.Adapter.Fetch(ctx, s, selected, file, w.MaxBytes)
	closeErr := file.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		return finalize("FAILED", category(e, "DOWNLOAD_FAILED"), "DELIVERY_FAILED", 0)
	}
	asset, e := media.Validate(ctx, path, w.MaxBytes, selected)
	if e != nil {
		return finalize("FAILED", category(e, "INVALID_MEDIA"), "DELIVERY_FAILED", 0)
	}
	if selected.Processing {
		if e = w.step(ctx, &x, "PROCESSING"); e != nil {
			return finalize("FAILED", "PROCESSING_FAILED", "DELIVERY_FAILED", 0)
		}
		path, e = media.Remux(ctx, path, filepath.Join(dir, "processed"), w.MaxBytes, selected)
		if e != nil {
			return finalize("FAILED", category(e, "PROCESSING_FAILED"), "DELIVERY_FAILED", 0)
		}
		outputFormat := selected
		outputFormat.Processing = false
		outputFormat.SourceContainer = ""
		asset, e = media.Validate(ctx, path, w.MaxBytes, outputFormat)
		if e != nil {
			return finalize("FAILED", category(e, "INVALID_MEDIA"), "DELIVERY_FAILED", 0)
		}
	}
	key, e := storage.ObjectKey(workspace, x.ID+":"+strconv.FormatInt(x.Lease.Generation, 10))
	if e != nil {
		return finalize("FAILED", "TEMP_STORAGE_FAILED", "DELIVERY_FAILED", 0)
	}
	f, e := os.Open(path)
	if e != nil {
		return finalize("FAILED", "TEMP_STORAGE_FAILED", "DELIVERY_FAILED", 0)
	}
	e = w.Storage.Put(ctx, workspace, key, f, storage.Metadata{Size: asset.Bytes, ContentType: asset.ContentType, ExpiresAt: time.Now().Add(w.Timeout)})
	f.Close()
	if e != nil {
		return finalize("FAILED", "TEMP_STORAGE_FAILED", "DELIVERY_FAILED", 0)
	}
	defer func() {
		clean, stop := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
		defer stop()
		w.Storage.Delete(clean, workspace, key)
	}()
	var chat int64
	e = w.tx(ctx, x, func(tx pgx.Tx) error {
		var cancelled bool
		if e := tx.QueryRow(ctx, "SELECT cancel_requested FROM download_jobs WHERE id=$1 FOR UPDATE", id).Scan(&cancelled); e != nil {
			return e
		}
		if cancelled {
			return context.Canceled
		}
		if e := fence(ctx, tx, x.Lease); e != nil {
			return e
		}
		if e := tx.QueryRow(ctx, "SELECT telegram_id FROM telegram_accounts WHERE user_id=$1", u).Scan(&chat); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, "INSERT INTO download_assets(workspace_id,download_job_id,storage_key,size_bytes) VALUES($1,$2,$3,$4)", workspace, id, key, asset.Bytes); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, "UPDATE download_jobs SET state='UPLOADING',updated_at=now() WHERE id=$1", id)
		return e
	})
	if e != nil {
		return finalize("FAILED", "DOWNLOAD_CANCELLED", "DELIVERY_FAILED", 0)
	}
	// UPLOADING is the durable point of no cancellation. A crash after this marker
	// becomes unknown on reclaim and cannot blindly repeat an external upload.
	result, err := w.Delivery.DeliverMedia(ctx, chat, asset)
	if err != nil {
		return finalize("FAILED", category(err, "DELIVERY_FAILED"), "DELIVERY_FAILED", 0)
	}
	if result.Outcome == "DELIVERY_ACKNOWLEDGED" && result.MessageID > 0 {
		return finalize("COMPLETED", "", "DELIVERY_ACKNOWLEDGED", result.MessageID)
	}
	if result.Retry {
		for n := 0; n < 2; n++ {
			timer := time.NewTimer(time.Duration(n+1) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return finalize("FAILED", "DELIVERY_FAILED", "DELIVERY_FAILED", 0)
			case <-timer.C:
			}
			result, err = w.Delivery.DeliverMedia(ctx, chat, asset)
			if err != nil || !result.Retry {
				break
			}
		}
		if result.Outcome == "DELIVERY_ACKNOWLEDGED" && result.MessageID > 0 {
			return finalize("COMPLETED", "", "DELIVERY_ACKNOWLEDGED", result.MessageID)
		}
	}
	if result.Outcome == "DELIVERY_OUTCOME_UNKNOWN" {
		return finalize("FAILED", "DELIVERY_OUTCOME_UNKNOWN", result.Outcome, 0)
	}
	return finalize("FAILED", "DELIVERY_FAILED", "DELIVERY_FAILED", 0)
}
func category(e error, fallback string) string {
	var f media.Failure
	if errors.As(e, &f) {
		return string(f)
	}
	if errors.Is(e, context.Canceled) {
		return "DOWNLOAD_CANCELLED"
	}
	return fallback
}
func (w *Worker) Poll(ctx context.Context, concurrency int) error {
	if concurrency < 1 || concurrency > 32 {
		return database.ErrInvalid
	}
	if e := w.Cleanup(ctx); e != nil {
		return e
	}
	lastCleanup := time.Now()
	var running sync.WaitGroup
	defer running.Wait()
	slots := make(chan struct{}, concurrency)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if cleanupDue(lastCleanup) {
				if e := w.Cleanup(ctx); e != nil {
					return e
				}
				lastCleanup = time.Now()
			}
			all, e := w.targets(ctx)
			if e != nil {
				return e
			}
			for _, t := range all {
				select {
				case slots <- struct{}{}:
					running.Add(1)
					go func(t target) {
						defer running.Done()
						defer func() { <-slots }()
						if e := w.RunOne(ctx, t.u, t.ws, t.id); e != nil && !errors.Is(e, database.ErrConflict) {
							slog.Warn("downloader work failed", "category", category(e, "WORKER_FAILURE"))
						}
					}(t)
				default:
				}
			}
		}
	}
}

func (w *Worker) notify(ctx context.Context, tx pgx.Tx, x work, body string, markup any) error {
	return database.Event(ctx, tx, x.Workspace, "telmivo.jobs.notification", "bot.menu", "downloader-worker", map[string]any{"user_id": x.User, "body": body, "markup": markup, "idempotency_key": "downloader-bot:" + x.ID + ":" + x.State})
}

func retryableProvider(e error) bool {
	var f media.Failure
	if !errors.As(e, &f) {
		return false
	}
	return f == media.Failure("PROVIDER_RATE_LIMITED") || f == media.Failure("PROVIDER_TEMPORARY_FAILURE")
}

type target struct{ u, ws, id string }

// Trusted internal outbox coordinates identify the submitter; each read then
// revalidates membership and ownership. No privileged SQL function is needed.
func (w *Worker) targets(ctx context.Context) ([]target, error) {
	rows, e := w.Product.DB.Pool.Query(ctx, `SELECT DISTINCT workspace_id::text,payload->>'user_id' FROM outbox_events
 WHERE event_type='downloader.queued' AND workspace_id IS NOT NULL AND payload->>'user_id' IS NOT NULL
 AND workspace_id::text || (payload->>'user_id') > $1 ORDER BY 1,2 LIMIT 128`, w.workspaceAfter)
	if e != nil {
		return nil, e
	}
	var customers []target
	for rows.Next() {
		var t target
		if e = rows.Scan(&t.ws, &t.u); e != nil {
			rows.Close()
			return nil, e
		}
		customers = append(customers, t)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	if len(customers) == 0 {
		w.workspaceAfter = ""
		return nil, nil
	}
	last := customers[len(customers)-1]
	w.workspaceAfter = last.ws + last.u
	var all []target
	for _, t := range customers {
		ids, e := w.Product.Dispatch(ctx, t.u, t.ws)
		if errors.Is(e, database.ErrForbidden) {
			continue
		}
		if e != nil {
			return nil, e
		}
		for _, id := range ids {
			all = append(all, target{u: t.u, ws: t.ws, id: id})
		}
	}
	return all, nil
}
