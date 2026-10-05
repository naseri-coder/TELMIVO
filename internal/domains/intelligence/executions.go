package intelligence

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/supportplugins"
	provider "github.com/naseri-coder/TELMIVO/internal/platform/ai"
	"github.com/naseri-coder/TELMIVO/internal/platform/businessevents"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	assistant "github.com/naseri-coder/TELMIVO/internal/plugins/aiassistant"
	workflow "github.com/naseri-coder/TELMIVO/internal/plugins/automation"
	"strconv"
	"strings"
	"time"
)

type Step struct {
	Type  string `json:"type"`
	State string `json:"state"`
	Code  string `json:"code"`
}
type Execution struct {
	ID        string               `json:"id"`
	Workflow  string               `json:"workflow_id"`
	EventID   string               `json:"event_id"`
	State     string               `json:"state"`
	Code      string               `json:"code"`
	Index     int                  `json:"action_index"`
	Attempt   int                  `json:"attempt"`
	Started   *time.Time           `json:"started_at"`
	Completed *time.Time           `json:"completed_at"`
	Steps     []Step               `json:"steps"`
	Result    assistant.Structured `json:"result"`
}

func (s *Service) History(ctx context.Context, actor string, scope sdk.Scope, after string) ([]Execution, error) {
	scope.PluginID = workflow.ID
	out := []Execution{}
	e := s.tenant(ctx, actor, scope, "automation.manage", "workflow.execute", func(tx pgx.Tx) error {
		rs, e := tx.Query(ctx, "SELECT id::text,workflow_id::text,event_id,state,code,action_index,attempt,started_at,completed_at,results,result FROM automation_executions WHERE workspace_id=$1 AND bot_id=$2 AND ($3='' OR id::text>$3) ORDER BY id LIMIT 50", scope.AccountID, scope.BotID, after)
		if e != nil {
			return e
		}
		defer rs.Close()
		for rs.Next() {
			var x Execution
			var steps, res []byte
			if e = rs.Scan(&x.ID, &x.Workflow, &x.EventID, &x.State, &x.Code, &x.Index, &x.Attempt, &x.Started, &x.Completed, &steps, &res); e != nil {
				return e
			}
			if json.Unmarshal(steps, &x.Steps) != nil || json.Unmarshal(res, &x.Result) != nil {
				return database.ErrInvalid
			}
			out = append(out, x)
		}
		return rs.Err()
	})
	return out, e
}

