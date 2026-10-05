//go:build integration

package integration

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/policies"
	"github.com/naseri-coder/TELMIVO/internal/domains/uploads"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/storage"
	files "github.com/naseri-coder/TELMIVO/internal/plugins/filemanager"
)

func r4cService(t *testing.T, root string) (*uploads.Service, func()) {
	t.Helper()
	local, e := storage.NewLocal(root, 2<<20)
	must(t, e)
	return &uploads.Service{DB: db, Storage: storage.Scoped{Provider: local}, Now: func() time.Time { return time.Now().UTC() }}, func() { must(t, local.Close()) }
}

func TestR4CUploaderLifecycleAndQuota(t *testing.T) {
	u := fixture(t, false)
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE subscriptions SET plan_key='PRO',previous_plan_key=NULL,expires_at=NULL WHERE workspace_id=$1", u.WorkspaceID)
		return e
	})
	s, closeStorage := r4cService(t, t.TempDir())
	defer closeStorage()

	wf, e := s.SaveWorkflow(ctx, u.UserID, u.WorkspaceID, "", "r4c-workflow", uploads.WorkflowChange{
		Name: "اسناد داخلی", ApprovalRequired: true, AllowedTypes: []string{"text/plain"},
		MaxBytes: 64, RetentionDays: 30, Destination: "OBJECT_STORAGE", Enabled: true,
		Key: "r4c-workflow-main",
	})
	must(t, e)
	if wf.ID == "" || wf.Revision != 1 || !wf.ApprovalRequired {
		t.Fatal("workflow not persisted", wf)
	}

	in := uploads.Create{WorkflowID: wf.ID, Name: "note.txt", ContentType: "text/plain", SizeBytes: 5, Key: "r4c-upload-main"}
	item, e := s.Create(ctx, u.UserID, u.WorkspaceID, "r4c-create", in)
	must(t, e)
	if item.State != "RECEIVED" || item.Revision != 1 {
		t.Fatal("unexpected create", item)
	}
	replay, e := s.Create(ctx, u.UserID, u.WorkspaceID, "r4c-create-replay", in)
	must(t, e)
	if replay.ID != item.ID {
		t.Fatal("idempotent create duplicated")
	}
	conflict := in
	conflict.Name = "changed.txt"
	if _, e = s.Create(ctx, u.UserID, u.WorkspaceID, "r4c-conflict", conflict); !errors.Is(e, database.ErrConflict) {
		t.Fatal("changed idempotent request accepted", e)
	}

	item, e = s.PutContent(ctx, u.UserID, u.WorkspaceID, item.ID, "r4c-content", strings.NewReader("hello"), 5)
	must(t, e)
	if item.State != "PENDING_APPROVAL" || len(item.SHA256) != 64 {
		t.Fatal("content lifecycle", item)
	}
	r, meta, e := s.Open(ctx, u.UserID, u.WorkspaceID, item.ID)
	must(t, e)
	b, e := io.ReadAll(r)
	_ = r.Close()
	must(t, e)
	if string(b) != "hello" || meta.Size != 5 {
		t.Fatal("private object mismatch", string(b), meta)
	}
	item, e = s.Action(ctx, u.UserID, u.WorkspaceID, item.ID, "r4c-approve", uploads.Action{Action: "APPROVE", ExpectedRevision: item.Revision})
	must(t, e)
	if item.State != "STORED" {
		t.Fatal("approval did not store", item)
	}

	second, e := s.Create(ctx, u.UserID, u.WorkspaceID, "r4c-create-reject", uploads.Create{
		WorkflowID: wf.ID, Name: "reject.txt", ContentType: "text/plain", SizeBytes: 4, Key: "r4c-upload-reject",
	})
	must(t, e)
	second, e = s.PutContent(ctx, u.UserID, u.WorkspaceID, second.ID, "r4c-content-reject", strings.NewReader("nope"), 4)
	must(t, e)
	second, e = s.Action(ctx, u.UserID, u.WorkspaceID, second.ID, "r4c-reject", uploads.Action{Action: "REJECT", ExpectedRevision: second.Revision})
	must(t, e)
	if second.State != "REJECTED" {
		t.Fatal(second)
	}

	largeWF, e := s.SaveWorkflow(ctx, u.UserID, u.WorkspaceID, "", "r4c-workflow-large", uploads.WorkflowChange{
		Name: "حد بزرگ", ApprovalRequired: false, AllowedTypes: []string{"application/octet-stream"},
		MaxBytes: 5_000_000_000, RetentionDays: 1, Destination: "OBJECT_STORAGE", Enabled: true,
		Key: "r4c-workflow-large",
	})
	must(t, e)
	_, e = s.Create(ctx, u.UserID, u.WorkspaceID, "r4c-too-large", uploads.Create{
		WorkflowID: largeWF.ID, Name: "too-large.bin", ContentType: "application/octet-stream",
		SizeBytes: 2_000_000_001, Key: "r4c-upload-too-large",
	})
	var denied *policies.Denied
	if !errors.As(e, &denied) || denied.Decision.Reason != "QUOTA_EXCEEDED" {
		t.Fatal("policy max-file cap not enforced", e)
	}

	other := fixture(t, false)
	if _, e = s.Detail(ctx, other.UserID, u.WorkspaceID, item.ID); !errors.Is(e, database.ErrForbidden) {
		t.Fatal("cross-workspace read allowed", e)
	}
}

