//go:build integration

package integration

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/commerce"
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	"github.com/naseri-coder/TELMIVO/internal/domains/intelligence"
	provider "github.com/naseri-coder/TELMIVO/internal/platform/ai"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/platform/storage"
	assistant "github.com/naseri-coder/TELMIVO/internal/plugins/aiassistant"
	files "github.com/naseri-coder/TELMIVO/internal/plugins/filemanager"
)

func r4fEnableFileManager(t *testing.T, f r3d6Data) {
	t.Helper()
	m, err := f.core.Registry.Manifest(files.ID)
	must(t, err)
	f.r3d5FixtureData.commercialFixture.save(t, commerce.Version{
		Kind: commerce.PluginKind, Key: files.ID,
		Definition: commerce.Definition{Plugin: &commerce.Plugin{Scope: sdk.PerBot, SaleEnabled: false, Capabilities: m.Permissions, Prices: r3d2Prices(0)}},
	}, factoryVersion(t, f.s, f.admin, f.u, commerce.PluginKind, files.ID))
	catalog, err := f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
	must(t, err)
	plan, ok := catalog.Get(commerce.PlanKind, f.plan)
	if !ok {
		t.Fatal("fixture plan unavailable")
	}
	plan.Definition.Plan.Plugins[files.ID] = commerce.Included
	f.r3d5FixtureData.commercialFixture.save(t, plan, plan.Version)
	scope := f.scope(files.ID)
	i, err := f.core.Change(ctx, f.u.UserID, scope, sdk.Install, 0, json.RawMessage(`{}`), "r4f")
	must(t, err)
	_, err = f.core.Change(ctx, f.u.UserID, scope, sdk.Activate, i.Revision, nil, "r4f")
	must(t, err)
}

func r4fUploaderSource(t *testing.T, f r3d6Data, bodyText string) (string, func()) {
	t.Helper()
	local, err := storage.NewLocal(t.TempDir(), 2<<20)
	must(t, err)
	scoped := &storage.Scoped{Provider: local}
	f.ai.Storage = scoped
	var id string
	tenant(t, f.u, func(tx pgx.Tx) error { return tx.QueryRow(ctx, "SELECT gen_random_uuid()::text").Scan(&id) })
	key, err := storage.ObjectKey(f.u.WorkspaceID, "r4f:"+id)
	must(t, err)
	must(t, scoped.Put(ctx, f.u.WorkspaceID, key, strings.NewReader(bodyText), storage.Metadata{
		Size: int64(len(bodyText)), ContentType: "text/plain", ExpiresAt: f.clock.at.Add(time.Hour),
	}))
	tenant(t, f.u, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO uploads(id,workspace_id,creator_id,customer_bot_id,state,metadata,original_name,content_type,size_bytes,storage_key,expires_at)
			VALUES($1,$2,$3,$4,'STORED','{}',$5,'text/plain',$6,$7,$8)`,
			id, f.u.WorkspaceID, f.u.UserID, f.bot, "r4f-source.txt", int64(len(bodyText)), key, f.clock.at.Add(time.Hour))
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO upload_files(workspace_id,upload_id,storage_key,media_type,size_bytes)
			VALUES($1,$2,$3,'text/plain',$4)`, f.u.WorkspaceID, id, key, int64(len(bodyText)))
		return err
	})
	return id, func() { must(t, local.Close()) }
}

