package supportplugins

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/plugins/crm"
	tickets "github.com/naseri-coder/TELMIVO/internal/plugins/ticketing"
	"strconv"
	"time"
)

type contactsHandle struct {
	s     *Service
	tx    pgx.Tx
	p     string
	scope sdk.Scope
}

func (h contactsHandle) Resolve(ctx context.Context, r crm.ResolveRequest) (crm.Contact, error) {
	h.scope.PluginID = crm.ID
	if _, e := h.s.check(ctx, h.tx, h.scope, "crm.write", true); e != nil {
		return crm.Contact{}, e
	}
	return h.s.resolve(ctx, h.tx, h.p, h.scope, r)
}
func (s *Service) ticket(ctx context.Context, tx pgx.Tx, scope sdk.Scope, ref string) (tickets.Ticket, error) {
	var t tickets.Ticket
	var tags []byte
	e := tx.QueryRow(ctx, `SELECT t.reference,t.contact_id::text,c.definition->>'name',t.department_id::text,d.definition->>'name',t.subject,t.status,t.priority,COALESCE(t.assignee_id::text,''),t.tags,t.revision,t.created_at,t.updated_at,t.closed_at,t.first_response_at,t.resolved_at,t.response_due_at,t.resolution_due_at FROM support_tickets t JOIN crm_contacts c ON c.id=t.contact_id AND c.workspace_id=t.workspace_id AND c.bot_id=t.bot_id JOIN support_departments d ON d.id=t.department_id AND d.workspace_id=t.workspace_id AND d.bot_id=t.bot_id WHERE t.workspace_id=$1 AND t.bot_id=$2 AND t.reference=$3`, scope.AccountID, scope.BotID, ref).Scan(&t.Reference, &t.ContactID, &t.ContactName, &t.DepartmentID, &t.DepartmentName, &t.Subject, &t.Status, &t.Priority, &t.Assignee, &tags, &t.Revision, &t.Created, &t.Updated, &t.Closed, &t.FirstResponse, &t.Resolved, &t.ResponseDue, &t.ResolutionDue)
	if e == nil {
		e = json.Unmarshal(tags, &t.Tags)
		t.Evaluate(s.now())
	}
	return t, e
}
func (s *Service) department(ctx context.Context, tx pgx.Tx, scope sdk.Scope, id string) (tickets.Department, error) {
	var d tickets.Department
	var b []byte
	e := tx.QueryRow(ctx, "SELECT id::text,definition,revision FROM support_departments WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3", scope.AccountID, scope.BotID, id).Scan(&d.ID, &b, &d.Revision)
	if e == nil {
		var x tickets.Department
		e = json.Unmarshal(b, &x)
		x.ID = d.ID
		x.Revision = d.Revision
		d = x
	}
	return d, e
}
func (s *Service) receive(ctx context.Context, tx pgx.Tx, p string, scope sdk.Scope, in tickets.Inbound) (tickets.Ticket, error) {
	var t tickets.Ticket
	// Both absent and empty attachments represent the same inbound request.
	if in.Attachments == nil {
		in.Attachments = []tickets.Attachment{}
	}
	fp := fingerprint(in)
	if in.User <= 0 || len(in.Body) == 0 || len(in.Body) > 4000 || !keyValid(in.Key) || !tickets.AttachmentsValid(in.Attachments) {
		return t, database.ErrInvalid
	}
	var saved, ref string
	e := tx.QueryRow(ctx, "SELECT ticket_reference,fingerprint FROM support_messages WHERE workspace_id=$1 AND bot_id=$2 AND operation_key=$3", scope.AccountID, scope.BotID, in.Key).Scan(&ref, &saved)
	if e == nil {
		if saved != fp {
			return t, database.ErrConflict
		}
		return s.ticket(ctx, tx, scope, ref)
	}
	if !noRows(e) {
		return t, e
	}
	var dep tickets.Department
	if in.Department == "" {
		e = tx.QueryRow(ctx, "SELECT id::text FROM support_departments WHERE workspace_id=$1 AND bot_id=$2 AND (definition->>'enabled')::boolean ORDER BY (definition->>'ordering')::int,id LIMIT 1", scope.AccountID, scope.BotID).Scan(&in.Department)
		if e != nil {
			return t, database.ErrInvalid
		}
	}
	dep, e = s.department(ctx, tx, scope, in.Department)
	if e != nil || !dep.Enabled {
		return t, database.ErrInvalid
	}
	var identity tickets.Contacts = contactsHandle{s, tx, p, scope}
	c, e := identity.Resolve(ctx, crm.ResolveRequest{Identity: crm.Identity{Provider: "telegram", Key: strconv.FormatInt(in.User, 10)}, Name: in.Name, Username: in.Username, Locale: in.Locale})
	if e != nil {
		return t, e
	}
	if c.Status != "ACTIVE" {
		return t, database.ErrForbidden
	}
	e = tx.QueryRow(ctx, "SELECT reference FROM support_tickets WHERE workspace_id=$1 AND bot_id=$2 AND contact_id=$3 AND department_id=$4 AND status IN('OPEN','PENDING_CUSTOMER','PENDING_AGENT')", scope.AccountID, scope.BotID, c.ID, dep.ID).Scan(&ref)
	now := s.now()
	if noRows(e) {
		count, e := s.count(ctx, tx, scope, "ticketing.active_tickets.count", "support_tickets", "AND status IN('OPEN','PENDING_CUSTOMER','PENDING_AGENT')", nil)
		if e != nil {
			return t, e
		}
		if e = s.occupancy(ctx, tx, p, scope, "ticketing.active_tickets.count", count+1); e != nil {
			return t, e
		}
		ref = reference()
		subject := in.Body
		if len(subject) > 180 {
			subject = string([]rune(subject)[:min(80, len([]rune(subject)))])
		}
		var response, resolution *time.Time
		if dep.ResponseSeconds > 0 {
			v := now.Add(time.Duration(dep.ResponseSeconds) * time.Second)
			response = &v
		}
		if dep.ResolutionSeconds > 0 {
			v := now.Add(time.Duration(dep.ResolutionSeconds) * time.Second)
			resolution = &v
		}
		_, e = tx.Exec(ctx, "INSERT INTO support_tickets(reference,workspace_id,bot_id,contact_id,department_id,subject,status,priority,created_at,updated_at,response_due_at,resolution_due_at) VALUES($1,$2,$3,$4,$5,$6,'OPEN','NORMAL',$7,$7,$8,$9)", ref, scope.AccountID, scope.BotID, c.ID, dep.ID, subject, now, response, resolution)
		if e != nil {
			return t, e
		}
		if e = audit(ctx, tx, p, scope, "ticket.created", ref, map[string]any{"contact": c.ID, "department": dep.ID}); e != nil {
			return t, e
		}
	} else if e != nil {
		return t, e
	}
	if e = s.quota(ctx, tx, p, scope, "ticketing.messages.monthly", 1, true); e != nil {
		return t, e
	}
	_, e = tx.Exec(ctx, "INSERT INTO support_messages(workspace_id,bot_id,ticket_reference,kind,body,attachments,operation_key,fingerprint,delivery,created_at) VALUES($1,$2,$3,'CUSTOMER',$4,$5,$6,$7,'NOT_APPLICABLE',$8)", scope.AccountID, scope.BotID, ref, in.Body, body(in.Attachments), in.Key, fp, now)
	if e != nil {
		return t, e
	}
	if e = audit(ctx, tx, p, scope, "ticket.customer_message", ref, map[string]any{}); e != nil {
		return t, e
	}
	_, e = tx.Exec(ctx, "UPDATE support_tickets SET status='PENDING_AGENT',revision=revision+1,updated_at=$4 WHERE workspace_id=$1 AND bot_id=$2 AND reference=$3", scope.AccountID, scope.BotID, ref, now)
	if e != nil {
		return t, e
	}
	return s.ticket(ctx, tx, scope, ref)
}

