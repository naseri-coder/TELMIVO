package queue

import (
	"context"
	"errors"
	"github.com/naseri-coder/TELMIVO/internal/platform/installationlicense"

	"github.com/naseri-coder/TELMIVO/internal/platform/database"
)

var ErrExternalOutcomeUnknown = errors.New("external outcome unknown")

// Process preserves atomic inbox/business writes for ordinary handlers. External
// handlers instead commit an attempt fence and scrubbed uncertainty evidence
// before transport. A crash, cancellation or lost commit acknowledgement cannot
// make the same event eligible for a second external attempt. The tradeoff is
// at-most-once attempts: unconfirmed deliveries require operator reconciliation.
func Process(ctx context.Context, db *database.Store, durable string, event Envelope, external bool, handler Handler) error {
	if durable == "" || !uuidPattern.MatchString(event.ID) || !uuidPattern.MatchString(event.WorkspaceID) || event.Type == "" || handler == nil {
		return database.ErrInvalid
	}
	if e := installationlicense.CheckRuntime(ctx); e != nil {
		return e
	}
	tx, e := db.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback(context.Background())
		}
	}()
	if e = database.Scope(ctx, tx, event.WorkspaceID); e != nil {
		return e
	}
	tag, e := tx.Exec(ctx, "INSERT INTO inbox_events(consumer,event_id,workspace_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING", durable, event.ID, event.WorkspaceID)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		// Preserve immutable (consumer,event_id) idempotency. A legacy
		// NULL provenance must never be guessed/backfilled or replayed.
		// A conflicting recorded workspace must not silently suppress
		// events from another workspace.
		var knownWorkspace string
		if e = tx.QueryRow(ctx, "SELECT COALESCE(workspace_id::text,'') FROM inbox_events WHERE consumer=$1 AND event_id=$2", durable, event.ID).Scan(&knownWorkspace); e != nil {
			return e
		}
		if knownWorkspace != "" && knownWorkspace != event.WorkspaceID {
			return database.ErrForbidden
		}
		return tx.Commit(ctx)
	}
	if external {
		// Neither the transport payload nor credentials belong in recovery evidence.
		// The external uncertainty fence records a syntactically valid
		// envelope workspace claim before any non-idempotent handler.
		// This is NOT producer authentication or tenant-RLS authority.
		// Existing conflicting/legacy DLQ state cannot authorize a new
		// external attempt.
		fence, fenceErr := tx.Exec(ctx, "INSERT INTO dead_letter_events(consumer,event_id,workspace_id,error_code,payload) VALUES($1,$2,$3,'EXTERNAL_OUTCOME_UNCONFIRMED',$4) ON CONFLICT DO NOTHING", durable, event.ID, event.WorkspaceID, map[string]any{"workspace_id": event.WorkspaceID})
		if fenceErr != nil {
			return fenceErr
		}
		if fence.RowsAffected() != 1 {
			return ErrExternalOutcomeUnknown
		}
		if e = installationlicense.CheckRuntime(ctx); e != nil {
			return e
		}
		if e = tx.Commit(ctx); e != nil {
			return e
		}
		// Release before acquiring: the fence also works with a one-connection pool.
		tx, e = db.Pool.Begin(ctx)
		if e != nil {
			return e
		}
		if e = database.Scope(ctx, tx, event.WorkspaceID); e != nil {
			return e
		}
	}
	if e = handler(ctx, tx, event); e != nil {
		if external && errors.Is(e, ErrExternalOutcomeUnknown) {
			// The handler has durably classified the attempt as UNKNOWN. Commit
			// that domain evidence, preserve the pre-transport uncertainty
			// record and ACK the broker message. The inbox fence still makes
			// any automatic resend impossible.
			if e = tx.Commit(ctx); e != nil {
				return e
			}
			tx = nil
			return nil
		}
		return e
	}
	if external {
		// Confirmation is atomic with the handler's durable result. If this commit
		// fails, uncertainty evidence remains while the attempt fence stays sealed.
		if _, e = tx.Exec(ctx, "DELETE FROM dead_letter_events WHERE consumer=$1 AND event_id=$2 AND workspace_id=$3 AND error_code='EXTERNAL_OUTCOME_UNCONFIRMED'", durable, event.ID, event.WorkspaceID); e != nil {
			return e
		}
	}
	e = tx.Commit(ctx)
	if e == nil {
		tx = nil
	}
	return e
}
