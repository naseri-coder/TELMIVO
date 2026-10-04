package intelligence

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/pluginplatform"
	"github.com/naseri-coder/TELMIVO/internal/domains/supportplugins"
	provider "github.com/naseri-coder/TELMIVO/internal/platform/ai"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	gate "github.com/naseri-coder/TELMIVO/internal/plugins/accessgate"
	assistant "github.com/naseri-coder/TELMIVO/internal/plugins/aiassistant"
	"strings"
)

type toolArgs struct {
	Value     string `json:"value"`
	Field     string `json:"field"`
	Reference string `json:"reference"`
	Policy    string `json:"policy"`
}

func validateTool(config assistant.Config, in Chat, t provider.Tool) (toolArgs, error) {
	if !assistant.Contains(config.Tools, t.Name) || len(t.Arguments) > 4096 {
		return toolArgs{}, &provider.Failure{Code: "TOOL_DENIED"}
	}
	var a toolArgs
	if strict(t.Arguments, &a) != nil || len(a.Value) > 1000 || len(a.Field) > 64 || len(a.Reference) > 100 || len(a.Policy) > 100 {
		return toolArgs{}, &provider.Failure{Code: "INVALID_TOOL_ARGUMENTS"}
	}
	write := assistant.Contains(assistant.WriteTools, t.Name)
	if write && (!config.WriteEnabled || !in.ConfirmWrites || in.User > 0 || in.Purpose == "CLASSIFY") {
		return toolArgs{}, &provider.Failure{Code: "WRITE_TOOL_DENIED"}
	}
	return a, nil
}

func (s *Service) tool(ctx context.Context, actor string, scope sdk.Scope, p prepared, in Chat, t provider.Tool) (any, error) {
	a, err := validateTool(p.Config, in, t)
	if err != nil {
		return nil, err
	}
	if e := s.recheck(ctx, actor, scope, p.Config, p.Model); e != nil {
		return nil, e
	}
	if e := s.tenant(ctx, actor, scope, "ai.invoke", "ai.invoke", func(tx pgx.Tx) error { return s.quota(ctx, tx, scope, "ai.tool_calls.monthly", 1, true) }); e != nil {
		return nil, e
	}
	switch t.Name {
	case "contact.summary", "contact.add_tag":
		if p.Contact == "" {
			return nil, &provider.Failure{Code: "TOOL_CONTEXT_MISSING"}
		}
		c, e := s.Support.Contact(ctx, actor, scope, p.Contact)
		if e != nil {
			return nil, e
		}
		if t.Name == "contact.add_tag" {
			if !assistant.Contains(c.Tags, a.Value) {
				c.Tags = append(c.Tags, a.Value)
			}
			_, e = s.Support.SaveContact(ctx, actor, scope, c.ID, c, c.Revision)
			if e != nil {
				return nil, e
			}
		}
		return map[string]any{"name": c.Name, "tags": c.Tags, "fields": c.Fields}, nil
	case "ticket.context", "ticket.priority", "ticket.note", "ticket.assign", "ticket.status":
		if p.Ticket == "" {
			return nil, &provider.Failure{Code: "TOOL_CONTEXT_MISSING"}
		}
		d, e := s.Support.Detail(ctx, actor, scope, p.Ticket, "")
		if e != nil {
			return nil, e
		}
		if in.User > 0 && d.Ticket.ContactID != p.Contact {
			return nil, database.ErrForbidden
		}
		if t.Name == "ticket.context" {
			messages := []string{}
			for _, m := range d.Messages.Items {
				if m.Kind == "CUSTOMER" || m.Kind == "AGENT" {
					messages = append(messages, m.Body)
				}
			}
			if len(messages) > 6 {
				messages = messages[len(messages)-6:]
			}
			return map[string]any{"subject": d.Ticket.Subject, "status": d.Ticket.Status, "priority": d.Ticket.Priority, "messages": messages}, nil
		}
		if t.Name == "ticket.note" {
			_, e = s.Support.Reply(ctx, actor, scope, p.Ticket, supportplugins.Reply{Body: a.Value, Internal: true, Key: "ai-tool:" + p.ID + ":" + hash(t)[:16], Expected: d.Ticket.Revision})
			return map[string]bool{"applied": e == nil}, e
		}
		x, e := s.Support.Act(ctx, actor, scope, p.Ticket, supportplugins.Action{Kind: strings.TrimPrefix(t.Name, "ticket."), Value: a.Value, Expected: d.Ticket.Revision})
		return map[string]any{"status": x.Status, "priority": x.Priority}, e
	case "file.metadata":
		if !assistant.Contains(p.Config.Files, a.Reference) {
			return nil, &provider.Failure{Code: "TOOL_DENIED"}
		}
		sc := scope
		sc.PluginID = "file-manager"
		files, e := s.Content.Files(ctx, actor, sc, "")
		if e != nil {
			return nil, e
		}
		for _, f := range files {
			if f.Reference == a.Reference && f.Enabled {
				if f.Policy != "" {
					allowed, e := s.gate(ctx, actor, scope, f.Policy, in.User)
					if e != nil || !allowed {
						return nil, &provider.Failure{Code: "ACCESS_DENIED"}
					}
				}
				return map[string]any{"title": f.Title, "description": f.Description, "category": f.Category}, nil
			}
		}
		return nil, database.ErrInvalid
	case "access.evaluate":
		allowed, e := s.gate(ctx, actor, scope, a.Policy, in.User)
		return map[string]bool{"allowed": allowed}, e
	}
	return nil, &provider.Failure{Code: "TOOL_DENIED"}
}
func (s *Service) gate(ctx context.Context, actor string, scope sdk.Scope, policy string, user int64) (bool, error) {
	scope.PluginID = gate.ID
	if user <= 0 || s.Content == nil {
		return false, database.ErrInvalid
	}
	r := &pluginplatform.Runtime{Service: s.Content.Core, Handler: s.Content}
	d, e := (gate.Plugin{Runtime: r.Bind(actor, scope)}).Authorize(ctx, policy, user)
	return d.Allowed, e
}

