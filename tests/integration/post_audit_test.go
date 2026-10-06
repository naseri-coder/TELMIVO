//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	apple "github.com/naseri-coder/TELMIVO/internal/domains/appleservices"
	"github.com/naseri-coder/TELMIVO/internal/domains/channels"
	"github.com/naseri-coder/TELMIVO/internal/domains/downloads"
	"github.com/naseri-coder/TELMIVO/internal/domains/entitlements"
	"github.com/naseri-coder/TELMIVO/internal/domains/intelligence"
	"github.com/naseri-coder/TELMIVO/internal/domains/policies"
	"github.com/naseri-coder/TELMIVO/internal/domains/uploads"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/storage"
	assistant "github.com/naseri-coder/TELMIVO/internal/plugins/aiassistant"
	workflow "github.com/naseri-coder/TELMIVO/internal/plugins/automation"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPostAuditChannelResourceAdmission(t *testing.T) {
	u := fixture(t, false)
	tenant(t, u, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "INSERT INTO temporary_grants(workspace_id,feature_key,enabled,limit_value,expires_at,reason) VALUES($1,'channels',true,1,now()+interval '1 hour','synthetic admission')", u.WorkspaceID)
		return err
	})
	service := &channels.Service{DB: db, Authority: &r4bTransport{}}
	gate := make(chan struct{})
	results := make(chan error, 2)
	for _, chat := range []int64{-110001, -110002} {
		go func(chat int64) {
			<-gate
			results <- db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
				_, err := service.Connect(ctx, tx, u.UserID, u.WorkspaceID, "post-audit", chat)
				return err
			})
		}(chat)
	}
	close(gate)
	success := 0
	for i := 0; i < 2; i++ {
		select {
		case err := <-results:
			if err == nil {
				success++
			} else {
				var denied *entitlements.Denied
				if !errors.As(err, &denied) {
					t.Fatal("unexpected infrastructure failure", err)
				}
			}
		case <-time.After(10 * time.Second):
			t.Fatal("admission deadlock")
		}
	}
	if success != 1 || number(t, u, "SELECT count(*) FROM channels WHERE workspace_id=$1", u.WorkspaceID) != 1 {
		t.Fatal("resource cap exceeded", success)
	}
}

func TestPostAuditUploaderCurrentAdmission(t *testing.T) {
	u := fixture(t, false)
	tenant(t, u, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE subscriptions SET plan_key='PRO' WHERE workspace_id=$1", u.WorkspaceID)
		return err
	})
	s, closeStorage := r4cService(t, t.TempDir())
	defer closeStorage()
	change := uploads.WorkflowChange{Name: "Admission", ApprovalRequired: true, AllowedTypes: []string{"text/plain"}, MaxBytes: 64, RetentionDays: 30, Destination: "OBJECT_STORAGE", Enabled: true, Key: "post-audit-workflow"}
	wf, err := s.SaveWorkflow(ctx, u.UserID, u.WorkspaceID, "", "post-audit", change)
	must(t, err)
	in := uploads.Create{WorkflowID: wf.ID, Name: "synthetic.txt", ContentType: "text/plain", SizeBytes: 5, Key: "post-audit-upload"}
	item, err := s.Create(ctx, u.UserID, u.WorkspaceID, "post-audit", in)
	must(t, err)
	change.Enabled = false
	change.ExpectedRevision = wf.Revision
	wf, err = s.SaveWorkflow(ctx, u.UserID, u.WorkspaceID, wf.ID, "post-audit", change)
	must(t, err)
	if _, err = s.Create(ctx, u.UserID, u.WorkspaceID, "post-audit", in); !errors.Is(err, database.ErrForbidden) {
		t.Fatal("disabled replay admitted", err)
	}
	if _, err = s.PutContent(ctx, u.UserID, u.WorkspaceID, item.ID, "post-audit", strings.NewReader("hello"), 5); !errors.Is(err, database.ErrForbidden) {
		t.Fatal("disabled write admitted", err)
	}
	change.Enabled = true
	change.ExpectedRevision = wf.Revision
	wf, err = s.SaveWorkflow(ctx, u.UserID, u.WorkspaceID, wf.ID, "post-audit", change)
	must(t, err)
	item, err = s.PutContent(ctx, u.UserID, u.WorkspaceID, item.ID, "post-audit", strings.NewReader("hello"), 5)
	must(t, err)
	change.Enabled = false
	change.ExpectedRevision = wf.Revision
	_, err = s.SaveWorkflow(ctx, u.UserID, u.WorkspaceID, wf.ID, "post-audit", change)
	must(t, err)
	if _, err = s.Action(ctx, u.UserID, u.WorkspaceID, item.ID, "post-audit", uploads.Action{Action: "APPROVE", ExpectedRevision: item.Revision}); !errors.Is(err, database.ErrForbidden) {
		t.Fatal("disabled approval admitted", err)
	}
	_, err = s.Action(ctx, u.UserID, u.WorkspaceID, item.ID, "post-audit", uploads.Action{Action: "REJECT", ExpectedRevision: item.Revision})
	must(t, err)
}

