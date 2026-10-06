//go:build integration

package integration

import (
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/naseri-coder/TELMIVO/db/migrations"
	"github.com/naseri-coder/TELMIVO/internal/domains/commerce"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	assistant "github.com/naseri-coder/TELMIVO/internal/plugins/aiassistant"
	workflow "github.com/naseri-coder/TELMIVO/internal/plugins/automation"
)

func TestR4GMigrationChainAndTenantHardening(t *testing.T) {
	files, err := migrations.Files.ReadDir(".")
	must(t, err)
	names := []string{}
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".sql") {
			names = append(names, f.Name())
		}
	}
	sort.Strings(names)
	must(t, migrations.ValidateChain(names, migrations.CurrentCheckpoint))
	historicalCount, historicalFinal, err := migrations.Checkpoint("R4G")
	must(t, err)
	must(t, migrations.ValidateChain(names[:historicalCount], "R4G"))
	if names[historicalCount-1] != historicalFinal {
		t.Fatal("unexpected historical final migration")
	}
	assertAppliedCheckpoint(t, owner, migrations.CurrentCheckpoint)
	var applied int
	must(t, owner.Pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations WHERE name<'0024_'").Scan(&applied))
	if applied != 23 {
		t.Fatalf("applied migrations=%d want=23", applied)
	}

	requiredRLS := []string{
		"channel_posts",
		"channel_post_history",
		"commerce_payment_intents",
		"commerce_payment_events",
		"commerce_refunds",
		"automation_schedule_state",
		"ai_knowledge_chunks",
	}
	var hardened int
	must(t, owner.Pool.QueryRow(ctx, `
		SELECT count(*)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname='public'
		  AND c.relname=ANY($1::text[])
		  AND c.relrowsecurity
		  AND c.relforcerowsecurity
	`, requiredRLS).Scan(&hardened))
	if hardened != len(requiredRLS) {
		t.Fatalf("R4 FORCE RLS coverage=%d want=%d", hardened, len(requiredRLS))
	}
}

func TestR4GCrossFeatureContinuity(t *testing.T) {
	f := r3d7Fixture(t)

	automationManifest, err := f.core.Registry.Manifest(workflow.ID)
	must(t, err)
	if automationManifest.Version != "1.1.0" || automationManifest.DataVersion != 2 {
		t.Fatal("Automation 2.0 manifest drift", automationManifest.Version, automationManifest.DataVersion)
	}
	assistantManifest, err := f.core.Registry.Manifest(assistant.ID)
	must(t, err)
	if assistantManifest.Version != "1.0.0" || assistantManifest.DataVersion != 1 {
		t.Fatal("AI Assistant 2.0 compatibility manifest drift", assistantManifest.Version, assistantManifest.DataVersion)
	}

	catalog, err := f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
	must(t, err)
	if _, ok := catalog.Get(commerce.QuotaKind, "ai.cost_microunits.monthly"); !ok {
		t.Fatal("R4F AI cost quota missing from commercial catalog")
	}

	var uploaderStatus, uploaderVersion string
	must(t, owner.Pool.QueryRow(ctx, "SELECT status,version FROM plugins WHERE key='uploader'").Scan(&uploaderStatus, &uploaderVersion))
	if uploaderStatus != "ENABLED" || uploaderVersion != "0.2.0" {
		t.Fatal("R4C uploader catalog drift", uploaderStatus, uploaderVersion)
	}

	var channelsEnabled bool
	var channelsRollout string
	must(t, owner.Pool.QueryRow(ctx, "SELECT enabled,rollout_status FROM feature_flags WHERE feature_key='channels'").Scan(&channelsEnabled, &channelsRollout))
	if !channelsEnabled || channelsRollout != "ON" {
		t.Fatal("R4B channel manager rollout drift", channelsEnabled, channelsRollout)
	}
}

