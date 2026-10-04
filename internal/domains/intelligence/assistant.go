package intelligence

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	provider "github.com/naseri-coder/TELMIVO/internal/platform/ai"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	assistant "github.com/naseri-coder/TELMIVO/internal/plugins/aiassistant"
	"github.com/naseri-coder/TELMIVO/internal/plugins/crm"
	tickets "github.com/naseri-coder/TELMIVO/internal/plugins/ticketing"
	"strconv"
	"strings"
	"time"
)

type Chat struct {
	Key           string `json:"idempotency_key"`
	Text          string `json:"text"`
	Ticket        string `json:"ticket"`
	User          int64  `json:"-"`
	Name          string `json:"-"`
	Purpose       string `json:"-"`
	ConfirmWrites bool   `json:"confirm_writes"`
}
type Result struct {
	ID         string               `json:"id"`
	Thread     string               `json:"thread"`
	State      string               `json:"state"`
	Code       string               `json:"code"`
	Text       string               `json:"text"`
	Structured assistant.Structured `json:"structured"`
	Handoff    string               `json:"handoff"`
	Replayed   bool                 `json:"replayed"`
	Delivery   string               `json:"delivery"`
}
type prepared struct {
	Config         assistant.Config
	Model          provider.Model
	Request        provider.Request
	Contact        string
	Thread         string
	ID             string
	Ticket         string
	Subject        string
	ThreadRevision int64
}

