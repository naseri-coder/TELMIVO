package pluginplatform

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
)

// CapabilityHandler is trusted Core code. It must validate resources inside the
// supplied scope and must not accept tenant/bot overrides from a plugin's payload.
// No external effect is implemented here; transactional/outbox adapters come later.
type CapabilityHandler interface {
	Invoke(context.Context, sdk.Scope, string, json.RawMessage) (json.RawMessage, error)
}

type Runtime struct {
	Service *Service
	Handler CapabilityHandler
}

type session struct {
	runtime *Runtime
	actor   string
	scope   sdk.Scope
}

func (r *Runtime) Bind(actor string, scope sdk.Scope) sdk.Invoker {
	return session{runtime: r, actor: actor, scope: scope}
}

func (bound session) Invoke(ctx context.Context, capability string, payload json.RawMessage) (json.RawMessage, error) {
	r := bound.runtime
	if r == nil || r.Service == nil || r.Service.DB == nil || r.Service.Registry == nil || r.Handler == nil {
		return nil, sdk.ErrPermission
	}
	if !sdk.KnownCapability(capability) {
		return nil, sdk.ErrPermission
	}
	if len(payload) > 65536 || !json.Valid(payload) {
		return nil, sdk.ErrConfiguration
	}
	s := r.Service
	var result json.RawMessage
	err := s.DB.Tenant(ctx, bound.actor, bound.scope.AccountID, false, func(tx pgx.Tx) error {
		var err error
		if h, ok := r.Handler.(TransactionalCapabilityHandler); ok {
			if err := h.PrepareTx(ctx, tx, bound.scope); err != nil {
				return err
			}
		}
		if err := s.ValidateExecutionTx(ctx, tx, bound.actor, bound.scope, capability, true); err != nil {
			return err
		}
		if h, ok := r.Handler.(TransactionalCapabilityHandler); ok {
			result, err = h.InvokeTx(ctx, tx, bound.actor, bound.scope, capability, append(json.RawMessage(nil), payload...))
			return err
		}
		result, err = r.Handler.Invoke(ctx, bound.scope, capability, append(json.RawMessage(nil), payload...))
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// TransactionalCapabilityHandler is trusted Core composition, never a Plugin SDK handle.
type TransactionalCapabilityHandler interface {
	PrepareTx(context.Context, pgx.Tx, sdk.Scope) error
	InvokeTx(context.Context, pgx.Tx, string, sdk.Scope, string, json.RawMessage) (json.RawMessage, error)
}

type transactionalAdmission interface {
	AuthorizeTx(context.Context, pgx.Tx, sdk.Admission) (sdk.Grant, error)
}

type executionPolicy struct {
	policy transactionalAdmission
	tx     pgx.Tx
}

func (p executionPolicy) Authorize(ctx context.Context, in sdk.Admission) (sdk.Grant, error) {
	return p.policy.AuthorizeTx(ctx, p.tx, in)
}

// ValidateExecutionTx retains the runtime's admission checks for trusted composed operations.
// Management can configure active plugins on DRAFT bots; external effects require RUNNING.
func (s *Service) ValidateExecutionTx(ctx context.Context, tx pgx.Tx, actor string, scope sdk.Scope, capability string, running bool) error {
	// Trusted transactional policies reuse the caller's connection. Parallel
	// runtime requests must not exhaust the pool awaiting nested admission.
	if p, ok := s.Policy.(transactionalAdmission); ok {
		bound := *s
		bound.Policy = executionPolicy{policy: p, tx: tx}
		s = &bound
	}
	if err := lockBot(ctx, tx, actor, scope.AccountID, scope.BotID); err != nil {
		return err
	}
	var botStatus string
	if err := tx.QueryRow(ctx, "SELECT status FROM customer_bots WHERE workspace_id=$1 AND id::text=$2", scope.AccountID, scope.BotID).Scan(&botStatus); err != nil {
		return err
	}
	if running && botStatus != "RUNNING" {
		return sdk.ErrState
	}
	all, err := records(ctx, tx, scope.AccountID, scope.BotID)
	if err != nil {
		return err
	}
	i, ok := find(all, scope.PluginID)
	if !ok || i.State != sdk.Active {
		return sdk.ErrState
	}
	m, err := s.Registry.Manifest(scope.PluginID)
	if err != nil {
		return err
	}
	if !sdk.Contains(m.Permissions, capability) || !sdk.Contains(i.Capabilities, capability) {
		return sdk.ErrPermission
	}
	if err := s.checkActiveAdmission(ctx, actor, all); err != nil {
		return err
	}
	if _, err := s.admit(ctx, actor, scope, sdk.Execute, capability); err != nil {
		return err
	}
	return nil
}
