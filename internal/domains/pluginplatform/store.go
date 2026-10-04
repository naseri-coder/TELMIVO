package pluginplatform

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
)

// lockBot serializes the complete graph, not merely the target installation row.
// Every runtime invocation takes the same lock, preventing disable/uninstall races.
func lockBot(ctx context.Context, tx pgx.Tx, actor, workspace, bot string) error {
	if !database.Permission(ctx, tx, actor, workspace, "bots.manage") {
		return database.ErrForbidden
	}
	var id string
	err := tx.QueryRow(ctx, "SELECT id::text FROM customer_bots WHERE workspace_id=$1 AND id::text=$2 FOR UPDATE", workspace, bot).Scan(&id)
	if err == pgx.ErrNoRows {
		return database.ErrForbidden
	}
	return err
}

func records(ctx context.Context, tx pgx.Tx, workspace, bot string) ([]sdk.Installation, error) {
	rows, err := tx.Query(ctx, `SELECT plugin_id,plugin_version,data_version,state,configuration,data,capabilities,license_scope,policy_revision,revision FROM plugin_installations WHERE workspace_id=$1 AND bot_id::text=$2 ORDER BY plugin_id`, workspace, bot)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []sdk.Installation{}
	for rows.Next() {
		i := sdk.Installation{Scope: sdk.Scope{AccountID: workspace, BotID: bot}}
		var conf, data, caps []byte
		if err := rows.Scan(&i.Scope.PluginID, &i.Version, &i.DataVersion, &i.State, &conf, &data, &caps, &i.LicenseScope, &i.PolicyRevision, &i.Revision); err != nil {
			return nil, err
		}
		i.Configuration, i.Data = conf, data
		if json.Unmarshal(caps, &i.Capabilities) != nil {
			return nil, sdk.ErrPermission
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func find(records []sdk.Installation, id string) (sdk.Installation, bool) {
	for _, i := range records {
		if i.Scope.PluginID == id {
			return i, true
		}
	}
	return sdk.Installation{}, false
}

func persist(ctx context.Context, tx pgx.Tx, i sdk.Installation, previous int64) error {
	// No DELETE path. Configuration and plugin data remain unchanged on all transitions.
	caps, err := json.Marshal(i.Capabilities)
	if err != nil {
		return err
	}
	if previous == 0 {
		_, err = tx.Exec(ctx, `INSERT INTO plugin_installations(workspace_id,bot_id,plugin_id,plugin_version,data_version,state,configuration,data,capabilities,license_scope,policy_revision) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, i.Scope.AccountID, i.Scope.BotID, i.Scope.PluginID, i.Version, i.DataVersion, i.State, []byte(i.Configuration), []byte(i.Data), caps, i.LicenseScope, i.PolicyRevision)
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE plugin_installations SET state=$1,capabilities=$2,license_scope=$3,policy_revision=$4,revision=revision+1 WHERE workspace_id=$5 AND bot_id::text=$6 AND plugin_id=$7 AND revision=$8`, i.State, caps, i.LicenseScope, i.PolicyRevision, i.Scope.AccountID, i.Scope.BotID, i.Scope.PluginID, previous)
	if err == nil && tag.RowsAffected() != 1 {
		return database.ErrConflict
	}
	return err
}
