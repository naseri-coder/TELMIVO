// Package supportplugins brokers infrastructure for independent CRM/Ticketing plugins.
package supportplugins

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/commerce"
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	platform "github.com/naseri-coder/TELMIVO/internal/domains/pluginplatform"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/plugins/crm"
	tickets "github.com/naseri-coder/TELMIVO/internal/plugins/ticketing"
	"log/slog"
	"strings"
	"time"
)

type Clock interface{ Now() time.Time }
type Service struct {
	DB         *database.Store
	Core       *platform.Service
	Commercial *commerce.Service
	Provider   contentplugins.Provider
	Clock      Clock
}

func (s *Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock.Now().UTC().Truncate(time.Microsecond)
	}
	return time.Now().UTC().Truncate(time.Microsecond)
}
func body(v any) []byte        { b, _ := json.Marshal(v); return b }
func fingerprint(v any) string { h := sha256.Sum256(body(v)); return hex.EncodeToString(h[:]) }
func reference() string {
	b := make([]byte, 24)
	if _, e := rand.Read(b); e != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func keyValid(k string) bool { return len(k) > 0 && len(k) <= 100 && strings.TrimSpace(k) == k }
func (s *Service) PrepareTx(ctx context.Context, tx pgx.Tx, scope sdk.Scope) error {
	if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock_shared(7100213)"); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,7100213))", scope.AccountID)
	return e
}
func principal(ctx context.Context, tx pgx.Tx, scope sdk.Scope) (string, error) {
	var actor string
	e := tx.QueryRow(ctx, `SELECT m.user_id::text FROM workspace_members m JOIN role_permissions p ON p.role_key=m.role_key AND p.permission_key='bots.manage' WHERE m.workspace_id=$1 AND m.role_key='OWNER' ORDER BY m.user_id LIMIT 1`, scope.AccountID).Scan(&actor)
	return actor, e
}
func (s *Service) check(ctx context.Context, tx pgx.Tx, scope sdk.Scope, cap string, running bool) (string, error) {
	actor, e := principal(ctx, tx, scope)
	if e != nil {
		return "", e
	}
	e = s.Core.ValidateExecutionTx(ctx, tx, actor, scope, cap, running)
	return actor, e
}
func (s *Service) tenant(ctx context.Context, actor string, scope sdk.Scope, permission, cap string, fn func(pgx.Tx, string) error) error {
	return s.DB.Tenant(ctx, actor, scope.AccountID, false, func(tx pgx.Tx) error {
		if !database.Permission(ctx, tx, actor, scope.AccountID, permission) {
			return database.ErrForbidden
		}
		if e := s.PrepareTx(ctx, tx, scope); e != nil {
			return e
		}
		p, e := s.check(ctx, tx, scope, cap, false)
		if e != nil {
			return e
		}
		return fn(tx, p)
	})
}
func (s *Service) quota(ctx context.Context, tx pgx.Tx, p string, scope sdk.Scope, metric string, amount int64, consume bool) error {
	if s.Commercial == nil {
		return sdk.ErrPermission
	}
	r, e := s.Commercial.QuotaTx(ctx, tx, p, scope.AccountID, scope.BotID, metric, amount, consume)
	if e != nil {
		return e
	}
	if !r.Allowed {
		return &contentplugins.QuotaDenied{Result: r}
	}
	return nil
}

// Current occupancy uses the same generic limit evaluator; account lock serializes recount.
func (s *Service) occupancy(ctx context.Context, tx pgx.Tx, p string, scope sdk.Scope, metric string, count int64) error {
	return s.quota(ctx, tx, p, scope, metric, count, false)
}
func audit(ctx context.Context, tx pgx.Tx, actor string, scope sdk.Scope, action, id string, summary any) error {
	slog.InfoContext(ctx, action, "resource", id)
	return database.Audit(ctx, tx, actor, scope.AccountID, action, "support_plugin", id, "support-plugin-operation", map[string]any{}, summary)
}
func (s *Service) Invoke(context.Context, sdk.Scope, string, json.RawMessage) (json.RawMessage, error) {
	return nil, sdk.ErrPermission
}
func (s *Service) InvokeTx(ctx context.Context, tx pgx.Tx, actor string, scope sdk.Scope, cap string, payload json.RawMessage) (json.RawMessage, error) {
	if scope.PluginID == crm.ID && cap == "crm.write" {
		var r crm.ResolveRequest
		if json.Unmarshal(payload, &r) != nil {
			return nil, database.ErrInvalid
		}
		c, e := s.resolve(ctx, tx, actor, scope, r)
		return body(c), e
	}
	if scope.PluginID == tickets.ID && cap == "tickets.write" {
		var r tickets.Inbound
		if json.Unmarshal(payload, &r) != nil {
			return nil, database.ErrInvalid
		}
		t, e := s.receive(ctx, tx, actor, scope, r)
		return body(t), e
	}
	return nil, sdk.ErrPermission
}
func (s *Service) Inbound(ctx context.Context, scope sdk.Scope, in tickets.Inbound) (tickets.Ticket, error) {
	var actor string
	tx, e := s.DB.Pool.Begin(ctx)
	if e != nil {
		return tickets.Ticket{}, e
	}
	defer tx.Rollback(ctx)
	if e = database.Scope(ctx, tx, scope.AccountID); e != nil {
		return tickets.Ticket{}, e
	}
	actor, e = principal(ctx, tx, scope)
	if e != nil {
		return tickets.Ticket{}, e
	}
	// Release the identity lookup before the runtime owns its transaction.
	if e = tx.Rollback(ctx); e != nil {
		return tickets.Ticket{}, e
	}
	scope.PluginID = tickets.ID
	return (tickets.Plugin{Runtime: (&platform.Runtime{Service: s.Core, Handler: s}).Bind(actor, scope)}).Receive(ctx, in)
}
func noRows(e error) bool { return errors.Is(e, pgx.ErrNoRows) }
