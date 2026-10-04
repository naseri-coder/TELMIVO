package contentplugins

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	gate "github.com/naseri-coder/TELMIVO/internal/plugins/accessgate"
	files "github.com/naseri-coder/TELMIVO/internal/plugins/filemanager"
)

func (s *Service) file(ctx context.Context, tx pgx.Tx, scope sdk.Scope, ref string) (files.File, error) {
	var f files.File
	var b []byte
	e := tx.QueryRow(ctx, `SELECT reference,telegram_file_id,telegram_file_unique_id,type,definition,COALESCE(access_policy_id::text,''),revision,created_at,updated_at FROM content_files WHERE workspace_id=$1 AND bot_id=$2 AND reference=$3`, scope.AccountID, scope.BotID, ref).Scan(&f.Reference, &f.FileID, &f.UniqueID, &f.Kind, &b, &f.Policy, &f.Revision, &f.Created, &f.Updated)
	if e == nil {
		var d files.File
		e = json.Unmarshal(b, &d)
		f.Title = d.Title
		f.Description = d.Description
		f.Category = d.Category
		f.Enabled = d.Enabled
	}
	return f, e
}
func (s *Service) Files(ctx context.Context, actor string, scope sdk.Scope, search string) ([]files.File, error) {
	out := []files.File{}
	if len(search) > 200 {
		return out, database.ErrInvalid
	}
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		if e := s.manage(ctx, tx, actor, scope, "files.read"); e != nil {
			return e
		}
		rs, e := tx.Query(ctx, `SELECT reference FROM content_files WHERE workspace_id=$1 AND bot_id=$2 AND ($3='' OR position(lower($3) in lower((definition->>'title')||' '||(definition->>'category')||' '||(definition->>'description')))>0) ORDER BY created_at DESC,reference LIMIT 200`, scope.AccountID, scope.BotID, search)
		if e != nil {
			return e
		}
		refs := []string{}
		for rs.Next() {
			var r string
			if e = rs.Scan(&r); e != nil {
				break
			}
			refs = append(refs, r)
		}
		rs.Close()
		if e != nil {
			return e
		}
		if e = rs.Err(); e != nil {
			return e
		}
		for _, r := range refs {
			f, e := s.file(ctx, tx, scope, r)
			if e != nil {
				return e
			}
			e = tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE state='SENT'),count(*) FILTER(WHERE state='DENIED'),count(DISTINCT requester_digest) FILTER(WHERE state='SENT'),max(completed_at) FILTER(WHERE state='SENT') FROM content_delivery_events WHERE workspace_id=$1 AND bot_id=$2 AND file_reference=$3`, scope.AccountID, scope.BotID, r).Scan(&f.Delivered, &f.Denied, &f.Unique, &f.Last)
			if e != nil {
				return e
			}
			out = append(out, f)
		}
		return nil
	})
	return out, e
}

type FileChange struct {
	File     files.File `json:"file"`
	Key      string     `json:"idempotency_key"`
	Expected int64      `json:"expected_revision"`
}

func (s *Service) SaveFile(ctx context.Context, actor string, scope sdk.Scope, ref, corr string, in FileChange) (files.File, error) {
	var out files.File
	f := in.File
	f.Reference = ""
	f.Revision = 0
	f.Created = files.File{}.Created
	f.Updated = files.File{}.Updated
	f.Delivered = 0
	f.Denied = 0
	f.Unique = 0
	f.Last = nil
	if !f.Valid() || in.Expected < 0 || !operation(in.Key) {
		return out, database.ErrInvalid
	}
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		if e := s.manage(ctx, tx, actor, scope, "files.write"); e != nil {
			return e
		}
		previousPolicy := ""
		if ref != "" {
			old, e := s.file(ctx, tx, scope, ref)
			if e != nil {
				return e
			}
			previousPolicy = old.Policy
		}
		if f.Policy != "" && (ref == "" || f.Policy != previousPolicy) {
			g := scope
			g.PluginID = gate.ID
			if e := s.manage(ctx, tx, actor, g, "telegram.read_member_status"); e != nil {
				return e
			}
			p, e := s.policy(ctx, tx, g, f.Policy)
			if e != nil {
				return e
			}
			if !p.Enabled {
				return database.ErrInvalid
			}
		}
		if ref == "" {
			if in.Expected != 0 {
				return database.ErrConflict
			}
			var saved string
			e := tx.QueryRow(ctx, "SELECT reference,fingerprint FROM content_files WHERE bot_id=$1 AND operation_key=$2", scope.BotID, in.Key).Scan(&ref, &saved)
			if e == nil {
				if saved != hash(f) {
					return database.ErrConflict
				}
				out, e = s.file(ctx, tx, scope, ref)
				return e
			}
			if !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
			if e = s.quota(ctx, tx, actor, scope, "files.count", 1); e != nil {
				return e
			}
			ref = opaque()
			if ref == "" {
				return database.ErrInvalid
			}
			_, e = tx.Exec(ctx, `INSERT INTO content_files(reference,workspace_id,bot_id,telegram_file_id,telegram_file_unique_id,type,definition,access_policy_id,operation_key,fingerprint) VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::uuid,$9,$10)`, ref, scope.AccountID, scope.BotID, f.FileID, f.UniqueID, f.Kind, jsonBody(f), f.Policy, in.Key, hash(f))
			if e != nil {
				return e
			}
		} else {
			old, e := s.file(ctx, tx, scope, ref)
			if e != nil {
				return e
			}
			if old.FileID != f.FileID || old.UniqueID != f.UniqueID || old.Kind != f.Kind {
				return database.ErrInvalid
			}
			tag, e := tx.Exec(ctx, `UPDATE content_files SET definition=$4,access_policy_id=NULLIF($5,'')::uuid,revision=revision+1,updated_at=now() WHERE workspace_id=$1 AND bot_id=$2 AND reference=$3 AND revision=$6`, scope.AccountID, scope.BotID, ref, jsonBody(f), f.Policy, in.Expected)
			if e != nil {
				return e
			}
			if tag.RowsAffected() != 1 {
				return database.ErrConflict
			}
		}
		var e error
		out, e = s.file(ctx, tx, scope, ref)
		if e != nil {
			return e
		}
		return database.Audit(ctx, tx, actor, scope.AccountID, "file_manager.metadata", "content_file", ref, corr, map[string]any{}, map[string]any{"revision": out.Revision, "enabled": out.Enabled, "protected": out.Policy != ""})
	})
	return out, e
}