func TestR4CFileManagerHandoffIdentifier(t *testing.T) {
	id := "upload_11111111-1111-1111-1111-111111111111"
	f := files.File{FileID: id, UniqueID: id, Kind: "document", Title: "فایل آپلودشده", Enabled: true}
	if !f.Valid() {
		t.Fatal("Uploader handoff identifier is incompatible with File Manager")
	}
}

func TestR4CBrowserFixture(t *testing.T) {
	u, _ := r3aLogin(t, 98010001, false)
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE subscriptions SET plan_key='PRO',previous_plan_key=NULL,expires_at=NULL WHERE workspace_id=$1", u.WorkspaceID)
		return e
	})
}

func TestR4CRecoveryPreparation(t *testing.T) {
	u := fixture(t, false)
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE subscriptions SET plan_key='PRO',previous_plan_key=NULL,expires_at=NULL WHERE workspace_id=$1", u.WorkspaceID)
		return e
	})
	s, closeStorage := r4cService(t, t.TempDir())
	defer closeStorage()
	wf, e := s.SaveWorkflow(ctx, u.UserID, u.WorkspaceID, "", "r4c-recovery-workflow", uploads.WorkflowChange{
		Name: "Recovery Workflow", ApprovalRequired: true, AllowedTypes: []string{"text/plain"},
		MaxBytes: 1024, RetentionDays: 7, Destination: "OBJECT_STORAGE", Enabled: true,
		Key: "r4c-recovery-workflow",
	})
	must(t, e)
	item, e := s.Create(ctx, u.UserID, u.WorkspaceID, "r4c-recovery-create", uploads.Create{
		WorkflowID: wf.ID, Name: "recovery.txt", ContentType: "text/plain", SizeBytes: 8, Key: "r4c-recovery-upload",
	})
	must(t, e)
	item, e = s.PutContent(ctx, u.UserID, u.WorkspaceID, item.ID, "r4c-recovery-content", strings.NewReader("recovery"), 8)
	must(t, e)
	if item.State != "PENDING_APPROVAL" {
		t.Fatal(item)
	}
}

func TestR4CRecovered(t *testing.T) {
	store, e := database.Open(ctx, os.Getenv("TEST_RESTORE_MIGRATION_DATABASE_URL"), false)
	must(t, e)
	defer store.Pool.Close()
	var state, name, sha string
	var filesCount int
	must(t, store.Pool.QueryRow(ctx, `SELECT state,original_name,sha256 FROM uploads WHERE operation_key='r4c-recovery-upload'`).Scan(&state, &name, &sha))
	must(t, store.Pool.QueryRow(ctx, `SELECT count(*) FROM upload_files f JOIN uploads u ON u.id=f.upload_id AND u.workspace_id=f.workspace_id WHERE u.operation_key='r4c-recovery-upload'`).Scan(&filesCount))
	if state != "PENDING_APPROVAL" || name != "recovery.txt" || len(sha) != 64 || filesCount != 1 {
		t.Fatal("uploader relational recovery lost", state, name, len(sha), filesCount)
	}
	// Object bytes live in the private object store and are intentionally outside the PostgreSQL restore drill.
}
