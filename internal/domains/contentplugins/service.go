// Package contentplugins is the trusted Core broker for two independently installed plugins.
package contentplugins

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/commerce"
	platform "github.com/naseri-coder/TELMIVO/internal/domains/pluginplatform"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	gate "github.com/naseri-coder/TELMIVO/internal/plugins/accessgate"
	"github.com/naseri-coder/TELMIVO/internal/plugins/crm"
	files "github.com/naseri-coder/TELMIVO/internal/plugins/filemanager"
	"github.com/naseri-coder/TELMIVO/internal/plugins/ticketing"
	"strings"
)

type Transport interface {
	gate.Membership
	SendFile(context.Context, int64, string, string, string) (int64, string)
	Prompt(context.Context, int64, string, any) error
	Connect(context.Context, string, string) (string, error)
}
type Provider interface {
	ForBot(context.Context, pgx.Tx, sdk.Scope) (Transport, error)
}
type SupportHandler interface {
	HandleSupport(context.Context, sdk.Scope, string, Update) error
}
type Service struct {
	Support    SupportHandler
	DB         *database.Store
	Core       *platform.Service
	Commercial *commerce.Service
	Provider   Provider
	DigestKey  []byte
}

func Registry(extra ...sdk.Manifest) (*platform.Registry, error) {
	m := []sdk.Manifest{gate.Manifest(), files.Manifest(), crm.Manifest(), ticketing.Manifest()}
	m = append(m, extra...)
	return platform.NewRegistry("0.3.0", m)
}
func hash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func opaque() string {
	b := make([]byte, 24)
	if _, e := rand.Read(b); e != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func (s *Service) requester(bot string, user int64) string {
	h := hmac.New(sha256.New, s.DigestKey)
	b, _ := json.Marshal([]any{bot, user})
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}
func (s *Service) PrepareTx(ctx context.Context, tx pgx.Tx, scope sdk.Scope) error {
	if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock_shared(7100213)"); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,7100213))", scope.AccountID)
	return e
}
func (s *Service) tenant(ctx context.Context, actor string, scope sdk.Scope, fn func(pgx.Tx) error) error {
	return s.DB.Tenant(ctx, actor, scope.AccountID, false, func(tx pgx.Tx) error {
		if e := s.PrepareTx(ctx, tx, scope); e != nil {
			return e
		}
		return fn(tx)
	})
}
func (s *Service) manage(ctx context.Context, tx pgx.Tx, actor string, scope sdk.Scope, cap string) error {
	return s.Core.ValidateExecutionTx(ctx, tx, actor, scope, cap, false)
}

type QuotaDenied struct{ Result commerce.QuotaResult }

func (q *QuotaDenied) Error() string { return q.Result.Code }
func (s *Service) quota(ctx context.Context, tx pgx.Tx, actor string, scope sdk.Scope, metric string, amount int64) error {
	r, e := s.Commercial.QuotaTx(ctx, tx, actor, scope.AccountID, scope.BotID, metric, amount, true)
	if e != nil {
		return e
	}
	if !r.Allowed {
		return &QuotaDenied{r}
	}
	return nil
}
func jsonBody(v any) []byte { b, _ := json.Marshal(v); return b }
func operation(key string) bool {
	return len(key) > 0 && len(key) <= 100 && strings.TrimSpace(key) == key
}
func (s *Service) policy(ctx context.Context, tx pgx.Tx, scope sdk.Scope, id string) (gate.Policy, error) {
	var p gate.Policy
	var b []byte
	e := tx.QueryRow(ctx, "SELECT id::text,definition,revision,created_at,updated_at FROM content_access_policies WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3", scope.AccountID, scope.BotID, id).Scan(&p.ID, &b, &p.Revision, &p.Created, &p.Updated)
	if e == nil {
		var def gate.Policy
		e = json.Unmarshal(b, &def)
		def.ID = p.ID
		def.Revision = p.Revision
		def.Created = p.Created
		def.Updated = p.Updated
		p = def
	}
	return p, e
}
func (s *Service) Policies(ctx context.Context, actor string, scope sdk.Scope) ([]gate.Policy, error) {
	out := []gate.Policy{}
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		if e := s.manage(ctx, tx, actor, scope, "telegram.read_member_status"); e != nil {
			return e
		}
		rs, e := tx.Query(ctx, "SELECT id::text FROM content_access_policies WHERE workspace_id=$1 AND bot_id=$2 ORDER BY created_at,id", scope.AccountID, scope.BotID)
		if e != nil {
			return e
		}
		ids := []string{}
		for rs.Next() {
			var id string
			if e = rs.Scan(&id); e != nil {
				break
			}
			ids = append(ids, id)
		}
		rs.Close()
		if e != nil {
			return e
		}
		for _, id := range ids {
			p, e := s.policy(ctx, tx, scope, id)
			if e != nil {
				return e
			}
			out = append(out, p)
		}
		return rs.Err()
	})
	return out, e
}

