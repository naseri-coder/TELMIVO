package uploads

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/entitlements"
	"github.com/naseri-coder/TELMIVO/internal/domains/policies"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/storage"
)

const (
	featureKey = "uploader"
	pluginKey  = "uploader"
)

var contentTypePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.+-]{0,63}/[a-z0-9][a-z0-9.+-]{0,127}$`)

type Service struct {
	DB      *database.Store
	Storage storage.Scoped
	Now     func() time.Time
}

type Workflow struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	ApprovalRequired bool      `json:"approval_required"`
	AllowedTypes     []string  `json:"allowed_types"`
	MaxBytes         int64     `json:"max_bytes"`
	RetentionDays    int       `json:"retention_days"`
	Destination      string    `json:"destination"`
	Enabled          bool      `json:"enabled"`
	Revision         int64     `json:"revision"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type WorkflowChange struct {
	Name             string   `json:"name"`
	ApprovalRequired bool     `json:"approval_required"`
	AllowedTypes     []string `json:"allowed_types"`
	MaxBytes         int64    `json:"max_bytes"`
	RetentionDays    int      `json:"retention_days"`
	Destination      string   `json:"destination"`
	Enabled          bool     `json:"enabled"`
	Key              string   `json:"idempotency_key"`
	ExpectedRevision int64    `json:"expected_revision"`
}

