package contentplugins

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	platform "github.com/naseri-coder/TELMIVO/internal/domains/pluginplatform"
	"github.com/naseri-coder/TELMIVO/internal/platform/businessevents"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	gate "github.com/naseri-coder/TELMIVO/internal/plugins/accessgate"
	files "github.com/naseri-coder/TELMIVO/internal/plugins/filemanager"
	"log/slog"
)

type Result struct {
	Code      string        `json:"code"`
	Allowed   bool          `json:"allowed"`
	MessageID int64         `json:"message_id,omitempty"`
	Access    gate.Decision `json:"access"`
	Replayed  bool          `json:"replayed,omitempty"`
}

func (s *Service) Invoke(context.Context, sdk.Scope, string, json.RawMessage) (json.RawMessage, error) {
	return nil, sdk.ErrPermission
}

type txGate struct {
	s     *Service
	tx    pgx.Tx
	actor string
	scope sdk.Scope
}

func (g txGate) Authorize(ctx context.Context, id string, user int64) (gate.Decision, error) {
	scope := g.scope
	scope.PluginID = gate.ID
	if e := g.s.Core.ValidateExecutionTx(ctx, g.tx, g.actor, scope, "telegram.read_member_status", true); e != nil {
		return gate.Decision{Code: "ACCESS_GATE_UNAVAILABLE", Checks: []gate.Check{}}, nil
	}
	return g.s.evaluate(ctx, g.tx, g.actor, scope, id, user)
}
func (s *Service) evaluate(ctx context.Context, tx pgx.Tx, actor string, scope sdk.Scope, id string, user int64) (gate.Decision, error) {
	p, e := s.policy(ctx, tx, scope, id)
	if errors.Is(e, pgx.ErrNoRows) {
		return gate.Decision{Code: "POLICY_MISSING", Checks: []gate.Check{}}, nil
	}
	if e != nil {
		return gate.Decision{}, e
	}
	if !p.Enabled {
		return gate.Decision{Code: "POLICY_DISABLED", Checks: []gate.Check{}}, nil
	}
	if e = s.quota(ctx, tx, actor, scope, "membership_checks.monthly", int64(len(p.Requirements))); e != nil {
		var denied *QuotaDenied
		if errors.As(e, &denied) {
			return gate.Decision{Code: denied.Result.Code, Checks: []gate.Check{}}, nil
		}
		return gate.Decision{}, e
	}
	var m gate.Membership
	if s.Provider != nil {
		tr, e := s.transport(ctx, tx, scope)
		if e == nil {
			m = tr
		}
	}
	d := gate.Evaluate(ctx, p, user, m)
	slog.InfoContext(ctx, "access_gate_evaluated", "code", d.Code, "allowed", d.Allowed)
	return d, nil
}
func (s *Service) InvokeTx(ctx context.Context, tx pgx.Tx, actor string, scope sdk.Scope, cap string, payload json.RawMessage) (json.RawMessage, error) {
	if scope.PluginID == gate.ID && cap == "telegram.read_member_status" {
		var r gate.Request
		if json.Unmarshal(payload, &r) != nil || r.User <= 0 {
			return nil, database.ErrInvalid
		}
		d, e := s.evaluate(ctx, tx, actor, scope, r.Policy, r.User)
		return jsonBody(d), e
	}
	if scope.PluginID != files.ID || cap != "telegram.send" {
		return nil, sdk.ErrPermission
	}
	var req files.Request
	if json.Unmarshal(payload, &req) != nil || req.User <= 0 || !operation(req.Key) || !files.ReferencePattern.MatchString(req.Reference) {
		return nil, database.ErrInvalid
	}
	var state, fingerprint string
	e := tx.QueryRow(ctx, `SELECT state,fingerprint FROM content_delivery_events WHERE workspace_id=$1 AND bot_id=$2 AND request_key=$3 FOR UPDATE`, scope.AccountID, scope.BotID, req.Key).Scan(&state, &fingerprint)
	if e != nil {
		return nil, e
	}
	if state != "SENDING" || fingerprint != s.requestHash(req) {
		return nil, database.ErrConflict
	}
	f, e := s.file(ctx, tx, scope, req.Reference)
	if e != nil {
		return nil, e
	}
	d, e := files.Authorize(ctx, f, req.User, txGate{s, tx, actor, scope})
	if e != nil {
		return nil, e
	}
	result := Result{Code: d.Code, Access: d}
	next := "DENIED"
	if d.Allowed {
		tr, e := s.transport(ctx, tx, scope)
		if e != nil {
			result.Code = "CHECK_UNAVAILABLE"
			next = "FAILED"
		} else {
			result.MessageID, result.Code = tr.SendFile(ctx, req.User, f.Kind, f.FileID, f.Title)
			next = "FAILED"
			if result.Code == "DELIVERY_ACKNOWLEDGED" && result.MessageID > 0 {
				next = "SENT"
				result.Allowed = true
			} else if result.Code == "DELIVERY_OUTCOME_UNKNOWN" {
				next = "UNKNOWN"
			}
		}
	}
	_, e = tx.Exec(ctx, `UPDATE content_delivery_events SET state=$4,result=$5,completed_at=now() WHERE workspace_id=$1 AND bot_id=$2 AND request_key=$3 AND state='SENDING'`, scope.AccountID, scope.BotID, req.Key, next, jsonBody(result))
	if e == nil && (next == "SENT" || next == "DENIED") {
		kind := "file.delivery_denied"
		if next == "SENT" {
			kind = "file.delivery_allowed"
		}
		e = businessevents.Emit(businessevents.WithUser(ctx, req.User), tx, actor, scope, kind, f.Reference, map[string][]string{"file.reference": {f.Reference}})
	}
	slog.InfoContext(ctx, "file_delivery_result", "code", result.Code, "state", next)
	return jsonBody(result), e
}
func (s *Service) Deliver(ctx context.Context, actor string, scope sdk.Scope, req files.Request) (Result, error) {
	scope.PluginID = files.ID
	result := Result{}
	if req.User <= 0 || !operation(req.Key) || !files.ReferencePattern.MatchString(req.Reference) || len(s.DigestKey) < 32 {
		return result, database.ErrInvalid
	}
	fresh := false
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		if e := s.Core.ValidateExecutionTx(ctx, tx, actor, scope, "telegram.send", true); e != nil {
			return e
		}
		var state, fingerprint string
		var body []byte
		e := tx.QueryRow(ctx, `SELECT state,fingerprint,result FROM content_delivery_events WHERE workspace_id=$1 AND bot_id=$2 AND request_key=$3`, scope.AccountID, scope.BotID, req.Key).Scan(&state, &fingerprint, &body)
		if e == nil {
			if fingerprint != s.requestHash(req) {
				return database.ErrConflict
			}
			if state == "SENDING" {
				result.Code = "DELIVERY_OUTCOME_UNKNOWN"
			} else if e = json.Unmarshal(body, &result); e != nil {
				return e
			}
			result.Replayed = true
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		f, e := s.file(ctx, tx, scope, req.Reference)
		if errors.Is(e, pgx.ErrNoRows) {
			result.Code = "FILE_NOT_FOUND"
			return nil
		}
		if e != nil {
			return e
		}
		if !f.Enabled {
			result.Code = "FILE_DISABLED"
			return nil
		}
		if e = s.quota(ctx, tx, actor, scope, "downloads.monthly", 1); e != nil {
			var q *QuotaDenied
			if errors.As(e, &q) {
				result.Code = q.Error()
				return nil
			}
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO content_delivery_events(workspace_id,bot_id,request_key,file_reference,requester_digest,fingerprint,state) VALUES($1,$2,$3,$4,$5,$6,'SENDING')`, scope.AccountID, scope.BotID, req.Key, req.Reference, s.requester(scope.BotID, req.User), s.requestHash(req))
		fresh = e == nil
		return e
	})
	if e != nil || !fresh {
		return result, e
	}
	// Durable SENDING precedes any external effect: unknown outcomes never auto-resend.
	r := &platform.Runtime{Service: s.Core, Handler: s}
	body, e := (files.Plugin{Runtime: r.Bind(actor, scope)}).Deliver(ctx, req)
	if e == nil {
		e = json.Unmarshal(body, &result)
	}
	if e != nil {
		result.Code = "EXECUTION_BLOCKED"
		state := "UNKNOWN"
		if errors.Is(e, sdk.ErrState) || errors.Is(e, sdk.ErrPermission) || errors.Is(e, sdk.ErrDependency) || errors.Is(e, sdk.ErrVersion) {
			state = "DENIED"
		}
		s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE content_delivery_events SET state=$5,result=$4,completed_at=now() WHERE workspace_id=$1 AND bot_id=$2 AND request_key=$3 AND state='SENDING'`, scope.AccountID, scope.BotID, req.Key, jsonBody(result), state)
			return err
		})
		return result, e
	}
	return result, nil
}
