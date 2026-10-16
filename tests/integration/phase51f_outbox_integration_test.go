//go:build integration

package integration

import (
	"context"
	"testing"
)

// Native PostgreSQL acceptance gate. This test requires the existing
// isolated TEST_DATABASE_URL / TEST_MIGRATION_DATABASE_URL TestMain sandbox;
// it MUST NEVER be invoked against an actual installation database.
// JetStream broker ACK-loss E2E is a separate, still-open release gate.
func TestLICA9Phase51FPostgresOutboxUnknownQuarantine(t *testing.T) {
	must(t, owner.Migrate(ctx))
	var eventID string
	must(t, db.Pool.QueryRow(ctx,
		"INSERT INTO outbox_events(subject,event_type,payload,correlation_id) VALUES('telmivo.events.phase51f','phase51f.synthetic','{}'::jsonb,'phase51f-isolated') RETURNING id::text",
	).Scan(&eventID))
	// UNKNOWN may be retained in this disposable native-test database when
	// the test fails. It must not be deleted or reopened without evidence.
	tx, err := db.Pool.Begin(ctx)
	must(t, err)
	tag, err := tx.Exec(ctx,
		"UPDATE outbox_events SET delivery_state='UNKNOWN',delivery_attempted_at=clock_timestamp(),lease_until=clock_timestamp()+interval '60 seconds',lease_owner='phase51f-test-owner',attempts=attempts+1 WHERE id=$1 AND delivery_state='READY'",
		eventID)
	must(t, err)
	if tag.RowsAffected() != 1 {
		t.Fatal("pre-transport UNKNOWN claim not persisted")
	}
	must(t, tx.Commit(ctx))

	var state string
	var attempts int
	must(t, db.Pool.QueryRow(ctx,
		"SELECT delivery_state,attempts FROM outbox_events WHERE id=$1",
		eventID).Scan(&state, &attempts))
	if state != "UNKNOWN" || attempts != 1 {
		t.Fatalf("durable claim: %s attempts=%d", state, attempts)
	}
	// Expiring the time-based lease must not restore delivery eligibility.
	_, err = db.Pool.Exec(ctx,
		"UPDATE outbox_events SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1", eventID)
	must(t, err)
	var selectable bool
	must(t, db.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM outbox_events WHERE id=$1 AND delivery_state='READY' AND published_at IS NULL AND (lease_until IS NULL OR lease_until<now()))",
		eventID).Scan(&selectable))
	if selectable {
		t.Fatal("UNKNOWN dispatch was auto-replayed after lease expiry")
	}
	_, err = db.Pool.Exec(ctx,
		"UPDATE outbox_events SET delivery_state='READY' WHERE id=$1", eventID)
	pgCode(t, err, "P0001")

	// Only a separate, corroborated broker ACK may justify marking PUBLISHED.
	// Here a synthetic ACK is modeled solely in the disposable test database.
	tag, err = db.Pool.Exec(ctx,
		"UPDATE outbox_events SET delivery_state='PUBLISHED',published_at=clock_timestamp(),lease_until=NULL,lease_owner=NULL WHERE id=$1 AND delivery_state='UNKNOWN' AND lease_owner='phase51f-test-owner'",
		eventID)
	must(t, err)
	if tag.RowsAffected() != 1 {
		t.Fatal("broker-acknowledged transition did not match owner fence")
	}
	must(t, db.Pool.QueryRow(ctx,
		"SELECT delivery_state FROM outbox_events WHERE id=$1", eventID).Scan(&state))
	if state != "PUBLISHED" {
		t.Fatalf("confirmed outbox remained in %s", state)
	}
	_, err = db.Pool.Exec(ctx,
		"UPDATE outbox_events SET delivery_state='READY',published_at=NULL WHERE id=$1", eventID)
	pgCode(t, err, "P0001")
	_, err = db.Pool.Exec(context.Background(),
		"DELETE FROM outbox_events WHERE id=$1 AND delivery_state='PUBLISHED'", eventID)
	must(t, err)
}