type postAuditDeleteFailure struct {
	storage.StorageProvider
	attempts atomic.Int64
}

func (p *postAuditDeleteFailure) Delete(ctx context.Context, key string) error {
	if p.attempts.Add(1) == 1 {
		return storage.ErrUnavailable
	}
	return p.StorageProvider.Delete(ctx, key)
}
func TestPostAuditManualDeletionRecovery(t *testing.T) {
	u := fixture(t, false)
	tenant(t, u, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE subscriptions SET plan_key='PRO' WHERE workspace_id=$1", u.WorkspaceID)
		return err
	})
	s, closeStorage := r4cService(t, t.TempDir())
	defer closeStorage()
	mock := &postAuditDeleteFailure{StorageProvider: s.Storage.Provider}
	s.Storage.Provider = mock
	wf, err := s.SaveWorkflow(ctx, u.UserID, u.WorkspaceID, "", "post-audit", uploads.WorkflowChange{Name: "Deletion", AllowedTypes: []string{"text/plain"}, MaxBytes: 64, RetentionDays: 30, Destination: "OBJECT_STORAGE", Enabled: true, Key: "post-audit-deletion-wf"})
	must(t, err)
	item, err := s.Create(ctx, u.UserID, u.WorkspaceID, "post-audit", uploads.Create{WorkflowID: wf.ID, Name: "synthetic.txt", ContentType: "text/plain", SizeBytes: 5, Key: "post-audit-delete-upload"})
	must(t, err)
	item, err = s.PutContent(ctx, u.UserID, u.WorkspaceID, item.ID, "post-audit", strings.NewReader("hello"), 5)
	must(t, err)
	_, err = s.Action(ctx, u.UserID, u.WorkspaceID, item.ID, "post-audit", uploads.Action{Action: "DELETE", ExpectedRevision: item.Revision})
	if !errors.Is(err, storage.ErrUnavailable) {
		t.Fatal(err)
	}
	if n := number(t, u, "SELECT count(*) FROM uploads WHERE id=$1 AND state='PROCESSING' AND error_code='MANUAL_DELETE_PENDING'", item.ID); n != 1 {
		t.Fatal("intent lost")
	}
	s.Now = func() time.Time { return time.Now().UTC().Add(6 * time.Minute) }
	n, err := s.CleanupExpired(ctx, u.UserID, u.WorkspaceID, 20)
	must(t, err)
	if n != 1 || mock.attempts.Load() != 2 || number(t, u, "SELECT count(*) FROM uploads WHERE id=$1 AND state='EXPIRED' AND error_code=''", item.ID) != 1 {
		t.Fatal("manual deletion not recovered", n, mock.attempts.Load())
	}
}

