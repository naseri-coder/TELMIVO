// Package intelligence brokers bounded Automation and AI through authoritative services.
package intelligence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/commerce"
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	platform "github.com/naseri-coder/TELMIVO/internal/domains/pluginplatform"
	"github.com/naseri-coder/TELMIVO/internal/domains/supportplugins"
	provider "github.com/naseri-coder/TELMIVO/internal/platform/ai"
	cred "github.com/naseri-coder/TELMIVO/internal/platform/crypto"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"log/slog"
	"time"
)

type Clock interface{ Now() time.Time }
type Service struct {
	DB          *database.Store
	Core        *platform.Service
	Commercial  *commerce.Service
	Support     *supportplugins.Service
	Content     *contentplugins.Service
	Provider    provider.Provider
	Credentials cred.CredentialStore
	Clock       Clock
	Synthetic   bool
}

func (s *Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock.Now().UTC().Truncate(time.Microsecond)
	}
	return time.Now().UTC().Truncate(time.Microsecond)
}
func body(v any) []byte            { b, _ := json.Marshal(v); return b }
func hash(v any) string            { h := sha256.Sum256(body(v)); return hex.EncodeToString(h[:]) }
func key(k string) bool            { return len(k) > 0 && len(k) <= 100 }
func strict(b []byte, v any) error { return decodeStrict(b, v) }
func owner(ctx context.Context, tx pgx.Tx, scope sdk.Scope) (string, error) {
	var a string
	e := tx.QueryRow(ctx, "SELECT user_id::text FROM workspace_members WHERE workspace_id=$1 AND role_key='OWNER' ORDER BY user_id LIMIT 1", scope.AccountID).Scan(&a)
	return a, e
}
func (s *Service) lock(ctx context.Context, tx pgx.Tx, scope sdk.Scope) error {
	_, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock_shared(7100213)")
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,7100213))", scope.AccountID)
	return e
}
func (s *Service) validate(ctx context.Context, tx pgx.Tx, actor string, scope sdk.Scope, permission, cap string, running bool) error {
	if !database.Permission(ctx, tx, actor, scope.AccountID, permission) {
		return database.ErrForbidden
	}
	if e := s.lock(ctx, tx, scope); e != nil {
		return e
	}
	p, e := owner(ctx, tx, scope)
	if e != nil {
		return e
	}
	return s.Core.ValidateExecutionTx(ctx, tx, p, scope, cap, running)
}
func (s *Service) tenant(ctx context.Context, actor string, scope sdk.Scope, permission, cap string, fn func(pgx.Tx) error) error {
	return s.DB.Tenant(ctx, actor, scope.AccountID, false, func(tx pgx.Tx) error {
		if e := s.validate(ctx, tx, actor, scope, permission, cap, false); e != nil {
			return e
		}
		return fn(tx)
	})
}
func (s *Service) quota(ctx context.Context, tx pgx.Tx, scope sdk.Scope, metric string, n int64, consume bool) error {
	p, e := owner(ctx, tx, scope)
	if e != nil {
		return e
	}
	q, e := s.Commercial.QuotaTx(ctx, tx, p, scope.AccountID, scope.BotID, metric, n, consume)
	if e != nil {
		return e
	}
	if !q.Allowed {
		return &contentplugins.QuotaDenied{Result: q}
	}
	return nil
}
func audit(ctx context.Context, tx pgx.Tx, actor string, scope sdk.Scope, action, id string, summary any) error {
	slog.InfoContext(ctx, action, "resource", id)
	return database.Audit(ctx, tx, actor, scope.AccountID, action, "intelligence", id, "intelligence-operation", map[string]any{}, summary)
}
func failure(e error) string {
	if e == nil {
		return ""
	}
	var p *provider.Failure
	var q *contentplugins.QuotaDenied
	switch {
	case errors.As(e, &p):
		return p.Code
	case errors.As(e, &q):
		return q.Result.Code
	case errors.Is(e, database.ErrForbidden), errors.Is(e, sdk.ErrPermission):
		return "ACCESS_DENIED"
	case errors.Is(e, database.ErrConflict):
		return "STALE_OR_CONFLICT"
	case errors.Is(e, sdk.ErrState):
		return "PLUGIN_UNAVAILABLE"
	case errors.Is(e, database.ErrInvalid):
		return "INVALID_INPUT"
	}
	return "EXECUTION_FAILED"
}
