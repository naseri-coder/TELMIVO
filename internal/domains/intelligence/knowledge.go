package intelligence

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	provider "github.com/naseri-coder/TELMIVO/internal/platform/ai"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	assistant "github.com/naseri-coder/TELMIVO/internal/plugins/aiassistant"
)

const maxImportedKnowledgeBytes = 64 << 10

type IngestRequest struct {
	SourceKind string `json:"source_kind"`
	Reference  string `json:"reference"`
	Name       string `json:"name"`
	Key        string `json:"idempotency_key"`
}

type rankedKnowledge struct {
	name, kind, ref, body string
	score                 float64
}

func chunks(body string) []string {
	r := []rune(body)
	const size, overlap = 1200, 120
	out := []string{}
	for start := 0; start < len(r) && len(out) < 64; {
		end := start + size
		if end > len(r) {
			end = len(r)
		}
		part := strings.TrimSpace(string(r[start:end]))
		if part != "" {
			out = append(out, part)
		}
		if end == len(r) {
			break
		}
		start = end - overlap
	}
	return out
}

func (s *Service) reindexKnowledge(ctx context.Context, tx pgx.Tx, scope sdk.Scope, k Knowledge) error {
	if _, err := tx.Exec(ctx, "DELETE FROM ai_knowledge_chunks WHERE workspace_id=$1 AND bot_id=$2 AND knowledge_id=$3", scope.AccountID, scope.BotID, k.ID); err != nil {
		return err
	}
	if !k.Enabled {
		return nil
	}
	for i, part := range chunks(k.Body) {
		vector, err := json.Marshal(provider.FeatureVector(part))
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO ai_knowledge_chunks(workspace_id,bot_id,knowledge_id,ordinal,body,vector,fingerprint)
			VALUES($1,$2,$3,$4,$5,$6,$7)`, scope.AccountID, scope.BotID, k.ID, i, part, vector, hash([]any{k.ID, i, part})); err != nil {
			return err
		}
	}
	return nil
}

func tokenSet(v string) map[string]bool {
	out := map[string]bool{}
	for _, token := range strings.FieldsFunc(strings.ToLower(v), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}) {
		if token != "" {
			out[token] = true
		}
	}
	return out
}

func lexicalScore(query, body string) float64 {
	q, b := tokenSet(query), tokenSet(body)
	if len(q) == 0 || len(b) == 0 {
		return 0
	}
	matched := 0
	for token := range q {
		if b[token] {
			matched++
		}
	}
	return float64(matched) / float64(len(q))
}

func (s *Service) retrieveHybrid(ctx context.Context, tx pgx.Tx, scope sdk.Scope, c assistant.Config, query string) ([]string, error) {
	out := []string{}
	if len(c.Knowledge) == 0 && len(c.Files) == 0 {
		return out, nil
	}
	if len(query) > 500 {
		query = query[:500]
	}
	rows, err := tx.Query(ctx, `SELECT k.name,k.source_kind,k.source_ref,ch.body,ch.vector
		FROM ai_knowledge_chunks ch
		JOIN ai_knowledge k ON k.id=ch.knowledge_id AND k.workspace_id=ch.workspace_id AND k.bot_id=ch.bot_id
		WHERE ch.workspace_id=$1 AND ch.bot_id=$2 AND k.enabled
		AND (k.id::text=ANY($3::text[]) OR (k.source_ref<>'' AND k.source_ref=ANY($4::text[])))
		ORDER BY k.id,ch.ordinal LIMIT 256`, scope.AccountID, scope.BotID, c.Knowledge, c.Files)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	qv := provider.FeatureVector(query)
	ranked := []rankedKnowledge{}
	for rows.Next() {
		var item rankedKnowledge
		var raw []byte
		if err = rows.Scan(&item.name, &item.kind, &item.ref, &item.body, &raw); err != nil {
			return out, err
		}
		lexical := lexicalScore(query, item.body)
		item.score = lexical
		if c.Retrieval() == "HYBRID" {
			var v provider.Vector
			if json.Unmarshal(raw, &v) != nil {
				return out, database.ErrInvalid
			}
			semantic := provider.Cosine(qv, v)
			if semantic < 0 {
				semantic = 0
			}
			item.score = semantic*0.70 + lexical*0.30
		}
		if item.score > 0 {
			ranked = append(ranked, item)
		}
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })
	limit := c.RetrievalLimit()
	if len(ranked) < limit {
		limit = len(ranked)
	}
	for _, item := range ranked[:limit] {
		label := item.name
		if item.kind != "MANUAL" {
			label += " [" + item.kind + "]"
		}
		out = append(out, label+": "+item.body)
	}
	return out, nil
}

func (s *Service) sourceKnowledge(ctx context.Context, tx pgx.Tx, scope sdk.Scope, kind, ref string) (Knowledge, error) {
	var k Knowledge
	err := tx.QueryRow(ctx, `SELECT id::text,name,body,enabled,revision,source_kind,source_ref
		FROM ai_knowledge WHERE workspace_id=$1 AND bot_id=$2 AND source_kind=$3 AND source_ref=$4`,
		scope.AccountID, scope.BotID, kind, ref).Scan(&k.ID, &k.Name, &k.Body, &k.Enabled, &k.Revision, &k.SourceKind, &k.SourceRef)
	return k, err
}

func (s *Service) saveImported(ctx context.Context, actor string, scope sdk.Scope, in IngestRequest, name, bodyText string) (Knowledge, error) {
	var out Knowledge
	fp := hash([]any{in.SourceKind, in.Reference, name, bodyText})
	err := s.tenant(ctx, actor, scope, "ai.manage", "ai.invoke", func(tx pgx.Tx) error {
		var savedFP string
		err := tx.QueryRow(ctx, "SELECT id::text,revision,fingerprint,name,body,enabled,source_kind,source_ref FROM ai_knowledge WHERE workspace_id=$1 AND bot_id=$2 AND operation_key=$3",
			scope.AccountID, scope.BotID, in.Key).Scan(&out.ID, &out.Revision, &savedFP, &out.Name, &out.Body, &out.Enabled, &out.SourceKind, &out.SourceRef)
		if err == nil {
			if savedFP != fp || out.SourceKind != in.SourceKind || out.SourceRef != in.Reference {
				return database.ErrConflict
			}
			return s.reindexKnowledge(ctx, tx, scope, out)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		existing, err := s.sourceKnowledge(ctx, tx, scope, in.SourceKind, in.Reference)
		if err == nil {
			if hash([]any{existing.SourceKind, existing.SourceRef, existing.Name, existing.Body}) != fp {
				tag, updateErr := tx.Exec(ctx, "UPDATE ai_knowledge SET name=$4,body=$5,enabled=true,fingerprint=$6,revision=revision+1 WHERE workspace_id=$1 AND bot_id=$2 AND id=$3 AND revision=$7",
					scope.AccountID, scope.BotID, existing.ID, name, bodyText, fp, existing.Revision)
				if updateErr != nil {
					return updateErr
				}
				if tag.RowsAffected() != 1 {
					return database.ErrConflict
				}
				existing.Name, existing.Body, existing.Enabled, existing.Revision = name, bodyText, true, existing.Revision+1
				if updateErr = s.reindexKnowledge(ctx, tx, scope, existing); updateErr != nil {
					return updateErr
				}
				out = existing
				return audit(ctx, tx, actor, scope, "ai.knowledge.refreshed", out.ID, map[string]any{"source_kind": in.SourceKind, "source_ref": in.Reference, "revision": out.Revision})
			}
			out = existing
			return s.reindexKnowledge(ctx, tx, scope, out)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var count int
		if err = tx.QueryRow(ctx, "SELECT count(*) FROM ai_knowledge WHERE workspace_id=$1 AND bot_id=$2", scope.AccountID, scope.BotID).Scan(&count); err != nil {
			return err
		}
		if count >= 100 {
			return database.ErrInvalid
		}
		err = tx.QueryRow(ctx, `INSERT INTO ai_knowledge(workspace_id,bot_id,name,body,enabled,operation_key,fingerprint,source_kind,source_ref)
			VALUES($1,$2,$3,$4,true,$5,$6,$7,$8) RETURNING id::text,revision`,
			scope.AccountID, scope.BotID, name, bodyText, in.Key, fp, in.SourceKind, in.Reference).Scan(&out.ID, &out.Revision)
		if err != nil {
			return err
		}
		out.Name, out.Body, out.Enabled, out.SourceKind, out.SourceRef = name, bodyText, true, in.SourceKind, in.Reference
		if err = s.reindexKnowledge(ctx, tx, scope, out); err != nil {
			return err
		}
		return audit(ctx, tx, actor, scope, "ai.knowledge.ingested", out.ID, map[string]any{"source_kind": in.SourceKind, "source_ref": in.Reference, "revision": out.Revision})
	})
	return out, err
}

func (s *Service) Ingest(ctx context.Context, actor string, scope sdk.Scope, in IngestRequest) (Knowledge, error) {
	scope.PluginID = assistant.ID
	in.SourceKind = strings.ToUpper(strings.TrimSpace(in.SourceKind))
	in.Reference = strings.TrimSpace(in.Reference)
	in.Name = strings.TrimSpace(in.Name)
	if !key(in.Key) || in.Reference == "" || len(in.Reference) > 200 || len(in.Name) > 100 || (in.SourceKind != "FILE_MANAGER" && in.SourceKind != "UPLOADER") {
		return Knowledge{}, database.ErrInvalid
	}
	var name, bodyText, storageKey, contentType string
	var size int64
	err := s.tenant(ctx, actor, scope, "ai.manage", "ai.invoke", func(tx pgx.Tx) error {
		switch in.SourceKind {
		case "FILE_MANAGER":
			if !database.Permission(ctx, tx, actor, scope.AccountID, "files.read") {
				return database.ErrForbidden
			}
			var raw []byte
			if err := tx.QueryRow(ctx, "SELECT definition FROM content_files WHERE workspace_id=$1 AND bot_id=$2 AND reference=$3", scope.AccountID, scope.BotID, in.Reference).Scan(&raw); err != nil {
				return err
			}
			var meta struct {
				Title       string `json:"title"`
				Description string `json:"description"`
				Category    string `json:"category"`
				Enabled     bool   `json:"enabled"`
			}
			if json.Unmarshal(raw, &meta) != nil || !meta.Enabled {
				return database.ErrInvalid
			}
			name = in.Name
			if name == "" {
				name = meta.Title
			}
			bodyText = strings.TrimSpace(meta.Title + "\n" + meta.Description + "\nCategory: " + meta.Category)
		case "UPLOADER":
			if !database.Permission(ctx, tx, actor, scope.AccountID, "uploads.read") {
				return database.ErrForbidden
			}
			var original, state, bot string
			if err := tx.QueryRow(ctx, `SELECT COALESCE(original_name,''),COALESCE(content_type,''),COALESCE(size_bytes,0),COALESCE(storage_key,''),state,COALESCE(customer_bot_id::text,'')
				FROM uploads WHERE workspace_id=$1 AND id::text=$2`, scope.AccountID, in.Reference).
				Scan(&original, &contentType, &size, &storageKey, &state, &bot); err != nil {
				return err
			}
			if state != "STORED" || bot != scope.BotID || size <= 0 || size > maxImportedKnowledgeBytes ||
				!assistant.Contains([]string{"text/plain", "text/markdown", "application/json"}, contentType) {
				return database.ErrInvalid
			}
			name = in.Name
			if name == "" {
				name = original
			}
		}
		return nil
	})
	if err != nil {
		return Knowledge{}, err
	}
	if in.SourceKind == "UPLOADER" {
		if s.Storage == nil {
			return Knowledge{}, database.ErrInvalid
		}
		body, err := s.Storage.Open(ctx, scope.AccountID, storageKey)
		if err != nil {
			return Knowledge{}, err
		}
		defer body.Close()
		raw, err := io.ReadAll(io.LimitReader(body, maxImportedKnowledgeBytes+1))
		if err != nil || len(raw) == 0 || len(raw) > maxImportedKnowledgeBytes || !utf8.Valid(raw) {
			return Knowledge{}, database.ErrInvalid
		}
		bodyText = strings.TrimSpace(string(raw))
	}
	if name == "" || bodyText == "" || len([]byte(bodyText)) > maxImportedKnowledgeBytes {
		return Knowledge{}, database.ErrInvalid
	}
	return s.saveImported(ctx, actor, scope, in, name, bodyText)
}