func TestR4FHybridRetrievalIngestionAndCost(t *testing.T) {
	f := r3d6Fixture(t)
	priced, err := f.ai.SaveModel(ctx, f.admin.UserID, f.u.WorkspaceID, intelligence.ModelChange{
		Model: provider.Model{
			ID: "synthetic-" + f.bot, Name: "Synthetic priced", Provider: "synthetic", Identifier: "fixture",
			Enabled: true, Tools: true, Structured: true, MaxInput: 32768, MaxOutput: 512,
			InputMicrounitsPerMillion: 1_000_000, OutputMicrounitsPerMillion: 2_000_000,
		},
		Expected: 1,
	})
	must(t, err)

	manual, err := f.ai.SaveKnowledge(ctx, f.u.UserID, f.scope(assistant.ID), intelligence.Knowledge{
		Name: "Refund manual", Body: "refund policy requires a receipt and allows thirty days", Enabled: true,
	}, "r4f-manual", 0)
	must(t, err)

	r4fEnableFileManager(t, f)
	file, err := f.ai.Content.SaveFile(ctx, f.u.UserID, f.scope(files.ID), "", "r4f", contentplugins.FileChange{
		File: files.File{FileID: "r4f-file", UniqueID: "r4f-file-unique", Kind: "document", Title: "Course refunds", Description: "refund policy for course access and receipt verification", Category: "Support", Enabled: true},
		Key: "r4f-file",
	})
	must(t, err)
	fileKnowledge, err := f.ai.Ingest(ctx, f.u.UserID, f.scope(assistant.ID), intelligence.IngestRequest{
		SourceKind: "FILE_MANAGER", Reference: file.Reference, Key: "r4f-file-ingest",
	})
	must(t, err)
	if fileKnowledge.SourceKind != "FILE_MANAGER" || fileKnowledge.SourceRef != file.Reference {
		t.Fatal("file manager source identity lost", fileKnowledge)
	}

	uploadID, closeStorage := r4fUploaderSource(t, f, "refund receipts are retained for billing support and customer review")
	defer closeStorage()
	uploadKnowledge, err := f.ai.Ingest(ctx, f.u.UserID, f.scope(assistant.ID), intelligence.IngestRequest{
		SourceKind: "UPLOADER", Reference: uploadID, Key: "r4f-upload-ingest",
	})
	must(t, err)
	if uploadKnowledge.SourceKind != "UPLOADER" || uploadKnowledge.SourceRef != uploadID {
		t.Fatal("uploader source identity lost", uploadKnowledge)
	}

	if number(t, f.u, "SELECT count(*) FROM ai_knowledge_chunks WHERE bot_id=$1", f.bot) < 3 {
		t.Fatal("knowledge corpus was not indexed")
	}

	cfg := f.config
	cfg.Model = priced.ID
	cfg.Knowledge = []string{manual.ID}
	cfg.Sources = []string{"FILE_MANAGER:" + file.Reference, "UPLOADER:" + uploadID}
	cfg.RetrievalMode = "HYBRID"
	cfg.MaxKnowledgeChunks = 4
	cfg.MaxCostMicrounits = 120000
	cfg, err = f.ai.SaveConfig(ctx, f.u.UserID, f.scope(assistant.ID), cfg, f.config.Revision)
	must(t, err)

	result, err := f.ai.Chat(ctx, f.u.UserID, f.scope(assistant.ID), intelligence.Chat{
		Key: "r4f-hybrid-chat", Text: "refund receipt support policy", Purpose: "PLAYGROUND",
	})
	must(t, err)
	if result.State != "SUCCEEDED" || len(f.p.requests) != 1 || len(f.p.requests[0].Knowledge) < 3 {
		t.Fatal("hybrid knowledge was not supplied to provider", result, f.p.requests)
	}
	usage, err := f.ai.Usage(ctx, f.u.UserID, f.scope(assistant.ID), "")
	must(t, err)
	if len(usage) != 1 || usage[0].Cost != 20 || usage[0].Retrieved < 3 {
		t.Fatal("usage/cost accounting mismatch", usage)
	}

	cfg.MaxCostMicrounits = 1
	cfg, err = f.ai.SaveConfig(ctx, f.u.UserID, f.scope(assistant.ID), cfg, cfg.Revision)
	must(t, err)
	_, err = f.ai.Chat(ctx, f.u.UserID, f.scope(assistant.ID), intelligence.Chat{
		Key: "r4f-cost-denied", Text: "refund policy", Purpose: "PLAYGROUND",
	})
	var failure *provider.Failure
	if !errors.As(err, &failure) || failure.Code != "AI_COST_LIMIT" || f.p.calls != 1 {
		t.Fatal("cost ceiling did not stop provider call", err, f.p.calls)
	}
}

