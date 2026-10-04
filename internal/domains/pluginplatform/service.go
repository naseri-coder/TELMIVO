package pluginplatform

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
)

// Service is an opt-in domain entry point; existing R3 transports do not call it yet.
type Service struct {
	DB       *database.Store
	Registry *Registry
	Policy   sdk.AdmissionPolicy
	Hooks    sdk.LifecycleValidator
}

func (s *Service) admit(ctx context.Context, actor string, scope sdk.Scope, action sdk.Action, capability string) (sdk.Grant, error) {
	if s.Policy == nil {
		return sdk.Grant{}, sdk.ErrPermission
	}
	m, err := s.Registry.Manifest(scope.PluginID)
	if err != nil {
		return sdk.Grant{}, err
	}
	g, err := s.Policy.Authorize(ctx, sdk.Admission{ActorID: actor, Scope: scope, Action: action, Capability: capability, Manifest: m.Clone()})
	if err != nil {
		return sdk.Grant{}, err
	}
	if err := sdk.ValidateGrant(m, g); err != nil {
		return sdk.Grant{}, err
	}
	if capability != "" && !sdk.Contains(g.Capabilities, capability) {
		return sdk.Grant{}, sdk.ErrPermission
	}
	return g, nil
}

func (s *Service) checkActiveAdmission(ctx context.Context, actor string, records []sdk.Installation) error {
	if err := s.Registry.validateRecords(records); err != nil {
		return err
	}
	for _, i := range records {
		if i.State == sdk.Active {
			if _, err := s.admit(ctx, actor, i.Scope, sdk.Execute, ""); err != nil {
				return err
			}
		}
	}
	return nil
}

// Change owns its transaction. ExpectedRevision=0 is required for first install.
// Stale requests fail; configuration can only be supplied at initial install.
func (s *Service) Change(ctx context.Context, actor string, scope sdk.Scope, action sdk.Action, expectedRevision int64, configuration json.RawMessage, correlation string) (sdk.Installation, error) {
	var out sdk.Installation
	if s.DB == nil {
		return out, database.ErrInvalid
	}
	err := s.DB.Tenant(ctx, actor, scope.AccountID, false, func(tx pgx.Tx) error {
		var e error
		out, e = s.ChangeTx(ctx, tx, actor, scope, action, expectedRevision, configuration, correlation)
		return e
	})
	if err != nil {
		return sdk.Installation{}, err
	}
	return out, nil
}

// ChangeTx preserves the same authorization, bot lock, validation and audit as
// Change. Trusted Core callers may compose creation and lifecycle in one tenant
// transaction; plugins never receive this transaction. The caller commits it.
func (s *Service) ChangeTx(ctx context.Context, tx pgx.Tx, actor string, scope sdk.Scope, action sdk.Action, expectedRevision int64, configuration json.RawMessage, correlation string) (sdk.Installation, error) {
	var result sdk.Installation
	if s.DB == nil || s.Registry == nil || !sdk.ValidID(scope.PluginID) || expectedRevision < 0 || correlation == "" || len(correlation) > 150 {
		return result, database.ErrInvalid
	}
	m, err := s.Registry.Manifest(scope.PluginID)
	if err != nil {
		return result, err
	}
	if err := lockBot(ctx, tx, actor, scope.AccountID, scope.BotID); err != nil {
		return sdk.Installation{}, err
	}
	all, err := records(ctx, tx, scope.AccountID, scope.BotID)
	if err != nil {
		return sdk.Installation{}, err
	}
	i, exists := find(all, scope.PluginID)
	if !exists {
		i = sdk.Installation{Scope: scope, State: sdk.Available}
	}
	if i.Revision != expectedRevision {
		return sdk.Installation{}, database.ErrConflict
	}
	next, err := Transition(i.State, action)
	if err != nil {
		return sdk.Installation{}, err
	}
	if action == sdk.Install {
		if err := m.Settings.ValidateConfiguration(configuration); err != nil {
			return sdk.Installation{}, err
		}
		i.Configuration = append(json.RawMessage(nil), configuration...)
		i.Data = json.RawMessage(`{}`)
		i.Version, i.DataVersion = m.Version, m.DataVersion
	} else if len(configuration) != 0 {
		return sdk.Installation{}, sdk.ErrConfiguration
	}
	// Activation requires an exact installed version and data-schema match.
	// Inactive records never execute upgrade or cleanup code.
	if action == sdk.Activate && (i.Version != m.Version || i.DataVersion != m.DataVersion) {
		return sdk.Installation{}, sdk.ErrVersion
	}
	if action == sdk.Install || action == sdk.Activate {
		g, err := s.admit(ctx, actor, scope, action, "")
		if err != nil {
			return sdk.Installation{}, err
		}
		i.Capabilities = append([]string{}, g.Capabilities...)
		i.LicenseScope, i.PolicyRevision = g.LicenseScope, g.PolicyRevision
	}
	active := activeSet(all)
	active[scope.PluginID] = next == sdk.Active
	if err := s.Registry.ValidateActive(active); err != nil {
		return sdk.Installation{}, err
	}
	if action == sdk.Activate {
		if err := s.checkActiveAdmission(ctx, actor, all); err != nil {
			return sdk.Installation{}, err
		}
	}
	for _, h := range m.LifecycleHooks {
		if h == action {
			if s.Hooks == nil {
				return sdk.Installation{}, sdk.ErrState
			}
			// Copy the snapshot so a validator cannot replace persisted state or data.
			b, _ := json.Marshal(i)
			var snapshot sdk.Installation
			_ = json.Unmarshal(b, &snapshot)
			if err := s.Hooks.Validate(ctx, action, snapshot, m.Clone()); err != nil {
				return sdk.Installation{}, err
			}
		}
	}
	i.State = next
	if err := persist(ctx, tx, i, expectedRevision); err != nil {
		return sdk.Installation{}, err
	}
	if err := database.Audit(ctx, tx, actor, scope.AccountID, "plugin.lifecycle", "bot_plugin", scope.BotID+":"+scope.PluginID, correlation, map[string]any{"revision": expectedRevision}, map[string]any{"state": next, "version": i.Version}); err != nil {
		return sdk.Installation{}, err
	}
	i.Revision++
	return i, nil
}

func (s *Service) View(ctx context.Context, actor string, scope sdk.Scope) (sdk.Installation, error) {
	var out sdk.Installation
	if s.DB == nil || s.Registry == nil {
		return out, database.ErrInvalid
	}
	if _, err := s.Registry.Manifest(scope.PluginID); err != nil {
		return out, err
	}
	err := s.DB.Tenant(ctx, actor, scope.AccountID, false, func(tx pgx.Tx) error {
		if err := lockBot(ctx, tx, actor, scope.AccountID, scope.BotID); err != nil {
			return err
		}
		all, err := records(ctx, tx, scope.AccountID, scope.BotID)
		if err != nil {
			return err
		}
		var exists bool
		out, exists = find(all, scope.PluginID)
		if !exists {
			out = sdk.Installation{Scope: scope, State: sdk.Available}
		}
		return nil
	})
	return out, err
}
