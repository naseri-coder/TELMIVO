package intelligence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	provider "github.com/naseri-coder/TELMIVO/internal/platform/ai"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	assistant "github.com/naseri-coder/TELMIVO/internal/plugins/aiassistant"
)

type ModelChange struct {
	Model    provider.Model `json:"model"`
	Endpoint string         `json:"endpoint"`
	Key      string         `json:"api_key"`
	Expected int64          `json:"expected_revision"`
}

// Prevent accidental request-format logging from exposing the operator key.
func (in ModelChange) String() string {
	return fmt.Sprintf("ModelChange{ID:%s, Expected:%d, Key:[REDACTED]}", in.Model.ID, in.Expected)
}
func (in ModelChange) GoString() string { return in.String() }

func (s *Service) SaveModel(ctx context.Context, actor, account string, in ModelChange) (provider.Model, error) {
	m := in.Model
	if !m.Valid() || in.Expected < 0 || len(in.Key) > 4096 || m.Provider == "synthetic" && !s.Synthetic || m.Provider == "openai-compatible" && !provider.EndpointValid(in.Endpoint) {
		return provider.Model{}, database.ErrInvalid
	}
	m.Revision = in.Expected + 1
	e := s.DB.Tenant(ctx, actor, account, true, func(tx pgx.Tx) error {
		if !database.PlatformPermission(ctx, tx, actor, "platform.commercial.manage") {
			return database.ErrForbidden
		}
		var current int64
		err := tx.QueryRow(ctx, "SELECT revision FROM ai_models WHERE id=$1 FOR UPDATE", m.ID).Scan(&current)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if current != in.Expected {
			return database.ErrConflict
		}
		encrypted := []byte{}
		var kid string
		if m.Provider == "openai-compatible" {
			if in.Key != "" {
				if s.Credentials == nil {
					return sdk.ErrPermission
				}
				secret, e := s.Credentials.Seal(ctx, "ai-provider:"+m.ID, []byte(in.Key))
				if e != nil {
					return e
				}
				encrypted = secret.Ciphertext
				kid = secret.KeyID
			} else {
				if e := tx.QueryRow(ctx, "SELECT key_id,encrypted_key FROM ai_models WHERE id=$1", m.ID).Scan(&kid, &encrypted); e != nil {
					return database.ErrInvalid
				}
			}
		}
		tag, e := tx.Exec(ctx, `INSERT INTO ai_models(id,definition,endpoint,key_id,encrypted_key,revision,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(id) DO UPDATE SET definition=EXCLUDED.definition,endpoint=EXCLUDED.endpoint,key_id=EXCLUDED.key_id,encrypted_key=EXCLUDED.encrypted_key,revision=EXCLUDED.revision,updated_at=EXCLUDED.updated_at WHERE ai_models.revision=$8`, m.ID, body(m), in.Endpoint, kid, encrypted, m.Revision, s.now(), in.Expected)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return database.ErrConflict
		}
		return audit(ctx, tx, actor, sdk.Scope{AccountID: account}, "ai.model.updated", m.ID, map[string]any{"revision": m.Revision, "enabled": m.Enabled})
	})
	return m, e
}
func (s *Service) model(ctx context.Context, tx pgx.Tx, id string) (provider.Model, error) {
	var m provider.Model
	var b []byte
	var endpoint, kid string
	var secret []byte
	var rev int64
	e := tx.QueryRow(ctx, "SELECT definition,endpoint,key_id,encrypted_key,revision FROM ai_models WHERE id=$1", id).Scan(&b, &endpoint, &kid, &secret, &rev)
	if e != nil {
		return m, e
	}
	e = json.Unmarshal(b, &m)
	m.Endpoint = endpoint
	m.Secret.KeyID = kid
	m.Secret.Ciphertext = secret
	m.Revision = rev
	if !m.Enabled || !m.Valid() || m.Provider == "synthetic" && !s.Synthetic {
		return m, &provider.Failure{Code: "MODEL_UNAVAILABLE"}
	}
	return m, e
}
func (s *Service) Models(ctx context.Context, actor string, scope sdk.Scope) ([]provider.Model, error) {
	scope.PluginID = assistant.ID
	out := []provider.Model{}
	e := s.tenant(ctx, actor, scope, "ai.invoke", "ai.invoke", func(tx pgx.Tx) error {
		rs, e := tx.Query(ctx, "SELECT definition,revision FROM ai_models WHERE (definition->>'enabled')::boolean ORDER BY id LIMIT 100")
		if e != nil {
			return e
		}
		defer rs.Close()
		for rs.Next() {
			var b []byte
			var rev int64
			if e = rs.Scan(&b, &rev); e != nil {
				return e
			}
			var m provider.Model
			if e = json.Unmarshal(b, &m); e != nil {
				return e
			}
			m.Revision = rev
			if m.Provider != "synthetic" || s.Synthetic {
				out = append(out, m)
			}
		}
		return rs.Err()
	})
	return out, e
}
func (s *Service) config(ctx context.Context, tx pgx.Tx, scope sdk.Scope) (assistant.Config, error) {
	var c assistant.Config
	var b []byte
	var rev int64
	e := tx.QueryRow(ctx, "SELECT definition,revision FROM ai_configurations WHERE workspace_id=$1 AND bot_id=$2", scope.AccountID, scope.BotID).Scan(&b, &rev)
	if e != nil {
		return c, e
	}
	e = json.Unmarshal(b, &c)
	c.Revision = rev
	return c, e
}
func (s *Service) Config(ctx context.Context, actor string, scope sdk.Scope) (assistant.Config, error) {
	scope.PluginID = assistant.ID
	var out assistant.Config
	e := s.tenant(ctx, actor, scope, "ai.manage", "ai.invoke", func(tx pgx.Tx) error {
		var e error
		out, e = s.config(ctx, tx, scope)
		if errors.Is(e, pgx.ErrNoRows) {
			out = assistant.Config{Name: "دستیار", Mode: "AI_WITH_HANDOFF", Tools: []string{}, Knowledge: []string{}, Files: []string{}, Recent: 6, Memory: true, OutputTokens: 512, RetrievalMode: "HYBRID", MaxKnowledgeChunks: 4}
			return nil
		}
		return e
	})
	return out, e
}
func (s *Service) SaveConfig(ctx context.Context, actor string, scope sdk.Scope, c assistant.Config, expected int64) (assistant.Config, error) {
	scope.PluginID = assistant.ID
	if !c.Valid() || expected < 0 {
		return c, database.ErrInvalid
	}
	c.Revision = 0
	e := s.tenant(ctx, actor, scope, "ai.manage", "ai.invoke", func(tx pgx.Tx) error {
		var current int64
		err := tx.QueryRow(ctx, "SELECT revision FROM ai_configurations WHERE workspace_id=$1 AND bot_id=$2 FOR UPDATE", scope.AccountID, scope.BotID).Scan(&current)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if current != expected {
			return database.ErrConflict
		}
		m, e := s.model(ctx, tx, c.Model)
		if e != nil {
			return e
		}
		if len(c.Tools) > 0 && !m.Tools || c.OutputTokens > m.MaxOutput {
			return database.ErrInvalid
		}
		for _, id := range c.Knowledge {
			var yes bool
			if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM ai_knowledge WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3 AND enabled)", scope.AccountID, scope.BotID, id).Scan(&yes); e != nil {
				return e
			}
			if !yes {
				return database.ErrInvalid
			}
		}
		for _, ref := range c.Files {
			var yes bool
			if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM ai_knowledge WHERE workspace_id=$1 AND bot_id=$2 AND source_ref=$3 AND source_kind IN('FILE_MANAGER','UPLOADER') AND enabled)", scope.AccountID, scope.BotID, ref).Scan(&yes); e != nil {
				return e
			}
			if !yes {
				return database.ErrInvalid
			}
		}
		tag, e := tx.Exec(ctx, `INSERT INTO ai_configurations(workspace_id,bot_id,definition,revision) VALUES($1,$2,$3,1) ON CONFLICT(workspace_id,bot_id) DO UPDATE SET definition=EXCLUDED.definition,revision=ai_configurations.revision+1 WHERE ai_configurations.revision=$4`, scope.AccountID, scope.BotID, body(c), expected)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return database.ErrConflict
		}
		return audit(ctx, tx, actor, scope, "ai.configuration.updated", scope.BotID, map[string]any{"revision": expected + 1})
	})
	c.Revision = expected + 1
	return c, e
}