func TestPostAuditImmutableIngestionReplay(t *testing.T) {
	f := r3d6Fixture(t)
	id, closeStorage := r4fUploaderSource(t, f, "original source body")
	defer closeStorage()
	request := intelligence.IngestRequest{SourceKind: "UPLOADER", Reference: id, Key: "post-audit-ingest-1"}
	first, err := f.ai.Ingest(ctx, f.u.UserID, f.scope(assistant.ID), request)
	must(t, err)
	var object string
	tenant(t, f.u, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT storage_key FROM uploads WHERE id=$1", id).Scan(&object)
	})
	text := "replaced source body"
	must(t, f.ai.Storage.Put(ctx, f.u.WorkspaceID, object, strings.NewReader(text), storage.Metadata{Size: int64(len(text)), ContentType: "text/plain", ExpiresAt: f.clock.at.Add(time.Hour)}))
	request.Key = "post-audit-ingest-2"
	second, err := f.ai.Ingest(ctx, f.u.UserID, f.scope(assistant.ID), request)
	must(t, err)
	if second.ID != first.ID || second.Body != text || second.Revision <= first.Revision {
		t.Fatal("refresh failed", second)
	}
	for _, original := range []intelligence.Knowledge{first, second} {
		request.Key = fmt.Sprintf("post-audit-ingest-%d", original.Revision)
		got, err := f.ai.Ingest(ctx, f.u.UserID, f.scope(assistant.ID), request)
		must(t, err)
		if got != original {
			t.Fatal("receipt changed", got, original)
		}
	}
	if _, err = f.ai.SaveKnowledge(ctx, f.u.UserID, f.scope(assistant.ID), intelligence.Knowledge{Name: "manual collision", Body: "synthetic body", Enabled: true}, "post-audit-ingest-2", 0); !errors.Is(err, database.ErrConflict) {
		t.Fatal("import receipt reused for manual creation", err)
	}
	if _, err = f.ai.SaveKnowledge(ctx, f.u.UserID, f.scope(assistant.ID), intelligence.Knowledge{Name: "manual source", Body: "synthetic body", Enabled: true}, "post-audit-manual-key", 0); err != nil {
		t.Fatal(err)
	}
	request.Key = "post-audit-manual-key"
	if _, err = f.ai.Ingest(ctx, f.u.UserID, f.scope(assistant.ID), request); !errors.Is(err, database.ErrConflict) {
		t.Fatal("manual key reused for ingestion", err)
	}
	request.Key = "post-audit-ingest-2"
	request.Name = "changed client title"
	if _, err = f.ai.Ingest(ctx, f.u.UserID, f.scope(assistant.ID), request); !errors.Is(err, database.ErrConflict) {
		t.Fatal("changed request title accepted", err)
	}
	request.Name = ""
	another, cleanup := r4fUploaderSource(t, f, "another source")
	defer cleanup()
	request.Reference = another
	if _, err = f.ai.Ingest(ctx, f.u.UserID, f.scope(assistant.ID), request); !errors.Is(err, database.ErrConflict) {
		t.Fatal("conflicting ingestion replay admitted", err)
	}
	if n := number(t, f.u, "SELECT count(*) FROM ai_ingestion_receipts WHERE bot_id=$1", f.bot); n != 2 {
		t.Fatal("receipt count", n)
	}
	other := fixture(t, false)
	if n := number(t, other, "SELECT count(*) FROM ai_ingestion_receipts WHERE bot_id=$1", f.bot); n != 0 {
		t.Fatal("cross-workspace receipt leak", n)
	}
	for _, sql := range []string{"UPDATE ai_ingestion_receipts SET fingerprint=repeat('b',64) WHERE bot_id=$1", "DELETE FROM ai_ingestion_receipts WHERE bot_id=$1"} {
		err := db.Tenant(ctx, f.u.UserID, f.u.WorkspaceID, false, func(tx pgx.Tx) error { _, err := tx.Exec(ctx, sql, f.bot); return err })
		pgCode(t, err, "P0001")
	}
	if n := number(t, f.u, "SELECT count(*) FROM ai_ingestion_receipts WHERE bot_id=$1", f.bot); n != 2 {
		t.Fatal("receipt mutation escaped rollback", n)
	}
}

