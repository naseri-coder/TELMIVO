package entitlements

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/entitlement"
	"time"
)

func EffectivePlan(ctx context.Context, tx pgx.Tx, workspace string) (string, error) {
	var plan string
	e := tx.QueryRow(ctx, "SELECT CASE WHEN scheduled_at<=now() THEN scheduled_plan_key WHEN expires_at<=now() THEN previous_plan_key ELSE plan_key END FROM subscriptions WHERE workspace_id=$1", workspace).Scan(&plan)
	return plan, e
}
func Resolve(ctx context.Context, tx pgx.Tx, workspace, role, feature, plugin, action string, requested int64) (entitlement.Decision, error) {
	var i entitlement.Input
	i.Now = time.Now()
	i.Requested = requested
	i.PluginAllowed = true
	i.DependenciesValid = true
	plan, e := EffectivePlan(ctx, tx, workspace)
	if e != nil {
		return entitlement.Decision{}, e
	}
	e = tx.QueryRow(ctx, "SELECT f.enabled,e.enabled,e.limit_value,EXISTS(SELECT 1 FROM role_permissions rp WHERE rp.role_key=$3 AND rp.permission_key=$4) FROM feature_flags f JOIN plan_entitlements e ON e.feature_key=f.feature_key JOIN plans p ON p.key=e.plan_key WHERE f.feature_key=$1 AND e.plan_key=$2 AND p.active", feature, plan, role, action).Scan(&i.Global, &i.Plan.Enabled, &i.Plan.Limit, &i.Permission)
	if e != nil {
		return entitlement.Decision{Reason: "FEATURE_UNCONFIGURED"}, nil
	}
	for _, x := range []struct {
		table string
		dest  **entitlement.Rule
	}{{"temporary_grants", &i.Grant}, {"workspace_entitlement_overrides", &i.Override}} {
		var r entitlement.Rule
		e = tx.QueryRow(ctx, "SELECT enabled,limit_value,expires_at FROM "+x.table+" WHERE workspace_id=$1 AND feature_key=$2", workspace, feature).Scan(&r.Enabled, &r.Limit, &r.ExpiresAt)
		if e == nil {
			*x.dest = &r
		} else if e != pgx.ErrNoRows {
			return entitlement.Decision{}, e
		}
	}
	e = tx.QueryRow(ctx, "SELECT COALESCE(sum(used),0)::bigint FROM usage_counters WHERE workspace_id=$1 AND feature_key=$2 AND period_start<=now() AND period_end>now()", workspace, feature).Scan(&i.Usage)
	if e != nil {
		return entitlement.Decision{}, e
	}
	if plugin != "" {
		e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM plan_plugin_entitlements pe JOIN plugins p ON p.key=pe.plugin_key WHERE pe.plan_key=$1 AND p.key=$2 AND pe.enabled AND p.status IN('ENABLED','BETA') AND p.required_feature=$3)", plan, plugin, feature).Scan(&i.PluginAllowed)
		if e != nil {
			return entitlement.Decision{}, e
		}
		e = tx.QueryRow(ctx, "WITH RECURSIVE deps(k) AS (SELECT dependency_key FROM plugin_dependencies WHERE plugin_key=$2 UNION SELECT d.dependency_key FROM plugin_dependencies d JOIN deps ON d.plugin_key=deps.k) SELECT NOT EXISTS(SELECT 1 FROM deps LEFT JOIN plan_plugin_entitlements pe ON pe.plugin_key=deps.k AND pe.plan_key=$1 LEFT JOIN plugins p ON p.key=deps.k WHERE COALESCE(pe.enabled,false)=false OR p.status='DISABLED')", plan, plugin).Scan(&i.DependenciesValid)
		if e != nil {
			return entitlement.Decision{}, e
		}
	}
	return entitlement.Resolve(i), nil
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
	d, e := Resolve(ctx, tx, workspace, role, feature, "", permission, n)
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
	_, e := tx.Exec(ctx, "INSERT INTO usage_counters(workspace_id,feature_key,used,period_start,period_end) VALUES($1,$2,$3,date_trunc('month',now()),date_trunc('month',now())+interval '1 month') ON CONFLICT(workspace_id,feature_key,period_start) DO UPDATE SET used=usage_counters.used+EXCLUDED.used", workspace, feature, n)
	return e
}
func Reserve(ctx context.Context, tx pgx.Tx, user, workspace, feature, permission string, n int64) error {
	if n < 1 {
		return &Denied{Reason: "INVALID_USAGE"}
	}
	if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", workspace+":"+feature); e != nil {
		return e
	}
	if e := Require(ctx, tx, user, workspace, feature, permission, n); e != nil {
		return e
	}
	return Consume(ctx, tx, workspace, feature, n)
}