func TestR4GRecovered(t *testing.T) {
	store, err := database.Open(ctx, os.Getenv("TEST_RESTORE_MIGRATION_DATABASE_URL"), false)
	must(t, err)
	defer store.Pool.Close()

	assertAppliedCheckpoint(t, store, migrations.CurrentCheckpoint)
	var migrationsCount int
	must(t, store.Pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations WHERE name<'0024_'").Scan(&migrationsCount))
	if migrationsCount != 23 {
		t.Fatal("restored migration chain incomplete", migrationsCount)
	}

	var channelPosts int
	must(t, store.Pool.QueryRow(ctx, "SELECT count(*) FROM channel_posts WHERE operation_key='r4b-recovery'").Scan(&channelPosts))
	if channelPosts != 1 {
		t.Fatal("R4B recovery marker lost", channelPosts)
	}

	var uploadState, uploadName, uploadSHA string
	var uploadFiles int
	must(t, store.Pool.QueryRow(ctx, "SELECT state,original_name,sha256 FROM uploads WHERE operation_key='r4c-recovery-upload'").Scan(&uploadState, &uploadName, &uploadSHA))
	must(t, store.Pool.QueryRow(ctx, `
		SELECT count(*)
		FROM upload_files f
		JOIN uploads u ON u.id=f.upload_id AND u.workspace_id=f.workspace_id
		WHERE u.operation_key='r4c-recovery-upload'
	`).Scan(&uploadFiles))
	if uploadState != "PENDING_APPROVAL" || uploadName != "recovery.txt" || len(uploadSHA) != 64 || uploadFiles != 1 {
		t.Fatal("R4C recovery marker lost", uploadState, uploadName, len(uploadSHA), uploadFiles)
	}

	var paymentState, orderState string
	var paymentEvents int
	must(t, store.Pool.QueryRow(ctx, `
		SELECT p.state,o.state
		FROM commerce_payment_intents p
		JOIN commerce_orders o ON o.id=p.order_id AND o.workspace_id=p.workspace_id AND o.bot_id=p.bot_id
		WHERE p.operation_key='r4d-recovery-checkout'
	`).Scan(&paymentState, &orderState))
	must(t, store.Pool.QueryRow(ctx, `
		SELECT count(*)
		FROM commerce_payment_events e
		JOIN commerce_payment_intents p ON p.id=e.payment_id
		WHERE p.operation_key='r4d-recovery-checkout'
	`).Scan(&paymentEvents))
	if paymentState != "PAID" || orderState != "CONFIRMED" || paymentEvents != 1 {
		t.Fatal("R4D recovery marker lost", paymentState, orderState, paymentEvents)
	}

	var occurrence int64
	var last, next time.Time
	var executionState string
	must(t, store.Pool.QueryRow(ctx, `
		SELECT s.occurrence,s.last_at,s.next_at
		FROM automation_schedule_state s
		JOIN automation_workflows w ON w.id=s.workflow_id
		WHERE w.operation_key='r4e-recovery'
	`).Scan(&occurrence, &last, &next))
	must(t, store.Pool.QueryRow(ctx, `
		SELECT e.state
		FROM automation_executions e
		JOIN automation_workflows w ON w.id=e.workflow_id
		WHERE w.operation_key='r4e-recovery'
		ORDER BY e.created_at DESC LIMIT 1
	`).Scan(&executionState))
	if occurrence != 1 || last.IsZero() || !next.After(last) || executionState != "QUEUED" {
		t.Fatal("R4E recovery marker lost", occurrence, last, next, executionState)
	}

	var knowledgeID string
	must(t, store.Pool.QueryRow(ctx, "SELECT id::text FROM ai_knowledge WHERE operation_key='r4f-recovery'").Scan(&knowledgeID))
	var chunks int
	must(t, store.Pool.QueryRow(ctx, "SELECT count(*) FROM ai_knowledge_chunks WHERE knowledge_id=$1", knowledgeID).Scan(&chunks))
	if chunks == 0 {
		t.Fatal("R4F recovery marker lost")
	}
}
