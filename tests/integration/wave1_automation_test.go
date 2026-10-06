//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/commerce"
	workflow "github.com/naseri-coder/TELMIVO/internal/plugins/automation"
)

func TestWave1AutomationTenantQuotaIsolation(t *testing.T) {
	a, b := r3d6Fixture(t), r3d6Fixture(t)
	const metric = "automation.executions.monthly"
	for i, f := range []r3d6Data{a, b} {
		catalog, err := f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
		must(t, err)
		plan, ok := catalog.Get(commerce.PlanKind, f.plan)
		if !ok {
			t.Fatal("fixture plan missing")
		}
		limit := int64(1)
		if i == 1 {
			limit = 2
		}
		plan.Definition.Plan.Limits[metric] = commerce.Limit{Value: limit}
		f.r3d5FixtureData.save(t, plan, plan.Version)
	}
	quota, err := a.s.Consume(ctx, a.u.UserID, a.u.WorkspaceID, a.bot, metric, 1)
	must(t, err)
	if !quota.Allowed {
		t.Fatal("quota exhaustion fixture was not admitted")
	}
	var due time.Time
	ids := []string{}
	for _, f := range []r3d6Data{a, b} {
		w := f.wf("schedule.cron", workflow.Action{Type: "telegram.send", Value: "Synthetic bounded occurrence"})
		w.Schedule = workflow.Schedule{Kind: "CRON", Cron: "* * * * *", Timezone: "UTC", User: 10101001}
		saved := f.save(t, "wave1-cron", w)
		ids = append(ids, saved.ID)
		var next time.Time
		tenant(t, f.u, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT next_at FROM automation_schedule_state WHERE workflow_id=$1", saved.ID).Scan(&next)
		})
		if next.After(due) {
			due = next
		}
	}
	a.clock.at, b.clock.at = due, due
	for _, f := range []r3d6Data{a, b} {
		must(t, f.ai.TickSchedules(ctx, f.u.UserID, f.u.WorkspaceID))
		must(t, f.ai.TickSchedules(ctx, f.u.UserID, f.u.WorkspaceID))
	}
	denied := r4eExecution(t, a, ids[0])
	if denied.State != "SKIPPED" || denied.Code == "" || denied.Code == "EXECUTION_FAILED" {
		t.Fatal("quota denial was not a bounded durable business outcome", denied)
	}
	if healthy := r4eExecution(t, b, ids[1]); healthy.State != "QUEUED" {
		t.Fatal("healthy tenant did not progress", healthy)
	}
	for i, f := range []r3d6Data{a, b} {
		var occurrence int64
		var next time.Time
		tenant(t, f.u, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT occurrence,next_at FROM automation_schedule_state WHERE workflow_id=$1", ids[i]).Scan(&occurrence, &next)
		})
		if occurrence != 1 || !next.After(due) || number(t, f.u, "SELECT count(*) FROM automation_executions WHERE workflow_id=$1", ids[i]) != 1 {
			t.Fatal("duplicate occurrence or poisoned recurrence state", occurrence, next)
		}
	}
	// Make a second occurrence due before starting the real poller. Otherwise
	// the quota-denied schedule has already advanced and Poll never exercises it.
	var pollDue time.Time
	tenant(t, a.u, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT next_at FROM automation_schedule_state WHERE workflow_id=$1", ids[0]).Scan(&pollDue)
	})
	a.clock.at, b.clock.at = pollDue, pollDue
	// The real shared automation poller must continue; a peer producer using the
	// same worker context must remain active. All transport is the fixture mock.
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- a.ai.Poll(workerCtx) }()
	defer func() {
		cancel()
		select {
		case err := <-finished:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("poller terminated unexpectedly: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("poller failed to stop after explicit cancellation")
		}
	}()
	heartbeat := make(chan struct{}, 1)
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
				select {
				case heartbeat <- struct{}{}:
				default:
				}
			}
		}
	}()
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-finished:
			finished <- err // Preserve the result for shutdown verification.
			t.Fatalf("tenant denial terminated shared automation poller: %v", err)
		case <-deadline.C:
			t.Fatal("healthy execution did not finish while poller remained alive")
		case <-ticker.C:
			if number(t, b.u, "SELECT count(*) FROM automation_executions WHERE workflow_id=$1 AND state='SUCCEEDED'", ids[1]) != 2 ||
				number(t, a.u, "SELECT count(*) FROM automation_executions WHERE workflow_id=$1 AND state='SKIPPED'", ids[0]) != 2 {
				continue
			}
			select {
			case <-heartbeat:
			case <-time.After(time.Second):
				t.Fatal("peer worker producer stopped")
			}
			if workerCtx.Err() != nil || number(t, b.u, "SELECT count(*) FROM automation_executions WHERE workflow_id=$1", ids[1]) != 2 {
				t.Fatal("worker context cancelled or healthy execution duplicated")
			}
			for i, f := range []r3d6Data{a, b} {
				var occurrence int64
				var next time.Time
				tenant(t, f.u, func(tx pgx.Tx) error {
					return tx.QueryRow(ctx, "SELECT occurrence,next_at FROM automation_schedule_state WHERE workflow_id=$1", ids[i]).Scan(&occurrence, &next)
				})
				if occurrence != 2 || !next.After(pollDue) {
					t.Fatal("poller poisoned recurrence", occurrence, next)
				}
			}
			return
		}
	}
}
