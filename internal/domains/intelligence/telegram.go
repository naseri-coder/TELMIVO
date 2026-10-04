package intelligence

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	"github.com/naseri-coder/TELMIVO/internal/domains/supportplugins"
	provider "github.com/naseri-coder/TELMIVO/internal/platform/ai"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	assistant "github.com/naseri-coder/TELMIVO/internal/plugins/aiassistant"
	workflow "github.com/naseri-coder/TELMIVO/internal/plugins/automation"
	"strconv"
	"strings"
	"time"
)

func (s *Service) Send(ctx context.Context, actor string, scope sdk.Scope, user int64, text, k string) error {
	permission := "ai.invoke"
	if scope.PluginID == workflow.ID {
		permission = "automation.manage"
	} else {
		scope.PluginID = assistant.ID
	}
	if user <= 0 || text == "" || len(text) > 4000 || !key(k) || s.Content == nil || s.Content.Provider == nil {
		return database.ErrInvalid
	}
	fresh := false
	var tr contentplugins.Transport
	e := s.tenant(ctx, actor, scope, permission, "telegram.send", func(tx pgx.Tx) error {
		if e := s.validate(ctx, tx, actor, scope, permission, "telegram.send", true); e != nil {
			return e
		}
		var fp, state string
		e := tx.QueryRow(ctx, "SELECT fingerprint,state FROM intelligence_deliveries WHERE workspace_id=$1 AND bot_id=$2 AND operation_key=$3", scope.AccountID, scope.BotID, k).Scan(&fp, &state)
		if e == nil {
			if fp != hash([]any{actor, user, text}) {
				return database.ErrConflict
			}
			if state != "SENT" {
				return &provider.Failure{Code: "TELEGRAM_OUTCOME_UNKNOWN", Unknown: true}
			}
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		tr, e = s.Content.Provider.ForBot(ctx, tx, scope)
		if e != nil {
			return e
		}
		if _, ok := tr.(supportplugins.Sender); !ok {
			return sdk.ErrPermission
		}
		_, e = tx.Exec(ctx, "INSERT INTO intelligence_deliveries(workspace_id,bot_id,operation_key,fingerprint,state,created_at) VALUES($1,$2,$3,$4,'SENDING',$5)", scope.AccountID, scope.BotID, k, hash([]any{actor, user, text}), s.now())
		fresh = e == nil
		return e
	})
	if e != nil || !fresh {
		return e
	}
	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	_, code := tr.(supportplugins.Sender).SupportReply(sendCtx, user, text, nil)
	cancel()
	state := "FAILED"
	if code == "DELIVERY_ACKNOWLEDGED" {
		state = "SENT"
	} else if code == "DELIVERY_OUTCOME_UNKNOWN" {
		state = "UNKNOWN"
	}
	finalCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	e = s.DB.Tenant(finalCtx, actor, scope.AccountID, false, func(tx pgx.Tx) error {
		_, e := tx.Exec(finalCtx, "UPDATE intelligence_deliveries SET state=$4,completed_at=$5 WHERE workspace_id=$1 AND bot_id=$2 AND operation_key=$3 AND state='SENDING'", scope.AccountID, scope.BotID, k, state, s.now())
		return e
	})
	if e != nil || state != "SENT" {
		return &provider.Failure{Code: "TELEGRAM_OUTCOME_UNKNOWN", Unknown: true}
	}
	return nil
}

// The existing authenticated Telegram webhook owns identity verification.
func (s *Service) HandleSupport(ctx context.Context, scope sdk.Scope, corr string, u contentplugins.Update) error {
	if u.Message == nil || u.Message.Chat.Type != "private" || u.Message.From.ID != u.Message.Chat.ID {
		return nil
	}
	m := u.Message
	if strings.HasPrefix(m.Text, "/support") || m.Document != nil || m.Video != nil || m.Audio != nil || len(m.Photos) > 0 {
		return s.Support.HandleSupport(ctx, scope, corr, u)
	}
	scope.PluginID = assistant.ID
	actor, e := s.principal(ctx, scope)
	if e != nil {
		return e
	}
	var active bool
	var handoff bool
	var mode string
	e = s.DB.Tenant(ctx, actor, scope.AccountID, false, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM plugin_installations WHERE workspace_id=$1 AND bot_id=$2 AND plugin_id=$3 AND state='ACTIVE')", scope.AccountID, scope.BotID, assistant.ID).Scan(&active); err != nil {
			return err
		}
		if !active {
			return nil
		}
		config, err := s.config(ctx, tx, scope)
		if err != nil {
			return err
		}
		mode = config.Mode
		return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ai_requests r JOIN ai_threads t ON t.id=r.thread_id WHERE t.workspace_id=$1 AND t.bot_id=$2 AND t.subject=$3 AND r.state='SUCCEEDED' AND COALESCE(r.result->>'handoff','')<>'' AND EXISTS(SELECT 1 FROM support_tickets st WHERE st.workspace_id=t.workspace_id AND st.bot_id=t.bot_id AND st.contact_id=t.contact_id AND st.status NOT IN('CLOSED','RESOLVED')))`, scope.AccountID, scope.BotID, "telegram:"+strconv.FormatInt(m.From.ID, 10)).Scan(&handoff)
	})
	if e != nil {
		return e
	}
	if !active || handoff || mode == "AGENT_ASSIST" {
		return s.Support.HandleSupport(ctx, scope, corr, u)
	}
	name := m.From.Name
	if name == "" {
		name = "مشتری تلگرام"
	}
	r, e := s.Chat(ctx, actor, scope, Chat{Key: "ai-telegram:" + strconv.FormatInt(u.ID, 10), Text: m.Text, User: m.From.ID, Name: name, Purpose: "CUSTOMER"})
	if e != nil {
		return e
	}
	if r.State != "SUCCEEDED" {
		return &provider.Failure{Code: r.Code, Unknown: r.State == "UNKNOWN"}
	}
	return s.Send(ctx, actor, scope, m.From.ID, r.Text, "ai-send:"+r.ID)
}
