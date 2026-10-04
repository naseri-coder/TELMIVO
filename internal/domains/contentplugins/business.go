package contentplugins

import (
	"context"
	"github.com/jackc/pgx/v5"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	gate "github.com/naseri-coder/TELMIVO/internal/plugins/accessgate"
)

// BusinessAccess reuses authoritative policy, entitlement and membership quota checks.
func (s *Service) BusinessAccess(ctx context.Context, actor string, scope sdk.Scope, policy string, user int64) (gate.Decision, error) {
	var d gate.Decision
	e := s.DB.Tenant(ctx, actor, scope.AccountID, false, func(tx pgx.Tx) error {
		var e error
		d, e = (txGate{s, tx, actor, scope}).Authorize(ctx, policy, user)
		return e
	})
	return d, e
}
