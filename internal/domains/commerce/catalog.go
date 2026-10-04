package commerce

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
)

type Service struct {
	DB    *database.Store
	Clock Clock
}

func (s *Service) now() time.Time {
	if s.Clock == nil {
		return UTCClock{}.Now()
	}
	return s.Clock.Now().UTC().Truncate(time.Microsecond)
}
func (s *Service) admin(ctx context.Context, actor, account string, fn func(pgx.Tx) error) error {
	return s.DB.Tenant(ctx, actor, account, true, func(tx pgx.Tx) error {
		if !database.PlatformPermission(ctx, tx, actor, "platform.commercial.manage") {
			return database.ErrForbidden
		}
		if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(7100213)"); e != nil {
			return e
		}
		return fn(tx)
	})
}
func readCatalog(ctx context.Context, tx pgx.Tx, now time.Time) (Catalog, error) {
	rows, e := tx.Query(ctx, `SELECT DISTINCT ON(kind,key) kind,key,version,enabled,effective_at,definition FROM commercial_catalog WHERE effective_at<=$1 ORDER BY kind,key,version DESC`, now)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	c := Catalog{}
	for rows.Next() {
		var v Version
		var b []byte
		if e = rows.Scan(&v.Kind, &v.Key, &v.Version, &v.Enabled, &v.EffectiveAt, &b); e != nil {
			return nil, e
		}
		if e = decodeStrict(b, &v.Definition); e != nil || !v.Validate() {
			return nil, database.ErrInvalid
		}
		if c[v.Kind] == nil {
			c[v.Kind] = map[string]Version{}
		}
		c[v.Kind][v.Key] = v
	}
	return c, rows.Err()
}
func (s *Service) Catalog(ctx context.Context, actor, account string) (Catalog, error) {
	var out Catalog
	e := s.DB.Tenant(ctx, actor, account, true, func(tx pgx.Tx) error {
		if !database.PlatformPermission(ctx, tx, actor, "platform.commercial.read") {
			return database.ErrForbidden
		}
		var e error
		out, e = readCatalog(ctx, tx, s.now())
		return e
	})
	return out, e
}
func (s *Service) SaveCatalog(ctx context.Context, actor, account, correlation string, in CatalogChange) (Version, error) {
	v := in.Version
	now := s.now()
	if v.EffectiveAt.IsZero() {
		v.EffectiveAt = now
	}
	v.EffectiveAt = v.EffectiveAt.UTC().Truncate(time.Microsecond)
	if !v.Validate() || in.Expected < 0 || in.Reason == "" || v.EffectiveAt.Before(now) {
		return Version{}, database.ErrInvalid
	}
	e := s.admin(ctx, actor, account, func(tx pgx.Tx) error {
		var version int64
		var last *time.Time
		if e := tx.QueryRow(ctx, "SELECT COALESCE(max(version),0),max(effective_at) FROM commercial_catalog WHERE kind=$1 AND key=$2", v.Kind, v.Key).Scan(&version, &last); e != nil {
			return e
		}
		if version != in.Expected {
			return database.ErrConflict
		}
		if last != nil && v.EffectiveAt.Before(*last) {
			return database.ErrInvalid
		}
		c, e := readCatalog(ctx, tx, v.EffectiveAt)
		if e != nil {
			return e
		}
		var plugins []string
		var metrics []string
		if v.Definition.Plan != nil {
			for k := range v.Definition.Plan.Plugins {
				plugins = append(plugins, k)
			}
			for k := range v.Definition.Plan.Limits {
				metrics = append(metrics, k)
			}
		}
		if v.Definition.Product != nil {
			plugins = v.Definition.Product.Plugins
			for k := range v.Definition.Product.Quotas {
				metrics = append(metrics, k)
			}
		}
		for _, k := range plugins {
			if member, ok := c.Get(PluginKind, k); !ok {
				return database.ErrInvalid
			} else if v.Definition.Product != nil && member.Definition.Plugin.Scope != v.Definition.Product.Scope {
				return database.ErrInvalid
			}
		}
		for _, k := range metrics {
			if metric, ok := c.Get(QuotaKind, k); !ok {
				return database.ErrInvalid
			} else if v.Definition.Product != nil && metric.Definition.Quota.Scope != v.Definition.Product.Scope {
				return database.ErrInvalid
			}
		}
		v.Version = version + 1
		if v.Kind == PlanKind {
			if !database.PlatformPermission(ctx, tx, actor, "platform.plans.manage") {
				return database.ErrForbidden
			}
			if v.Key == "FREE" && !v.Enabled {
				return database.ErrInvalid
			}
			// Keep the established plan catalog/RBAC and assignments authoritative.
			if _, e = tx.Exec(ctx, `INSERT INTO plans(key,name,active) VALUES($1,$2,$3) ON CONFLICT(key) DO UPDATE SET name=EXCLUDED.name,active=EXCLUDED.active,version=plans.version+1,updated_at=now()`, v.Key, v.Definition.Plan.Name, v.Enabled); e != nil {
				return e
			}
			// Scheduled commercial policy cannot prematurely change the legacy plan catalog.
			if v.EffectiveAt.After(now) {
				return database.ErrInvalid
			}
		}
		b, e := json.Marshal(v.Definition)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO commercial_catalog(kind,key,version,enabled,effective_at,definition,actor_id,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, v.Kind, v.Key, v.Version, v.Enabled, v.EffectiveAt, b, actor, in.Reason); e != nil {
			return e
		}
		return database.Audit(ctx, tx, actor, account, "commercial.catalog.version", "commercial_catalog", string(v.Kind)+":"+v.Key, correlation, map[string]any{"version": version}, v)
	})
	return v, e
}