func TestR4FLegacyKnowledgeLazyIndex(t *testing.T) {
	f := r3d6Fixture(t)
	var id string
	tenant(t, f.u, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO ai_knowledge(workspace_id,bot_id,name,body,enabled,operation_key,fingerprint)
			VALUES($1,$2,'Legacy knowledge','legacy refund knowledge before R4F',true,'r4f-legacy','legacy-fingerprint')
			RETURNING id::text`, f.u.WorkspaceID, f.bot).Scan(&id)
	})
	if number(t, f.u, "SELECT count(*) FROM ai_knowledge_chunks WHERE knowledge_id=$1", id) != 0 {
		t.Fatal("legacy fixture unexpectedly pre-indexed")
	}
	cfg := f.config
	cfg.Knowledge = []string{id}
	cfg.Sources = []string{}
	cfg.RetrievalMode = "HYBRID"
	cfg.MaxKnowledgeChunks = 4
	_, err := f.ai.SaveConfig(ctx, f.u.UserID, f.scope(assistant.ID), cfg, f.config.Revision)
	must(t, err)
	result, err := f.ai.Chat(ctx, f.u.UserID, f.scope(assistant.ID), intelligence.Chat{
		Key: "r4f-legacy-query", Text: "legacy refund", Purpose: "PLAYGROUND",
	})
	must(t, err)
	if result.State != "SUCCEEDED" || len(f.p.requests) != 1 || len(f.p.requests[0].Knowledge) == 0 {
		t.Fatal("legacy knowledge was not retrieved", result, f.p.requests)
	}
	if number(t, f.u, "SELECT count(*) FROM ai_knowledge_chunks WHERE knowledge_id=$1", id) == 0 {
		t.Fatal("legacy knowledge was not lazily indexed")
	}
}

func TestR4FBrowserFixture(t *testing.T) {
	f := r3d6Fixture(t)
	u, _ := r3aLogin(t, 98014001, false)
	tenant(t, u, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE subscriptions SET plan_key=$1,expires_at=NULL WHERE workspace_id=$2", f.plan, u.WorkspaceID)
		return err
	})
	bot := r3dBot(t, u, "r4f-browser")
	scope := sdk.Scope{AccountID: u.WorkspaceID, BotID: bot}
	for _, id := range []string{"crm", "ticketing", assistant.ID} {
		scope.PluginID = id
		i, err := f.core.Change(ctx, u.UserID, scope, sdk.Install, 0, json.RawMessage(`{}`), "r4f")
		must(t, err)
		_, err = f.core.Change(ctx, u.UserID, scope, sdk.Activate, i.Revision, nil, "r4f")
		must(t, err)
	}
	knowledge, err := f.ai.SaveKnowledge(ctx, u.UserID, scope, intelligence.Knowledge{Name: "R4F browser knowledge", Body: "refund policy browser source", Enabled: true}, "r4f-browser-knowledge", 0)
	must(t, err)
	cfg := f.config
	cfg.Revision = 0
	cfg.Knowledge = []string{knowledge.ID}
	cfg.Sources = []string{}
	cfg.RetrievalMode = "HYBRID"
	cfg.MaxKnowledgeChunks = 4
	cfg.MaxCostMicrounits = 120000
	_, err = f.ai.SaveConfig(ctx, u.UserID, scope, cfg, 0)
	must(t, err)
	tenant(t, u, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE customer_bots SET name='R4F browser bot' WHERE id=$1", bot)
		return err
	})
}

func TestR4FRecoveryPreparation(t *testing.T) {
	f := r3d6Fixture(t)
	knowledge, err := f.ai.SaveKnowledge(ctx, f.u.UserID, f.scope(assistant.ID), intelligence.Knowledge{
		Name: "R4F retained", Body: "retained hybrid knowledge after backup restore", Enabled: true,
	}, "r4f-recovery", 0)
	must(t, err)
	if number(t, f.u, "SELECT count(*) FROM ai_knowledge_chunks WHERE knowledge_id=$1", knowledge.ID) == 0 {
		t.Fatal("recovery knowledge not indexed")
	}
}

func TestR4FRecovered(t *testing.T) {
	store, err := database.Open(ctx, os.Getenv("TEST_RESTORE_MIGRATION_DATABASE_URL"), false)
	must(t, err)
	defer store.Pool.Close()
	var knowledgeID string
	must(t, store.Pool.QueryRow(ctx, "SELECT id::text FROM ai_knowledge WHERE operation_key='r4f-recovery'").Scan(&knowledgeID))
	var chunks int
	must(t, store.Pool.QueryRow(ctx, "SELECT count(*) FROM ai_knowledge_chunks WHERE knowledge_id=$1", knowledgeID).Scan(&chunks))
	if chunks == 0 {
		t.Fatal("R4F vector corpus was lost during restore")
	}
}