// Run reserves every action durably. An interrupted reservation is UNKNOWN, never replayed.
func (s *Service) Run(ctx context.Context, actor string, scope sdk.Scope, id string) error {
	scope.PluginID = workflow.ID
	var w workflow.Workflow
	var event businessevents.Event
	var x Execution
	var runner string
	execute := false
	e := s.tenant(ctx, actor, scope, "automation.manage", "workflow.execute", func(tx pgx.Tx) error {
		var wb, eb, rb, sb []byte
		var available time.Time
		var lease *time.Time
		e := tx.QueryRow(ctx, "SELECT state,actor_id::text,snapshot,event,action_index,attempt,result,results,available_at,lease_until FROM automation_executions WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3 FOR UPDATE", scope.AccountID, scope.BotID, id).Scan(&x.State, &runner, &wb, &eb, &x.Index, &x.Attempt, &rb, &sb, &available, &lease)
		if e != nil {
			return e
		}
		if json.Unmarshal(wb, &w) != nil || json.Unmarshal(eb, &event) != nil || json.Unmarshal(sb, &x.Steps) != nil || json.Unmarshal(rb, &x.Result) != nil {
			return database.ErrInvalid
		}
		if x.State == "RUNNING" {
			if lease == nil || !s.now().Before(*lease) {
				_, e = tx.Exec(ctx, "UPDATE automation_executions SET state='FAILED',code='INTERRUPTED_OUTCOME_UNKNOWN',completed_at=$4 WHERE workspace_id=$1 AND bot_id=$2 AND id=$3", scope.AccountID, scope.BotID, id, s.now())
				return e
			}
			return database.ErrConflict
		}
		if x.State != "QUEUED" || s.now().Before(available) {
			return nil
		}
		current, _, e := s.workflow(ctx, tx, scope, w.ID)
		if e != nil {
			return e
		}
		if !current.Enabled {
			_, e = tx.Exec(ctx, "UPDATE automation_executions SET state='CANCELLED',code='WORKFLOW_DISABLED',completed_at=$4 WHERE workspace_id=$1 AND bot_id=$2 AND id=$3", scope.AccountID, scope.BotID, id, s.now())
			return e
		}
		if e = s.validate(ctx, tx, runner, scope, "automation.manage", "workflow.execute", true); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, "UPDATE automation_executions SET state='RUNNING',started_at=COALESCE(started_at,$4),lease_until=$5 WHERE workspace_id=$1 AND bot_id=$2 AND id=$3", scope.AccountID, scope.BotID, id, s.now(), s.now().Add(workflow.MaxDuration))
		execute = e == nil
		return e
	})
	if e != nil || !execute {
		return e
	}
	ctx, cancel := context.WithTimeout(ctx, workflow.MaxDuration)
	defer cancel()
	event.Depth++
	event.Workflows = append(append([]string{}, event.Workflows...), w.ID)
	event.Causation = id
	ctx = businessevents.With(ctx, event)
	for i := x.Index; i < len(w.Actions); i++ {
		a := w.Actions[i]
		x.Index = i
		step := Step{Type: a.Type, State: "SENDING"}
		x.Steps = append(x.Steps, step)
		if e = s.persistStep(ctx, runner, scope, id, x, "RUNNING", "", false); e != nil {
			return e
		}
		value := a.Value
		if value == "$ai.priority" {
			value = x.Result.Priority
		} else if value == "$ai.category" {
			value = x.Result.Category
		}
		e = s.action(ctx, runner, scope, id, i, x.Attempt, a, value, event, &x.Result)
		code := failure(e)
		state := "SUCCEEDED"
		retry := false
		if e != nil {
			state = "FAILED"
			var p *provider.Failure
			if errors.As(e, &p) {
				if p.Unknown {
					state = "UNKNOWN"
				}
				retry = p.Retryable && !p.Unknown && x.Attempt < w.MaxAttempts
			}
		}
		x.Steps[len(x.Steps)-1] = Step{Type: a.Type, State: state, Code: code}
		if retry {
			x.Attempt++
			x.Steps = x.Steps[:len(x.Steps)-1]
			return s.persistStep(context.WithoutCancel(ctx), runner, scope, id, x, "QUEUED", code, false)
		}
		if e != nil {
			finish := s.persistStep(context.WithoutCancel(ctx), runner, scope, id, x, "FAILED", code, true)
			if finish != nil {
				return finish
			}
			return e
		}
		x.Index = i + 1
		if e = s.persistStep(ctx, runner, scope, id, x, "RUNNING", "", false); e != nil {
			return e
		}
	}
	return s.persistStep(context.WithoutCancel(ctx), runner, scope, id, x, "SUCCEEDED", "", true)
}
func (s *Service) persistStep(ctx context.Context, actor string, scope sdk.Scope, id string, x Execution, state, code string, done bool) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.DB.Tenant(ctx, actor, scope.AccountID, false, func(tx pgx.Tx) error {
		var completed any
		if done {
			completed = s.now()
		}
		tag, e := tx.Exec(ctx, "UPDATE automation_executions SET state=$4,code=$5,action_index=$6,attempt=$7,results=$8,result=$9,completed_at=$10,available_at=$11 WHERE workspace_id=$1 AND bot_id=$2 AND id=$3 AND state='RUNNING'", scope.AccountID, scope.BotID, id, state, code, x.Index, x.Attempt, body(x.Steps), body(x.Result), completed, s.now().Add(time.Second))
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return database.ErrConflict
		}
		return audit(ctx, tx, actor, scope, "automation_execution_"+strings.ToLower(state), id, map[string]any{"code": code, "index": x.Index, "attempt": x.Attempt})
	})
}
func (s *Service) action(ctx context.Context, actor string, scope sdk.Scope, id string, index, attempt int, a workflow.Action, value string, event businessevents.Event, result *assistant.Structured) error {
	if e := s.tenant(ctx, actor, scope, "automation.manage", "workflow.execute", func(tx pgx.Tx) error {
		return s.available(ctx, tx, actor, scope, workflow.Workflow{Actions: []workflow.Action{a}})
	}); e != nil {
		return e
	}
	switch {
	case strings.HasPrefix(a.Type, "crm."):
		c, e := s.Support.Contact(ctx, actor, scope, event.Contact)
		if e != nil {
			return e
		}
		switch a.Type {
		case "crm.add_tag":
			if !assistant.Contains(c.Tags, value) {
				c.Tags = append(c.Tags, value)
			}
		case "crm.remove_tag":
			tags := []string{}
			for _, v := range c.Tags {
				if v != value {
					tags = append(tags, v)
				}
			}
			c.Tags = tags
		case "crm.set_field":
			if c.Fields == nil {
				c.Fields = map[string]string{}
			}
			c.Fields[a.Field] = value
		}
		_, e = s.Support.SaveContact(ctx, actor, scope, c.ID, c, c.Revision)
		return e
	case strings.HasPrefix(a.Type, "ticket."):
		d, e := s.Support.Detail(ctx, actor, scope, event.Ticket, "")
		if e != nil {
			return e
		}
		k := actionKey(id, index, attempt)
		if a.Type == "ticket.note" || a.Type == "ticket.reply" {
			r, e := s.Support.Reply(ctx, actor, scope, event.Ticket, supportplugins.Reply{Body: value, Internal: a.Type == "ticket.note", Key: k, Expected: d.Ticket.Revision})
			if e != nil {
				return e
			}
			if a.Type == "ticket.reply" && r.Delivery != "SENT" {
				return &provider.Failure{Code: "TELEGRAM_OUTCOME_UNKNOWN", Unknown: true}
			}
			return nil
		}
		_, e = s.Support.Act(ctx, actor, scope, event.Ticket, supportplugins.Action{Kind: strings.TrimPrefix(a.Type, "ticket."), Value: value, Expected: d.Ticket.Revision})
		return e
	case a.Type == "ai.invoke":
		d, e := s.Support.Detail(ctx, actor, scope, event.Ticket, "")
		if e != nil {
			return e
		}
		r, e := s.Chat(ctx, actor, scope, Chat{Key: actionKey(id, index, attempt), Text: d.Ticket.Subject, Ticket: event.Ticket, Purpose: "CLASSIFY"})
		if e != nil {
			return e
		}
		if r.State != "SUCCEEDED" || !r.Structured.Valid() {
			return &provider.Failure{Code: "INVALID_STRUCTURED_OUTPUT"}
		}
		*result = r.Structured
		return nil
	case a.Type == "ai.store_result":
		if !result.Valid() {
			return database.ErrInvalid
		}
		c, e := s.Support.Contact(ctx, actor, scope, event.Contact)
		if e != nil {
			return e
		}
		if c.Fields == nil {
			c.Fields = map[string]string{}
		}
		c.Fields[a.Field] = string(body(result))
		_, e = s.Support.SaveContact(ctx, actor, scope, c.ID, c, c.Revision)
		return e
	case a.Type == "telegram.send":
		if event.User <= 0 {
			return database.ErrInvalid
		}
		return s.Send(ctx, actor, scope, event.User, value, actionKey(id, index, attempt))
	}
	return database.ErrInvalid
}
func actionKey(id string, index, attempt int) string {
	return "automation:" + id + ":" + strconv.Itoa(index) + ":" + strconv.Itoa(attempt)
}
