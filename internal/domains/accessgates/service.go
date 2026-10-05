package accessgates

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
)

type MembershipService interface {
	Membership(context.Context, int64, int64) (Membership, error)
}

func Check(ctx context.Context, tx pgx.Tx, workspace string, user int64, service MembershipService) (bool, error) {
	rows, e := tx.Query(ctx, "SELECT id::text,mode,minimum_required FROM access_gates WHERE workspace_id=$1 AND enabled", workspace)
	if e != nil {
		return false, e
	}
	type gate struct {
		id   string
		mode Mode
		min  int
	}
	var gates []gate
	for rows.Next() {
		var g gate
		if e = rows.Scan(&g.id, &g.mode, &g.min); e != nil {
			rows.Close()
			return false, e
		}
		gates = append(gates, g)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return false, e
	}
	for _, g := range gates {
		rs, e := tx.Query(ctx, "SELECT telegram_chat_id FROM access_gate_channels WHERE gate_id=$1 ORDER BY position", g.id)
		if e != nil {
			return false, e
		}
		var ids []int64
		for rs.Next() {
			var id int64
			if e = rs.Scan(&id); e != nil {
				rs.Close()
				return false, e
			}
			ids = append(ids, id)
		}
		e = rs.Err()
		rs.Close()
		if e != nil {
			return false, e
		}
		var memberships []Membership
		for _, id := range ids {
			if service == nil {
				return false, errors.New("membership transport disabled")
			}
			m, e := service.Membership(ctx, id, user)
			if e != nil {
				return false, e
			}
			memberships = append(memberships, m)
		}
		allowed, e := Evaluate(true, false, g.mode, g.min, memberships)
		if e != nil || !allowed {
			return false, e
		}
	}
	return true, nil
}
