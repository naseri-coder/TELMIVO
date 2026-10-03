package entitlements

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/policies"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/entitlement"
)

func EffectivePlan(ctx context.Context, tx pgx.Tx, workspace string) (string, error) {
	var plan string
	e := tx.QueryRow(ctx, "SELECT CASE WHEN scheduled_at<=now() THEN scheduled_plan_key WHEN expires_at<=now() THEN previous_plan_key ELSE plan_key END FROM subscriptions WHERE workspace_id=$1", workspace).Scan(&plan)
	return plan, e
}
func Resolve(ctx context.Context, tx pgx.Tx, workspace, role, feature, plugin, action string, requested int64) (entitlement.Decision, error) {
	var owner string
	if e := tx.QueryRow(ctx, "SELECT owner_id::text FROM workspaces WHERE id=$1", workspace).Scan(&owner); e != nil {
		return entitlement.Decision{}, e
	}
	return ResolveForUser(ctx, tx, owner, workspace, role, feature, plugin, action, requested)
}
func ResolveForUser(ctx context.Context, tx pgx.Tx, user, workspace, role, feature, plugin, action string, requested int64) (entitlement.Decision, error) {
	d, e := policies.Resolve(ctx, tx, policies.Request{User: user, Workspace: workspace, Role: role, Feature: feature, Plugin: plugin, Permission: action, Requested: requested})
	return entitlement.Decision{Allowed: d.Allowed, Reason: d.Reason, Limit: d.Limit, Usage: d.Usage, Policy: d}, e
}
func Role(ctx context.Context, tx pgx.Tx, user, workspace string) (string, error) {
	var role string
	e := tx.QueryRow(ctx, "SELECT role_key FROM workspace_members WHERE workspace_id=$1 AND user_id=$2", workspace, user).Scan(&role)
	return role, e
}
func Require(ctx context.Context, tx pgx.Tx, user, workspace, feature, permission string, n int64) error {
	role, e := Role(ctx, tx, user, workspace)
	if e != nil {
		return e
	}
	d, e := ResolveForUser(ctx, tx, user, workspace, role, feature, "", permission, n)
	if e != nil {
		return e
	}
	if !d.Allowed {
		return &Denied{Reason: d.Reason}
	}
	return nil
}

type Denied struct{ Reason string }

func (e *Denied) Error() string { return e.Reason }
func Consume(ctx context.Context, tx pgx.Tx, workspace, feature string, n int64) error {
	if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", workspace+":"+feature); e != nil {
		return e
	}
	var window string
	if e := tx.QueryRow(ctx, "SELECT reset_window FROM features WHERE key=$1", feature).Scan(&window); e != nil {
		return e
	}
	period := "month"
	interval := "1 month"
	if window == "DAY" {
		period = "day"
		interval = "1 day"
	} else if window != "MONTH" {
		return database.ErrInvalid
	}
	_, e := tx.Exec(ctx, "INSERT INTO usage_counters(workspace_id,feature_key,used,period_start,period_end) VALUES($1,$2,$3,date_trunc($4,now()),date_trunc($4,now())+$5::interval) ON CONFLICT(workspace_id,feature_key,period_start) DO UPDATE SET used=usage_counters.used+EXCLUDED.used", workspace, feature, n, period, interval)
	return e
}
func Reserve(ctx context.Context, tx pgx.Tx, user, workspace, feature, permission string, n int64, operation ...string) error {
	if n < 1 {
		return &Denied{Reason: "INVALID_USAGE"}
	}
	if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", workspace+":"+feature); e != nil {
		return e
	}
	role, e := Role(ctx, tx, user, workspace)
	if e != nil {
		return e
	}
	resolved, e := policies.Resolve(ctx, tx, policies.Request{User: user, Workspace: workspace, Role: role, Feature: feature, Permission: permission, Requested: n})
	if e != nil {
		return e
	}
	if len(resolved.Resources) > 0 {
		if len(operation) != 1 {
			return database.ErrInvalid
		}
		_, e = policies.Reserve(ctx, tx, policies.Request{User: user, Workspace: workspace, Role: role, Feature: feature, Permission: permission, Requested: n}, operation[0])
		if e != nil {
			return e
		}
		return policies.Settle(ctx, tx, workspace, operation[0], true)
	}
	if e := Require(ctx, tx, user, workspace, feature, permission, n); e != nil {
		return e
	}
	var kind string
	if e := tx.QueryRow(ctx, "SELECT limit_kind FROM features WHERE key=$1", feature).Scan(&kind); e != nil {
		return e
	}
	if kind == "RESOURCE_COUNT" || kind == "NONE" {
		return nil
	}
	return Consume(ctx, tx, workspace, feature, n)
}
