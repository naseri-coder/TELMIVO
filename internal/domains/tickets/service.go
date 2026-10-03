package tickets

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/entitlements"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
)

type CreateInput struct {
	Subject  string `json:"subject"`
	Body     string `json:"body"`
	Priority string `json:"priority"`
	Key      string `json:"idempotency_key"`
}
type Ticket struct {
	ID        string `json:"id"`
	Subject   string `json:"subject"`
	Status    string `json:"status"`
	Priority  string `json:"priority"`
	CreatorID string `json:"creator_id"`
}

func Create(ctx context.Context, tx pgx.Tx, user, workspace, correlation string, in CreateInput) (string, error) {
	if len(in.Subject) < 1 || len(in.Subject) > 200 || len(in.Body) < 1 || len(in.Body) > 4000 || len(in.Key) < 8 || len(in.Key) > 200 {
		return "", database.ErrInvalid
	}
	if in.Priority == "" {
		in.Priority = "NORMAL"
	}
	if !ValidPriority(in.Priority) {
		return "", database.ErrInvalid
	}
	if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", workspace+":"+in.Key); e != nil {
		return "", e
	}
	var id, subject, body, actor string
	e := tx.QueryRow(ctx, "SELECT t.id::text,t.subject,m.body,m.actor_id::text FROM ticket_messages m JOIN tickets t ON t.id=m.ticket_id WHERE m.workspace_id=$1 AND m.idempotency_key=$2", workspace, in.Key).Scan(&id, &subject, &body, &actor)
	if e == nil {
		if actor != user || body != in.Body || subject != in.Subject {
			return "", database.ErrConflict
		}
		return id, nil
	}
	if e != pgx.ErrNoRows {
		return "", e
	}
	if e = entitlements.Reserve(ctx, tx, user, workspace, "tickets", "tickets.create", 1); e != nil {
		return "", e
	}
	e = tx.QueryRow(ctx, "INSERT INTO tickets(workspace_id,creator_id,subject,priority) VALUES($1,$2,$3,$4) RETURNING id::text", workspace, user, in.Subject, in.Priority).Scan(&id)
	if e != nil {
		return "", e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO ticket_messages(workspace_id,ticket_id,actor_id,body,idempotency_key) VALUES($1,$2,$3,$4,$5)", workspace, id, user, in.Body, in.Key); e != nil {
		return "", e
	}
	if e = database.Audit(ctx, tx, user, workspace, "ticket.create", "ticket", id, correlation, map[string]any{}, map[string]any{"status": "OPEN", "priority": in.Priority}); e != nil {
		return "", e
	}
	return id, database.Event(ctx, tx, workspace, "telmivo.events.ticket", "ticket.created", correlation, map[string]any{"ticket_id": id})
}

type ReplyInput struct {
	Body string `json:"body"`
	Key  string `json:"idempotency_key"`
}

func Reply(ctx context.Context, tx pgx.Tx, user, workspace, ticket, correlation string, in ReplyInput) error {
	if len(in.Body) < 1 || len(in.Body) > 4000 || len(in.Key) < 8 || len(in.Key) > 200 {
		return database.ErrInvalid
	}
	if e := entitlements.Require(ctx, tx, user, workspace, "tickets", "tickets.reply", 0); e != nil {
		return e
	}
	var creator, status string
	if e := tx.QueryRow(ctx, "SELECT creator_id::text,status FROM tickets WHERE id=$1 AND workspace_id=$2 FOR UPDATE", ticket, workspace).Scan(&creator, &status); e != nil {
		return e
	}

	var prior, actor, id string
	e := tx.QueryRow(ctx, "SELECT body,actor_id::text,ticket_id::text FROM ticket_messages WHERE workspace_id=$1 AND idempotency_key=$2", workspace, in.Key).Scan(&prior, &actor, &id)
	if e == nil {
		if prior != in.Body || actor != user || id != ticket {
			return database.ErrConflict
		}
		return nil
	}
	if e != pgx.ErrNoRows {
		return e
	}
	if status == "CLOSED" {
		return database.ErrInvalid
	}
	if _, e = tx.Exec(ctx, "INSERT INTO ticket_messages(workspace_id,ticket_id,actor_id,body,idempotency_key) VALUES($1,$2,$3,$4,$5)", workspace, ticket, user, in.Body, in.Key); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "UPDATE tickets SET status='WAITING_FOR_CUSTOMER',updated_at=now() WHERE id=$1", ticket); e != nil {
		return e
	}
	if e = database.Audit(ctx, tx, user, workspace, "ticket.reply", "ticket", ticket, correlation, map[string]any{"status": status}, map[string]any{"status": "WAITING_FOR_CUSTOMER"}); e != nil {
		return e
	}
	return database.Event(ctx, tx, workspace, "telmivo.jobs.notification", "ticket.reply", correlation, map[string]any{"user_id": creator, "ticket_id": ticket, "body": in.Body, "idempotency_key": in.Key})
}

type ActionInput struct {
	Action string `json:"action"`
	Value  string `json:"value"`
}

func Act(ctx context.Context, tx pgx.Tx, user, workspace, ticket, correlation string, in ActionInput) error {
	permission := "tickets.reply"
	if in.Action == "status" {
		permission = "tickets.close"
	}
	if in.Action == "assign" {
		permission = "tickets.assign"
	}
	if e := entitlements.Require(ctx, tx, user, workspace, "tickets", permission, 0); e != nil {
		return e
	}
	var status, priority string
	if e := tx.QueryRow(ctx, "SELECT status,priority FROM tickets WHERE id=$1 AND workspace_id=$2 FOR UPDATE", ticket, workspace).Scan(&status, &priority); e != nil {
		return e
	}
	var e error
	switch in.Action {
	case "status":
		if e = Transition(status, in.Value); e != nil {
			return database.ErrInvalid
		}
		_, e = tx.Exec(ctx, "UPDATE tickets SET status=$2,updated_at=now() WHERE id=$1", ticket, in.Value)
	case "priority":
		if !ValidPriority(in.Value) {
			return database.ErrInvalid
		}
		_, e = tx.Exec(ctx, "UPDATE tickets SET priority=$2,updated_at=now() WHERE id=$1", ticket, in.Value)
	case "note":
		if len(in.Value) < 1 || len(in.Value) > 4000 {
			return database.ErrInvalid
		}
		_, e = tx.Exec(ctx, "INSERT INTO ticket_internal_notes(workspace_id,ticket_id,actor_id,body) VALUES($1,$2,$3,$4)", workspace, ticket, user, in.Value)
	case "assign":
		var member bool
		if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM workspace_members WHERE workspace_id=$1 AND user_id=$2)", workspace, in.Value).Scan(&member); e != nil || !member {
			return database.ErrForbidden
		}
		_, e = tx.Exec(ctx, "UPDATE tickets SET assignee_id=$2,updated_at=now() WHERE id=$1", ticket, in.Value)
		if e == nil {
			_, e = tx.Exec(ctx, "INSERT INTO ticket_assignments(workspace_id,ticket_id,actor_id,assignee_id) VALUES($1,$2,$3,$4)", workspace, ticket, user, in.Value)
		}
	default:
		return database.ErrInvalid
	}
	if e != nil {
		return e
	}
	return database.Audit(ctx, tx, user, workspace, "ticket."+in.Action, "ticket", ticket, correlation, map[string]any{"status": status, "priority": priority}, map[string]any{"action": in.Action})
}
