//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/queue"
)

// PG16 ONLY, isolated native TestMain with genuine signed TEST V2 Manager.
// No NATS server or outbound provider call is necessary for this SQL test.
func TestLICA9A202JournalProvenanceIntegration(t *testing.T) {
	must(t, owner.Migrate(ctx))
	a := fixture(t, false)
	b := fixture(t, false)
	var newEvent string
	must(t, db.Pool.QueryRow(ctx, "SELECT gen_random_uuid()::text").Scan(&newEvent))
	const durable = "a2-02-provenance-test"

	// Valid event: workspace recorded in the durable Inbox primary-key fence.
	var callback int
	valid := queue.Envelope{ID: newEvent, WorkspaceID: a.WorkspaceID, Type: "a2.known"}
	handler := func(_ context.Context, _ pgx.Tx, _ queue.Envelope) error {
		callback++
		return nil
	}
	must(t, queue.Process(ctx, db, durable, valid, false, handler))
	var provenance string
	must(t, db.Pool.QueryRow(ctx,
		"SELECT COALESCE(workspace_id::text,'') FROM inbox_events WHERE consumer=$1 AND event_id=$2",
		durable, newEvent).Scan(&provenance))
	if provenance != a.WorkspaceID || callback != 1 {
		t.Fatalf("verified Inbox workspace not persisted: %q callbacks=%d", provenance, callback)
	}
	// Identical event is idempotent; changing the VERIFIED tenant is denied.
	must(t, queue.Process(ctx, db, durable, valid, false, handler))
	if callback != 1 {
		t.Fatal("same event ID processed more than once")
	}
	forged := valid
	forged.WorkspaceID = b.WorkspaceID
	if err := queue.Process(ctx, db, durable, forged, false, handler); !errors.Is(err, database.ErrForbidden) {
		t.Fatalf("cross-workspace Inbox collision not denied: %v", err)
	}
	if callback != 1 {
		t.Fatal("cross-workspace collision called the handler")
	}

	// A historical Inbox record has no verifiable tenant information; it
	// remains NULL, preserves the existing dedupe fence and cannot be claimed
	// by the new runtime merely because a message supplies a workspace.
	var legacyID string
	must(t, db.Pool.QueryRow(ctx, "INSERT INTO inbox_events(consumer,event_id) VALUES($1,gen_random_uuid()) RETURNING event_id::text",
		durable).Scan(&legacyID))
	legacy := valid
	legacy.ID = legacyID
	must(t, queue.Process(ctx, db, durable, legacy, false, handler))
	if callback != 1 {
		t.Fatal("legacy Inbox was replayed")
	}
	var stillUnknown bool
	must(t, db.Pool.QueryRow(ctx, "SELECT workspace_id IS NULL FROM inbox_events WHERE consumer=$1 AND event_id=$2",
		durable, legacyID).Scan(&stillUnknown))
	if !stillUnknown {
		t.Fatal("legacy provenance was fabricated")
	}

	// The external fence is created with verified provenance and removed
	// when the synthetic handler's own transaction commits successfully.
	var externalID string
	must(t, db.Pool.QueryRow(ctx, "SELECT gen_random_uuid()::text").Scan(&externalID))
	ext := valid
	ext.ID = externalID
	ext.Type = "a2.external"
	must(t, queue.Process(ctx, db, durable, ext, true, handler))
	var remaining int
	must(t, db.Pool.QueryRow(ctx,
		"SELECT count(*) FROM dead_letter_events WHERE consumer=$1 AND event_id=$2", durable, externalID).Scan(&remaining))
	if remaining != 0 {
		t.Fatal("committed external handler retained uncertainty fence")
	}

	// Legacy DLQ payload may contain an untrusted workspace string; no
	// automatic schema backfill can adopt it as verified provenance.
	var oldDLQ string
	must(t, db.Pool.QueryRow(ctx, `INSERT INTO dead_letter_events(consumer,event_id,error_code,payload)
		VALUES($1,gen_random_uuid(),'LEGACY_TEST',jsonb_build_object('workspace_id',$2::text))
		RETURNING event_id::text`, durable, b.WorkspaceID).Scan(&oldDLQ))
	must(t, db.Pool.QueryRow(ctx, "SELECT workspace_id IS NULL FROM dead_letter_events WHERE consumer=$1 AND event_id=$2",
		durable, oldDLQ).Scan(&stillUnknown))
	if !stillUnknown {
		t.Fatal("legacy DLQ payload was treated as authenticated workspace")
	}
}
