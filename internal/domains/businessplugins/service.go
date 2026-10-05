// Package businessplugins brokers independent Broadcast, Forms and Commerce plugins.
package businessplugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/commerce"
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	platform "github.com/naseri-coder/TELMIVO/internal/domains/pluginplatform"
	"github.com/naseri-coder/TELMIVO/internal/domains/supportplugins"
	"github.com/naseri-coder/TELMIVO/internal/platform/businessevents"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/plugins/crm"
	"strings"
	"time"
)

type Clock interface{ Now() time.Time }
type Service struct {
	DB         *database.Store
	Core       *platform.Service
	Commercial *commerce.Service
	Support    *supportplugins.Service
	Content    *contentplugins.Service
	Clock      Clock
	Fallback   contentplugins.SupportHandler
	PaymentProvider ExternalPaymentProvider
}

func (s *Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock.Now().UTC().Truncate(time.Microsecond)
	}
	return time.Now().UTC().Truncate(time.Microsecond)
}
func body(v any) []byte { b, _ := json.Marshal(v); return b }
func hash(v any) string { h := sha256.Sum256(body(v)); return hex.EncodeToString(h[:]) }
func key(k string) bool { return len(k) > 0 && len(k) <= 100 && strings.TrimSpace(k) == k }
func table(plugin string) string {
	return map[string]string{"broadcast": "broadcast_campaigns", "forms": "form_definitions", "commerce": "commerce_products"}[plugin]
}
func capFor(plugin string) string { return plugin + ".manage" }
func (s *Service) principal(ctx context.Context, tx pgx.Tx, scope sdk.Scope) (string, error) {
	var a string
	e := tx.QueryRow(ctx, "SELECT user_id::text FROM workspace_members WHERE workspace_id=$1 AND role_key='OWNER' ORDER BY user_id LIMIT 1", scope.AccountID).Scan(&a)
	return a, e
}
func (s *Service) admit(ctx context.Context, tx pgx.Tx, actor string, scope sdk.Scope, cap string, running bool) error {
	if table(scope.PluginID) == "" || !database.Permission(ctx, tx, actor, scope.AccountID, capFor(scope.PluginID)) {
		return database.ErrForbidden
	}
	if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock_shared(7100213)"); e != nil {
		return e
	}
	if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,7100213))", scope.AccountID); e != nil {
		return e
	}
	p, e := s.principal(ctx, tx, scope)
	if e != nil {
		return e
	}
	return s.Core.ValidateExecutionTx(ctx, tx, p, scope, cap, running)
}
func (s *Service) tenant(ctx context.Context, actor string, scope sdk.Scope, fn func(pgx.Tx) error) error {
	return s.DB.Tenant(ctx, actor, scope.AccountID, false, func(tx pgx.Tx) error {
		if e := s.admit(ctx, tx, actor, scope, capFor(scope.PluginID), false); e != nil {
			return e
		}
		return fn(tx)
	})
}
func (s *Service) crm(ctx context.Context, tx pgx.Tx, scope sdk.Scope, write bool) error {
	p, e := s.principal(ctx, tx, scope)
	if e != nil {
		return e
	}
	scope.PluginID = crm.ID
	cap := "crm.read"
	if write {
		cap = "crm.write"
	}
	return s.Core.ValidateExecutionTx(ctx, tx, p, scope, cap, false)
}
func (s *Service) quota(ctx context.Context, tx pgx.Tx, scope sdk.Scope, metric string, n int64, consume bool) error {
	p, e := s.principal(ctx, tx, scope)
	if e != nil {
		return e
	}
	q, e := s.Commercial.QuotaTx(ctx, tx, p, scope.AccountID, scope.BotID, metric, n, consume)
	if e != nil {
		return e
	}
	if !q.Allowed {
		return &contentplugins.QuotaDenied{Result: q}
	}
	return nil
}
func (s *Service) count(ctx context.Context, tx pgx.Tx, scope sdk.Scope, metric, t string) (int64, error) {
	var mode string
	e := tx.QueryRow(ctx, "SELECT definition->'quota'->>'scope' FROM commercial_catalog WHERE kind='QUOTA' AND key=$1 AND effective_at<=$2 ORDER BY version DESC LIMIT 1", metric, s.now()).Scan(&mode)
	if e != nil {
		return 0, e
	}
	bot := scope.BotID
	if mode == string(sdk.PerAccount) {
		bot = ""
	}
	var n int64
	e = tx.QueryRow(ctx, "SELECT count(*) FROM "+t+" WHERE workspace_id=$1 AND ($2='' OR bot_id::text=$2)", scope.AccountID, bot).Scan(&n)
	return n, e
}
func receipt(ctx context.Context, tx pgx.Tx, scope sdk.Scope, k, fp string) (json.RawMessage, bool, error) {
	var b []byte
	var old string
	e := tx.QueryRow(ctx, "SELECT fingerprint,result FROM business_operations WHERE workspace_id=$1 AND bot_id=$2 AND plugin_id=$3 AND operation_key=$4", scope.AccountID, scope.BotID, scope.PluginID, k).Scan(&old, &b)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if e != nil {
		return nil, false, e
	}
	if old != fp {
		return nil, false, database.ErrConflict
	}
	return b, true, nil
}
func record(ctx context.Context, tx pgx.Tx, scope sdk.Scope, k, fp string, v any) error {
	_, e := tx.Exec(ctx, "INSERT INTO business_operations(workspace_id,bot_id,plugin_id,operation_key,fingerprint,result) VALUES($1,$2,$3,$4,$5,$6)", scope.AccountID, scope.BotID, scope.PluginID, k, fp, body(v))
	return e
}
func emit(ctx context.Context, tx pgx.Tx, actor string, scope sdk.Scope, kind, id, contact string, fields map[string][]string) error {
	if e := database.Audit(ctx, tx, actor, scope.AccountID, kind, "business_plugin", id, "business-plugin-operation", map[string]any{}, map[string]any{"kind": kind}); e != nil {
		return e
	}
	ev := businessevents.Event{Contact: contact}
	return businessevents.Emit(businessevents.With(ctx, ev), tx, actor, scope, kind, id, fields)
}

