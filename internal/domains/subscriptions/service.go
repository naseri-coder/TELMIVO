package subscriptions

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/entitlements"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"time"
)

type Change struct {
	Plan        string     `json:"plan"`
	Reason      string     `json:"reason"`
	EffectiveAt *time.Time `json:"effective_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

func ChangePlan(ctx context.Context, tx pgx.Tx, actor, workspace, correlation string, c Change) error {
	if len(c.Reason) < 1 || len(c.Reason) > 1000 {
		return errors.New("reason required")
	}
	var active bool
	if e := tx.QueryRow(ctx, "SELECT active FROM plans WHERE key=$1", c.Plan).Scan(&active); e != nil || !active {
		return errors.New("inactive plan")
	}
	if _, e := tx.Exec(ctx, "SELECT 1 FROM subscriptions WHERE workspace_id=$1 FOR UPDATE", workspace); e != nil {
		return e
	}
	old, e := entitlements.EffectivePlan(ctx, tx, workspace)
	if e != nil {
		return e
	}
	now := time.Now()
	effective := now
	if c.EffectiveAt != nil {
		effective = *c.EffectiveAt
	}
	if c.ExpiresAt != nil && !c.ExpiresAt.After(effective) {
		return errors.New("invalid expiry")
	}
	if effective.After(now) {
		if c.ExpiresAt != nil {
			return errors.New("scheduled temporary plans not supported in V0.1")
		}
		_, e = tx.Exec(ctx, "UPDATE subscriptions SET scheduled_plan_key=$2,scheduled_at=$3,version=version+1,updated_at=now() WHERE workspace_id=$1", workspace, c.Plan, effective)
	} else {
		var prev *string
		if c.ExpiresAt != nil {
			prev = &old
		}
		_, e = tx.Exec(ctx, "UPDATE subscriptions SET plan_key=$2,previous_plan_key=$3,expires_at=$4,scheduled_plan_key=NULL,scheduled_at=NULL,version=version+1,updated_at=now() WHERE workspace_id=$1", workspace, c.Plan, prev, c.ExpiresAt)
	}
	if e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO subscription_events(workspace_id,actor_id,old_plan,new_plan,reason,effective_at,expires_at,correlation_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)", workspace, actor, old, c.Plan, c.Reason, effective, c.ExpiresAt, correlation); e != nil {
		return e
	}
	if e = database.Audit(ctx, tx, actor, workspace, "subscription.change", "subscription", workspace, correlation, map[string]any{"plan": old}, c); e != nil {
		return e
	}
	// No resource delete/restrict mutation occurs during downgrade. Creation checks current limits.
	return database.Event(ctx, tx, workspace, "telmivo.events.subscription", "subscription.changed", correlation, map[string]any{"workspace_id": workspace, "plan": c.Plan})
}
