package downloads

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/jobs"
	"github.com/naseri-coder/TELMIVO/internal/domains/policies"
	"github.com/naseri-coder/TELMIVO/internal/integrations/media"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"log/slog"
	"strings"
	"time"
)

type Product struct {
	DB        *database.Store
	Synthetic bool
	MaxBytes  int64
}
type View struct {
	ID        string         `json:"id"`
	Provider  string         `json:"provider"`
	Title     string         `json:"title"`
	State     string         `json:"state"`
	Formats   []media.Format `json:"formats"`
	Selected  string         `json:"selected_format"`
	Error     string         `json:"failure_category"`
	Created   time.Time      `json:"submitted_at"`
	Completed *time.Time     `json:"completed_at"`
	Bytes     int64          `json:"size_bytes"`
	Delivery  string         `json:"delivery_outcome"`
	CanCancel bool           `json:"can_cancel"`
}

func request(ctx context.Context, tx pgx.Tx, u, w string) (policies.Request, error) {
	r := policies.Request{User: u, Workspace: w, Feature: "downloads", Permission: "downloads.create", Requested: 1}
	e := tx.QueryRow(ctx, "SELECT role_key FROM workspace_members WHERE workspace_id=$1 AND user_id=$2", w, u).Scan(&r.Role)
	return r, e
}
func (p *Product) Submit(ctx context.Context, tx pgx.Tx, u, w, corr, raw, key string) (string, error) {
	s, e := media.Detect(raw)
	if e != nil {
		return "", e
	}
	if len(key) < 8 || len(key) > 180 {
		return "", database.ErrInvalid
	}
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "download:"+w+":"+key); e != nil {
		return "", e
	}
	var id, source, owner string
	e = tx.QueryRow(ctx, "SELECT id::text,source_url,creator_id::text FROM download_jobs WHERE workspace_id=$1 AND idempotency_key=$2", w, key).Scan(&id, &source, &owner)
	if e == nil {
		if source != s.URL || owner != u {
			return "", database.ErrConflict
		}
		return id, nil
	}
	if e != pgx.ErrNoRows {
		return "", e
	}
	r, e := request(ctx, tx, u, w)
	if e != nil {
		return "", e
	}
	check := r
	check.Requested = 0
	decision, e := policies.Resolve(ctx, tx, check)
	if e != nil {
		return "", e
	}
	if !decision.Allowed {
		return "", &policies.Denied{Decision: decision}
	}
	for _, resource := range decision.Resources {
		if resource.Unit == "BYTES" {
			return "", database.ErrForbidden
		}
	}
	mj, e := jobs.Create(ctx, tx, r, key, "downloader", corr)
	if e != nil {
		return "", e
	}
	e = tx.QueryRow(ctx, "INSERT INTO download_jobs(workspace_id,creator_id,provider,source_url,format_id,formats,state,synthetic,idempotency_key,media_job_id) VALUES($1,$2,$3,$4,'','[]','QUEUED',$5,$6,$7) RETURNING id::text", w, u, s.Provider, s.URL, p.Synthetic, key, mj).Scan(&id)
	if e != nil {
		return "", e
	}
	e = database.Audit(ctx, tx, u, w, "download.submit", "download_job", id, corr, map[string]any{}, map[string]any{"provider": s.Provider})
	if e != nil {
		return "", e
	}
	e = database.Event(ctx, tx, w, "telmivo.jobs.media", "downloader.queued", corr, map[string]string{"user_id": u, "job_id": id})
	if e == nil {
		slog.Info("downloader diagnostic", "event", "downloader_job_created", "download_job_id", id, "media_job_id", mj, "coordinate_id", CoordinateDiagnosticID(u, w), "state", "QUEUED", "transaction_phase", "PENDING_COMMIT")
	}
	return id, e
}
func (p *Product) Detail(ctx context.Context, tx pgx.Tx, u, w, id string, lock bool) (View, error) {
	v := View{Formats: []media.Format{}}
	q := "SELECT id::text,provider,title,state,formats,format_id,COALESCE(error_code,''),created_at,CASE WHEN state IN('COMPLETED','FAILED','CANCELLED') THEN updated_at END,COALESCE((SELECT a.size_bytes FROM download_assets a WHERE a.download_job_id=download_jobs.id ORDER BY a.created_at DESC,a.id DESC LIMIT 1),0),delivery_outcome,cancel_requested FROM download_jobs WHERE id=$1 AND workspace_id=$2 AND creator_id=$3 AND media_job_id IS NOT NULL"
	if lock {
		q += " FOR UPDATE"
	}
	var raw []byte
	var cancel bool
	e := tx.QueryRow(ctx, q, id, w, u).Scan(&v.ID, &v.Provider, &v.Title, &v.State, &raw, &v.Selected, &v.Error, &v.Created, &v.Completed, &v.Bytes, &v.Delivery, &cancel)
	if e != nil {
		return v, e
	}
	if json.Unmarshal(raw, &v.Formats) != nil {
		return v, database.ErrInvalid
	}
	v.CanCancel = !cancel && v.State != "COMPLETED" && v.State != "FAILED" && v.State != "CANCELLED" && v.State != "UPLOADING"
	return v, nil
}
func (p *Product) Select(ctx context.Context, tx pgx.Tx, u, w, id, f string) error {
	v, e := p.Detail(ctx, tx, u, w, id, true)
	if e != nil {
		return e
	}
	if v.State != "AWAITING_FORMAT" {
		return database.ErrConflict
	}
	var fresh bool
	e = tx.QueryRow(ctx, "SELECT format_expires_at>now() AND NOT cancel_requested FROM download_jobs WHERE id=$1", id).Scan(&fresh)
	if e != nil {
		return e
	}
	if !fresh {
		return media.Unavailable
	}
	var selected *media.Format
	for i := range v.Formats {
		if v.Formats[i].ID == f {
			selected = &v.Formats[i]
		}
	}
	if selected == nil {
		return media.Unavailable
	}
	if p.MaxBytes <= 0 || selected.Bytes > p.MaxBytes {
		return media.TooLarge
	}
	r, e := request(ctx, tx, u, w)
	if e != nil {
		return e
	}
	r.Requested = 0
	r.RequireReservation = false
	d, e := policies.Resolve(ctx, tx, r)
	if e != nil {
		return e
	}
	if !d.Allowed {
		return &policies.Denied{Decision: d}
	}
	for _, resource := range d.Resources {
		if resource.Unit == "BYTES" && (selected.Bytes <= 0 || selected.Bytes > resource.Limit) {
			return database.ErrForbidden
		}
	}
	tag, e := tx.Exec(ctx, "UPDATE download_jobs SET format_id=$2,updated_at=now() WHERE id=$1 AND (format_id='' OR format_id=$2)", id, f)
	if e == nil && tag.RowsAffected() != 1 {
		return database.ErrConflict
	}
	return e
}
func (p *Product) Cancel(ctx context.Context, tx pgx.Tx, u, w, id, corr string) error {
	v, e := p.Detail(ctx, tx, u, w, id, true)
	if e != nil {
		return e
	}
	if v.State == "CANCELLED" {
		return nil
	}
	if !v.CanCancel {
		return database.ErrConflict
	}
	_, e = tx.Exec(ctx, "UPDATE download_jobs SET cancel_requested=true,updated_at=now() WHERE id=$1", id)
	if e != nil {
		return e
	}
	return database.Audit(ctx, tx, u, w, "download.cancel", "download_job", id, corr, map[string]any{}, map[string]any{"requested": true})
}
func (p *Product) History(ctx context.Context, tx pgx.Tx, u, w, cursor string) ([]View, string, error) {
	out := []View{}
	at := time.Now().Add(time.Hour)
	id := "ffffffff-ffff-ffff-ffff-ffffffffffff"
	if cursor != "" {
		b, e := base64.RawURLEncoding.DecodeString(cursor)
		if e != nil {
			return out, "", database.ErrInvalid
		}
		parts := strings.Split(string(b), "|")
		if len(parts) != 2 {
			return out, "", database.ErrInvalid
		}
		at, e = time.Parse(time.RFC3339Nano, parts[0])
		if e != nil {
			return out, "", database.ErrInvalid
		}
		id = parts[1]
	}
	rows, e := tx.Query(ctx, "SELECT id::text FROM download_jobs WHERE workspace_id=$1 AND creator_id=$2 AND media_job_id IS NOT NULL AND (created_at,id)<($3,$4::uuid) ORDER BY created_at DESC,id DESC LIMIT 21", w, u, at, id)
	if e != nil {
		return out, "", e
	}
	var ids []string
	for rows.Next() {
		var s string
		if e = rows.Scan(&s); e != nil {
			rows.Close()
			return out, "", e
		}
		ids = append(ids, s)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, "", e
	}
	next := ""
	if len(ids) > 20 {
		ids = ids[:20]
		next = "pending"
	}
	for _, id := range ids {
		v, e := p.Detail(ctx, tx, u, w, id, false)
		if e != nil {
			return out, "", e
		}
		out = append(out, v)
	}
	if next != "" {
		v := out[len(out)-1]
		next = base64.RawURLEncoding.EncodeToString([]byte(v.Created.Format(time.RFC3339Nano) + "|" + v.ID))
	}
	return out, next, nil
}

// Dispatch accepts an existing authenticated identity, checks membership through
// Tenant, and returns only that customer's eligible jobs in the chosen workspace.
func (p *Product) Dispatch(ctx context.Context, u, ws string) ([]string, error) {
	ids := []string{}
	e := p.DB.Tenant(ctx, u, ws, false, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT d.id::text FROM download_jobs d LEFT JOIN job_leases j ON j.job_id=d.media_job_id
 WHERE d.workspace_id=$1 AND d.creator_id=$2 AND d.media_job_id IS NOT NULL AND d.state NOT IN('COMPLETED','FAILED','CANCELLED')
 AND (d.state<>'AWAITING_FORMAT' OR d.format_id<>'' OR d.cancel_requested OR d.format_expires_at<=clock_timestamp())
 AND (j.job_id IS NULL OR j.expires_at<=clock_timestamp()) ORDER BY d.created_at,d.id LIMIT 8`, ws, u)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				return e
			}
			ids = append(ids, id)
		}
		return rows.Err()
	})
	return ids, e
}
