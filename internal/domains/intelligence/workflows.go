package intelligence

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	provider "github.com/naseri-coder/TELMIVO/internal/platform/ai"
	"github.com/naseri-coder/TELMIVO/internal/platform/businessevents"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/platform/queue"
	workflow "github.com/naseri-coder/TELMIVO/internal/plugins/automation"
	"strconv"
	"strings"
	"time"
)

type WorkflowChange struct {
	Workflow workflow.Workflow `json:"workflow"`
	Key      string            `json:"idempotency_key"`
	Expected int64             `json:"expected_revision"`
}

func (s *Service) workflow(ctx context.Context, tx pgx.Tx, scope sdk.Scope, id string) (workflow.Workflow, string, error) {
	var w workflow.Workflow
	var b []byte
	var actor string
	var rev int64
	e := tx.QueryRow(ctx, "SELECT definition,actor_id::text,revision,created_at,updated_at FROM automation_workflows WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3", scope.AccountID, scope.BotID, id).Scan(&b, &actor, &rev, &w.Created, &w.Updated)
	if e != nil {
		return w, actor, e
	}
	created, updated := w.Created, w.Updated
	e = json.Unmarshal(b, &w)
	w.Created, w.Updated = created, updated
	w.ID = id
	w.Revision = rev
	return w, actor, e
}
func (s *Service) Workflows(ctx context.Context, actor string, scope sdk.Scope, after string) ([]workflow.Workflow, error) {
	scope.PluginID = workflow.ID
	out := []workflow.Workflow{}
	e := s.tenant(ctx, actor, scope, "automation.manage", "workflow.execute", func(tx pgx.Tx) error {
		rs, e := tx.Query(ctx, "SELECT id::text FROM automation_workflows WHERE workspace_id=$1 AND bot_id=$2 AND ($3='' OR id::text>$3) ORDER BY id LIMIT 50", scope.AccountID, scope.BotID, after)
		if e != nil {
			return e
		}
		ids := []string{}
		for rs.Next() {
			var id string
			if e = rs.Scan(&id); e != nil {
				break
			}
			ids = append(ids, id)
		}
		rs.Close()
		if e != nil {
			return e
		}
		for _, id := range ids {
			w, _, e := s.workflow(ctx, tx, scope, id)
			if e != nil {
				return e
			}
			out = append(out, w)
		}
		return rs.Err()
	})
	return out, e
}
func (s *Service) Preview(ctx context.Context, actor string, scope sdk.Scope, w workflow.Workflow, data map[string][]string) (map[string]any, error) {
	scope.PluginID = workflow.ID
	if !w.Valid() || len(body(data)) > 4096 {
		return nil, database.ErrInvalid
	}
	e := s.tenant(ctx, actor, scope, "automation.manage", "workflow.execute", func(tx pgx.Tx) error {
		if e := s.available(ctx, tx, actor, scope, w); e != nil {
			return e
		}
		return s.validateScheduleContext(ctx, tx, scope, w)
	})
	return map[string]any{"valid": e == nil, "matches": w.Conditions.Match(data), "mutated": false}, e
}
func (s *Service) available(ctx context.Context, tx pgx.Tx, actor string, scope sdk.Scope, w workflow.Workflow) error {
	for _, a := range w.Actions {
		target, cap, permission := "", "", ""
		switch {
		case strings.HasPrefix(a.Type, "crm."):
			target, cap, permission = "crm", "crm.write", "crm.manage"
		case strings.HasPrefix(a.Type, "ticket."):
			target, cap, permission = "ticketing", "tickets.write", "tickets.manage"
			if a.Type == "ticket.assign" {
				permission = "tickets.assign"
			}
			if a.Type == "ticket.status" {
				permission = "tickets.close"
			}
			if a.Type == "ticket.note" {
				permission = "tickets.notes"
			}
			if a.Type == "ticket.reply" {
				permission = "tickets.reply"
			}
		case strings.HasPrefix(a.Type, "ai."):
			target, cap, permission = "ai-assistant", "ai.invoke", "ai.invoke"
		case a.Type == "telegram.send":
			target, cap, permission = "automation", "telegram.send", "automation.manage"
		}
		if target != "" {
			scope.PluginID = target
			if e := s.validate(ctx, tx, actor, scope, permission, cap, false); e != nil {
				return e
			}
		}
	}
	return nil
}
func (s *Service) SaveWorkflow(ctx context.Context, actor string, scope sdk.Scope, id string, in WorkflowChange) (workflow.Workflow, error) {
	scope.PluginID = workflow.ID
	w := in.Workflow
	w.ID = ""
	w.Revision = 0
	w.Created = time.Time{}
	w.Updated = time.Time{}
	if !w.Valid() || !key(in.Key) || in.Expected < 0 {
		return w, database.ErrInvalid
	}
	fp := hash(w)
	e := s.tenant(ctx, actor, scope, "automation.manage", "workflow.execute", func(tx pgx.Tx) error {
		if e := s.available(ctx, tx, actor, scope, w); e != nil {
			return e
		}
		if e := s.validateScheduleContext(ctx, tx, scope, w); e != nil {
			return e
		}
		if id == "" {
			if in.Expected != 0 {
				return database.ErrConflict
			}
			var saved string
			e := tx.QueryRow(ctx, "SELECT id::text,fingerprint FROM automation_workflows WHERE workspace_id=$1 AND bot_id=$2 AND operation_key=$3", scope.AccountID, scope.BotID, in.Key).Scan(&id, &saved)
			if e == nil {
				if saved != fp {
					return database.ErrConflict
				}
				var e error
				w, _, e = s.workflow(ctx, tx, scope, id)
				if e != nil {
					return e
				}
				return s.syncSchedule(ctx, tx, scope, w)
			}
			if !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
			var count int64
			if e = tx.QueryRow(ctx, "SELECT count(*) FROM automation_workflows WHERE workspace_id=$1 AND bot_id=$2", scope.AccountID, scope.BotID).Scan(&count); e != nil {
				return e
			}
			if count >= workflow.MaxWorkflows {
				return &provider.Failure{Code: "WORKFLOW_LIMIT"}
			}
			if e = s.quota(ctx, tx, scope, "automation.workflows.count", count+1, false); e != nil {
				return e
			}
			e = tx.QueryRow(ctx, "INSERT INTO automation_workflows(workspace_id,bot_id,actor_id,definition,operation_key,fingerprint,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$7) RETURNING id::text", scope.AccountID, scope.BotID, actor, body(w), in.Key, fp, s.now()).Scan(&id)
			if e != nil {
				return e
			}
		} else {
			tag, e := tx.Exec(ctx, "UPDATE automation_workflows SET definition=$4,actor_id=$5,revision=revision+1,updated_at=$6 WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3 AND revision=$7", scope.AccountID, scope.BotID, id, body(w), actor, s.now(), in.Expected)
			if e != nil {
				return e
			}
			if tag.RowsAffected() != 1 {
				return database.ErrConflict
			}
		}
		var e error
		w, _, e = s.workflow(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		if e = s.syncSchedule(ctx, tx, scope, w); e != nil {
			return e
		}
		return audit(ctx, tx, actor, scope, "automation.workflow.updated", id, map[string]any{"revision": w.Revision, "enabled": w.Enabled, "schedule": w.ScheduleKind()})
	})
	return w, e
}

// DispatchTx runs in the existing inbox/outbox transaction; it never performs an action.
func (s *Service) DispatchTx(ctx context.Context, tx pgx.Tx, envelope queue.Envelope) error {
	if !workflow.Contains(workflow.Triggers, envelope.Type) || len(envelope.Payload) > 8192 {
		return database.ErrInvalid
	}
	var event businessevents.Event
	if strict(envelope.Payload, &event) != nil || event.Bot == "" || len(event.Workflows) > workflow.MaxDepth || event.Depth < 0 {
		return database.ErrInvalid
	}
	scope := sdk.Scope{AccountID: envelope.WorkspaceID, BotID: event.Bot, PluginID: workflow.ID}
	if event.Correlation == "" {
		event.Correlation = envelope.CorrelationID
		if event.Correlation == "" {
			event.Correlation = envelope.ID
		}
	}
	if e := s.lock(ctx, tx, scope); e != nil {
		return e
	}
	rs, e := tx.Query(ctx, "SELECT id::text FROM automation_workflows WHERE workspace_id=$1 AND bot_id=$2 AND (definition->>'enabled')::boolean AND definition->>'trigger'=$3 ORDER BY id LIMIT 100", scope.AccountID, scope.BotID, envelope.Type)
	if e != nil {
		return e
	}
	ids := []string{}
	for rs.Next() {
		var id string
		if e = rs.Scan(&id); e != nil {
			break
		}
		ids = append(ids, id)
	}
	rs.Close()
	if e != nil {
		return e
	}
	if e = s.hydrate(ctx, tx, scope, &event); e != nil {
		return e
	}
	for _, id := range ids {
		if _, e = s.enqueue(ctx, tx, scope, id, envelope.ID, event); e != nil {
			return e
		}
	}
	return rs.Err()
}
func (s *Service) hydrate(ctx context.Context, tx pgx.Tx, scope sdk.Scope, e *businessevents.Event) error {
	if e.Fields == nil {
		e.Fields = map[string][]string{}
	}
	if e.Ticket != "" {
		var priority, status, dep, contact string
		if err := tx.QueryRow(ctx, "SELECT priority,status,department_id::text,contact_id::text FROM support_tickets WHERE workspace_id=$1 AND bot_id=$2 AND reference=$3", scope.AccountID, scope.BotID, e.Ticket).Scan(&priority, &status, &dep, &contact); err != nil {
			return err
		}
		e.Contact = contact
		e.Fields["ticket.priority"] = []string{priority}
		e.Fields["ticket.status"] = []string{status}
		e.Fields["ticket.department"] = []string{dep}
	}
	if e.Contact != "" {
		var tags []byte
		if err := tx.QueryRow(ctx, "SELECT definition->'tags' FROM crm_contacts WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3", scope.AccountID, scope.BotID, e.Contact).Scan(&tags); err != nil {
			return err
		}
		var v []string
		if json.Unmarshal(tags, &v) != nil {
			return database.ErrInvalid
		}
		e.Fields["contact.tags"] = v
		var user string
		err := tx.QueryRow(ctx, "SELECT external_key FROM crm_identities WHERE workspace_id=$1 AND bot_id=$2 AND contact_id::text=$3 AND provider='telegram'", scope.AccountID, scope.BotID, e.Contact).Scan(&user)
		if err == nil {
			e.User, _ = strconv.ParseInt(user, 10, 64)
		}
	}
	return nil
}
func (s *Service) enqueue(ctx context.Context, tx pgx.Tx, scope sdk.Scope, id, eventID string, event businessevents.Event) (string, error) {
	w, actor, e := s.workflow(ctx, tx, scope, id)
	if e != nil {
		return "", e
	}
	var existing string
	var original []byte
	e = tx.QueryRow(ctx, "SELECT id::text,event FROM automation_executions WHERE workflow_id=$1 AND event_id=$2", id, eventID).Scan(&existing, &original)
	if e == nil {
		if strings.HasPrefix(eventID, "manual:") {
			var previous businessevents.Event
			if json.Unmarshal(original, &previous) != nil {
				return "", database.ErrInvalid
			}
			if previous.Actor != event.Actor || previous.Ticket != event.Ticket || previous.Contact != event.Contact {
				return "", database.ErrConflict
			}
		}
		return existing, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return "", e
	}
	availableAt := s.now().Add(w.Delay())
	if event.Fields == nil {
		event.Fields = map[string][]string{}
	}
	if w.ScheduleKind() == "SLA" || (w.ScheduleKind() == "EVENT" && w.DelaySeconds > 0) {
		event.Fields["schedule.kind"] = []string{w.ScheduleKind()}
		event.Fields["schedule.fire_at"] = []string{availableAt.UTC().Format(time.RFC3339)}
	}
	state, code := "QUEUED", ""
	if !w.Enabled {
		state, code = "SKIPPED", "WORKFLOW_DISABLED"
	} else if event.Depth >= workflow.MaxDepth || workflow.Contains(event.Workflows, id) {
		state, code = "SKIPPED", "LOOP_PREVENTED"
	} else if w.ScheduleKind() == "SLA" && event.Ticket == "" {
		state, code = "SKIPPED", "SLA_SUBJECT_MISSING"
	} else if !w.Conditions.Match(event.Fields) {
		state, code = "SKIPPED", "CONDITIONS_NOT_MATCHED"
	} else if err := s.validate(ctx, tx, actor, scope, "automation.manage", "workflow.execute", true); err != nil {
		if !admissionDenial(err) {
			return "", err
		}
		state, code = "SKIPPED", failure(err)
	} else if err := s.quota(ctx, tx, scope, "automation.executions.monthly", 1, true); err != nil {
		if !admissionDenial(err) {
			return "", err
		}
		state, code = "SKIPPED", failure(err)
	}
	var out string
	e = tx.QueryRow(ctx, "INSERT INTO automation_executions(workspace_id,bot_id,workflow_id,event_id,actor_id,snapshot,event,state,code,available_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id::text", scope.AccountID, scope.BotID, id, eventID, actor, body(w), body(event), state, code, availableAt, s.now()).Scan(&out)
	if e != nil {
		return "", e
	}
	return out, audit(ctx, tx, actor, scope, "automation.execution.queued", out, map[string]any{"state": state, "code": code})
}
func (s *Service) Manual(ctx context.Context, actor string, scope sdk.Scope, id, k, ticket, contact string) (string, error) {
	scope.PluginID = workflow.ID
	if !key(k) {
		return "", database.ErrInvalid
	}
	var out string
	e := s.tenant(ctx, actor, scope, "automation.manage", "workflow.execute", func(tx pgx.Tx) error {
		w, _, e := s.workflow(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		if w.Trigger != "manual" {
			return database.ErrInvalid
		}
		event := businessevents.Event{Bot: scope.BotID, Actor: actor, Ticket: ticket, Contact: contact, Fields: map[string][]string{}, Workflows: []string{}, Correlation: "manual:" + hash([]string{id, k})[:32]}
		if e = s.hydrate(ctx, tx, scope, &event); e != nil {
			return e
		}
		out, e = s.enqueue(ctx, tx, scope, id, "manual:"+k, event)
		return e
	})
	return out, e
}

// Options exposes only integrations currently admitted for this actual caller.
func (s *Service) Options(ctx context.Context, actor string, scope sdk.Scope) (map[string][]string, error) {
	scope.PluginID = workflow.ID
	out := map[string][]string{"triggers": {"manual", "schedule.cron"}, "actions": {}}
	e := s.tenant(ctx, actor, scope, "automation.manage", "workflow.execute", func(tx pgx.Tx) error {
		for _, a := range workflow.Actions {
			if s.available(ctx, tx, actor, scope, workflow.Workflow{Actions: []workflow.Action{{Type: a}}}) == nil {
				out["actions"] = append(out["actions"], a)
			}
		}
		for _, trigger := range workflow.Triggers {
			target, cap, permission := "", "", ""
			switch {
			case strings.HasPrefix(trigger, "crm."):
				target, cap, permission = "crm", "crm.read", "crm.read"
			case strings.HasPrefix(trigger, "ticket."):
				target, cap, permission = "ticketing", "tickets.read", "tickets.read"
			case strings.HasPrefix(trigger, "file."):
				target, cap, permission = "file-manager", "files.read", "bots.manage"
			}
			if target != "" {
				sc := scope
				sc.PluginID = target
				if s.validate(ctx, tx, actor, sc, permission, cap, false) == nil {
					out["triggers"] = append(out["triggers"], trigger)
				}
			}
		}
		return nil
	})
	return out, e
}
