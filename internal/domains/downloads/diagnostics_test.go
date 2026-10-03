package downloads

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"testing"
)

func TestDiagnosticPrivacyAndClasses(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{{nil, "SUCCESS"}, {database.ErrConflict, "CONFLICT"}, {database.ErrForbidden, "FORBIDDEN"}, {pgx.ErrNoRows, "NOT_FOUND"}, {context.Canceled, "CANCELLED"}, {context.DeadlineExceeded, "TIMEOUT"}, {errors.New("signed-url-secret"), "DB_OR_OPERATION_ERROR"}} {
		if got := diagnosticResult(tc.err); got != tc.want {
			t.Fatalf("got %s want %s", got, tc.want)
		}
	}
	a := CoordinateDiagnosticID("user-secret", "workspace-secret")
	if len(a) != 24 || a != CoordinateDiagnosticID("user-secret", "workspace-secret") || a == CoordinateDiagnosticID("other", "workspace-secret") {
		t.Fatal("coordinate correlation/redaction")
	}
}
