package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"net/http"
	"regexp"
	"time"
)

var keyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,63}$`)

func (a *API) adminMutation(w http.ResponseWriter, r *http.Request, action, entity, id string, fn func(pgx.Tx) (any, error)) {
	a.tenant(w, r, true, func(tx pgx.Tx) (any, error) {
		if _, e := tx.Exec(r.Context(), "SELECT pg_advisory_xact_lock(7100201)"); e != nil {
			return nil, e
		}
		before, old, e := catalogRevision(r.Context(), tx, entity, id, action)
		if e != nil {
			return nil, e
		}
		out, e := fn(tx)
		if e != nil {
			return nil, e
		}
		if action == "plan.entitlement.edit" || action == "plan.plugin.edit" {
			if _, e = tx.Exec(r.Context(), "UPDATE plans SET version=version+1,updated_at=now() WHERE key=$1", id); e != nil {
				return nil, e
			}
		}
		after, next, e := catalogRevision(r.Context(), tx, entity, id, action)
		if e != nil {
			return nil, e
		}
		if after == 0 && before > 0 {
			after = before + 1
		}
		e = database.Audit(r.Context(), tx, user(r), workspace(r), action, entity, id, correlation(r), map[string]any{"version": before, "state": old}, map[string]any{"version": after, "state": next, "result": out, "effective_at": time.Now()})
		return out, e
	})
}
func catalogRevision(ctx context.Context, tx pgx.Tx, entity, id, action string) (int64, any, error) {
	query := ""
	switch entity {
	case "plan":
		query = "SELECT version,jsonb_build_object('name',name,'active',active) FROM plans WHERE key=$1"
	case "plugin":
		query = "SELECT configuration_version,jsonb_build_object('module_version',version,'status',status) FROM plugins WHERE key=$1"
	case "feature":
		query = "SELECT configuration_version,jsonb_build_object('enabled',enabled,'status',rollout_status) FROM feature_flags WHERE feature_key=$1"
	case "entitlement":
		table := "workspace_entitlement_overrides"
		if action == "grant.set" || action == "grant.revoke" {
			table = "temporary_grants"
		}
		query = "SELECT version,jsonb_build_object('enabled',enabled,'limit',limit_value,'expires_at',expires_at) FROM " + table + " WHERE feature_key=$1"
	}
	if query == "" {
		return 0, nil, nil
	}
	var revision int64
	var raw []byte
	e := tx.QueryRow(ctx, query, id).Scan(&revision, &raw)
	if e == pgx.ErrNoRows {
		return 0, nil, nil
	}
	return revision, json.RawMessage(raw), e
}
func (a *API) createPlan(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Key   string `json:"key"`
		Name  string `json:"name"`
		Clone string `json:"clone"`
	}
	if decode(w, r, &in) != nil || !keyPattern.MatchString(in.Key) || len(in.Name) < 1 || len(in.Name) > 200 {
		fail(w, 400, "invalid")
		return
	}
	a.adminMutation(w, r, "plan.create", "plan", in.Key, func(tx pgx.Tx) (any, error) {
		if _, e := tx.Exec(r.Context(), "INSERT INTO plans(key,name) VALUES($1,$2)", in.Key, in.Name); e != nil {
			return nil, e
		}
		if in.Clone != "" {
			var exists bool
			if e := tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM plans WHERE key=$1)", in.Clone).Scan(&exists); e != nil || !exists {
				return nil, errors.New("source plan missing")
			}
			if _, e := tx.Exec(r.Context(), "INSERT INTO plan_entitlements SELECT $1,feature_key,enabled,limit_value FROM plan_entitlements WHERE plan_key=$2", in.Key, in.Clone); e != nil {
				return nil, e
			}
			if _, e := tx.Exec(r.Context(), "INSERT INTO plan_plugin_entitlements SELECT $1,plugin_key,enabled FROM plan_plugin_entitlements WHERE plan_key=$2", in.Key, in.Clone); e != nil {
				return nil, e
			}
		} else {
			if _, e := tx.Exec(r.Context(), "INSERT INTO plan_entitlements SELECT $1,key,false,NULL FROM features", in.Key); e != nil {
				return nil, e
			}
			if _, e := tx.Exec(r.Context(), "INSERT INTO plan_plugin_entitlements SELECT $1,key,false FROM plugins", in.Key); e != nil {
				return nil, e
			}
		}
		return in, nil
	})
}
func (a *API) editPlan(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Active  bool   `json:"active"`
		Name    string `json:"name"`
		Version int64  `json:"version"`
	}
	if decode(w, r, &in) != nil || len(in.Name) < 1 || len(in.Name) > 200 {
		fail(w, 400, "invalid")
		return
	}
	a.adminMutation(w, r, "plan.edit", "plan", r.PathValue("plan"), func(tx pgx.Tx) (any, error) {
		if !in.Active {
			var used bool
			if e := tx.QueryRow(r.Context(), "SELECT platform_plan_in_use($1,$2)", user(r), r.PathValue("plan")).Scan(&used); e != nil {
				return nil, e
			}
			if used || r.PathValue("plan") == "FREE" {
				return nil, database.ErrConflict
			}
		}
		tag, e := tx.Exec(r.Context(), "UPDATE plans SET active=$2,name=$3,version=version+1,updated_at=now() WHERE key=$1 AND version=$4", r.PathValue("plan"), in.Active, in.Name, in.Version)
		if e != nil {
			return nil, e
		}
		if tag.RowsAffected() != 1 {
			return nil, database.ErrConflict
		}
		return in, nil
	})
}
func (a *API) editEntitlement(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool    `json:"enabled"`
		Limit   *string `json:"limit"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	limit, e := parseLimit(in.Limit)
	if e != nil {
		fail(w, 400, "invalid")
		return
	}
	a.adminMutation(w, r, "plan.entitlement.edit", "plan", r.PathValue("plan"), func(tx pgx.Tx) (any, error) {
		_, e := tx.Exec(r.Context(), "INSERT INTO plan_entitlements(plan_key,feature_key,enabled,limit_value) VALUES($1,$2,$3,$4) ON CONFLICT(plan_key,feature_key) DO UPDATE SET enabled=EXCLUDED.enabled,limit_value=EXCLUDED.limit_value", r.PathValue("plan"), r.PathValue("feature"), in.Enabled, limit)
		if e != nil {
			return nil, e
		}
		return map[string]any{"feature": r.PathValue("feature"), "enabled": in.Enabled, "limit": in.Limit}, nil
	})
}
func validatePlugins(ctx context.Context, tx pgx.Tx) error {
	var bad bool
	e := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM plan_plugin_entitlements pe JOIN plugin_dependencies d ON d.plugin_key=pe.plugin_key LEFT JOIN plan_plugin_entitlements dependency ON dependency.plan_key=pe.plan_key AND dependency.plugin_key=d.dependency_key WHERE pe.enabled AND NOT COALESCE(dependency.enabled,false)) OR EXISTS(SELECT 1 FROM plugins p JOIN plugin_dependencies d ON d.plugin_key=p.key JOIN plugins dependency ON dependency.key=d.dependency_key WHERE p.status IN('ENABLED','BETA') AND dependency.status='DISABLED')").Scan(&bad)
	if e != nil {
		return e
	}
	if bad {
		return database.ErrConflict
	}
	return nil
}
func (a *API) editPluginEntitlement(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	a.adminMutation(w, r, "plan.plugin.edit", "plan", r.PathValue("plan"), func(tx pgx.Tx) (any, error) {
		_, e := tx.Exec(r.Context(), "INSERT INTO plan_plugin_entitlements(plan_key,plugin_key,enabled) VALUES($1,$2,$3) ON CONFLICT(plan_key,plugin_key) DO UPDATE SET enabled=EXCLUDED.enabled", r.PathValue("plan"), r.PathValue("plugin"), in.Enabled)
		if e != nil {
			return nil, e
		}
		if e = validatePlugins(r.Context(), tx); e != nil {
			return nil, e
		}
		return map[string]any{"plugin": r.PathValue("plugin"), "enabled": in.Enabled}, nil
	})
}
func (a *API) editPluginStatus(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status string `json:"status"`
	}
	if decode(w, r, &in) != nil || (in.Status != "DISABLED" && in.Status != "ENABLED" && in.Status != "BETA") {
		fail(w, 400, "invalid")
		return
	}
	a.adminMutation(w, r, "plugin.status", "plugin", r.PathValue("plugin"), func(tx pgx.Tx) (any, error) {
		tag, e := tx.Exec(r.Context(), "UPDATE plugins SET status=$2,configuration_version=configuration_version+1,updated_at=now() WHERE key=$1", r.PathValue("plugin"), in.Status)
		if e != nil {
			return nil, e
		}
		if tag.RowsAffected() != 1 {
			return nil, pgx.ErrNoRows
		}
		if e = validatePlugins(r.Context(), tx); e != nil {
			return nil, e
		}
		return in, nil
	})
}
func (a *API) editFlag(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool   `json:"enabled"`
		Status  string `json:"status"`
	}
	if decode(w, r, &in) != nil || (in.Status != "ON" && in.Status != "OFF" && in.Status != "BETA") || in.Enabled == (in.Status == "OFF") {
		fail(w, 400, "invalid")
		return
	}
	a.adminMutation(w, r, "feature.flag.edit", "feature", r.PathValue("feature"), func(tx pgx.Tx) (any, error) {
		tag, e := tx.Exec(r.Context(), "UPDATE feature_flags SET enabled=$2,rollout_status=$3,configuration_version=configuration_version+1,updated_at=now() WHERE feature_key=$1", r.PathValue("feature"), in.Enabled, in.Status)
		if e != nil {
			return nil, e
		}
		if tag.RowsAffected() != 1 {
			return nil, pgx.ErrNoRows
		}
		return in, nil
	})
}

