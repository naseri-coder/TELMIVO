package supportplugins

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	tickets "github.com/naseri-coder/TELMIVO/internal/plugins/ticketing"
	"strings"
)

type DepartmentChange struct {
	Department tickets.Department `json:"department"`
	Key        string             `json:"idempotency_key"`
	Expected   int64              `json:"expected_revision"`
}

func (s *Service) Departments(ctx context.Context, actor string, scope sdk.Scope) ([]tickets.Department, error) {
	scope.PluginID = tickets.ID
	out := []tickets.Department{}
	e := s.tenant(ctx, actor, scope, "tickets.read", "tickets.read", func(tx pgx.Tx, p string) error {
		rs, e := tx.Query(ctx, "SELECT id::text,definition,revision FROM support_departments WHERE workspace_id=$1 AND bot_id=$2 ORDER BY (definition->>'ordering')::int,id LIMIT 100", scope.AccountID, scope.BotID)
		if e != nil {
			return e
		}
		defer rs.Close()
		for rs.Next() {
			var d tickets.Department
			var b []byte
			if e = rs.Scan(&d.ID, &b, &d.Revision); e != nil {
				return e
			}
			var def tickets.Department
			if e = json.Unmarshal(b, &def); e != nil {
				return e
			}
			def.ID = d.ID
			def.Revision = d.Revision
			out = append(out, def)
		}
		return rs.Err()
	})
	return out, e
}
func (s *Service) SaveDepartment(ctx context.Context, actor string, scope sdk.Scope, id string, in DepartmentChange) (tickets.Department, error) {
	scope.PluginID = tickets.ID
	var out tickets.Department
	d := in.Department
	d.ID = ""
	d.Revision = 0
	if !d.Valid() || !keyValid(in.Key) {
		return out, database.ErrInvalid
	}
	e := s.tenant(ctx, actor, scope, "tickets.manage", "tickets.write", func(tx pgx.Tx, p string) error {
		if id == "" {
			if in.Expected != 0 {
				return database.ErrConflict
			}
			var saved string
			e := tx.QueryRow(ctx, "SELECT id::text,fingerprint FROM support_departments WHERE workspace_id=$1 AND bot_id=$2 AND operation_key=$3", scope.AccountID, scope.BotID, in.Key).Scan(&id, &saved)
			if e == nil {
				if saved != fingerprint(d) {
					return database.ErrConflict
				}
				var err error
				out, err = s.department(ctx, tx, scope, id)
				return err
			}
			if !noRows(e) {
				return e
			}
			var n int
			if e = tx.QueryRow(ctx, "SELECT count(*) FROM support_departments WHERE workspace_id=$1 AND bot_id=$2", scope.AccountID, scope.BotID).Scan(&n); e != nil {
				return e
			}
			if n >= 100 {
				return database.ErrInvalid
			}
			e = tx.QueryRow(ctx, "INSERT INTO support_departments(workspace_id,bot_id,definition,operation_key,fingerprint) VALUES($1,$2,$3,$4,$5) RETURNING id::text", scope.AccountID, scope.BotID, body(d), in.Key, fingerprint(d)).Scan(&id)
			if e != nil {
				return e
			}
		} else {
			tag, e := tx.Exec(ctx, "UPDATE support_departments SET definition=$4,revision=revision+1 WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3 AND revision=$5", scope.AccountID, scope.BotID, id, body(d), in.Expected)
			if e != nil {
				return e
			}
			if tag.RowsAffected() != 1 {
				return database.ErrConflict
			}
		}
		if e := audit(ctx, tx, actor, scope, "ticket.department.saved", id, map[string]any{"enabled": d.Enabled}); e != nil {
			return e
		}
		var e error
		out, e = s.department(ctx, tx, scope, id)
		return e
	})
	return out, e
}

type TemplateChange struct {
	Template tickets.Template `json:"template"`
	Key      string           `json:"idempotency_key"`
	Expected int64            `json:"expected_revision"`
}

