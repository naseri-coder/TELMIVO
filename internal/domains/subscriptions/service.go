package subscriptions

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/entitlements"
	"github.com/naseri-coder/TELMIVO/internal/platform/auth"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"time"
)

type Change struct {
	Key         string     `json:"idempotency_key"`
	Plan        string     `json:"plan"`
	Reason      string     `json:"reason"`
	EffectiveAt *time.Time `json:"effective_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

func ChangePlan(ctx context.Context, tx pgx.Tx, actor, workspace, correlation string, c Change) error {
	if !database.PlatformPermission(ctx, tx, actor, "platform.plans.manage") {
		return database.ErrForbidden
	}
	if len(c.Key) < 8 || len(c.Key) > 200 {
		return database.ErrInvalid
	}
	if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(7100201)"); e != nil {
		return e
	}
	raw, _ := json.Marshal(struct {
		Actor  string
		Change Change
	}{actor, c})
	fingerprint := auth.Digest(string(raw))
	var previous string
	e := tx.QueryRow(ctx, "SELECT fingerprint FROM subscription_operations WHERE workspace_id=$1 AND idempotency_key=$2", workspace, c.Key).Scan(&previous)
	if e == nil {
		if previous != fingerprint {
			return database.ErrConflict
		}
		return nil
	}
	if e != pgx.ErrNoRows {
		return e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO subscription_operations(workspace_id,idempotency_key,fingerprint) VALUES($1,$2,$3)", workspace, c.Key, fingerprint); e != nil {
		return e
	}
	if len(c.Reason) < 1 || len(c.Reason) > 1000 {
		return database.ErrInvalid
	}
	var active bool
	if e := tx.QueryRow(ctx, "SELECT active FROM plans WHERE key=$1", c.Plan).Scan(&active); e != nil || !active {
		return database.ErrInvalid
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
		return database.ErrInvalid
	}
	if effective.After(now) {
		if c.ExpiresAt != nil {
			return database.ErrInvalid
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