func (s *Service) Chat(ctx context.Context, actor string, scope sdk.Scope, in Chat) (Result, error) {
	scope.PluginID = assistant.ID
	ctx, cancelRequest := context.WithTimeout(ctx, 60*time.Second)
	defer cancelRequest()
	var out Result
	if !key(in.Key) || len(in.Text) == 0 || len(in.Text) > 4000 || in.User < 0 {
		return out, database.ErrInvalid
	}
	p := prepared{Ticket: in.Ticket}
	if in.Purpose == "" {
		in.Purpose = "PLAYGROUND"
	}
	if in.User > 0 {
		p.Subject = "telegram:" + strconv.FormatInt(in.User, 10)
	} else {
		p.Subject = "actor:" + actor + ":" + in.Ticket
	}
	if in.Ticket != "" {
		d, e := s.Support.Detail(ctx, actor, scope, in.Ticket, "")
		if e != nil {
			return out, e
		}
		p.Contact = d.Ticket.ContactID
		safe := []provider.Message{}
		for _, m := range d.Messages.Items {
			if m.Kind == "CUSTOMER" || m.Kind == "AGENT" {
				safe = append(safe, provider.Message{Role: "user", Content: m.Kind + ": " + m.Body})
			}
		}
		if len(safe) > 6 {
			safe = safe[len(safe)-6:]
		}
		p.Request.Context = safe
	}
	if in.User > 0 {
		ownerID, e := s.principal(ctx, scope)
		if e != nil {
			return out, e
		}
		c, e := s.Support.Resolve(ctx, ownerID, scope, crm.ResolveRequest{Identity: crm.Identity{Provider: "telegram", Key: strconv.FormatInt(in.User, 10)}, Name: in.Name})
		if e != nil {
			return out, e
		}
		p.Contact = c.ID
	}
	fresh := false
	started := time.Now()
	e := s.tenant(ctx, actor, scope, "ai.invoke", "ai.invoke", func(tx pgx.Tx) error {
		var fp, state string
		var b []byte
		e := tx.QueryRow(ctx, "SELECT id::text,thread_id::text,state,result,fingerprint,delivery FROM ai_requests WHERE workspace_id=$1 AND bot_id=$2 AND operation_key=$3", scope.AccountID, scope.BotID, in.Key).Scan(&out.ID, &out.Thread, &state, &b, &fp, &out.Delivery)
		if e == nil {
			if fp != hash([]any{actor, p.Subject, in.Text, in.Ticket, in.Purpose, in.ConfirmWrites}) {
				return database.ErrConflict
			}
			if state == "SENDING" {
				out.State = "UNKNOWN"
				out.Code = "PROVIDER_OUTCOME_UNKNOWN"
			} else {
				if json.Unmarshal(b, &out) != nil {
					return database.ErrInvalid
				}
			}
			out.Replayed = true
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if err := s.validate(ctx, tx, actor, scope, "ai.invoke", "ai.invoke", true); err != nil {
			return err
		}
		p.Config, e = s.config(ctx, tx, scope)
		if e != nil {
			return e
		}
		p.Model, e = s.model(ctx, tx, p.Config.Model)
		if e != nil {
			return e
		}
		if in.Purpose == "CLASSIFY" && !p.Model.Structured {
			return database.ErrInvalid
		}
		if in.Purpose == "PLAYGROUND" && !database.Permission(ctx, tx, actor, scope.AccountID, "ai.manage") {
			return database.ErrForbidden
		}
		if in.User > 0 && p.Config.Mode == "AGENT_ASSIST" {
			return sdk.ErrPermission
		}
		if e = tx.QueryRow(ctx, "INSERT INTO ai_threads(workspace_id,bot_id,subject,contact_id) VALUES($1,$2,$3,NULLIF($4,'')::uuid) ON CONFLICT(bot_id,subject) DO UPDATE SET revision=ai_threads.revision+1 RETURNING id::text,revision", scope.AccountID, scope.BotID, p.Subject, p.Contact).Scan(&p.Thread, &p.ThreadRevision); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE ai_requests SET state='UNKNOWN',code='INTERRUPTED_PROVIDER_OUTCOME',result=jsonb_build_object('id',id::text,'thread',thread_id::text,'state','UNKNOWN','code','INTERRUPTED_PROVIDER_OUTCOME'),completed_at=$2 WHERE thread_id=$1 AND state='SENDING' AND created_at<$3`, p.Thread, s.now(), s.now().Add(-2*time.Minute)); e != nil {
			return e
		}
		var pending bool
		if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM ai_requests WHERE thread_id=$1 AND state='SENDING')", p.Thread).Scan(&pending); e != nil {
			return e
		}
		if pending {
			return database.ErrConflict
		}
		if p.Config.Memory {
			var history []byte
			if e = tx.QueryRow(ctx, "SELECT context FROM ai_threads WHERE id=$1", p.Thread).Scan(&history); e != nil {
				return e
			}
			var recent []provider.Message
			if json.Unmarshal(history, &recent) != nil {
				return database.ErrInvalid
			}
			if len(recent) > p.Config.Recent {
				recent = recent[len(recent)-p.Config.Recent:]
			}
			p.Request.Context = append(p.Request.Context, recent...)
		}
		p.Request.Instructions = p.Config.Instructions + "\nLanguage: " + p.Config.Language
		p.Request.Context = append(p.Request.Context, provider.Message{Role: "user", Content: in.Text})
		p.Request.MaxOutput = p.Config.OutputTokens
		p.Request.Structured = in.Purpose == "CLASSIFY"
		if in.Purpose != "CLASSIFY" {
			p.Request.Tools = append([]string{}, p.Config.Tools...)
		}
		p.Request.Knowledge, e = s.retrieve(ctx, tx, scope, p.Config, in.Text)
		if e != nil {
			return e
		}
		if len(body(p.Request)) > p.Model.MaxInput {
			return &provider.Failure{Code: "CONTEXT_TOO_LARGE"}
		}
		for metric, n := range map[string]int64{"ai.requests.monthly": 1, "ai.input_tokens.monthly": 32768 * assistant.MaxCalls, "ai.output_tokens.monthly": p.Config.OutputTokens * assistant.MaxCalls} {
			if e = s.quota(ctx, tx, scope, metric, n, true); e != nil {
				return e
			}
		}
		e = tx.QueryRow(ctx, "INSERT INTO ai_requests(workspace_id,bot_id,thread_id,actor_id,operation_key,fingerprint,state,model_id,created_at) VALUES($1,$2,$3,$4,$5,$6,'SENDING',$7,$8) RETURNING id::text", scope.AccountID, scope.BotID, p.Thread, actor, in.Key, hash([]any{actor, p.Subject, in.Text, in.Ticket, in.Purpose, in.ConfirmWrites}), p.Model.ID, s.now()).Scan(&p.ID)
		if e != nil {
			return e
		}
		fresh = true
		return audit(ctx, tx, actor, scope, "ai_request_started", p.ID, map[string]any{"model": p.Model.ID})
	})
	if e != nil || !fresh {
		return out, e
	}
	out = Result{ID: p.ID, Thread: p.Thread, State: "SUCCEEDED", Delivery: "NONE"}
	var input, output int64
	toolCount := 0
	handoff := in.Text == "/human" && in.User > 0
	var answer provider.Response
	if !handoff {
		for round := 0; round < assistant.MaxCalls; round++ {
			if e = s.recheck(ctx, actor, scope, p.Config, p.Model); e != nil {
				break
			}
			if s.Provider == nil {
				e = &provider.Failure{Code: "MODEL_UNAVAILABLE"}
				break
			}
			callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			answer, e = s.Provider.Generate(callCtx, p.Model, p.Request)
			cancel()
			if e != nil {
				break
			}
			if answer.InputTokens < 0 || answer.InputTokens > 32768 || answer.OutputTokens < 0 || answer.OutputTokens > p.Config.OutputTokens || len(answer.Text) > assistant.MaxOutput || len(answer.Tools) > 4 {
				e = &provider.Failure{Code: "MALFORMED_PROVIDER_RESPONSE"}
				break
			}
			input += answer.InputTokens
			output += answer.OutputTokens
			if len(answer.Tools) == 0 {
				break
			}
			if round >= assistant.MaxRounds || toolCount+len(answer.Tools) > assistant.MaxTools {
				e = &provider.Failure{Code: "TOOL_ROUND_LIMIT"}
				break
			}
			for _, tool := range answer.Tools {
				if _, err := validateTool(p.Config, in, tool); err != nil {
					e = err
					break
				}
			}
			if e != nil {
				break
			}
			for _, tool := range answer.Tools {
				toolCount++
				value, err := s.tool(ctx, actor, scope, p, in, tool)
				if err != nil {
					e = err
					break
				}
				p.Request.Context = append(p.Request.Context, provider.Message{Role: "user", Content: "UNTRUSTED_TOOL_RESULT: " + string(body(value))})
			}
			if e != nil {
				break
			}
			if len(body(p.Request)) > p.Model.MaxInput {
				e = &provider.Failure{Code: "CONTEXT_TOO_LARGE"}
				break
			}
		}
		handoff = answer.Handoff && in.User > 0
	}
	if e == nil && in.Purpose == "CLASSIFY" {
		if strict(answer.Structured, &out.Structured) != nil || !out.Structured.Valid() {
			e = &provider.Failure{Code: "INVALID_STRUCTURED_OUTPUT"}
		}
	} else if e == nil && !handoff {
		out.Text = answer.Text
		if out.Text == "" {
			e = &provider.Failure{Code: "MALFORMED_PROVIDER_RESPONSE"}
		}
	}
	if e == nil && handoff {
		if p.Config.Mode != "AI_WITH_HANDOFF" {
			e = &provider.Failure{Code: "HANDOFF_UNAVAILABLE"}
		} else {
			ownerID, err := s.principal(ctx, scope)
			if err != nil {
				e = err
			} else {
				ticket, err := s.Support.Inbound(ctx, scope, tickets.Inbound{User: in.User, Name: in.Name, Body: in.Text, Key: "ai-handoff:" + p.ID})
				_ = ownerID
				e = err
				if e == nil {
					out.Handoff = ticket.Reference
					out.Text = "درخواست شما به کارشناس ارجاع شد."
				}
			}
		}
	}
	if e != nil {
		out.State = "FAILED"
		out.Code = failure(e)
		var f *provider.Failure
		if errors.As(e, &f) && f.Unknown {
			out.State = "UNKNOWN"
		}
		out.Text = ""
	}
	finalCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	finish := s.DB.Tenant(finalCtx, actor, scope.AccountID, false, func(tx pgx.Tx) error {
		if err := s.lock(finalCtx, tx, scope); err != nil {
			return err
		}
		tag, err := tx.Exec(finalCtx, "UPDATE ai_requests SET state=$4,code=$5,result=$6,input_tokens=$7,output_tokens=$8,tool_calls=$9,completed_at=$10,latency_ms=$11 WHERE workspace_id=$1 AND bot_id=$2 AND id=$3 AND state='SENDING'", scope.AccountID, scope.BotID, p.ID, out.State, out.Code, body(out), input, output, toolCount, s.now(), time.Since(started).Milliseconds())
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return database.ErrConflict
		}
		recent := []provider.Message{}
		if p.Config.Memory && out.State == "SUCCEEDED" {
			for _, m := range p.Request.Context {
				if m.Role == "user" && !stringsHasToolPrefix(m.Content) {
					recent = append(recent, m)
				}
			}
			recent = append(recent, provider.Message{Role: "assistant", Content: out.Text})
			if len(recent) > p.Config.Recent {
				recent = recent[len(recent)-p.Config.Recent:]
			}
		}
		_, err = tx.Exec(finalCtx, "UPDATE ai_threads SET context=$4,revision=revision+1 WHERE workspace_id=$1 AND bot_id=$2 AND id=$3", scope.AccountID, scope.BotID, p.Thread, body(recent))
		if err != nil {
			return err
		}
		return audit(finalCtx, tx, actor, scope, "ai_request_completed", p.ID, map[string]any{"state": out.State, "code": out.Code, "input_tokens": input, "output_tokens": output, "tool_calls": toolCount})
	})
	if finish != nil {
		return Result{ID: p.ID, Thread: p.Thread, State: "UNKNOWN", Code: "FINALIZE_FAILED"}, finish
	}
	return out, e
}
func (s *Service) recheck(ctx context.Context, actor string, scope sdk.Scope, c assistant.Config, m provider.Model) error {
	return s.tenant(ctx, actor, scope, "ai.invoke", "ai.invoke", func(tx pgx.Tx) error {
		if e := s.validate(ctx, tx, actor, scope, "ai.invoke", "ai.invoke", true); e != nil {
			return e
		}
		current, e := s.config(ctx, tx, scope)
		if e != nil {
			return e
		}
		model, e := s.model(ctx, tx, m.ID)
		if e != nil {
			return e
		}
		if current.Revision != c.Revision || model.Revision != m.Revision {
			return database.ErrConflict
		}
		return nil
	})
}
func (s *Service) principal(ctx context.Context, scope sdk.Scope) (string, error) {
	var out string
	tx, e := s.DB.Pool.Begin(ctx)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	if e = database.Scope(ctx, tx, scope.AccountID); e != nil {
		return out, e
	}
	return owner(ctx, tx, scope)
}
func stringsHasToolPrefix(s string) bool { return strings.HasPrefix(s, "UNTRUSTED_TOOL_RESULT: ") }
func timeDuration(n int) time.Duration   { return time.Duration(n) * time.Second }
