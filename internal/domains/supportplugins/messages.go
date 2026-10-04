package supportplugins

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	tickets "github.com/naseri-coder/TELMIVO/internal/plugins/ticketing"
	"strconv"
)

type Reply struct {
	Body        string               `json:"body"`
	Key         string               `json:"idempotency_key"`
	Expected    int64                `json:"expected_revision"`
	Internal    bool                 `json:"internal"`
	Template    string               `json:"template_id"`
	Attachments []tickets.Attachment `json:"attachments"`
}
type ReplyResult struct {
	ID       string `json:"message_id"`
	Delivery string `json:"delivery"`
	Replayed bool   `json:"replayed"`
}
type Sender interface {
	SupportReply(context.Context, int64, string, []tickets.Attachment) (int64, string)
}

func (s *Service) Reply(ctx context.Context, actor string, scope sdk.Scope, ref string, in Reply) (ReplyResult, error) {
	scope.PluginID = tickets.ID
	out := ReplyResult{}
	if !keyValid(in.Key) || len(in.Body) > 4000 || !tickets.AttachmentsValid(in.Attachments) || in.Internal && len(in.Attachments) > 0 {
		return out, database.ErrInvalid
	}
	permission := "tickets.reply"
	if in.Internal {
		permission = "tickets.notes"
	}
	fresh := false
	e := s.tenant(ctx, actor, scope, permission, "tickets.write", func(tx pgx.Tx, p string) error {
		var saved string
		e := tx.QueryRow(ctx, "SELECT id::text,delivery,fingerprint FROM support_messages WHERE workspace_id=$1 AND bot_id=$2 AND operation_key=$3", scope.AccountID, scope.BotID, in.Key).Scan(&out.ID, &out.Delivery, &saved)
		if e == nil {
			if saved != fingerprint([]any{actor, ref, in}) {
				return database.ErrConflict
			}
			out.Replayed = true
			return nil
		}
		if !noRows(e) {
			return e
		}
		t, e := s.ticket(ctx, tx, scope, ref)
		if e != nil {
			return e
		}
		if t.Revision != in.Expected {
			return database.ErrConflict
		}
		if !tickets.Active(t.Status) {
			return database.ErrInvalid
		}
		text := in.Body
		if in.Template != "" {
			var b []byte
			e = tx.QueryRow(ctx, "SELECT definition FROM support_templates WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3", scope.AccountID, scope.BotID, in.Template).Scan(&b)
			if e != nil {
				return e
			}
			var template tickets.Template
			if e = json.Unmarshal(b, &template); e != nil {
				return e
			}
			if !template.Enabled {
				return database.ErrInvalid
			}
			text = template.Body
		}
		if len(text) == 0 {
			return database.ErrInvalid
		}
		if e = s.quota(ctx, tx, p, scope, "ticketing.messages.monthly", 1, true); e != nil {
			return e
		}
		kind := "AGENT"
		out.Delivery = "SENDING"
		if in.Internal {
			kind = "INTERNAL"
			out.Delivery = "NOT_APPLICABLE"
		}
		e = tx.QueryRow(ctx, "INSERT INTO support_messages(workspace_id,bot_id,ticket_reference,kind,body,attachments,actor_id,operation_key,fingerprint,delivery,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id::text", scope.AccountID, scope.BotID, ref, kind, text, body(in.Attachments), actor, in.Key, fingerprint([]any{actor, ref, in}), out.Delivery, s.now()).Scan(&out.ID)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, "UPDATE support_tickets SET revision=revision+1,updated_at=$4 WHERE workspace_id=$1 AND bot_id=$2 AND reference=$3", scope.AccountID, scope.BotID, ref, s.now())
		if e != nil {
			return e
		}
		if e = audit(ctx, tx, actor, scope, "ticket.message."+kind, ref, map[string]any{"message_id": out.ID}); e != nil {
			return e
		}
		fresh = !in.Internal
		return nil
	})
	if e != nil || !fresh {
		return out, e
	}
	// A durable SENDING row commits before external effects. Same operation never resends.
	e = s.tenant(ctx, actor, scope, "tickets.reply", "telegram.send", func(tx pgx.Tx, p string) error {
		if _, e := s.check(ctx, tx, scope, "telegram.send", true); e != nil {
			return e
		}
		var state, text, identity string
		var attachments []byte
		e := tx.QueryRow(ctx, `SELECT m.delivery,m.body,m.attachments,i.external_key FROM support_messages m JOIN support_tickets t ON t.reference=m.ticket_reference AND t.workspace_id=m.workspace_id AND t.bot_id=m.bot_id JOIN crm_identities i ON i.contact_id=t.contact_id AND i.workspace_id=t.workspace_id AND i.bot_id=t.bot_id AND i.provider='telegram' WHERE m.workspace_id=$1 AND m.bot_id=$2 AND m.id=$3 FOR UPDATE OF m`, scope.AccountID, scope.BotID, out.ID).Scan(&state, &text, &attachments, &identity)
		if e != nil {
			return e
		}
		if state != "SENDING" {
			out.Delivery = state
			out.Replayed = true
			return nil
		}
		var a []tickets.Attachment
		if e = json.Unmarshal(attachments, &a); e != nil {
			return e
		}
		user, e := strconv.ParseInt(identity, 10, 64)
		if e != nil {
			return database.ErrInvalid
		}
		if s.Provider == nil {
			return sdk.ErrPermission
		}
		tr, e := s.Provider.ForBot(ctx, tx, scope)
		if e != nil {
			return e
		}
		sender, ok := tr.(Sender)
		if !ok {
			return sdk.ErrPermission
		}
		_, code := sender.SupportReply(ctx, user, text, a)
		out.Delivery = "FAILED"
		if code == "DELIVERY_ACKNOWLEDGED" {
			out.Delivery = "SENT"
		} else if code == "DELIVERY_OUTCOME_UNKNOWN" {
			out.Delivery = "UNKNOWN"
		}
		_, e = tx.Exec(ctx, "UPDATE support_messages SET delivery=$4 WHERE workspace_id=$1 AND bot_id=$2 AND id=$3", scope.AccountID, scope.BotID, out.ID, out.Delivery)
		if e != nil {
			return e
		}
		if out.Delivery == "SENT" {
			_, e = tx.Exec(ctx, "UPDATE support_tickets SET first_response_at=COALESCE(first_response_at,$4),status=CASE WHEN status IN('OPEN','PENDING_AGENT') THEN 'PENDING_CUSTOMER' ELSE status END,revision=revision+1,updated_at=$4 WHERE workspace_id=$1 AND bot_id=$2 AND reference=$3", scope.AccountID, scope.BotID, ref, s.now())
			if e != nil {
				return e
			}
		}
		return audit(ctx, tx, actor, scope, "ticket.delivery", ref, map[string]any{"message_id": out.ID, "outcome": out.Delivery})
	})
	if e != nil {
		out.Delivery = "UNKNOWN"
		return out, e
	}
	return out, nil
}
