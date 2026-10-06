package database

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"testing"
)

type permissionRow struct {
	allowed bool
	err     error
}

func (r permissionRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*dest[0].(*bool) = r.allowed
	return nil
}

type permissionTx struct {
	pgx.Tx
	row pgx.Row
}

func (t permissionTx) QueryRow(context.Context, string, ...any) pgx.Row { return t.row }
func TestPermissionPreservesInfrastructureFailure(t *testing.T) {
	failure := errors.New("synthetic database failure")
	for _, c := range []struct {
		row  permissionRow
		want error
	}{
		{permissionRow{allowed: true}, nil}, {permissionRow{}, ErrForbidden}, {permissionRow{err: failure}, failure},
	} {
		if got := RequirePermission(context.Background(), permissionTx{row: c.row}, "actor", "workspace", "automation.manage"); !errors.Is(got, c.want) {
			t.Fatalf("got %v want %v", got, c.want)
		}
	}
}
