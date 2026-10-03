package downloads

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"log/slog"
)

// CoordinateDiagnosticID correlates dispatch without exposing customer identities.
// Identifiers are structured log fields only, never metric labels.
func CoordinateDiagnosticID(user, workspace string) string {
	h := sha256.Sum256([]byte(workspace + ":" + user))
	return hex.EncodeToString(h[:12])
}
func diagnosticResult(e error) string {
	switch {
	case e == nil:
		return "SUCCESS"
	case errors.Is(e, database.ErrConflict):
		return "CONFLICT"
	case errors.Is(e, database.ErrForbidden):
		return "FORBIDDEN"
	case errors.Is(e, pgx.ErrNoRows):
		return "NOT_FOUND"
	case errors.Is(e, context.Canceled):
		return "CANCELLED"
	case errors.Is(e, context.DeadlineExceeded):
		return "TIMEOUT"
	case errors.Is(e, database.ErrInvalid):
		return "INVALID"
	default:
		return "DB_OR_OPERATION_ERROR"
	}
}
func workerDiagnostic(event string, x work, fields ...any) {
	base := []any{"event", event, "download_job_id", x.ID, "media_job_id", x.MediaID,
		"coordinate_id", CoordinateDiagnosticID(x.User, x.Workspace), "generation", x.Lease.Generation, "state", x.State}
	slog.Info("downloader diagnostic", append(base, fields...)...)
}
