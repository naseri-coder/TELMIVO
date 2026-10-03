package policies

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/entitlement"
)

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if s == v {
			return true
		}
	}
	return false
}

// Resolve is used by runtime decisions, reservations and administration preview.
// Plan overrides are a preview-only request input; callers must authorize simulation.
func Resolve(ctx context.Context, tx pgx.Tx, r Request) (Decision, error) {
	d := Decision{Feature: r.Feature, Plugin: r.Plugin, Resources: map[string]Resource{}, Trials: []Trial{}, Capabilities: []string{}, Sources: []string{"PLAN"}, Offers: []Version{}}
	if r.Now.IsZero() {
		r.Now = time.Now()
	}
	if r.Requested < 0 {
		return d, database.ErrInvalid
	}
	if r.Group != "" {
		var valid bool
		if e := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM groups WHERE workspace_id=$1 AND id::text=$2)", r.Workspace, r.Group).Scan(&valid); e != nil {
			return d, e
		}
		if !valid {
			return d, database.ErrForbidden
		}
	}
	if r.Installation != "" {
		var valid bool
		if e := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM customer_bots WHERE workspace_id=$1 AND id::text=$2)", r.Workspace, r.Installation).Scan(&valid); e != nil {
			return d, e
		}
		if !valid {
			return d, database.ErrForbidden
		}
	}
	if e := SetUser(ctx, tx, r.User); e != nil {
		return d, e
	}
	if r.Plan == "" {
		if e := tx.QueryRow(ctx, "SELECT CASE WHEN scheduled_at<=now() THEN scheduled_plan_key WHEN expires_at<=now() THEN previous_plan_key ELSE plan_key END FROM subscriptions WHERE workspace_id=$1", r.Workspace).Scan(&r.Plan); e != nil {
			return d, e
		}
	}
	var i entitlement.Input
	i.Now = r.Now
	i.Requested = r.Requested
	i.PluginAllowed = true
	i.DependenciesValid = true
	e := tx.QueryRow(ctx, `SELECT ff.enabled,pe.enabled,pe.limit_value,EXISTS(SELECT 1 FROM role_permissions WHERE role_key=$3 AND permission_key=$4) FROM feature_flags ff JOIN plan_entitlements pe ON pe.feature_key=ff.feature_key JOIN plans p ON p.key=pe.plan_key WHERE ff.feature_key=$1 AND pe.plan_key=$2 AND p.active`, r.Feature, r.Plan, r.Role, r.Permission).Scan(&i.Global, &i.Plan.Enabled, &i.Plan.Limit, &i.Permission)
	if e == pgx.ErrNoRows {
		d.Reason = "FEATURE_UNCONFIGURED"
		return d, nil
	}
	if e != nil {
		return d, e
	}
	vs, e := versions(ctx, tx, r.Now)
	if e != nil {
		return d, e
	}
	grants := []Definition{}
	pluginGrants := []string{}
	rs, e := tx.Query(ctx, `SELECT policy_key,policy_version,ends_at,snapshot FROM trial_enrollments WHERE (scope='PER_USER' AND user_id=$1) OR (scope='PER_WORKSPACE' AND subject_id=$2) OR (scope='PER_GROUP' AND subject_id=$3) OR (scope='PER_INSTALLATION' AND subject_id=$4) ORDER BY started_at,policy_key`, r.User, r.Workspace, r.Group, r.Installation)
	if e != nil {
		return d, e
	}
	for rs.Next() {
		var t Trial
		var raw []byte
		if e = rs.Scan(&t.Key, &t.Version, &t.EndsAt, &raw); e != nil {
			rs.Close()
			return d, e
		}
		g, err := decodeDefinition(raw)
		if err != nil {
			rs.Close()
			return d, err
		}
		t.Fallback = g.Fallback
		t.State = "EXPIRED"
		if r.Now.Before(t.EndsAt) {
			t.State = "ACTIVE"
			grants = append(grants, g)
			pluginGrants = append(pluginGrants, g.Plugins...)
			d.Sources = append(d.Sources, g.GrantKind)
		}
		d.Trials = append(d.Trials, t)
	}
	e = rs.Err()
	rs.Close()
	if e != nil {
		return d, e
	}
	for _, v := range vs {
		if v.Kind == "GRANT" && v.Workspace == r.Workspace {
			grants = append(grants, v.Definition)
			pluginGrants = append(pluginGrants, v.Definition.Plugins...)
			d.Sources = append(d.Sources, v.Definition.GrantKind)
		}
	}
	for _, g := range grants {
		if contains(g.Features, r.Feature) {
			i.Plan.Enabled = true
		}
	}
	for _, p := range []struct {
		table string
		dest  **entitlement.Rule
	}{{"temporary_grants", &i.Grant}, {"workspace_entitlement_overrides", &i.Override}} {
		var rule entitlement.Rule
		e = tx.QueryRow(ctx, "SELECT enabled,limit_value,expires_at FROM "+p.table+" WHERE workspace_id=$1 AND feature_key=$2", r.Workspace, r.Feature).Scan(&rule.Enabled, &rule.Limit, &rule.ExpiresAt)
		if e == nil {
			*p.dest = &rule
			d.Sources = append(d.Sources, strings.ToUpper(p.table))
		} else if e != pgx.ErrNoRows {
			return d, e
		}
	}
	caps := map[string]bool{}
	if r.Plugin != "" {
		var enabled, planAllowed bool
		var required string
		e = tx.QueryRow(ctx, "SELECT status IN('ENABLED','BETA'),required_feature,EXISTS(SELECT 1 FROM plan_plugin_entitlements WHERE plan_key=$2 AND plugin_key=$1 AND enabled) FROM plugins WHERE key=$1", r.Plugin, r.Plan).Scan(&enabled, &required, &planAllowed)
		if e != nil {
			return d, e
		}
		i.PluginAllowed = enabled && required == r.Feature && (planAllowed || contains(pluginGrants, r.Plugin))
		deps, e := tx.Query(ctx, `WITH RECURSIVE deps(k) AS (SELECT dependency_key FROM plugin_dependencies WHERE plugin_key=$1 UNION SELECT dependency_key FROM plugin_dependencies JOIN deps ON plugin_key=deps.k) SELECT k,p.status IN('ENABLED','BETA'),COALESCE(pe.enabled,false) FROM deps JOIN plugins p ON p.key=k LEFT JOIN plan_plugin_entitlements pe ON pe.plugin_key=k AND pe.plan_key=$2`, r.Plugin, r.Plan)
		if e != nil {
			return d, e
		}
		for deps.Next() {
			var k string
			var on, allowed bool
			if e = deps.Scan(&k, &on, &allowed); e != nil {
				deps.Close()
				return d, e
			}
			if !on || (!allowed && !contains(pluginGrants, k)) {
				i.DependenciesValid = false
			}
		}
		e = deps.Err()
		deps.Close()
		if e != nil {
			return d, e
		}
		for _, g := range grants {
			if contains(g.Plugins, r.Plugin) {
				for _, c := range g.Capabilities {
					caps[c] = true
				}
			}
		}
	}
	resources := map[string]Version{}
	// Plan resource definitions precede workspace replacements; ties are deterministic by key.
	sort.SliceStable(vs, func(a, b int) bool {
		if vs[a].Definition.Source == vs[b].Definition.Source {
			return vs[a].Key < vs[b].Key
		}
		return vs[a].Definition.Source == "PLAN"
	})
	for _, v := range vs {
		if !one(v.Kind, "QUOTA", "RESOURCE") || v.Feature != r.Feature || (v.Plugin != "" && v.Plugin != r.Plugin) || (v.Plan != "" && v.Plan != r.Plan) || (v.Workspace != "" && v.Workspace != r.Workspace) {
			continue
		}
		if v.Kind == "RESOURCE" && v.Definition.Source == "WORKSPACE_OVERRIDE" {
			continue
		}
		resources[v.Definition.Metric] = v
		for _, c := range v.Definition.Capabilities {
			caps[c] = true
		}
	}
	// Grants augment the selected metric's base; they cannot invent an unconfigured metric.
	for _, metric := range sortedKeys(resources) {
		v := resources[metric]
		for _, g := range grants {
			if l, ok := g.Limits[metric]; ok {
				v.Definition.Limit, e = Merge(v.Definition.Limit, l.Amount, l.Strategy)
				if e != nil {
					return d, e
				}
			}
		}
		resources[metric] = v
	}
	for _, v := range vs {
		if v.Kind == "RESOURCE" && v.Definition.Source == "WORKSPACE_OVERRIDE" && v.Workspace == r.Workspace && v.Feature == r.Feature && (v.Plugin == "" || v.Plugin == r.Plugin) {
			base, ok := resources[v.Definition.Metric]
			if !ok {
				return d, database.ErrInvalid
			}
			base.Definition.Limit, e = Merge(base.Definition.Limit, v.Definition.Limit, v.Definition.Strategy)
			if e != nil {
				return d, e
			}
			resources[v.Definition.Metric] = base
			d.Sources = append(d.Sources, "WORKSPACE_OVERRIDE")
		}
	}
	if r.Plugin != "" {
		capsRows, err := tx.Query(ctx, "SELECT metric,unit FROM infrastructure_safety_caps WHERE metric LIKE $1", r.Plugin+".%")
		if err != nil {
			return d, err
		}
		for capsRows.Next() {
			var metric, unit string
			if err = capsRows.Scan(&metric, &unit); err != nil {
				capsRows.Close()
				return d, err
			}
			v, ok := resources[metric]
			if !ok || v.Definition.Unit != unit {
				capsRows.Close()
				d.Reason = "SAFETY_POLICY_UNCONFIGURED"
				return d, nil
			}
		}
		err = capsRows.Err()
		capsRows.Close()
		if err != nil {
			return d, err
		}
	}
	for _, metric := range sortedKeys(resources) {
		v := resources[metric]
		x := Resource{Primary: v.Kind == "QUOTA", Limit: v.Definition.Limit, Unit: v.Definition.Unit, Window: v.Definition.Window, Mode: v.Definition.Mode, TTL: v.Definition.ReservationSeconds, CountFailure: v.Definition.CountFailure, PolicyKey: v.Key, PolicyVersion: v.Version}
		if e = tx.QueryRow(ctx, "SELECT limit_value FROM infrastructure_safety_caps WHERE metric=$1", metric).Scan(&x.SafetyCap); e != nil && e != pgx.ErrNoRows {
			return d, e
		}
		if x.SafetyCap != nil {
			x.Limit = min(x.Limit, *x.SafetyCap)
		}
		x.Bucket, x.ResetAt, e = Bucket(v.Definition, r.Now)
		if e != nil {
			return d, e
		}
		if x.Mode != "ITEM" {
			if one(v.Definition.Window, "CALENDAR_DAY", "CALENDAR_WEEK", "CALENDAR_MONTH") {
				start, err := time.Parse(time.RFC3339, strings.SplitN(x.Bucket, ":", 3)[2])
				if err != nil {
					return d, err
				}
				e = tx.QueryRow(ctx, "SELECT COALESCE(sum(amount) FILTER(WHERE state='COMMITTED'),0)::bigint,COALESCE(sum(amount) FILTER(WHERE state='RESERVED' AND expires_at>$4),0)::bigint FROM quota_reservations WHERE workspace_id=$1 AND metric=$2 AND created_at>=$3 AND created_at<$5", r.Workspace, metric, start, r.Now, *x.ResetAt).Scan(&x.Used, &x.Reserved)
				if e != nil {
					return d, e
				}
			} else {
				if v.Definition.Window == "ROLLING_WINDOW" {
					e = tx.QueryRow(ctx, "SELECT COALESCE(sum(amount),0)::bigint FROM quota_reservations WHERE workspace_id=$1 AND metric=$2 AND state='COMMITTED' AND settled_at>$3", r.Workspace, metric, r.Now.Add(-time.Duration(v.Definition.WindowSeconds)*time.Second)).Scan(&x.Used)
				} else if x.Mode != "CONCURRENT" {
					e = tx.QueryRow(ctx, "SELECT COALESCE((SELECT used FROM quota_usage WHERE workspace_id=$1 AND metric=$2 AND bucket=$3),0)", r.Workspace, metric, x.Bucket).Scan(&x.Used)
				}
				if e != nil {
					return d, e
				}
				e = tx.QueryRow(ctx, "SELECT COALESCE(sum(amount),0)::bigint FROM quota_reservations WHERE workspace_id=$1 AND metric=$2 AND (bucket=$3 OR $5) AND state='RESERVED' AND expires_at>$4", r.Workspace, metric, x.Bucket, r.Now, v.Definition.Window == "ROLLING_WINDOW").Scan(&x.Reserved)
				if e != nil {
					return d, e
				}
			}
		}
		x.Remaining = max(int64(0), x.Limit-x.Used-x.Reserved)
		d.Resources[metric] = x
	}
	if len(resources) == 0 {
		var kind string
		e = tx.QueryRow(ctx, "SELECT limit_kind FROM features WHERE key=$1", r.Feature).Scan(&kind)
		if e != nil {
			return d, e
		}
		switch kind {
		case "RESOURCE_COUNT":
			tables := map[string]string{"channels": "channels", "groups": "groups", "bots": "customer_bots", "team": "workspace_members"}
			table, ok := tables[r.Feature]
			if !ok {
				d.Reason = "LIMIT_UNIMPLEMENTED"
				return d, nil
			}
			e = tx.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE workspace_id=$1", r.Workspace).Scan(&i.Usage)
		case "USAGE", "CREDIT":
			e = tx.QueryRow(ctx, "SELECT COALESCE(sum(used),0)::bigint FROM usage_counters WHERE workspace_id=$1 AND feature_key=$2 AND period_start<=now() AND period_end>now()", r.Workspace, r.Feature).Scan(&i.Usage)
		case "NONE":
		case "CAPACITY", "ITEM_SIZE":
			d.Reason = "LIMIT_UNCONFIGURED"
			return d, nil
		}
		if e != nil {
			return d, e
		}
	}
	// A configured quota replaces the legacy feature counter, not an additional monthly restriction.
	if len(resources) > 0 {
		i.Plan.Limit = nil
		if i.Grant != nil {
			i.Grant.Limit = nil
		}
		if i.Override != nil {
			i.Override.Limit = nil
		}
		i.Usage = 0
	}
	base := entitlement.Resolve(i)
	d.Allowed = base.Allowed
	d.Reason = base.Reason
	if !d.Allowed && d.Reason == "FEATURE_DISABLED" {
		for _, t := range d.Trials {
			if t.State == "EXPIRED" {
				d.Reason = "TRIAL_EXPIRED"
				break
			}
		}
	}
	d.Limit = base.Limit
	d.Usage = base.Usage
	for _, c := range sortedKeys(caps) {
		var allowed bool
		e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM plugin_capabilities c JOIN role_permissions rp ON rp.permission_key=c.permission_key WHERE c.key=$1 AND c.plugin_key=$2 AND rp.role_key=$3)", c, r.Plugin, r.Role).Scan(&allowed)
		if e != nil {
			return d, e
		}
		if allowed {
			d.Capabilities = append(d.Capabilities, c)
		}
	}
	for metric, amount := range r.Amounts {
		if _, ok := d.Resources[metric]; !ok || amount < 0 {
			return d, database.ErrInvalid
		}
	}
	if d.Allowed && r.Capability != "" && !contains(d.Capabilities, r.Capability) {
		d.Allowed = false
		d.Reason = "CAPABILITY_DISABLED"
	}
	if d.Allowed && r.RequireReservation && !reservable(d.Resources, r.Requested, r.Amounts) {
		d.Allowed = false
		d.Reason = "QUOTA_POLICY_UNCONFIGURED"
	}
	for _, metric := range sortedKeys(d.Resources) {
		x := d.Resources[metric]
		amount := r.Amounts[metric]
		if x.Mode == "CONCURRENT" && r.Requested > 0 {
			amount = max(amount, 1)
		}
		if resources[metric].Kind == "QUOTA" {
			amount = max(amount, r.Requested)
			d.Limit = &x.Limit
			d.Usage = x.Used
			d.Reserved = x.Reserved
			d.Remaining = &x.Remaining
			d.ResetAt = x.ResetAt
		}
		if amount < 0 {
			return d, database.ErrInvalid
		}
		if d.Allowed && amount > 0 && amount > x.Remaining {
			d.Allowed = false
			d.Reason = "QUOTA_EXCEEDED"
		}
	}
	if !d.Allowed {
		for _, v := range vs {
			if v.Kind == "PAYWALL" && v.Feature == r.Feature && contains(v.Definition.Reasons, d.Reason) {
				d.Paywall = v.Key
				for _, offerKey := range v.Definition.Offers {
					for _, o := range vs {
						if o.Key == offerKey && o.Kind == "OFFER" {
							d.Offers = append(d.Offers, o)
						}
					}
				}
				break
			}
		}
	}
	sort.Strings(d.Sources)
	return d, nil
}
