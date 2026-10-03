package policies

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
)

func Permission(kind string, write bool) string {
	if kind == "OFFER" && write {
		return "platform.offers.manage"
	}
	if one(kind, "PRICE", "OFFER", "PAYWALL") {
		if write {
			return "platform.commercial.manage"
		}
		return "platform.commercial.read"
	}
	if write {
		return "platform.access_policies.manage"
	}
	return "platform.access_policies.read"
}
func versions(ctx context.Context, tx pgx.Tx, now time.Time) ([]Version, error) {
	rs, e := tx.Query(ctx, `SELECT DISTINCT ON(key) key,version,kind,COALESCE(feature_key,''),COALESCE(plugin_key,''),COALESCE(plan_key,''),COALESCE(workspace_id::text,''),enabled,definition,effective_from,effective_until,reason FROM policy_versions WHERE effective_from<=$1 ORDER BY key,version DESC`, now)
	if e != nil {
		return nil, e
	}
	defer rs.Close()
	out := []Version{}
	for rs.Next() {
		var v Version
		var b []byte
		if e = rs.Scan(&v.Key, &v.Version, &v.Kind, &v.Feature, &v.Plugin, &v.Plan, &v.Workspace, &v.Enabled, &b, &v.EffectiveFrom, &v.EffectiveUntil, &v.Reason); e != nil {
			return nil, e
		}
		v.Definition, e = decodeDefinition(b)
		if e != nil {
			return nil, e
		}
		if v.Enabled && (v.EffectiveUntil == nil || now.Before(*v.EffectiveUntil)) {
			out = append(out, v)
		}
	}
	return out, rs.Err()
}

