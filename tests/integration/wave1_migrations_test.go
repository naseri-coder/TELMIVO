//go:build integration

package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"

	"github.com/naseri-coder/TELMIVO/db/migrations"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
)

func assertAppliedCheckpoint(t *testing.T, store *database.Store, checkpoint string) {
	t.Helper()
	rows, err := store.Pool.Query(ctx, "SELECT name,checksum FROM schema_migrations ORDER BY name")
	must(t, err)
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name, checksum string
		must(t, rows.Scan(&name, &checksum))
		contents, err := migrations.Files.ReadFile(name)
		must(t, err)
		h := sha256.Sum256(contents)
		if checksum != hex.EncodeToString(h[:]) {
			t.Fatalf("applied migration checksum drift: %s", name)
		}
		names = append(names, name)
	}
	must(t, rows.Err())
	must(t, migrations.ValidateChain(names, checkpoint))
}

func TestWave1MigrationChainAndReapply(t *testing.T) {
	assertAppliedCheckpoint(t, owner, migrations.CurrentCheckpoint)
	must(t, owner.Migrate(ctx))
	must(t, owner.Migrate(ctx))
	assertAppliedCheckpoint(t, owner, migrations.CurrentCheckpoint)
}

func TestWave1MigrationRecovered(t *testing.T) {
	store, err := database.Open(ctx, os.Getenv("TEST_RESTORE_MIGRATION_DATABASE_URL"), false)
	must(t, err)
	defer store.Pool.Close()
	assertAppliedCheckpoint(t, store, migrations.CurrentCheckpoint)
}