type Detail struct {
	Ticket   tickets.Ticket        `json:"ticket"`
	Contact  crm.Contact           `json:"contact"`
	Messages Page[tickets.Message] `json:"messages"`
}

func (s *Service) Detail(ctx context.Context, actor string, scope sdk.Scope, ref, after string) (Detail, error) {
	scope.PluginID = tickets.ID
	var out Detail
	out.Messages.Items = []tickets.Message{}
	start, e := cursor(after)
	if e != nil {
		return out, e
	}
	e = s.tenant(ctx, actor, scope, "tickets.read", "tickets.read", func(tx pgx.Tx, p string) error {
		var e error
		out.Ticket, e = s.ticket(ctx, tx, scope, ref)
		if e != nil {
			return e
		}
		out.Contact, e = s.contact(ctx, tx, scope, out.Ticket.ContactID)
		if e != nil {
			return e
		}
		if !database.Permission(ctx, tx, actor, scope.AccountID, "crm.read") {
			out.Contact = crm.Contact{ID: out.Contact.ID, Name: out.Contact.Name}
		}
		allowNotes := database.Permission(ctx, tx, actor, scope.AccountID, "tickets.notes")
		rs, e := tx.Query(ctx, `SELECT id::text,kind,body,attachments,COALESCE(actor_id::text,''),delivery,created_at FROM support_messages WHERE workspace_id=$1 AND bot_id=$2 AND ticket_reference=$3 AND ($4 OR kind<>'INTERNAL') AND ($5='' OR (created_at::text||':'||id::text)>$5) ORDER BY created_at,id LIMIT 101`, scope.AccountID, scope.BotID, ref, allowNotes, start)
		if e != nil {
			return e
		}
		defer rs.Close()
		for rs.Next() {
			var m tickets.Message
			var b []byte
			if e = rs.Scan(&m.ID, &m.Kind, &m.Body, &b, &m.Actor, &m.Delivery, &m.Created); e != nil {
				return e
			}
			if e = json.Unmarshal(b, &m.Attachments); e != nil {
				return e
			}
			out.Messages.Items = append(out.Messages.Items, m)
		}
		if e = rs.Err(); e != nil {
			return e
		}
		if len(out.Messages.Items) > 100 {
			m := out.Messages.Items[99]
			var token string
			e = tx.QueryRow(ctx, "SELECT created_at::text||':'||id::text FROM support_messages WHERE id=$1", m.ID).Scan(&token)
			if e != nil {
				return e
			}
			out.Messages.Next = base64.RawURLEncoding.EncodeToString([]byte(token))
			out.Messages.Items = out.Messages.Items[:100]
		}
		return nil
	})
	return out, e
}
func (s *Service) Tickets(ctx context.Context, actor string, scope sdk.Scope, f Filter) (Page[tickets.Ticket], error) {
	scope.PluginID = tickets.ID
	out := Page[tickets.Ticket]{Items: []tickets.Ticket{}}
	if !f.valid() {
		return out, database.ErrInvalid
	}
	after, e := cursor(f.Cursor)
	if e != nil {
		return out, e
	}
	e = s.tenant(ctx, actor, scope, "tickets.read", "tickets.read", func(tx pgx.Tx, p string) error {
		rs, e := tx.Query(ctx, `SELECT t.reference FROM support_tickets t JOIN crm_contacts c ON c.id=t.contact_id AND c.workspace_id=t.workspace_id AND c.bot_id=t.bot_id WHERE t.workspace_id=$1 AND t.bot_id=$2 AND ($3='' OR t.reference>$3) AND ($4='' OR t.reference ^@ $4 OR lower(t.subject) ^@ lower($4) OR lower(c.definition->>'name') ^@ lower($4)) AND ($5='' OR t.status=$5) AND ($6='' OR t.priority=$6) AND ($7='' OR t.department_id::text=$7) AND ($8='' OR COALESCE(t.assignee_id::text,'UNASSIGNED')=$8) AND ($9='' OR t.tags ? $9) AND ($10='' OR t.contact_id::text=$10) ORDER BY t.reference LIMIT $11`, scope.AccountID, scope.BotID, after, f.Search, f.Status, f.Priority, f.Department, f.Assignee, f.Tag, f.Contact, f.Limit+1)
		if e != nil {
			return e
		}
		ids := []string{}
		for rs.Next() {
			var id string
			if e = rs.Scan(&id); e != nil {
				rs.Close()
				return e
			}
			ids = append(ids, id)
		}
		rs.Close()
		if e = rs.Err(); e != nil {
			return e
		}
		if len(ids) > f.Limit {
			out.Next = base64.RawURLEncoding.EncodeToString([]byte(ids[f.Limit-1]))
			ids = ids[:f.Limit]
		}
		for _, id := range ids {
			t, e := s.ticket(ctx, tx, scope, id)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, t)
		}
		return nil
	})
	return out, e
}