// Optimistic version check plus a per-key lock makes concurrent policy administration explicit.
func Save(ctx context.Context, tx pgx.Tx, actor, workspace, correlation string, in Change) (Version, error) {
	v := in.Version
	if !database.PlatformPermission(ctx, tx, actor, Permission(v.Kind, true)) {
		return v, database.ErrForbidden
	}
	if e := Validate(v); e != nil {
		return v, e
	}
	if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "policy:"+v.Key); e != nil {
		return v, e
	}
	var prior int64
	var kind string
	e := tx.QueryRow(ctx, "SELECT version,kind FROM policy_versions WHERE key=$1 ORDER BY version DESC LIMIT 1", v.Key).Scan(&prior, &kind)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return v, e
	}
	if prior != in.ExpectedVersion || (prior > 0 && kind != v.Kind) {
		return v, database.ErrConflict
	}
	if v.Kind == "QUOTA" || v.Kind == "RESOURCE" {
		var overlap bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM (SELECT DISTINCT ON(key) * FROM policy_versions ORDER BY key,version DESC) p WHERE key<>$1 AND kind=$2 AND feature_key IS NOT DISTINCT FROM NULLIF($3,'') AND plugin_key IS NOT DISTINCT FROM NULLIF($4,'') AND plan_key IS NOT DISTINCT FROM NULLIF($5,'') AND workspace_id IS NOT DISTINCT FROM NULLIF($6,'')::uuid AND definition->>'metric'=$7 AND enabled AND (effective_until IS NULL OR effective_until>$8) AND (definition->>'source' IS NOT DISTINCT FROM NULLIF($9,'')))`, v.Key, v.Kind, v.Feature, v.Plugin, v.Plan, v.Workspace, v.Definition.Metric, v.EffectiveFrom, v.Definition.Source).Scan(&overlap)
		if e != nil {
			return v, e
		}
		if overlap {
			return v, database.ErrConflict
		}
	}
	if v.Kind == "OFFER" {
		var valid bool
		e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM policy_versions WHERE key=$1 AND version=$2 AND kind='PRICE') AND EXISTS(SELECT 1 FROM plans WHERE key=$3 AND active)", v.Definition.PriceKey, v.Definition.PriceVersion, v.Definition.Destination).Scan(&valid)
		if e != nil {
			return v, e
		}
		if !valid {
			return v, database.ErrInvalid
		}
	}
	if v.Kind == "PAYWALL" {
		for _, offer := range v.Definition.Offers {
			var valid bool
			if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM policy_versions WHERE key=$1 AND kind='OFFER')", offer).Scan(&valid); e != nil {
				return v, e
			}
			if !valid {
				return v, database.ErrInvalid
			}
		}
	}
	for _, feature := range v.Definition.Features {
		var valid bool
		if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM features WHERE key=$1)", feature).Scan(&valid); e != nil {
			return v, e
		}
		if !valid {
			return v, database.ErrInvalid
		}
	}
	for _, plugin := range v.Definition.Plugins {
		var valid bool
		if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM plugins WHERE key=$1)", plugin).Scan(&valid); e != nil {
			return v, e
		}
		if !valid {
			return v, database.ErrInvalid
		}
	}
	for _, capability := range v.Definition.Capabilities {
		var valid bool
		if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM plugin_capabilities WHERE key=$1)", capability).Scan(&valid); e != nil {
			return v, e
		}
		if !valid {
			return v, database.ErrInvalid
		}
	}
	v.Version = prior + 1
	_, e = tx.Exec(ctx, `INSERT INTO policy_versions(key,version,kind,feature_key,plugin_key,plan_key,workspace_id,enabled,definition,effective_from,effective_until,actor_id,reason) VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,'')::uuid,$8,$9,$10,$11,$12,$13)`, v.Key, v.Version, v.Kind, v.Feature, v.Plugin, v.Plan, v.Workspace, v.Enabled, v.Definition, v.EffectiveFrom, v.EffectiveUntil, actor, v.Reason)
	if e != nil {
		return v, e
	}
	e = database.Audit(ctx, tx, actor, workspace, "policy.version.create", v.Kind, v.Key, correlation, map[string]any{"version": prior}, map[string]any{"version": v.Version, "effective_from": v.EffectiveFrom, "reason": v.Reason})
	return v, e
}

type Subject struct {
	User, Workspace, Group, Installation, Source, Event string
	New                                                 bool
	Now                                                 time.Time
}

func SetUser(ctx context.Context, tx pgx.Tx, user string) error {
	_, e := tx.Exec(ctx, "SELECT set_config('app.user_id',$1,true)", user)
	return e
}

// Enrollment never updates a previous snapshot; once-per-subject uniqueness spans versions and entry points.
func Enroll(ctx context.Context, tx pgx.Tx, s Subject, trigger string) error {
	if s.Now.IsZero() {
		s.Now = time.Now()
	}
	if e := SetUser(ctx, tx, s.User); e != nil {
		return e
	}
	if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "trial-user:"+s.User); e != nil {
		return e
	}
	vs, e := versions(ctx, tx, s.Now)
	if e != nil {
		return e
	}
	for _, v := range vs {
		d := v.Definition
		if v.Kind != "TRIAL" || d.Trigger != trigger || (d.Eligibility == "NEW_USER" && !s.New) {
			continue
		}
		subject := ""
		switch d.Scope {
		case "PER_USER":
			subject = s.User
		case "PER_WORKSPACE":
			subject = s.Workspace
		case "PER_GROUP":
			subject = s.Group
		case "PER_INSTALLATION":
			subject = s.Installation
		}
		if subject == "" {
			continue
		}
		var claimed bool
		if d.Scope == "PER_GROUP" {
			if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM group_claims WHERE group_id::text=$1 AND workspace_id=$2)", subject, s.Workspace).Scan(&claimed); e != nil {
				return e
			}
			if !claimed {
				return database.ErrForbidden
			}
		}
		if d.Scope == "PER_INSTALLATION" {
			var valid bool
			if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM customer_bots WHERE workspace_id=$1 AND id::text=$2)", s.Workspace, subject).Scan(&valid); e != nil {
				return e
			}
			if !valid {
				return database.ErrForbidden
			}
		}
		cycle := s.Event
		if d.Once {
			cycle = "ONCE"
		}
		if cycle == "" {
			return database.ErrInvalid
		}
		if d.MaxSubjects > 0 {
			var n int64
			if e = tx.QueryRow(ctx, "SELECT count(*) FROM trial_enrollments WHERE policy_key=$1 AND user_id=$2", v.Key, s.User).Scan(&n); e != nil {
				return e
			}
			if n >= d.MaxSubjects {
				continue
			}
		}
		if d.Stacking == "KEEP_FIRST" {
			var active bool
			if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM trial_enrollments WHERE scope=$1 AND subject_id=$2 AND ends_at>$3)", d.Scope, subject, s.Now).Scan(&active); e != nil {
				return e
			}
			if active {
				continue
			}
		}
		_, e = tx.Exec(ctx, `INSERT INTO trial_enrollments(policy_key,policy_version,workspace_id,user_id,scope,subject_id,cycle_key,source,started_at,ends_at,snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(policy_key,scope,subject_id,cycle_key) DO NOTHING`, v.Key, v.Version, s.Workspace, s.User, d.Scope, subject, cycle, s.Source, s.Now, s.Now.Add(time.Duration(d.Duration)*time.Second), d)
		if e != nil {
			return e
		}
	}
	return nil
}
func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
func fingerprint(in any) string { b, _ := json.Marshal(in); return string(b) }