func TestPostAuditFoundationExpiry(t *testing.T) {
	u := fixture(t, false)
	r5aWorkspaceGrant(t, u.UserID, u.WorkspaceID, 5)
	now := time.Now().UTC()
	service := &apple.Service{DB: db, Clock: func() time.Time { return now }}
	for _, kind := range []apple.Kind{apple.IdentityVerification, apple.BusinessConnection, apple.DirectoryProvisioning, apple.ManagedAccount, apple.Reconciliation} {
		expiry := now.Add(time.Minute)
		op, err := service.CreateOperation(ctx, u.UserID, u.WorkspaceID, "post-audit", apple.CreateRequest{Kind: kind, Key: "post-audit-expiry-" + string(kind), ExpiresAt: &expiry})
		must(t, err)
		now = expiry
		op, err = service.Transition(ctx, u.UserID, u.WorkspaceID, op.ID, "post-audit", apple.TransitionRequest{State: apple.Expired, Expected: op.Revision, Reason: "deadline reached"})
		must(t, err)
		if op.State != apple.Expired || op.Revision != 2 {
			t.Fatal("expiry not durable", op)
		}
	}
}

func TestPostAuditScheduleEditOrdering(t *testing.T) {
	f := r3d6Fixture(t)
	w := f.wf("schedule.cron", workflow.Action{Type: "telegram.send", Value: "Synthetic bounded effect"})
	w.Schedule = workflow.Schedule{Kind: "CRON", Cron: "* * * * *", Timezone: "UTC", User: 10101001}
	saved := f.save(t, "post-audit-cron", w)
	tenant(t, f.u, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT next_at FROM automation_schedule_state WHERE workflow_id=$1", saved.ID).Scan(&f.clock.at)
	})
	workCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	gate := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); <-gate; results <- f.ai.TickSchedules(workCtx, f.u.UserID, f.u.WorkspaceID) }()
	go func() {
		defer wg.Done()
		<-gate
		_, err := f.ai.SaveWorkflow(workCtx, f.u.UserID, f.scope(workflow.ID), saved.ID, intelligence.WorkflowChange{Workflow: w, Key: "post-audit-edit", Expected: saved.Revision})
		results <- err
	}()
	close(gate)
	wg.Wait()
	for i := 0; i < 2; i++ {
		must(t, <-results)
	}
	if n := number(t, f.u, "SELECT count(*) FROM automation_executions WHERE workflow_id=$1", saved.ID); n > 1 {
		t.Fatal("duplicate schedule occurrence", n)
	}
}

func TestPostAuditDownloaderReplayAuthorization(t *testing.T) {
	u := fixture(t, false)
	tenant(t, u, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE subscriptions SET plan_key='PRO' WHERE workspace_id=$1", u.WorkspaceID)
		return err
	})
	product := downloads.Product{DB: db, Synthetic: true, MaxBytes: 49 << 20}
	var id string
	tenant(t, u, func(tx pgx.Tx) error {
		var err error
		id, err = product.Submit(ctx, tx, u.UserID, u.WorkspaceID, "post-audit", "https://youtu.be/dQw4w9WgXcQ", "post-audit-download")
		return err
	})
	tenant(t, u, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE workspace_members SET role_key='ANALYST' WHERE workspace_id=$1 AND user_id=$2", u.WorkspaceID, u.UserID)
		return err
	})
	err := db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
		_, err := product.Submit(ctx, tx, u.UserID, u.WorkspaceID, "post-audit", "https://youtu.be/dQw4w9WgXcQ", "post-audit-download")
		return err
	})
	var denied *policies.Denied
	if !errors.As(err, &denied) {
		t.Fatal("expected current authorization denial", err)
	}
	for _, path := range []string{"/api/v1/downloader", "/api/v1/downloader/" + id} {
		if code := apiRequest(t, u, path, "GET", ""); code != 403 {
			t.Fatal("direct/history permission disagreement", path, code)
		}
	}
	if n := number(t, u, "SELECT count(*) FROM download_jobs WHERE workspace_id=$1", u.WorkspaceID); n != 1 {
		t.Fatal("replay created a new job", n)
	}
}
