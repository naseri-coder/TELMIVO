//go:build integration

package integration

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
)

// PostgreSQL16 TEST-only regression: use one pool connection. Previously,
// adminAllowed for a nonmember called Store.IsAdmin -> Pool.QueryRow while
// Tenant was already holding that pool's only connection. The second query
// timed out even though the user was a valid platform admin.
// This test runs only after TestMain validates disposable test DB destinations.
func TestLICA9A2TenantAuthorizationUsesSingleConnection(t *testing.T) {
	admin := fixture(t, true)
	tenantOwner := fixture(t, false)
	stranger := fixture(t, false)
	cfg, err := database.PoolConfig(os.Getenv("TEST_DATABASE_URL"))
	must(t, err)
	cfg.MaxConns = 1
	cfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	must(t, err)
	defer pool.Close()
	store := &database.Store{Pool: pool}
	pingCtx, cancelPing := context.WithTimeout(ctx, 5*time.Second)
	defer cancelPing()
	must(t, pool.Ping(pingCtx))

	run := func(actor, workspace string, allowAdmin bool, expectGranted bool) {
		t.Helper()
		callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		var callback int
		err := store.Tenant(callCtx, actor, workspace, allowAdmin, func(tx pgx.Tx) error {
			callback++
			var scope string
			return tx.QueryRow(callCtx, "SELECT current_setting('app.workspace_id',true)").Scan(&scope)
		})
		if expectGranted {
			if err != nil || callback != 1 {
				t.Fatalf("single-connection tenant access: granted=%t error=%v callback=%d", allowAdmin, err, callback)
			}
		} else if !errors.Is(err, database.ErrForbidden) || callback != 0 {
			t.Fatalf("unexpected tenant access: actor=%s allowed=%t error=%v callback=%d", actor, allowAdmin, err, callback)
		}
	}
	// The first case reproduces the old two-connection pool starvation.
	run(admin.UserID, tenantOwner.WorkspaceID, true, true)
	run(admin.UserID, tenantOwner.WorkspaceID, false, false)
	run(tenantOwner.UserID, tenantOwner.WorkspaceID, false, true)
	run(stranger.UserID, tenantOwner.WorkspaceID, true, false)
}
