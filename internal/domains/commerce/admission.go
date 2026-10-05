package commerce

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
)

func (s *Service) Decide(ctx context.Context, actor string, scope sdk.Scope) (Decision, error) {
	var d Decision
	now := s.now()
	e := s.DB.Tenant(ctx, actor, scope.AccountID, false, func(tx pgx.Tx) error {
		if e := requireBot(ctx, tx, scope.AccountID, scope.BotID); e != nil {
			return e
		}
		c, e := readCatalog(ctx, tx, now)
		if e != nil {
			return e
		}
		as, e := readAgreements(ctx, tx, scope.AccountID)
		if e != nil {
			return e
		}
		p, e := effectivePlan(ctx, tx, scope.AccountID, now)
		if e != nil {
			return e
		}
		d = Evaluate(c, as, p, scope.AccountID, scope.BotID, scope.PluginID, now)
		return nil
	})
	return d, e
}

// Authorize implements the existing R3D-1 contract. Commercial access and capability
// authorization are both required at install/activation and every invocation.
func (s *Service) Authorize(ctx context.Context, in sdk.Admission) (sdk.Grant, error) {
	var grant sdk.Grant
	e := s.DB.Tenant(ctx, in.ActorID, in.Scope.AccountID, false, func(tx pgx.Tx) error { var e error; grant, e = s.AuthorizeTx(ctx, tx, in); return e })
	return grant, e
}

// AuthorizeTx is trusted Core composition, allowing admission to see an atomically
// created bot without leaking a database handle into the Plugin SDK.
func (s *Service) AuthorizeTx(ctx context.Context, tx pgx.Tx, in sdk.Admission) (sdk.Grant, error) {
	var grant sdk.Grant
	now := s.now()
	if in.Manifest.PluginID != in.Scope.PluginID || in.Scope.BotID == "" {
		return grant, sdk.ErrPermission
	}
	switch in.Action {
	case sdk.Install, sdk.Activate, sdk.Execute:
	default:
		return grant, sdk.ErrPermission
	}
	if !database.Permission(ctx, tx, in.ActorID, in.Scope.AccountID, "bots.manage") {
		return grant, database.ErrForbidden
	}
	if e := requireBot(ctx, tx, in.Scope.AccountID, in.Scope.BotID); e != nil {
		return grant, e
	}
	c, e := readCatalog(ctx, tx, now)
	if e != nil {
		return grant, e
	}
	as, e := readAgreements(ctx, tx, in.Scope.AccountID)
	if e != nil {
		return grant, e
	}
	p, e := effectivePlan(ctx, tx, in.Scope.AccountID, now)
	if e != nil {
		return grant, e
	}
	d := Evaluate(c, as, p, in.Scope.AccountID, in.Scope.BotID, in.Scope.PluginID, now)
	if !d.Allowed {
		return grant, &Denied{Decision: d}
	}
	v, _ := c.Get(PluginKind, in.Scope.PluginID)
	if (in.Action != sdk.Execute || in.Capability != "") && Metadata(v.Definition).Operational != "OPERATIONAL" {
		return grant, sdk.ErrPermission
	}
	if in.Action != sdk.Execute || in.Capability != "" {
		for _, dep := range in.Manifest.Dependencies {
			if policy, ok := c.Get(PluginKind, dep.PluginID); !ok || Metadata(policy.Definition).Operational != "OPERATIONAL" {
				return grant, sdk.ErrDependency
			}
		}
	}
	for _, cap := range in.Manifest.Permissions {
		if !sdk.KnownCapability(cap) || !contains(v.Definition.Plugin.Capabilities, cap) {
			return grant, sdk.ErrPermission
		}
	}
	if in.Capability != "" && !contains(in.Manifest.Permissions, in.Capability) {
		return grant, sdk.ErrPermission
	}
	grant = sdk.Grant{Capabilities: append([]string{}, in.Manifest.Permissions...), LicenseScope: d.Scope, PolicyRevision: d.Revision}
	return grant, nil
}

var _ sdk.AdmissionPolicy = (*Service)(nil)