type accessRule struct {
	Enabled   bool       `json:"enabled"`
	Limit     *string    `json:"limit"`
	ExpiresAt *time.Time `json:"expires_at"`
	Reason    string     `json:"reason"`
}

func (a *API) setRule(w http.ResponseWriter, r *http.Request, table, action string, temporary bool) {
	var in accessRule
	if decode(w, r, &in) != nil || len(in.Reason) < 1 || len(in.Reason) > 1000 || (temporary && in.ExpiresAt == nil) || (in.ExpiresAt != nil && !in.ExpiresAt.After(time.Now())) {
		fail(w, 400, "invalid")
		return
	}
	limit, e := parseLimit(in.Limit)
	if e != nil {
		fail(w, 400, "invalid")
		return
	}
	a.adminMutation(w, r, action, "entitlement", r.PathValue("feature"), func(tx pgx.Tx) (any, error) {
		_, e := tx.Exec(r.Context(), "INSERT INTO "+table+"(workspace_id,feature_key,enabled,limit_value,expires_at,reason) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(workspace_id,feature_key) DO UPDATE SET enabled=EXCLUDED.enabled,limit_value=EXCLUDED.limit_value,expires_at=EXCLUDED.expires_at,reason=EXCLUDED.reason,version="+table+".version+1", workspace(r), r.PathValue("feature"), in.Enabled, limit, in.ExpiresAt, in.Reason)
		return in, e
	})
}
func (a *API) override(w http.ResponseWriter, r *http.Request) {
	a.setRule(w, r, "workspace_entitlement_overrides", "override.set", false)
}
func (a *API) grant(w http.ResponseWriter, r *http.Request) {
	a.setRule(w, r, "temporary_grants", "grant.set", true)
}
func (a *API) removeRule(w http.ResponseWriter, r *http.Request, table, action string) {
	a.adminMutation(w, r, action, "entitlement", r.PathValue("feature"), func(tx pgx.Tx) (any, error) {
		tag, e := tx.Exec(r.Context(), "DELETE FROM "+table+" WHERE workspace_id=$1 AND feature_key=$2", workspace(r), r.PathValue("feature"))
		if e != nil {
			return nil, e
		}
		if tag.RowsAffected() != 1 {
			return nil, pgx.ErrNoRows
		}
		return map[string]bool{"removed": true}, nil
	})
}
func (a *API) removeOverride(w http.ResponseWriter, r *http.Request) {
	a.removeRule(w, r, "workspace_entitlement_overrides", "override.remove")
}
func (a *API) removeGrant(w http.ResponseWriter, r *http.Request) {
	a.removeRule(w, r, "temporary_grants", "grant.revoke")
}
