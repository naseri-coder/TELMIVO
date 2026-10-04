package pluginplatform

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
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
		if err := lockBot(ctx, tx, bound.actor, bound.scope.AccountID, bound.scope.BotID); err != nil {
			return err
		}
		all, err := records(ctx, tx, bound.scope.AccountID, bound.scope.BotID)
		if err != nil {
			return err
		}
		i, ok := find(all, bound.scope.PluginID)
		if !ok || i.State != sdk.Active {
			return sdk.ErrState
		}
		m, err := s.Registry.Manifest(bound.scope.PluginID)
		if err != nil {
			return err
		}
		if !sdk.Contains(m.Permissions, capability) || !sdk.Contains(i.Capabilities, capability) {
			return sdk.ErrPermission
		}
		if err := s.checkActiveAdmission(ctx, bound.actor, all); err != nil {
			return err
		}
		if _, err := s.admit(ctx, bound.actor, bound.scope, sdk.Execute, capability); err != nil {
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