// Retrieval is deliberately replaceable; PostgreSQL full-text lookup avoids new infrastructure.
func (s *Service) retrieve(ctx context.Context, tx pgx.Tx, scope sdk.Scope, c assistant.Config, query string) ([]string, error) {
	out := []string{}
	if len(c.Knowledge) == 0 {
		return out, nil
	}
	if len(query) > 500 {
		query = query[:500]
	}
	rs, e := tx.Query(ctx, "SELECT name,body FROM ai_knowledge WHERE workspace_id=$1 AND bot_id=$2 AND id::text=ANY($3::text[]) AND enabled AND to_tsvector('simple',body) @@ plainto_tsquery('simple',$4) ORDER BY id LIMIT 4", scope.AccountID, scope.BotID, c.Knowledge, query)
	if e != nil {
		return out, e
	}
	defer rs.Close()
	for rs.Next() {
		var name, b string
		if e = rs.Scan(&name, &b); e != nil {
			return out, e
		}
		out = append(out, name+": "+b)
	}
	return out, rs.Err()
}
func (s *Service) ResetThread(ctx context.Context, actor string, scope sdk.Scope, id string, expected int64) error {
	scope.PluginID = assistant.ID
	return s.tenant(ctx, actor, scope, "ai.manage", "ai.invoke", func(tx pgx.Tx) error {
		var pending bool
		if e := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM ai_requests WHERE thread_id::text=$1 AND state='SENDING')", id).Scan(&pending); e != nil {
			return e
		}
		if pending {
			return database.ErrConflict
		}
		tag, e := tx.Exec(ctx, "UPDATE ai_threads SET context='[]',revision=revision+1 WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3 AND revision=$4", scope.AccountID, scope.BotID, id, expected)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return database.ErrConflict
		}
		return audit(ctx, tx, actor, scope, "ai.thread.reset", id, map[string]any{"revision": expected + 1})
	})
}