func (s *Service) Templates(ctx context.Context, actor string, scope sdk.Scope) ([]tickets.Template, error) {
	scope.PluginID = tickets.ID
	out := []tickets.Template{}
	e := s.tenant(ctx, actor, scope, "tickets.reply", "tickets.read", func(tx pgx.Tx, p string) error {
		rs, e := tx.Query(ctx, "SELECT id::text,definition,revision FROM support_templates WHERE workspace_id=$1 AND bot_id=$2 ORDER BY id LIMIT 100", scope.AccountID, scope.BotID)
		if e != nil {
			return e
		}
		defer rs.Close()
		for rs.Next() {
			var id string
			var b []byte
			var rev int64
			if e = rs.Scan(&id, &b, &rev); e != nil {
				return e
			}
			var d tickets.Template
			if e = json.Unmarshal(b, &d); e != nil {
				return e
			}
			d.ID = id
			d.Revision = rev
			out = append(out, d)
		}
		return rs.Err()
	})
	return out, e
}
func (s *Service) SaveTemplate(ctx context.Context, actor string, scope sdk.Scope, id string, in TemplateChange) (tickets.Template, error) {
	scope.PluginID = tickets.ID
	d := in.Template
	d.ID = ""
	d.Revision = 0
	if strings.TrimSpace(d.Name) == "" || len(d.Name) > 100 || strings.TrimSpace(d.Body) == "" || len(d.Body) > 4000 || !keyValid(in.Key) {
		return d, database.ErrInvalid
	}
	e := s.tenant(ctx, actor, scope, "tickets.manage", "tickets.write", func(tx pgx.Tx, p string) error {
		if id == "" {
			if in.Expected != 0 {
				return database.ErrConflict
			}
			var saved string
			e := tx.QueryRow(ctx, "SELECT id::text,fingerprint FROM support_templates WHERE workspace_id=$1 AND bot_id=$2 AND operation_key=$3", scope.AccountID, scope.BotID, in.Key).Scan(&id, &saved)
			if e == nil {
				if saved != fingerprint(d) {
					return database.ErrConflict
				}
				d.ID = id
				d.Revision = 1
				return nil
			}
			if !noRows(e) {
				return e
			}
			var n int
			if e = tx.QueryRow(ctx, "SELECT count(*) FROM support_templates WHERE workspace_id=$1 AND bot_id=$2", scope.AccountID, scope.BotID).Scan(&n); e != nil {
				return e
			}
			if n >= 100 {
				return database.ErrInvalid
			}
			e = tx.QueryRow(ctx, "INSERT INTO support_templates(workspace_id,bot_id,definition,operation_key,fingerprint) VALUES($1,$2,$3,$4,$5) RETURNING id::text", scope.AccountID, scope.BotID, body(d), in.Key, fingerprint(d)).Scan(&id)
			if e != nil {
				return e
			}
			d.Revision = 1
		} else {
			tag, e := tx.Exec(ctx, "UPDATE support_templates SET definition=$4,revision=revision+1 WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3 AND revision=$5", scope.AccountID, scope.BotID, id, body(d), in.Expected)
			if e != nil {
				return e
			}
			if tag.RowsAffected() != 1 {
				return database.ErrConflict
			}
			d.Revision = in.Expected + 1
		}
		d.ID = id
		return audit(ctx, tx, actor, scope, "ticket.template.saved", id, map[string]any{"revision": d.Revision, "enabled": d.Enabled})
	})
	return d, e
}

type Agent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *Service) Agents(ctx context.Context, actor string, scope sdk.Scope) ([]Agent, error) {
	scope.PluginID = tickets.ID
	out := []Agent{}
	e := s.tenant(ctx, actor, scope, "tickets.read", "tickets.read", func(tx pgx.Tx, p string) error {
		rs, e := tx.Query(ctx, `SELECT u.id::text,u.display_name FROM workspace_members m JOIN users u ON u.id=m.user_id JOIN role_permissions p ON p.role_key=m.role_key AND p.permission_key='tickets.reply' WHERE m.workspace_id=$1 ORDER BY u.id LIMIT 100`, scope.AccountID)
		if e != nil {
			return e
		}
		defer rs.Close()
		for rs.Next() {
			var a Agent
			if e = rs.Scan(&a.ID, &a.Name); e != nil {
				return e
			}
			out = append(out, a)
		}
		return rs.Err()
	})
	return out, e
}
