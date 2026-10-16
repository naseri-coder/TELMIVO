package migrations

import (
	"strings"
	"testing"
)

// Source-contract tests only. This DOES NOT execute PostgreSQL DDL.
// The real PG16 migration/rollback and production readiness remain NO-GO.
func TestLICA9Phase51FOutboxMigrationContract(t *testing.T) {
	raw, err := Files.ReadFile("0019_outbox_unknown_delivery.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, fragment := range []string{
		"ADD COLUMN delivery_state text NOT NULL DEFAULT 'READY'",
		"ADD COLUMN delivery_attempted_at timestamptz",
		"delivery_state='PUBLISHED'",
		"delivery_attempted_at=published_at",
		"UPDATE outbox_events SET delivery_state='UNKNOWN'",
		"delivery_attempted_at=clock_timestamp()",
		"WHERE published_at IS NULL AND attempts>0",
		"CHECK (delivery_state<>'READY' OR attempts=0)",
		"outbox attempt increment requires READY to UNKNOWN fence",
		"CHECK (delivery_state IN ('READY','UNKNOWN','PUBLISHED'))",
		"CHECK ((delivery_state='PUBLISHED')=(published_at IS NOT NULL))",
		"CREATE INDEX outbox_delivery_ready",
		"CREATE INDEX outbox_delivery_unknown",
		"CREATE TRIGGER outbox_delivery_transition",
		"IF OLD.delivery_state='UNKNOWN' AND NEW.delivery_state='READY' THEN",
		"IF OLD.delivery_state='READY' AND NEW.delivery_state='PUBLISHED' THEN",
		"IF OLD.delivery_state='READY' AND NEW.delivery_state='UNKNOWN' THEN",
		"OLD.delivery_state='UNKNOWN' AND NEW.delivery_state='PUBLISHED'",
		"unknown outbox attempt is durable and requires reconciliation",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("outbox migration lost invariant %q", fragment)
		}
	}
	quarantine := strings.Index(sql, "UPDATE outbox_events SET delivery_state='UNKNOWN'")
	backfill := strings.Index(sql, "UPDATE outbox_events SET delivery_state='PUBLISHED'")
	constraint := strings.Index(sql, "ADD CONSTRAINT outbox_delivery_attempt_consistency")
	trigger := strings.Index(sql, "CREATE TRIGGER outbox_delivery_transition")
	if quarantine < 0 || backfill <= quarantine || constraint <= backfill || trigger <= constraint {
		t.Fatalf("legacy unacknowledged attempts must be quarantined before published backfill and constraints")
	}
	if strings.Contains(sql, "DROP TABLE outbox_events") || strings.Contains(sql, "TRUNCATE") {
		t.Fatal("outbox migration contains destructive operation")
	}
}
