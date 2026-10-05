//go:build integration

package integration

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/intelligence"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/platform/queue"
	workflow "github.com/naseri-coder/TELMIVO/internal/plugins/automation"
)

func r4eExecution(t *testing.T, f r3d6Data, workflowID string) intelligence.Execution {
	t.Helper()
	h, err := f.ai.History(ctx, f.u.UserID, f.scope(workflow.ID), "")
	must(t, err)
	for _, x := range h {
		if x.Workflow == workflowID {
			return x
		}
	}
	t.Fatalf("execution for workflow %s not found", workflowID)
	return intelligence.Execution{}
}

func TestR4ECronSchedule(t *testing.T) {
	f := r3d6Fixture(t)
	invalid := f.wf("schedule.cron", workflow.Action{Type: "telegram.send", Value: "Invalid target"})
	invalid.Schedule = workflow.Schedule{Kind: "CRON", Cron: "* * * * *", Timezone: "UTC", Contact: "00000000-0000-0000-0000-000000000000"}
	_, err := f.ai.SaveWorkflow(ctx, f.u.UserID, f.scope(workflow.ID), "", intelligence.WorkflowChange{Workflow: invalid, Key: "r4e-invalid-context"})
	if !errors.Is(err, database.ErrInvalid) {
		t.Fatal("invalid cron target accepted", err)
	}
	w := f.wf("schedule.cron", workflow.Action{Type: "telegram.send", Value: "Scheduled ping"})
	w.Schedule = workflow.Schedule{Kind: "CRON", Cron: "* * * * *", Timezone: "UTC", User: 10101001}
	saved := f.save(t, "r4e-cron", w)
	var next time.Time
	tenant(t, f.u, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT next_at FROM automation_schedule_state WHERE workflow_id=$1", saved.ID).Scan(&next)
	})
	if !next.After(f.clock.at) {
		t.Fatal("cron next occurrence not in the future", next, f.clock.at)
	}
	f.clock.at = next
	must(t, f.ai.TickSchedules(ctx, f.u.UserID, f.u.WorkspaceID))
	x := r4eExecution(t, f, saved.ID)
	if x.State != "QUEUED" || !x.Available.Equal(next) {
		t.Fatal("cron execution not materialized at due time", x)
	}
	before := f.transport.replies.Load()
	must(t, f.ai.Run(ctx, f.u.UserID, f.scope(workflow.ID), x.ID))
	if f.transport.replies.Load() != before+1 {
		t.Fatal("cron external effect not executed exactly once")
	}
	must(t, f.ai.TickSchedules(ctx, f.u.UserID, f.u.WorkspaceID))
	h, err := f.ai.History(ctx, f.u.UserID, f.scope(workflow.ID), "")
	must(t, err)
	count := 0
	for _, v := range h {
		if v.Workflow == saved.ID {
			count++
		}
	}
	if count != 1 {
		t.Fatal("cron tick duplicated same occurrence", count)
	}
	var occurrence int64
	var next2 time.Time
	tenant(t, f.u, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT occurrence,next_at FROM automation_schedule_state WHERE workflow_id=$1", saved.ID).Scan(&occurrence, &next2)
	})
	if occurrence != 1 || !next2.After(next) {
		t.Fatal("cron state did not advance", occurrence, next2)
	}
}