type Change[T any] struct {
	Definition T      `json:"definition"`
	Key        string `json:"idempotency_key"`
	Expected   int64  `json:"expected_revision"`
}
type Definition struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Revision int64           `json:"revision"`
	Data     json.RawMessage `json:"definition"`
}

func get(ctx context.Context, tx pgx.Tx, scope sdk.Scope, id string) (Definition, error) {
	var d Definition
	e := tx.QueryRow(ctx, "SELECT id::text,name,revision,definition FROM "+table(scope.PluginID)+" WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3", scope.AccountID, scope.BotID, id).Scan(&d.ID, &d.Name, &d.Revision, &d.Data)
	return d, e
}
func (s *Service) save(ctx context.Context, actor string, scope sdk.Scope, id, name, k string, expected int64, value any) (Definition, error) {
	var out Definition
	if !key(k) || expected < 0 || id == "" && expected != 0 || id != "" && expected == 0 {
		return out, database.ErrInvalid
	}
	fp := hash([]any{actor, id, expected, value})
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		b, exists, e := receipt(ctx, tx, scope, k, fp)
		if e != nil {
			return e
		}
		if exists {
			return json.Unmarshal(b, &out)
		}
		if id == "" {
			metric := scope.PluginID + ".definitions.count"
			n, e := s.count(ctx, tx, scope, metric, table(scope.PluginID))
			if e != nil {
				return e
			}
			var botCount int
			if e = tx.QueryRow(ctx, "SELECT count(*) FROM "+table(scope.PluginID)+" WHERE workspace_id=$1 AND bot_id=$2", scope.AccountID, scope.BotID).Scan(&botCount); e != nil {
				return e
			}
			if botCount >= 100 {
				return database.ErrInvalid
			}
			if e = s.quota(ctx, tx, scope, metric, n+1, false); e != nil {
				return e
			}
			e = tx.QueryRow(ctx, "INSERT INTO "+table(scope.PluginID)+"(workspace_id,bot_id,name,definition) VALUES($1,$2,$3,$4) RETURNING id::text", scope.AccountID, scope.BotID, name, body(value)).Scan(&id)
			if e != nil {
				return e
			}
		} else {
			if scope.PluginID == "broadcast" {
				var state string
				if e = tx.QueryRow(ctx, "SELECT state FROM broadcast_campaigns WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3", scope.AccountID, scope.BotID, id).Scan(&state); e != nil {
					return e
				}
				if state != "DRAFT" {
					return database.ErrConflict
				}
			}
			tag, e := tx.Exec(ctx, "UPDATE "+table(scope.PluginID)+" SET name=$4,definition=$5,revision=revision+1 WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3 AND revision=$6", scope.AccountID, scope.BotID, id, name, body(value), expected)
			if e != nil {
				return e
			}
			if tag.RowsAffected() != 1 {
				return database.ErrConflict
			}
		}
		out, e = get(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		return record(ctx, tx, scope, k, fp, out)
	})
	return out, e
}

type Page struct {
	Items []Definition `json:"items"`
	Next  string       `json:"next_cursor"`
}

func (s *Service) Definitions(ctx context.Context, actor string, scope sdk.Scope, after, search string) (Page, error) {
	out := Page{Items: []Definition{}}
	if len(after) > 36 || len(search) > 100 || table(scope.PluginID) == "" {
		return out, database.ErrInvalid
	}
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		rs, e := tx.Query(ctx, "SELECT id::text,name,revision,definition FROM "+table(scope.PluginID)+" WHERE workspace_id=$1 AND bot_id=$2 AND ($3='' OR id::text>$3) AND ($4='' OR lower(name) ^@ lower($4)) ORDER BY id LIMIT 51", scope.AccountID, scope.BotID, after, search)
		if e != nil {
			return e
		}
		defer rs.Close()
		for rs.Next() {
			var d Definition
			if e = rs.Scan(&d.ID, &d.Name, &d.Revision, &d.Data); e != nil {
				return e
			}
			out.Items = append(out.Items, d)
		}
		if len(out.Items) > 50 {
			out.Items = out.Items[:50]
			out.Next = out.Items[49].ID
		}
		return rs.Err()
	})
	return out, e
}
