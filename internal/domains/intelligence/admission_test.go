package intelligence

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
)

func TestScheduledAdmissionDenialsDoNotSwallowInfrastructureErrors(t *testing.T) {
	for _, err := range []error{&contentplugins.QuotaDenied{}, database.ErrForbidden, sdk.ErrPermission, sdk.ErrState} {
		if !admissionDenial(fmt.Errorf("wrapped: %w", err)) {
			t.Fatalf("business denial not recognized: %v", err)
		}
	}
	for _, err := range []error{nil, context.Canceled, context.DeadlineExceeded, errors.New("connection lost"), database.ErrInvalid, database.ErrConflict,
		&pgconn.PgError{Code: "40P01"}, &pgconn.PgError{Code: "23514"}, &pgconn.PgError{Code: "42501"}} {
		if admissionDenial(err) {
			t.Fatalf("unexpected/infrastructure error swallowed: %v", err)
		}
	}
}
