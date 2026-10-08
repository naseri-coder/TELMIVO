//go:build integration

package integration

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/installationlicense"
	"github.com/naseri-coder/TELMIVO/internal/platform/queue"
	"testing"
)

func TestInstallationLicenseRestrictedDatabaseAndQueue(t *testing.T) {
	u := fixture(t, false)
	restricted := installationlicense.RequireContext(ctx)
	called := false
	e := db.Tenant(restricted, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error { called = true; return nil })
	if !errors.Is(e, installationlicense.ErrRestricted) || called {
		t.Fatal("commercial transaction escaped restriction", e)
	}
	ro := installationlicense.ReadOnly(restricted)
	e = db.Tenant(ro, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error { var n int; return tx.QueryRow(ro, "SELECT 1").Scan(&n) })
	must(t, e)
	e = db.Tenant(ro, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error { _, e := tx.Exec(ro, "UPDATE users SET id=id WHERE id=$1", u.UserID); return e })
	if e == nil {
		t.Fatal("restricted GET wrote database")
	}
	event := queue.Envelope{ID: "00000000-0000-4000-8000-000000000001", WorkspaceID: u.WorkspaceID, Type: "license.test"}
	e = queue.Process(restricted, db, "license-test", event, true, func(context.Context, pgx.Tx, queue.Envelope) error { called = true; return nil })
	if !errors.Is(e, installationlicense.ErrRestricted) || called {
		t.Fatal("queue admission escaped restriction", e)
	}
}
