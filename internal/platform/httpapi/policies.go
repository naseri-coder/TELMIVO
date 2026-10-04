package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/policies"
)

func (a *API) policyScope(w http.ResponseWriter, r *http.Request, permission string, fn func(pgx.Tx) (any, error)) {
	if !a.DB.HasPlatformPermission(r.Context(), user(r), permission) {
		fail(w, 403, "forbidden")
		return
	}
	var out any
	e := a.DB.Tenant(r.Context(), user(r), workspace(r), true, func(tx pgx.Tx) error { var err error; out, err = fn(tx); return err })
	if e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}
func (a *API) policyList(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	if !strings.Contains("|TRIAL|QUOTA|RESOURCE|GRANT|PRICE|OFFER|PAYWALL|", "|"+kind+"|") {
		fail(w, 400, "invalid")
		return
	}
	a.policyScope(w, r, policies.Permission(kind, false), func(tx pgx.Tx) (any, error) {
		return rows(r.Context(), tx, `SELECT DISTINCT ON(key) key,version,kind,COALESCE(feature_key,'') AS feature,COALESCE(plugin_key,'') AS plugin,COALESCE(plan_key,'') AS plan,COALESCE(workspace_id::text,'') AS workspace,enabled,definition,effective_from,effective_until,reason FROM policy_versions WHERE kind=$1 ORDER BY key,version DESC`, kind)
	})
}
func (a *API) policySave(w http.ResponseWriter, r *http.Request) {
	var in policies.Change
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.policyScope(w, r, policies.Permission(in.Kind, true), func(tx pgx.Tx) (any, error) {
		return policies.Save(r.Context(), tx, user(r), workspace(r), correlation(r), in)
	})
}
func (a *API) policyPreview(w http.ResponseWriter, r *http.Request) {
	var in policies.Request
	if decode(w, r, &in) != nil || (in.Workspace != "" && in.Workspace != workspace(r)) {
		fail(w, 400, "invalid")
		return
	}
	in.Workspace = workspace(r)
	a.policyScope(w, r, "platform.access_policies.read", func(tx pgx.Tx) (any, error) {
		if in.User == "" {
			in.User = user(r)
		}
		if e := tx.QueryRow(r.Context(), "SELECT role_key FROM workspace_members WHERE workspace_id=$1 AND user_id=$2", in.Workspace, in.User).Scan(&in.Role); e != nil {
			return nil, e
		}
		if in.Permission == "" {
			if e := tx.QueryRow(r.Context(), "SELECT permission_key FROM features WHERE key=$1", in.Feature).Scan(&in.Permission); e != nil {
				return nil, e
			}
		}
		in.Now = time.Now()
		return policies.Resolve(r.Context(), tx, in)
	})
}
func (a *API) policyRuntime(w http.ResponseWriter, r *http.Request) {
	var in policies.Request
	if decode(w, r, &in) != nil || in.Plan != "" || in.User != "" || in.Role != "" || in.Workspace != "" {
		fail(w, 400, "invalid")
		return
	}
	in.User = user(r)
	in.Workspace = workspace(r)
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		if e := tx.QueryRow(r.Context(), "SELECT role_key FROM workspace_members WHERE workspace_id=$1 AND user_id=$2", in.Workspace, in.User).Scan(&in.Role); e != nil {
			return nil, e
		}
		if e := tx.QueryRow(r.Context(), "SELECT permission_key FROM features WHERE key=$1", in.Feature).Scan(&in.Permission); e != nil {
			return nil, e
		}
		return policies.Resolve(r.Context(), tx, in)
	})
}
func (a *API) safetyLimits(w http.ResponseWriter, r *http.Request) {
	a.policyScope(w, r, "platform.safety_limits.read", func(tx pgx.Tx) (any, error) {
		return rows(r.Context(), tx, "SELECT metric,limit_value::text AS limit_value,unit,description FROM infrastructure_safety_caps ORDER BY metric")
	})
}
func (a *API) adminPermissions(w http.ResponseWriter, r *http.Request) {
	rs, e := a.DB.Pool.Query(r.Context(), "SELECT permission_key FROM platform_admin_permissions WHERE user_id=$1 ORDER BY permission_key", user(r))
	if e != nil {
		handleError(w, e)
		return
	}
	defer rs.Close()
	out := []string{}
	for rs.Next() {
		var k string
		if e = rs.Scan(&k); e != nil {
			handleError(w, e)
			return
		}
		out = append(out, k)
	}
	if e = rs.Err(); e != nil {
		handleError(w, e)
		return
	}
	respond(w, 200, out)
}

// Expose catalog metadata through the existing plugin endpoint, never duplicate its registry.
func pluginGovernanceRows(r *http.Request, tx pgx.Tx) (any, error) {
	return rows(r.Context(), tx, "SELECT plugin_key,key,permission_key FROM plugin_capabilities ORDER BY key")
}

// Operational counters are scoped to the authorized workspace and use bounded states.
func (a *API) operations(w http.ResponseWriter, r *http.Request) {
	a.policyScope(w, r, "platform.safety_limits.read", func(tx pgx.Tx) (any, error) {
		jobs, e := rows(r.Context(), tx, "SELECT state,count(*) AS count FROM media_jobs WHERE workspace_id=$1 GROUP BY state ORDER BY state", workspace(r))
		if e != nil {
			return nil, e
		}
		quotas, e := rows(r.Context(), tx, "SELECT state,count(*) AS count FROM quota_reservations WHERE workspace_id=$1 GROUP BY state ORDER BY state", workspace(r))
		if e != nil {
			return nil, e
		}
		leases, e := rows(r.Context(), tx, "SELECT count(*) FILTER(WHERE terminal_state IS NULL AND expires_at<=now()) AS expired,count(*) FILTER(WHERE generation>1) AS reclaimed FROM job_leases WHERE workspace_id=$1", workspace(r))
		if e != nil {
			return nil, e
		}
		outbox, e := rows(r.Context(), tx, "SELECT count(*) FILTER(WHERE published_at IS NULL) AS pending,count(*) FILTER(WHERE attempts>1) AS retried FROM outbox_events WHERE workspace_id=$1", workspace(r))
		if e != nil {
			return nil, e
		}
		dlq, e := rows(r.Context(), tx, "SELECT count(*) AS count FROM dead_letter_events")
		if e != nil {
			return nil, e
		}
		return map[string]any{"dlq": dlq, "jobs": jobs, "quota_reservations": quotas, "leases": leases, "outbox": outbox}, nil
	})
}
