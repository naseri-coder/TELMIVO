package businessplugins

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/plugins/broadcast"
	"strconv"
)

func historyAfter(after string) int64 { v, _ := strconv.ParseInt(after, 10, 64); return v }
func validHistoryCursor(after string) bool {
	if after == "" {
		return true
	}
	v, e := strconv.ParseInt(after, 10, 64)
	return e == nil && v > 0 && fmtUser(v) == after
}
func (s *Service) campaignHistory(ctx context.Context, tx pgx.Tx, scope sdk.Scope, id string) error {
	_, e := tx.Exec(ctx, "INSERT INTO broadcast_campaign_history(workspace_id,bot_id,campaign_id,revision,state,created_at) SELECT workspace_id,bot_id,id,revision,state,$4 FROM broadcast_campaigns WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3", scope.AccountID, scope.BotID, id, s.now())
	return e
}
func (s *Service) CampaignHistory(ctx context.Context, actor string, scope sdk.Scope, id, after string) ([]map[string]any, error) {
	scope.PluginID = broadcast.ID
	out := []map[string]any{}
	if !validHistoryCursor(after) {
		return out, database.ErrInvalid
	}
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, "SELECT revision,state,created_at FROM broadcast_campaign_history WHERE workspace_id=$1 AND bot_id=$2 AND campaign_id::text=$3 AND revision>$4 ORDER BY revision LIMIT 50", scope.AccountID, scope.BotID, id, historyAfter(after))
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var revision int64
			var state string
			var at any
			if e = rows.Scan(&revision, &state, &at); e != nil {
				return e
			}
			out = append(out, map[string]any{"cursor": fmtUser(revision), "revision": revision, "state": state, "created_at": at})
		}
		return rows.Err()
	})
	return out, e
}