type Action struct {
	Kind     string   `json:"action"`
	Value    string   `json:"value"`
	Tags     []string `json:"tags"`
	Expected int64    `json:"expected_revision"`
}

func (s *Service) Act(ctx context.Context, actor string, scope sdk.Scope, ref string, a Action) (tickets.Ticket, error) {
	scope.PluginID = tickets.ID
	var out tickets.Ticket
	permission := "tickets.manage"
	if a.Kind == "assign" {
		permission = "tickets.assign"
	}
	if a.Kind == "status" || a.Kind == "reopen" {
		permission = "tickets.close"
	}
	e := s.tenant(ctx, actor, scope, permission, "tickets.write", func(tx pgx.Tx, p string) error {
		t, e := s.ticket(ctx, tx, scope, ref)
		if e != nil {
			return e
		}
		if a.Expected != t.Revision {
			return database.ErrConflict
		}
		previous := map[string]any{"status": t.Status, "priority": t.Priority, "assignee": t.Assignee, "department": t.DepartmentID}
		closed, resolved := t.Closed, t.Resolved
		switch a.Kind {
		case "status", "reopen":
			if !tickets.Transition(t.Status, a.Value, a.Kind == "reopen") {
				return database.ErrInvalid
			}
			if a.Kind == "reopen" {
				n, e := s.count(ctx, tx, scope, "ticketing.active_tickets.count", "support_tickets", "AND status IN('OPEN','PENDING_CUSTOMER','PENDING_AGENT')", nil)
				if e != nil {
					return e
				}
				if e = s.occupancy(ctx, tx, p, scope, "ticketing.active_tickets.count", n+1); e != nil {
					return e
				}
				closed = nil
				resolved = nil
			}
			t.Status = a.Value
			if t.Status == "CLOSED" {
				v := s.now()
				closed = &v
				if resolved == nil {
					resolved = &v
				}
			}
			if t.Status == "RESOLVED" && resolved == nil {
				v := s.now()
				resolved = &v
			}
		case "priority":
			if !tickets.Priority(a.Value) {
				return database.ErrInvalid
			}
			t.Priority = a.Value
		case "assign":
			if a.Value != "" && !database.Permission(ctx, tx, a.Value, scope.AccountID, "tickets.reply") {
				return database.ErrForbidden
			}
			t.Assignee = a.Value
		case "department":
			d, e := s.department(ctx, tx, scope, a.Value)
			if e != nil || !d.Enabled {
				return database.ErrInvalid
			}
			t.DepartmentID = d.ID
		case "tags":
			if !crm.TagsValid(a.Tags) {
				return database.ErrInvalid
			}
			t.Tags = a.Tags
		default:
			return database.ErrInvalid
		}
		_, e = tx.Exec(ctx, `UPDATE support_tickets SET status=$4,priority=$5,assignee_id=NULLIF($6,'')::uuid,department_id=$7,tags=$8,closed_at=$9,resolved_at=$10,revision=revision+1,updated_at=$11 WHERE workspace_id=$1 AND bot_id=$2 AND reference=$3`, scope.AccountID, scope.BotID, ref, t.Status, t.Priority, t.Assignee, t.DepartmentID, body(t.Tags), closed, resolved, s.now())
		if e != nil {
			return e
		}
		if a.Kind == "assign" && a.Value != "" {
			n, e := s.count(ctx, tx, scope, "ticketing.agents.count", "support_tickets", "AND assignee_id IS NOT NULL", nil)
			if e != nil {
				return e
			}
			if e = s.occupancy(ctx, tx, p, scope, "ticketing.agents.count", n); e != nil {
				return e
			}
		}
		if e = audit(ctx, tx, actor, scope, "ticket."+a.Kind, ref, map[string]any{"revision": t.Revision + 1, "value": a.Value, "tag_count": len(t.Tags), "previous": previous}); e != nil {
			return e
		}
		out, e = s.ticket(ctx, tx, scope, ref)
		return e
	})
	return out, e
}