type PolicyChange struct {
	Policy   gate.Policy `json:"policy"`
	Key      string      `json:"idempotency_key"`
	Expected int64       `json:"expected_revision"`
}

func (s *Service) SavePolicy(ctx context.Context, actor string, scope sdk.Scope, id, corr string, in PolicyChange) (gate.Policy, error) {
	var out gate.Policy
	in.Policy.ID = ""
	in.Policy.Revision = 0
	if !in.Policy.Valid() || in.Expected < 0 || !operation(in.Key) {
		return out, database.ErrInvalid
	}
	in.Policy.Created = gate.Policy{}.Created
	in.Policy.Updated = gate.Policy{}.Updated
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		if e := s.manage(ctx, tx, actor, scope, "telegram.read_member_status"); e != nil {
			return e
		}
		if id == "" {
			if in.Expected != 0 {
				return database.ErrConflict
			}
			var saved string
			e := tx.QueryRow(ctx, "SELECT id::text,fingerprint FROM content_access_policies WHERE bot_id=$1 AND operation_key=$2", scope.BotID, in.Key).Scan(&id, &saved)
			if e == nil {
				if saved != hash(in.Policy) {
					return database.ErrConflict
				}
				out, e = s.policy(ctx, tx, scope, id)
				return e
			}
			if !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
			e = tx.QueryRow(ctx, "INSERT INTO content_access_policies(workspace_id,bot_id,definition,operation_key,fingerprint) VALUES($1,$2,$3,$4,$5) RETURNING id::text", scope.AccountID, scope.BotID, jsonBody(in.Policy), in.Key, hash(in.Policy)).Scan(&id)
			if e != nil {
				return e
			}
		} else {
			tag, e := tx.Exec(ctx, "UPDATE content_access_policies SET definition=$4,revision=revision+1,updated_at=now() WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3 AND revision=$5", scope.AccountID, scope.BotID, id, jsonBody(in.Policy), in.Expected)
			if e != nil {
				return e
			}
			if tag.RowsAffected() != 1 {
				return database.ErrConflict
			}
		}
		var e error
		out, e = s.policy(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		return database.Audit(ctx, tx, actor, scope.AccountID, "access_gate.policy", "content_policy", id, corr, map[string]any{}, map[string]any{"revision": out.Revision, "enabled": out.Enabled})
	})
	return out, e
}

func (s *Service) requestHash(v any) string {
	h := hmac.New(sha256.New, s.DigestKey)
	h.Write(jsonBody(v))
	return hex.EncodeToString(h.Sum(nil))
}

func (s *Service) transport(ctx context.Context, tx pgx.Tx, scope sdk.Scope) (Transport, error) {
	if s.Provider == nil {
		return nil, sdk.ErrPermission
	}
	return s.Provider.ForBot(ctx, tx, scope)
}
