package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/naseri-coder/TELMIVO/db/migrations"
	"sort"
	"strings"
)

var ErrForbidden = errors.New("permission denied")
var ErrConflict = errors.New("conflicting operation")

type Store struct{ Pool *pgxpool.Pool }

func Open(ctx context.Context, dsn string, requireRestricted bool) (*Store, error) {
	p, e := pgxpool.New(ctx, dsn)
	if e != nil {
		return nil, errors.New("database configuration invalid")
	}
	if e = p.Ping(ctx); e != nil {
		p.Close()
		return nil, errors.New("database unavailable")
	}
	if requireRestricted {
		var dangerous bool
		e = p.QueryRow(ctx, "SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname=current_user").Scan(&dangerous)
		if e != nil || dangerous {
			p.Close()
			return nil, errors.New("runtime database role must not bypass RLS")
		}
	}
	return &Store{Pool: p}, nil
}
func (s *Store) Migrate(ctx context.Context) error {
	conn, e := s.Pool.Acquire(ctx)
	if e != nil {
		return e
	}
	defer conn.Release()
	if _, e = conn.Exec(ctx, "SELECT pg_advisory_lock(7100101)"); e != nil {
		return e
	}
	defer conn.Exec(context.Background(), "SELECT pg_advisory_unlock(7100101)")
	if _, e = conn.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations(name text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())"); e != nil {
		return e
	}
	fs, e := migrations.Files.ReadDir(".")
	if e != nil {
		return e
	}
	sort.Slice(fs, func(i, j int) bool { return fs[i].Name() < fs[j].Name() })
	for _, f := range fs {
		if !strings.HasSuffix(f.Name(), ".sql") {
			continue
		}
		b, e := migrations.Files.ReadFile(f.Name())
		if e != nil {
			return e
		}
		h := sha256.Sum256(b)
		checksum := hex.EncodeToString(h[:])
		var previous string
		e = conn.QueryRow(ctx, "SELECT checksum FROM schema_migrations WHERE name=$1", f.Name()).Scan(&previous)
		if e == nil {
			if previous != checksum {
				return fmt.Errorf("released migration drift: %s", f.Name())
			}
			continue
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		tx, e := conn.Begin(ctx)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, string(b)); e == nil {
			_, e = tx.Exec(ctx, "INSERT INTO schema_migrations(name,checksum) VALUES($1,$2)", f.Name(), checksum)
		}
		if e != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("migration %s failed: %w", f.Name(), e)
		}
		if e = tx.Commit(ctx); e != nil {
			return e
		}
	}
	return nil
}
func (s *Store) IsAdmin(ctx context.Context, user string) bool {
	var yes bool
	e := s.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM platform_admins WHERE user_id=$1)", user).Scan(&yes)
	return e == nil && yes
}
func Scope(ctx context.Context, tx pgx.Tx, workspace string) error {
	_, e := tx.Exec(ctx, "SELECT set_config('app.workspace_id',$1,true)", workspace)
	return e
}
func (s *Store) Tenant(ctx context.Context, user, workspace string, adminAllowed bool, fn func(pgx.Tx) error) error {
	if user == "" || workspace == "" {
		return ErrForbidden
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var member bool
	e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM workspace_members WHERE user_id=$1 AND workspace_id=$2)", user, workspace).Scan(&member)
	if e != nil {
		return ErrForbidden
	}
	if !member && !(adminAllowed && s.IsAdmin(ctx, user)) {
		return ErrForbidden
	}
	if e = Scope(ctx, tx, workspace); e != nil {
		return e
	}
	if e = fn(tx); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func Audit(ctx context.Context, tx pgx.Tx, actor, workspace, action, entity, id, correlation string, old, next any) error {
	_, e := tx.Exec(ctx, "INSERT INTO audit_logs(actor_id,workspace_id,action,entity,entity_id,correlation_id,old_summary,new_summary) VALUES($1,$2,$3,$4,$5,$6,$7,$8)", actor, workspace, action, entity, id, correlation, old, next)
	return e
}
func Event(ctx context.Context, tx pgx.Tx, workspace, subject, kind, correlation string, payload any) error {
	_, e := tx.Exec(ctx, "INSERT INTO outbox_events(workspace_id,subject,event_type,correlation_id,payload) VALUES($1,$2,$3,$4,$5)", workspace, subject, kind, correlation, payload)
	return e
}
func Permission(ctx context.Context, tx pgx.Tx, user, workspace, permission string) bool {
	var yes bool
	e := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM workspace_members m JOIN role_permissions p ON p.role_key=m.role_key WHERE m.user_id=$1 AND m.workspace_id=$2 AND p.permission_key=$3)", user, workspace, permission).Scan(&yes)
	return e == nil && yes
}
