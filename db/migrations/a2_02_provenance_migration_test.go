package migrations

import (
	"strings"
	"testing"
)

// Source contract only: the real PostgreSQL16 migration has NOT run here.
// Historical UNKNOWN/Inbox/DLQ state must never be rewritten or guessed.
func TestLICA9A202JournalProvenanceMigrationSource(t *testing.T) {
	raw, err := Files.ReadFile("0020_journal_workspace_provenance.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToUpper(string(raw))
	for _, fragment := range []string{
		"ALTER TABLE INBOX_EVENTS",
		"ADD COLUMN WORKSPACE_ID UUID REFERENCES WORKSPACES(ID)",
		"ALTER TABLE DEAD_LETTER_EVENTS",
		"COMMENT ON COLUMN INBOX_EVENTS.WORKSPACE_ID",
		"COMMENT ON COLUMN DEAD_LETTER_EVENTS.WORKSPACE_ID",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("missing journal provenance invariant: %s", fragment)
		}
	}
	// Every table MUST receive its OWN verified-provenance column; a single
	// ADD COLUMN anywhere in the migration is not sufficient.
	for _, table := range []string{"INBOX_EVENTS", "DEAD_LETTER_EVENTS"} {
		start := strings.Index(sql, "ALTER TABLE "+table)
		if start < 0 {
			t.Fatalf("missing journal table alteration: %s", table)
		}
		end := strings.Index(sql[start:], ";")
		if end < 0 || !strings.Contains(sql[start:start+end],
			"ADD COLUMN WORKSPACE_ID UUID REFERENCES WORKSPACES(ID)") {
			t.Errorf("journal lacks own workspace provenance column: %s", table)
		}
	}
	// Check executable SQL statements rather than rejecting explanatory
	// documentation comments that discuss the unsafe operations.
	for _, line := range strings.Split(sql, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "--") || line == "" {
			continue
		}
		for _, forbidden := range []string{
			"UPDATE INBOX_EVENTS", "UPDATE DEAD_LETTER_EVENTS",
			"DELETE FROM", "TRUNCATE", "DROP TABLE",
			"ENABLE ROW LEVEL SECURITY", "FORCE ROW LEVEL SECURITY",
			"ALTER TABLE OUTBOX_EVENTS",
		} {
			if strings.HasPrefix(line, forbidden) || strings.Contains(line, " "+forbidden) {
				t.Errorf("unsafe journal migration statement: %s", line)
			}
		}
	}
}