type Upload struct {
	ID            string    `json:"id"`
	WorkflowID    string    `json:"workflow_id"`
	CustomerBotID string    `json:"customer_bot_id,omitempty"`
	State         string    `json:"state"`
	Name          string    `json:"name"`
	ContentType   string    `json:"content_type"`
	SizeBytes     int64     `json:"size_bytes"`
	SHA256        string    `json:"sha256,omitempty"`
	ExpiresAt     time.Time `json:"expires_at"`
	ErrorCode     string    `json:"error_code,omitempty"`
	Revision      int64     `json:"revision"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Create struct {
	WorkflowID    string `json:"workflow_id"`
	CustomerBotID string `json:"customer_bot_id,omitempty"`
	Name          string `json:"name"`
	ContentType   string `json:"content_type"`
	SizeBytes     int64  `json:"size_bytes"`
	Key           string `json:"idempotency_key"`
}

type Action struct {
	Action           string `json:"action"`
	ExpectedRevision int64  `json:"expected_revision"`
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func operationKey(v string) bool {
	return len(v) >= 8 && len(v) <= 190 && strings.TrimSpace(v) == v
}

func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func validTypes(in []string) ([]string, bool) {
	if len(in) == 0 || len(in) > 32 {
		return nil, false
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, kind := range in {
		kind = strings.ToLower(strings.TrimSpace(kind))
		if !contentTypePattern.MatchString(kind) || seen[kind] {
			return nil, false
		}
		seen[kind] = true
		out = append(out, kind)
	}
	return out, true
}

func (s *Service) workflow(ctx context.Context, tx pgx.Tx, workspace, id string) (Workflow, error) {
	var out Workflow
	var raw []byte
	e := tx.QueryRow(ctx, `SELECT id::text,name,approval_required,allowed_types,max_bytes,retention_days,destination,enabled,revision,created_at,updated_at
		FROM upload_workflows WHERE workspace_id=$1 AND id::text=$2`, workspace, id).
		Scan(&out.ID, &out.Name, &out.ApprovalRequired, &raw, &out.MaxBytes, &out.RetentionDays, &out.Destination, &out.Enabled, &out.Revision, &out.CreatedAt, &out.UpdatedAt)
	if e == nil {
		e = json.Unmarshal(raw, &out.AllowedTypes)
	}
	return out, e
}

func (s *Service) upload(ctx context.Context, tx pgx.Tx, workspace, id string, lock bool) (Upload, string, error) {
	var out Upload
	var key string
	q := `SELECT id::text,COALESCE(workflow_id::text,''),COALESCE(customer_bot_id::text,''),state,
		COALESCE(original_name,''),COALESCE(content_type,''),COALESCE(size_bytes,0),COALESCE(storage_key,''),
		COALESCE(sha256,''),COALESCE(expires_at,created_at),COALESCE(error_code,''),revision,created_at,updated_at
		FROM uploads WHERE workspace_id=$1 AND id::text=$2`
	if lock {
		q += " FOR UPDATE"
	}
	e := tx.QueryRow(ctx, q, workspace, id).Scan(&out.ID, &out.WorkflowID, &out.CustomerBotID, &out.State, &out.Name, &out.ContentType, &out.SizeBytes, &key, &out.SHA256, &out.ExpiresAt, &out.ErrorCode, &out.Revision, &out.CreatedAt, &out.UpdatedAt)
	return out, key, e
}

func (s *Service) role(ctx context.Context, tx pgx.Tx, actor, workspace string) (string, error) {
	return entitlements.Role(ctx, tx, actor, workspace)
}

func (s *Service) decision(ctx context.Context, tx pgx.Tx, actor, workspace, capability string, amounts map[string]int64) (policies.Decision, policies.Request, error) {
	role, e := s.role(ctx, tx, actor, workspace)
	if e != nil {
		return policies.Decision{}, policies.Request{}, e
	}
	r := policies.Request{
		User: actor, Workspace: workspace, Role: role, Feature: featureKey, Plugin: pluginKey,
		Permission: "uploads.create", Capability: capability, Amounts: amounts, Now: s.now(),
	}
	d, e := policies.Resolve(ctx, tx, r)
	if e != nil {
		return d, r, e
	}
	if !d.Allowed {
		return d, r, &policies.Denied{Decision: d}
	}
	return d, r, nil
}

func (s *Service) Workflows(ctx context.Context, actor, workspace string) ([]Workflow, error) {
	out := []Workflow{}
	e := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		if !database.Permission(ctx, tx, actor, workspace, "uploads.read") {
			return database.ErrForbidden
		}
		rs, e := tx.Query(ctx, "SELECT id::text FROM upload_workflows WHERE workspace_id=$1 ORDER BY created_at,id", workspace)
		if e != nil {
			return e
		}
		var ids []string
		for rs.Next() {
			var id string
			if e = rs.Scan(&id); e != nil {
				break
			}
			ids = append(ids, id)
		}
		rs.Close()
		if e != nil {
			return e
		}
		for _, id := range ids {
			w, e := s.workflow(ctx, tx, workspace, id)
			if e != nil {
				return e
			}
			out = append(out, w)
		}
		return nil
	})
	return out, e
}

func (s *Service) SaveWorkflow(ctx context.Context, actor, workspace, id, correlation string, in WorkflowChange) (Workflow, error) {
	var out Workflow
	types, ok := validTypes(in.AllowedTypes)
	if !ok || strings.TrimSpace(in.Name) == "" || len(in.Name) > 120 || in.MaxBytes <= 0 || in.RetentionDays < 1 || in.RetentionDays > 3650 ||
		(in.Destination != "OBJECT_STORAGE" && in.Destination != "TELEGRAM") || in.ExpectedRevision < 0 || !operationKey(in.Key) {
		return out, database.ErrInvalid
	}
	in.Name = strings.TrimSpace(in.Name)
	in.AllowedTypes = types
	fp := digest(struct {
		Name, Destination string
		Approval          bool
		Types             []string
		Max               int64
		Retention         int
		Enabled           bool
	}{in.Name, in.Destination, in.ApprovalRequired, in.AllowedTypes, in.MaxBytes, in.RetentionDays, in.Enabled})
	e := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		if !database.Permission(ctx, tx, actor, workspace, "uploads.create") {
			return database.ErrForbidden
		}
		d, _, e := s.decision(ctx, tx, actor, workspace, "uploader.custom_workflow", nil)
		if e != nil {
			// Basic plans may use the product but cannot define custom workflows.
			if d.Allowed || capabilityAllowed(d, "uploader.use") {
				return database.ErrForbidden
			}
			return e
		}
		if id == "" {
			if in.ExpectedRevision != 0 {
				return database.ErrConflict
			}
			var existing, previous string
			e = tx.QueryRow(ctx, "SELECT id::text,fingerprint FROM upload_workflows WHERE workspace_id=$1 AND operation_key=$2", workspace, in.Key).Scan(&existing, &previous)
			if e == nil {
				if previous != fp {
					return database.ErrConflict
				}
				out, e = s.workflow(ctx, tx, workspace, existing)
				return e
			}
			if !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
			raw, _ := json.Marshal(in.AllowedTypes)
			e = tx.QueryRow(ctx, `INSERT INTO upload_workflows(workspace_id,name,approval_required,allowed_types,max_bytes,retention_days,destination,creator_id,enabled,operation_key,fingerprint)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id::text`,
				workspace, in.Name, in.ApprovalRequired, raw, in.MaxBytes, in.RetentionDays, in.Destination, actor, in.Enabled, in.Key, fp).Scan(&id)
			if e != nil {
				return e
			}
		} else {
			raw, _ := json.Marshal(in.AllowedTypes)
			tag, e := tx.Exec(ctx, `UPDATE upload_workflows SET name=$4,approval_required=$5,allowed_types=$6,max_bytes=$7,retention_days=$8,destination=$9,enabled=$10,revision=revision+1,updated_at=now()
				WHERE workspace_id=$1 AND id::text=$2 AND revision=$3`, workspace, id, in.ExpectedRevision, in.Name, in.ApprovalRequired, raw, in.MaxBytes, in.RetentionDays, in.Destination, in.Enabled)
			if e != nil {
				return e
			}
			if tag.RowsAffected() != 1 {
				return database.ErrConflict
			}
		}
		out, e = s.workflow(ctx, tx, workspace, id)
		if e != nil {
			return e
		}
		return database.Audit(ctx, tx, actor, workspace, "uploader.workflow", "upload_workflow", id, correlation, map[string]any{}, map[string]any{"revision": out.Revision, "enabled": out.Enabled})
	})
	return out, e
}

func capabilityAllowed(d policies.Decision, capability string) bool {
	for _, c := range d.Capabilities {
		if c == capability {
			return true
		}
	}
	return false
}

func (s *Service) Create(ctx context.Context, actor, workspace, correlation string, in Create) (Upload, error) {
	var out Upload
	in.Name = strings.TrimSpace(in.Name)
	in.ContentType = strings.ToLower(strings.TrimSpace(in.ContentType))
	if in.WorkflowID == "" || in.Name == "" || len(in.Name) > 255 || !contentTypePattern.MatchString(in.ContentType) || in.SizeBytes <= 0 || !operationKey(in.Key) {
		return out, database.ErrInvalid
	}
	fp := digest(in)
	e := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		if !database.Permission(ctx, tx, actor, workspace, "uploads.create") {
			return database.ErrForbidden
		}
		var previous string
		e := tx.QueryRow(ctx, "SELECT id::text,fingerprint FROM uploads WHERE workspace_id=$1 AND operation_key=$2", workspace, in.Key).Scan(&out.ID, &previous)
		if e == nil {
			if previous != fp {
				return database.ErrConflict
			}
			out, _, e = s.upload(ctx, tx, workspace, out.ID, false)
			return e
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		wf, e := s.workflow(ctx, tx, workspace, in.WorkflowID)
		if e != nil {
			return e
		}
		if !wf.Enabled {
			return database.ErrForbidden
		}
		allowed := map[string]bool{}
		for _, kind := range wf.AllowedTypes {
			allowed[kind] = true
		}
		p := Policy{Enabled: true, MaxBytes: wf.MaxBytes, AllowedTypes: allowed, ApprovalRequired: wf.ApprovalRequired}
		if e = p.Validate(in.SizeBytes, in.ContentType); e != nil {
			return database.ErrInvalid
		}
		amounts := map[string]int64{
			"uploader.max_file_bytes":     in.SizeBytes,
			"uploader.bytes_uploaded":     in.SizeBytes,
			"uploader.files_per_period":   1,
			"uploader.concurrent_uploads": 1,
		}
		d, req, e := s.decision(ctx, tx, actor, workspace, "uploader.use", amounts)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "quota:"+workspace); e != nil {
			return e
		}
		if limit := d.Resources["uploader.max_file_bytes"].Limit; limit <= 0 || in.SizeBytes > min64(limit, wf.MaxBytes) {
			return &policies.Denied{Decision: deniedDecision(d, "QUOTA_EXCEEDED")}
		}
		var storageBytes, activeFiles int64
		e = tx.QueryRow(ctx, `SELECT COALESCE(sum(size_bytes),0)::bigint,count(*)::bigint FROM uploads
			WHERE workspace_id=$1 AND state IN('RECEIVED','VALIDATING','PENDING_APPROVAL','PROCESSING','STORED') AND COALESCE(expires_at,now()+interval '1 second')>now()`, workspace).
			Scan(&storageBytes, &activeFiles)
		if e != nil {
			return e
		}
		if cap, ok := d.Resources["uploader.storage_bytes"]; !ok || storageBytes > cap.Limit-in.SizeBytes {
			return &policies.Denied{Decision: deniedDecision(d, "QUOTA_EXCEEDED")}
		}
		if cap, ok := d.Resources["uploader.active_files"]; !ok || activeFiles >= cap.Limit {
			return &policies.Denied{Decision: deniedDecision(d, "QUOTA_EXCEEDED")}
		}
		req.Amounts = amounts
		if _, e = policies.Reserve(ctx, tx, req, "upload:"+in.Key); e != nil {
			return e
		}
		if in.CustomerBotID != "" {
			var exists bool
			if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM customer_bots WHERE workspace_id=$1 AND id::text=$2)", workspace, in.CustomerBotID).Scan(&exists); e != nil {
				return e
			}
			if !exists {
				return database.ErrForbidden
			}
		}
		if e = tx.QueryRow(ctx, "SELECT gen_random_uuid()::text").Scan(&out.ID); e != nil {
			return e
		}
		key, e := storage.ObjectKey(workspace, "uploader:"+out.ID)
		if e != nil {
			return e
		}
		ingestExpiry := s.now().Add(15 * time.Minute)
		_, e = tx.Exec(ctx, `INSERT INTO uploads(id,workspace_id,creator_id,customer_bot_id,state,metadata,workflow_id,operation_key,fingerprint,original_name,content_type,size_bytes,storage_key,expires_at)
			VALUES($1,$2,$3,NULLIF($4,'')::uuid,'RECEIVED','{}',$5,$6,$7,$8,$9,$10,$11,$12)`,
			out.ID, workspace, actor, in.CustomerBotID, in.WorkflowID, in.Key, fp, in.Name, in.ContentType, in.SizeBytes, key, ingestExpiry)
		if e != nil {
			return e
		}
		out, _, e = s.upload(ctx, tx, workspace, out.ID, false)
		if e != nil {
			return e
		}
		return database.Audit(ctx, tx, actor, workspace, "uploader.create", "upload", out.ID, correlation, map[string]any{}, map[string]any{"state": out.State, "size_bytes": out.SizeBytes})
	})
	return out, e
}

func deniedDecision(d policies.Decision, reason string) policies.Decision {
	d.Allowed = false
	d.Reason = reason
	return d
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func (s *Service) List(ctx context.Context, actor, workspace string) ([]Upload, error) {
	out := []Upload{}
	e := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		if !database.Permission(ctx, tx, actor, workspace, "uploads.read") {
			return database.ErrForbidden
		}
		rs, e := tx.Query(ctx, "SELECT id::text FROM uploads WHERE workspace_id=$1 ORDER BY created_at DESC,id DESC LIMIT 200", workspace)
		if e != nil {
			return e
		}
		var ids []string
		for rs.Next() {
			var id string
			if e = rs.Scan(&id); e != nil {
				break
			}
			ids = append(ids, id)
		}
		rs.Close()
		if e != nil {
			return e
		}
		for _, id := range ids {
			u, _, e := s.upload(ctx, tx, workspace, id, false)
			if e != nil {
				return e
			}
			out = append(out, u)
		}
		return nil
	})
	return out, e
}

func (s *Service) Detail(ctx context.Context, actor, workspace, id string) (Upload, error) {
	var out Upload
	e := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		if !database.Permission(ctx, tx, actor, workspace, "uploads.read") {
			return database.ErrForbidden
		}
		var e error
		out, _, e = s.upload(ctx, tx, workspace, id, false)
		return e
	})
	return out, e
}

func (s *Service) failUpload(ctx context.Context, actor, workspace, id, code string) {
	_ = s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		u, _, e := s.upload(ctx, tx, workspace, id, true)
		if e != nil || (u.State != "RECEIVED" && u.State != "VALIDATING" && u.State != "PROCESSING") {
			return e
		}
		_, e = tx.Exec(ctx, "UPDATE uploads SET state='FAILED',error_code=$3,revision=revision+1,updated_at=now() WHERE workspace_id=$1 AND id::text=$2", workspace, id, code)
		if e == nil {
			_ = policies.Settle(ctx, tx, workspace, "upload:"+operationFromUpload(ctx, tx, workspace, id), false)
		}
		return e
	})
}

func operationFromUpload(ctx context.Context, tx pgx.Tx, workspace, id string) string {
	var key string
	_ = tx.QueryRow(ctx, "SELECT COALESCE(operation_key,'') FROM uploads WHERE workspace_id=$1 AND id::text=$2", workspace, id).Scan(&key)
	return key
}

func (s *Service) PutContent(ctx context.Context, actor, workspace, id, correlation string, r io.Reader, contentLength int64) (Upload, error) {
	var before Upload
	var key string
	var wf Workflow
	e := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		if !database.Permission(ctx, tx, actor, workspace, "uploads.create") {
			return database.ErrForbidden
		}
		var e error
		before, key, e = s.upload(ctx, tx, workspace, id, true)
		if e != nil {
			return e
		}
		if before.State != "RECEIVED" || contentLength != before.SizeBytes || s.Storage.Provider == nil {
			return database.ErrConflict
		}
		wf, e = s.workflow(ctx, tx, workspace, before.WorkflowID)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, "UPDATE uploads SET state='VALIDATING',revision=revision+1,updated_at=now() WHERE workspace_id=$1 AND id::text=$2", workspace, id)
		return e
	})
	if e != nil {
		return Upload{}, e
	}
	h := sha256.New()
	retention := time.Duration(wf.RetentionDays) * 24 * time.Hour
	if retention <= 0 {
		retention = 24 * time.Hour
	}
	expires := s.now().Add(retention)
	meta := storage.Metadata{Size: before.SizeBytes, ContentType: before.ContentType, ExpiresAt: expires}
	if e = s.Storage.Put(ctx, workspace, key, io.TeeReader(io.LimitReader(r, before.SizeBytes+1), h), meta); e != nil {
		s.failUpload(ctx, actor, workspace, id, "STORAGE_WRITE_FAILED")
		return Upload{}, e
	}
	m, e := s.Storage.Metadata(ctx, workspace, key)
	if e != nil || m.Size != before.SizeBytes {
		_ = s.Storage.Delete(ctx, workspace, key)
		s.failUpload(ctx, actor, workspace, id, "STORAGE_VERIFY_FAILED")
		if e == nil {
			e = storage.ErrUnavailable
		}
		return Upload{}, e
	}
	sum := hex.EncodeToString(h.Sum(nil))
	var out Upload
	e = s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		current, _, e := s.upload(ctx, tx, workspace, id, true)
		if e != nil {
			return e
		}
		if current.State != "VALIDATING" {
			return database.ErrConflict
		}
		_, e = tx.Exec(ctx, `INSERT INTO upload_files(workspace_id,upload_id,storage_key,media_type,size_bytes,sha256,expires_at)
			VALUES($1,$2,$3,$4,$5,$6,$7)`, workspace, id, key, before.ContentType, before.SizeBytes, sum, expires)
		if e != nil {
			return e
		}
		next := "PENDING_APPROVAL"
		if !wf.ApprovalRequired {
			next = "PROCESSING"
		}
		_, e = tx.Exec(ctx, "UPDATE uploads SET state=$3,sha256=$4,expires_at=$5,revision=revision+1,updated_at=now() WHERE workspace_id=$1 AND id::text=$2", workspace, id, next, sum, expires)
		if e != nil {
			return e
		}
		if next == "PROCESSING" {
			_, e = tx.Exec(ctx, "UPDATE uploads SET state='STORED',revision=revision+1,updated_at=now() WHERE workspace_id=$1 AND id::text=$2", workspace, id)
			if e != nil {
				return e
			}
		}
		if e = policies.Settle(ctx, tx, workspace, "upload:"+operationFromUpload(ctx, tx, workspace, id), true); e != nil {
			return e
		}
		out, _, e = s.upload(ctx, tx, workspace, id, false)
		if e != nil {
			return e
		}
		return database.Audit(ctx, tx, actor, workspace, "uploader.content", "upload", id, correlation, map[string]any{"state": "VALIDATING"}, map[string]any{"state": out.State, "sha256": sum})
	})
	if e != nil {
		_ = s.Storage.Delete(ctx, workspace, key)
		s.failUpload(ctx, actor, workspace, id, "PERSISTENCE_FAILED")
		return Upload{}, e
	}
	return out, nil
}

func (s *Service) Action(ctx context.Context, actor, workspace, id, correlation string, in Action) (Upload, error) {
	var current Upload
	var key string
	if in.ExpectedRevision < 1 || (in.Action != "APPROVE" && in.Action != "REJECT" && in.Action != "DELETE") {
		return current, database.ErrInvalid
	}
	if in.Action == "APPROVE" {
		e := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
			if !database.Permission(ctx, tx, actor, workspace, "uploads.create") {
				return database.ErrForbidden
			}
			var e error
			current, _, e = s.upload(ctx, tx, workspace, id, true)
			if e != nil {
				return e
			}
			if current.Revision != in.ExpectedRevision || current.State != "PENDING_APPROVAL" {
				return database.ErrConflict
			}
			_, e = tx.Exec(ctx, "UPDATE uploads SET state='PROCESSING',revision=revision+1,updated_at=now() WHERE workspace_id=$1 AND id::text=$2", workspace, id)
			if e != nil {
				return e
			}
			_, e = tx.Exec(ctx, "UPDATE uploads SET state='STORED',revision=revision+1,updated_at=now() WHERE workspace_id=$1 AND id::text=$2", workspace, id)
			if e != nil {
				return e
			}
			current, _, e = s.upload(ctx, tx, workspace, id, false)
			if e != nil {
				return e
			}
			return database.Audit(ctx, tx, actor, workspace, "uploader.approve", "upload", id, correlation, map[string]any{"state": "PENDING_APPROVAL"}, map[string]any{"state": "STORED"})
		})
		return current, e
	}

	e := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		if !database.Permission(ctx, tx, actor, workspace, "uploads.create") {
			return database.ErrForbidden
		}
		var e error
		current, key, e = s.upload(ctx, tx, workspace, id, true)
		if e != nil {
			return e
		}
		if current.Revision != in.ExpectedRevision {
			return database.ErrConflict
		}
		if (in.Action == "REJECT" && current.State != "PENDING_APPROVAL") || (in.Action == "DELETE" && current.State != "STORED") {
			return database.ErrConflict
		}
		_, e = tx.Exec(ctx, "UPDATE uploads SET state='PROCESSING',revision=revision+1,updated_at=now() WHERE workspace_id=$1 AND id::text=$2", workspace, id)
		return e
	})
	if e != nil {
		return Upload{}, e
	}
	if e = s.Storage.Delete(ctx, workspace, key); e != nil {
		s.failUpload(ctx, actor, workspace, id, "STORAGE_DELETE_FAILED")
		return Upload{}, e
	}
	terminal := "REJECTED"
	if in.Action == "DELETE" {
		terminal = "EXPIRED"
	}
	e = s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		u, _, e := s.upload(ctx, tx, workspace, id, true)
		if e != nil {
			return e
		}
		if u.State != "PROCESSING" {
			return database.ErrConflict
		}
		_, e = tx.Exec(ctx, "UPDATE uploads SET state=$3,revision=revision+1,updated_at=now() WHERE workspace_id=$1 AND id::text=$2", workspace, id, terminal)
		if e != nil {
			return e
		}
		current, _, e = s.upload(ctx, tx, workspace, id, false)
		if e != nil {
			return e
		}
		return database.Audit(ctx, tx, actor, workspace, "uploader."+strings.ToLower(in.Action), "upload", id, correlation, map[string]any{"state": "PROCESSING"}, map[string]any{"state": terminal})
	})
	if e != nil {
		s.failUpload(ctx, actor, workspace, id, "TERMINAL_PERSISTENCE_FAILED")
	}
	return current, e
}

func (s *Service) expireOne(ctx context.Context, actor, workspace, id string) (bool, error) {
	if s.Storage.Provider == nil {
		return false, storage.ErrUnavailable
	}
	var key string
	claimed := false
	e := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		u, k, e := s.upload(ctx, tx, workspace, id, true)
		if e != nil {
			return e
		}
		now := s.now()
		if u.ExpiresAt.IsZero() || u.ExpiresAt.After(now) {
			return nil
		}
		if u.State == "PROCESSING" {
			if (u.ErrorCode != "RETENTION_DELETE_PENDING" && u.ErrorCode != "RETENTION_DELETE_RETRY") || u.UpdatedAt.Add(5*time.Minute).After(now) {
				return nil
			}
			key = k
			claimed = true
			return nil
		}
		switch u.State {
		case "RECEIVED", "VALIDATING", "PENDING_APPROVAL", "STORED":
		default:
			return nil
		}
		tag, e := tx.Exec(ctx, `UPDATE uploads SET state='PROCESSING',error_code='RETENTION_DELETE_PENDING',revision=revision+1,updated_at=now()
			WHERE workspace_id=$1 AND id::text=$2 AND revision=$3 AND state=$4`, workspace, id, u.Revision, u.State)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return database.ErrConflict
		}
		key = k
		claimed = true
		return nil
	})
	if e != nil || !claimed {
		return false, e
	}

	if e = s.Storage.Delete(ctx, workspace, key); e != nil {
		_ = s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
			u, _, x := s.upload(ctx, tx, workspace, id, true)
			if x != nil || u.State != "PROCESSING" || (u.ErrorCode != "RETENTION_DELETE_PENDING" && u.ErrorCode != "RETENTION_DELETE_RETRY") {
				return x
			}
			_, x = tx.Exec(ctx, "UPDATE uploads SET error_code='RETENTION_DELETE_RETRY',revision=revision+1,updated_at=now() WHERE workspace_id=$1 AND id::text=$2 AND revision=$3", workspace, id, u.Revision)
			return x
		})
		return false, e
	}

	e = s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		u, _, e := s.upload(ctx, tx, workspace, id, true)
		if e != nil {
			return e
		}
		if u.State != "PROCESSING" || (u.ErrorCode != "RETENTION_DELETE_PENDING" && u.ErrorCode != "RETENTION_DELETE_RETRY") {
			return database.ErrConflict
		}
		if e = policies.Settle(ctx, tx, workspace, "upload:"+operationFromUpload(ctx, tx, workspace, id), false); e != nil {
			return e
		}
		tag, e := tx.Exec(ctx, "UPDATE uploads SET state='EXPIRED',error_code='',revision=revision+1,updated_at=now() WHERE workspace_id=$1 AND id::text=$2 AND revision=$3", workspace, id, u.Revision)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return database.ErrConflict
		}
		return database.Audit(ctx, tx, actor, workspace, "uploader.expire", "upload", id, "uploader-retention", map[string]any{"state": "PROCESSING"}, map[string]any{"state": "EXPIRED"})
	})
	return e == nil, e
}

func (s *Service) CleanupExpired(ctx context.Context, actor, workspace string, limit int) (int, error) {
	if limit < 1 || limit > 200 {
		return 0, database.ErrInvalid
	}
	now := s.now()
	ids := []string{}
	e := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		rs, e := tx.Query(ctx, `SELECT id::text FROM uploads
			WHERE workspace_id=$1 AND expires_at<=$2 AND (
				state IN('RECEIVED','VALIDATING','PENDING_APPROVAL','STORED') OR
				(state='PROCESSING' AND error_code IN('RETENTION_DELETE_PENDING','RETENTION_DELETE_RETRY') AND updated_at<=$3)
			)
			ORDER BY expires_at,id LIMIT $4`, workspace, now, now.Add(-5*time.Minute), limit)
		if e != nil {
			return e
		}
		defer rs.Close()
		for rs.Next() {
			var id string
			if e = rs.Scan(&id); e != nil {
				return e
			}
			ids = append(ids, id)
		}
		return rs.Err()
	})
	if e != nil {
		return 0, e
	}
	cleaned := 0
	for _, id := range ids {
		ok, e := s.expireOne(ctx, actor, workspace, id)
		if e != nil {
			return cleaned, e
		}
		if ok {
			cleaned++
		}
	}
	return cleaned, nil
}

func (s *Service) SweepExpired(ctx context.Context) (int, error) {
	if s.Storage.Provider == nil {
		return 0, storage.ErrUnavailable
	}
	rows, e := s.DB.Pool.Query(ctx, `SELECT user_id::text,workspace_id::text
		FROM workspace_members WHERE role_key='OWNER' ORDER BY workspace_id,user_id`)
	if e != nil {
		return 0, e
	}
	type owner struct {
		actor, workspace string
	}
	owners := []owner{}
	for rows.Next() {
		var o owner
		if e = rows.Scan(&o.actor, &o.workspace); e != nil {
			break
		}
		owners = append(owners, o)
	}
	rowsErr := rows.Err()
	rows.Close()
	if e != nil {
		return 0, e
	}
	if rowsErr != nil {
		return 0, rowsErr
	}
	total := 0
	for _, o := range owners {
		n, e := s.CleanupExpired(ctx, o.actor, o.workspace, 50)
		total += n
		if e != nil {
			return total, e
		}
	}
	return total, nil
}

func (s *Service) Open(ctx context.Context, actor, workspace, id string) (io.ReadCloser, storage.Metadata, error) {
	var key string
	e := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		if !database.Permission(ctx, tx, actor, workspace, "uploads.read") {
			return database.ErrForbidden
		}
		u, k, e := s.upload(ctx, tx, workspace, id, false)
		if e != nil {
			return e
		}
		if u.State != "STORED" && u.State != "PENDING_APPROVAL" {
			return database.ErrForbidden
		}
		key = k
		return nil
	})
	if e != nil {
		return nil, storage.Metadata{}, e
	}
	m, e := s.Storage.Metadata(ctx, workspace, key)
	if e != nil {
		return nil, storage.Metadata{}, e
	}
	r, e := s.Storage.Open(ctx, workspace, key)
	return r, m, e
}

func (s *Service) SignedURL(ctx context.Context, actor, workspace, id string, ttl time.Duration) (string, error) {
	var key string
	e := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		if !database.Permission(ctx, tx, actor, workspace, "uploads.read") {
			return database.ErrForbidden
		}
		u, k, e := s.upload(ctx, tx, workspace, id, false)
		if e != nil {
			return e
		}
		if u.State != "STORED" {
			return database.ErrForbidden
		}
		key = k
		return nil
	})
	if e != nil {
		return "", e
	}
	return s.Storage.SignedURL(ctx, workspace, key, ttl)
}