type Knowledge struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Body       string `json:"body"`
	Enabled    bool   `json:"enabled"`
	SourceKind string `json:"source_kind"`
	SourceRef  string `json:"source_ref"`
	Revision   int64  `json:"revision"`
}

func (s *Service) Knowledge(ctx context.Context, actor string, scope sdk.Scope) ([]Knowledge, error) {
	scope.PluginID = assistant.ID
	out := []Knowledge{}
	e := s.tenant(ctx, actor, scope, "ai.manage", "ai.invoke", func(tx pgx.Tx) error {
		rs, e := tx.Query(ctx, "SELECT id::text,name,body,enabled,source_kind,source_ref,revision FROM ai_knowledge WHERE workspace_id=$1 AND bot_id=$2 ORDER BY id LIMIT 100", scope.AccountID, scope.BotID)
		if e != nil {
			return e
		}
		defer rs.Close()
		for rs.Next() {
			var k Knowledge
			if e = rs.Scan(&k.ID, &k.Name, &k.Body, &k.Enabled, &k.SourceKind, &k.SourceRef, &k.Revision); e != nil {
				return e
			}
			out = append(out, k)
		}
		return rs.Err()
	})
	return out, e
}
func (s *Service) SaveKnowledge(ctx context.Context, actor string, scope sdk.Scope, k Knowledge, keyValue string, expected int64) (Knowledge, error) {
	scope.PluginID = assistant.ID
	if k.SourceKind == "" {
		k.SourceKind = "MANUAL"
	}
	k.SourceRef = ""
	if k.SourceKind != "MANUAL" || k.Name == "" || len(k.Name) > 100 || k.Body == "" || len(k.Body) > 4096 || !key(keyValue) || expected < 0 {
		return k, database.ErrInvalid
	}
	e := s.tenant(ctx, actor, scope, "ai.manage", "ai.invoke", func(tx pgx.Tx) error {
		if k.ID == "" {
			if expected != 0 {
				return database.ErrConflict
			}
			var fp string
			err := tx.QueryRow(ctx, "SELECT id::text,revision,fingerprint FROM ai_knowledge WHERE workspace_id=$1 AND bot_id=$2 AND operation_key=$3", scope.AccountID, scope.BotID, keyValue).Scan(&k.ID, &k.Revision, &fp)
			if err == nil {
				if fp != hash([]any{k.SourceKind, k.SourceRef, k.Name, k.Body, k.Enabled}) {
					return database.ErrConflict
				}
				return s.reindexKnowledge(ctx, tx, scope, k)
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			var count int
			if e := tx.QueryRow(ctx, "SELECT count(*) FROM ai_knowledge WHERE workspace_id=$1 AND bot_id=$2", scope.AccountID, scope.BotID).Scan(&count); e != nil {
				return e
			}
			if count >= 100 {
				return database.ErrInvalid
			}
			e := tx.QueryRow(ctx, "INSERT INTO ai_knowledge(workspace_id,bot_id,name,body,enabled,operation_key,fingerprint,source_kind,source_ref) VALUES($1,$2,$3,$4,$5,$6,$7,'MANUAL','') RETURNING id::text,revision,fingerprint", scope.AccountID, scope.BotID, k.Name, k.Body, k.Enabled, keyValue, hash([]any{k.SourceKind, k.SourceRef, k.Name, k.Body, k.Enabled})).Scan(&k.ID, &k.Revision, &fp)
			if e != nil {
				return e
			}
			if fp != hash([]any{k.SourceKind, k.SourceRef, k.Name, k.Body, k.Enabled}) {
				return database.ErrConflict
			}
		} else {
			tag, e := tx.Exec(ctx, "UPDATE ai_knowledge SET name=$4,body=$5,enabled=$6,revision=revision+1 WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3 AND revision=$7 AND source_kind='MANUAL'", scope.AccountID, scope.BotID, k.ID, k.Name, k.Body, k.Enabled, expected)
			if e != nil {
				return e
			}
			if tag.RowsAffected() != 1 {
				return database.ErrConflict
			}
			k.Revision = expected + 1
		}
		if e := s.reindexKnowledge(ctx, tx, scope, k); e != nil {
			return e
		}
		return audit(ctx, tx, actor, scope, "ai.knowledge.updated", k.ID, map[string]any{"revision": k.Revision})
	})
	return k, e
}