func TestR4EDelayedAndSLA(t *testing.T) {
	t.Run("delayed event", func(t *testing.T) {
		f := r3d6Fixture(t)
		ticket := f.inbound(t, "r4e-delay-ticket", "Delayed support")
		w := f.wf("manual", workflow.Action{Type: "ticket.priority", Value: "HIGH"})
		w.DelaySeconds = 120
		saved := f.save(t, "r4e-delay", w)
		id, err := f.ai.Manual(ctx, f.u.UserID, f.scope(workflow.ID), saved.ID, "r4e-delay-run", ticket.Reference, "")
		must(t, err)
		x := r4eExecution(t, f, saved.ID)
		must(t, f.ai.Run(ctx, f.u.UserID, f.scope(workflow.ID), id))
		d, err := f.srv.Detail(ctx, f.u.UserID, f.scope("ticketing"), ticket.Reference, "")
		must(t, err)
		if d.Ticket.Priority != "NORMAL" {
			t.Fatal("delayed workflow executed early")
		}
		f.clock.at = x.Available.Add(time.Second)
		must(t, f.ai.Run(ctx, f.u.UserID, f.scope(workflow.ID), id))
		d, err = f.srv.Detail(ctx, f.u.UserID, f.scope("ticketing"), ticket.Reference, "")
		must(t, err)
		if d.Ticket.Priority != "HIGH" {
			t.Fatal("delayed workflow did not execute after due time")
		}
	})

	t.Run("sla resolves before effect", func(t *testing.T) {
		f := r3d6Fixture(t)
		w := f.wf("ticket.created", workflow.Action{Type: "ticket.priority", Value: "HIGH"})
		w.Schedule = workflow.Schedule{Kind: "SLA", SLASeconds: 300}
		saved := f.save(t, "r4e-sla", w)
		ticket := f.inbound(t, "r4e-sla-ticket", "SLA support")
		var envelope queue.Envelope
		tenant(t, f.u, func(tx pgx.Tx) error {
			var payload []byte
			err := tx.QueryRow(ctx, "SELECT id::text,workspace_id::text,event_type,correlation_id,payload FROM outbox_events WHERE workspace_id=$1 AND event_type='ticket.created' ORDER BY created_at DESC LIMIT 1", f.u.WorkspaceID).Scan(&envelope.ID, &envelope.WorkspaceID, &envelope.Type, &envelope.CorrelationID, &payload)
			envelope.Payload = payload
			return err
		})
		must(t, db.Tenant(ctx, f.u.UserID, f.u.WorkspaceID, false, func(tx pgx.Tx) error {
			return f.ai.DispatchTx(ctx, tx, envelope)
		}))
		x := r4eExecution(t, f, saved.ID)
		if !x.Available.Equal(f.clock.at.Add(300 * time.Second)) {
			t.Fatal("SLA deadline mismatch", x.Available, f.clock.at)
		}
		tenant(t, f.u, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "UPDATE support_tickets SET status='RESOLVED',resolved_at=$4,revision=revision+1,updated_at=$4 WHERE workspace_id=$1 AND bot_id=$2 AND reference=$3", f.u.WorkspaceID, f.bot, ticket.Reference, f.clock.at)
			return err
		})
		f.clock.at = x.Available.Add(time.Second)
		must(t, f.ai.Run(ctx, f.u.UserID, f.scope(workflow.ID), x.ID))
		x = r4eExecution(t, f, saved.ID)
		if x.State != "CANCELLED" || x.Code != "SLA_RESOLVED" {
			t.Fatal("resolved SLA produced late effect", x)
		}
		d, err := f.srv.Detail(ctx, f.u.UserID, f.scope("ticketing"), ticket.Reference, "")
		must(t, err)
		if d.Ticket.Priority != "NORMAL" {
			t.Fatal("cancelled SLA changed ticket")
		}
	})
}

func TestR4EBrowserFixture(t *testing.T) {
	f := r3d6Fixture(t)
	u, _ := r3aLogin(t, 98013001, false)
	tenant(t, u, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE subscriptions SET plan_key=$1,expires_at=NULL WHERE workspace_id=$2", f.plan, u.WorkspaceID)
		return err
	})
	bot := r3dBot(t, u, "r4e-browser")
	scope := sdk.Scope{AccountID: u.WorkspaceID, BotID: bot, PluginID: workflow.ID}
	i, err := f.core.Change(ctx, u.UserID, scope, sdk.Install, 0, json.RawMessage(`{}`), "r4e")
	must(t, err)
	_, err = f.core.Change(ctx, u.UserID, scope, sdk.Activate, i.Revision, nil, "r4e")
	must(t, err)
	tenant(t, u, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE customer_bots SET name='R4E browser bot' WHERE id=$1", bot)
		return err
	})
}

func TestR4ERecoveryPreparation(t *testing.T) {
	f := r3d6Fixture(t)
	w := f.wf("schedule.cron", workflow.Action{Type: "telegram.send", Value: "Recovery schedule"})
	w.Schedule = workflow.Schedule{Kind: "CRON", Cron: "* * * * *", Timezone: "UTC", User: 10101998}
	saved := f.save(t, "r4e-recovery", w)
	var next time.Time
	tenant(t, f.u, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT next_at FROM automation_schedule_state WHERE workflow_id=$1", saved.ID).Scan(&next)
	})
	f.clock.at = next
	must(t, f.ai.TickSchedules(ctx, f.u.UserID, f.u.WorkspaceID))
	x := r4eExecution(t, f, saved.ID)
	if x.State != "QUEUED" {
		t.Fatal("recovery schedule not queued", x)
	}
}

func TestR4ERecovered(t *testing.T) {
	store, err := database.Open(ctx, os.Getenv("TEST_RESTORE_MIGRATION_DATABASE_URL"), false)
	must(t, err)
	defer store.Pool.Close()
	var occurrence int64
	var last, next time.Time
	var state string
	must(t, store.Pool.QueryRow(ctx, `SELECT s.occurrence,s.last_at,s.next_at
		FROM automation_schedule_state s JOIN automation_workflows w ON w.id=s.workflow_id
		WHERE w.operation_key='r4e-recovery'`).Scan(&occurrence, &last, &next))
	must(t, store.Pool.QueryRow(ctx, `SELECT e.state FROM automation_executions e JOIN automation_workflows w ON w.id=e.workflow_id WHERE w.operation_key='r4e-recovery' ORDER BY e.created_at DESC LIMIT 1`).Scan(&state))
	if occurrence != 1 || last.IsZero() || !next.After(last) || state != "QUEUED" {
		t.Fatal("R4E schedule recovery lost", occurrence, last, next, state)
	}
}
