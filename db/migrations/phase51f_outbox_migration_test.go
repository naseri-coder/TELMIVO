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
	backfill := strings.Index(sql, "UPDATE outbox_events SET delivery_state='PUBLISHED'")
	constraint := strings.Index(sql, "ADD CONSTRAINT outbox_delivery_attempt_consistency")
	trigger := strings.Index(sql, "CREATE TRIGGER outbox_delivery_transition")
	if backfill < 0 || constraint <= backfill || trigger <= constraint {
		t.Fatalf("historical PUBLISHED backfill must precede constraints/transition trigger")
	}
	if strings.Contains(sql, "DROP TABLE outbox_events") || strings.Contains(sql, "TRUNCATE") {
		t.Fatal("outbox migration contains destructive operation")
	}
}
