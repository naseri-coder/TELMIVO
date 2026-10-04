package downloads

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/policies"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
)

type CreateInput struct {
	URL      string `json:"url"`
	FormatID string `json:"format_id"`
	Key      string `json:"idempotency_key"`
}

func FixtureFormats() []Format {
	return []Format{{ID: "fixture-360", Quality: "360p", Kind: "video", SizeBytes: 1024}, {ID: "fixture-audio", Quality: "audio", Kind: "audio", SizeBytes: 512}}
}
func CreateSynthetic(ctx context.Context, tx pgx.Tx, user, workspace, correlation string, in CreateInput) (string, error) {
	provider, e := Provider(in.URL)
	if e != nil {
		return "", e
	}
	if len(in.URL) > 2048 || len(in.Key) < 8 || len(in.Key) > 200 {
		return "", database.ErrInvalid
	}
	formats := FixtureFormats()
	if _, e = Select(formats, in.FormatID); e != nil {
		return "", e
	}
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", workspace+":"+in.Key); e != nil {
		return "", e
	}
	var id, priorURL, priorFormat string
	e = tx.QueryRow(ctx, "SELECT id::text,source_url,format_id FROM download_jobs WHERE workspace_id=$1 AND idempotency_key=$2", workspace, in.Key).Scan(&id, &priorURL, &priorFormat)
	if e == nil {
		if priorURL != in.URL || priorFormat != in.FormatID {
			return "", database.ErrConflict
		}
		return id, nil
	}
	if e != pgx.ErrNoRows {
		return "", e
	}
	var role string
	if e = tx.QueryRow(ctx, "SELECT role_key FROM workspace_members WHERE workspace_id=$1 AND user_id=$2", workspace, user).Scan(&role); e != nil {
		return "", e
	}
	if _, e = policies.Reserve(ctx, tx, policies.Request{User: user, Workspace: workspace, Role: role, Feature: "downloads", Permission: "downloads.create", Requested: 1}, in.Key); e != nil {
		return "", e
	}
	e = tx.QueryRow(ctx, "INSERT INTO download_jobs(workspace_id,creator_id,provider,source_url,format_id,formats,state,synthetic,idempotency_key) VALUES($1,$2,$3,$4,$5,$6,'QUEUED',true,$7) RETURNING id::text", workspace, user, provider, in.URL, in.FormatID, formats, in.Key).Scan(&id)
	if e != nil {
		return "", e
	}
	if e = database.Audit(ctx, tx, user, workspace, "download.create", "download_job", id, correlation, map[string]any{}, map[string]any{"synthetic": true}); e != nil {
		return "", e
	}
	return id, database.Event(ctx, tx, workspace, "telmivo.jobs.download", "download.queued", correlation, map[string]any{"download_job_id": id})
}
func CompleteFixture(ctx context.Context, tx pgx.Tx, workspace, id string) error {
	var state, key string
	var synthetic bool
	if e := tx.QueryRow(ctx, "SELECT state,synthetic,idempotency_key FROM download_jobs WHERE id=$1 AND workspace_id=$2 FOR UPDATE", id, workspace).Scan(&state, &synthetic, &key); e != nil {
		return e
	}
	if !synthetic {
		return errors.New("live downloader disabled")
	}
	if state == "COMPLETED" {
		return nil
	}
	if state == "CANCELLED" {
		return policies.Settle(ctx, tx, workspace, key, false)
	}
	for _, next := range []string{"ANALYZING", "DOWNLOADING", "PROCESSING", "UPLOADING", "COMPLETED"} {
		if e := Transition(state, next); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, "UPDATE download_jobs SET state=$2,updated_at=now() WHERE id=$1", id, next); e != nil {
			return e
		}
		state = next
	}
	if e := policies.Settle(ctx, tx, workspace, key, true); e != nil {
		return e
	}
	// This tests durable orchestration, not a fabricated real media download or Telegram delivery.
	return nil
}
