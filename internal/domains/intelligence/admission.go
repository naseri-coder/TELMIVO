package intelligence

import (
	"errors"

	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
)

// Only explicit business admission denials become durable skipped occurrences.
// Database, transaction, cancellation and unexpected errors must abort polling.
func admissionDenial(err error) bool {
	var quota *contentplugins.QuotaDenied
	return errors.As(err, &quota) || errors.Is(err, database.ErrForbidden) ||
		errors.Is(err, sdk.ErrPermission) || errors.Is(err, sdk.ErrState) || errors.Is(err, sdk.ErrDependency)
}
