package businessplugins

import (
	"context"
	"github.com/jackc/pgx/v5"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"log/slog"
	"time"
)

// Poll uses the existing worker process, sequential bounded work and no new infrastructure.
func (s *Service) Poll(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	cursor := ""
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			rs, e := s.DB.Pool.Query(ctx, "SELECT user_id::text,workspace_id::text FROM workspace_members WHERE role_key='OWNER' AND workspace_id::text>$1 ORDER BY workspace_id LIMIT 20", cursor)
			if e != nil {
				return e
			}
			type member struct{ actor, account string }
			members := []member{}
			for rs.Next() {
				var m member
				if e = rs.Scan(&m.actor, &m.account); e != nil {
					break
				}
				members = append(members, m)
			}
			rowsErr := rs.Err()
			rs.Close()
			if e != nil {
				return e
			}
			if rowsErr != nil {
				return rowsErr
			}
			if len(members) == 0 {
				cursor = ""
				continue
			}
			for _, m := range members {
				cursor = m.account
				type pending struct{ id, bot string }
				items := []pending{}
				e = s.DB.Tenant(ctx, m.actor, m.account, false, func(tx pgx.Tx) error {
					rows, e := tx.Query(ctx, "SELECT c.id::text,c.bot_id::text FROM broadcast_campaigns c WHERE c.workspace_id=$1 AND (c.state IN('SCHEDULED','RUNNING') AND c.scheduled_at<=$2 OR EXISTS(SELECT 1 FROM plugin_broadcast_recipients r WHERE r.campaign_id=c.id AND r.workspace_id=c.workspace_id AND r.state='SENDING' AND r.lease_until<=$2)) ORDER BY c.id LIMIT 4", m.account, s.now())
					if e != nil {
						return e
					}
					defer rows.Close()
					for rows.Next() {
						var p pending
						if e = rows.Scan(&p.id, &p.bot); e != nil {
							return e
						}
						items = append(items, p)
					}
					return rows.Err()
				})
				if e != nil {
					return e
				}
				for _, p := range items {
					if e = s.DeliverOne(ctx, m.actor, sdk.Scope{AccountID: m.account, BotID: p.bot}, p.id); e != nil {
						slog.WarnContext(ctx, "broadcast_delivery_blocked", "code", "ADMISSION_OR_DELIVERY_FAILURE")
					}
				}
			}
		}
	}
}
