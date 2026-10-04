package businessplugins

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/plugins/broadcast"
	"time"
)

type Recipient struct {
	ID      string `json:"id"`
	Contact string `json:"contact"`
	User    int64  `json:"telegram_id,string"`
	State   string `json:"state"`
	Code    string `json:"code"`
	Ordinal int    `json:"ordinal"`
	Attempt int    `json:"attempt"`
}

func (s *Service) SaveCampaign(ctx context.Context, actor string, scope sdk.Scope, id string, in Change[broadcast.Campaign]) (Definition, error) {
	scope.PluginID = broadcast.ID
	c := in.Definition
	if !c.Valid() {
		return Definition{}, database.ErrInvalid
	}
	c.ID = ""
	c.Revision = 0
	c.State = ""
	c.Recipients = 0
	c.Sent = 0
	c.Failed = 0
	c.Unknown = 0
	return s.save(ctx, actor, scope, id, c.Name, in.Key, in.Expected, c)
}
func (s *Service) audience(ctx context.Context, tx pgx.Tx, scope sdk.Scope, a broadcast.Audience) ([]Recipient, error) {
	out := []Recipient{}
	if !a.Valid() {
		return out, database.ErrInvalid
	}
	if e := s.crm(ctx, tx, scope, false); e != nil {
		return out, e
	}
	tags := a.Tags
	if tags == nil {
		tags = []string{}
	}
	rs, e := tx.Query(ctx, `SELECT c.id::text,i.external_key::bigint FROM crm_contacts c JOIN crm_identities i ON i.workspace_id=c.workspace_id AND i.bot_id=c.bot_id AND i.contact_id=c.id AND i.provider='telegram' WHERE c.workspace_id=$1 AND c.bot_id=$2 AND c.definition->>'status'='ACTIVE' AND c.definition->'tags' ?& $3::text[] AND ($4='' OR c.definition->>'locale'=$4) AND ($5::timestamptz IS NULL OR c.first_seen>=$5) ORDER BY c.id LIMIT 10001`, scope.AccountID, scope.BotID, tags, a.Locale, a.Since)
	if e != nil {
		return out, e
	}
	defer rs.Close()
	for rs.Next() {
		var r Recipient
		if e = rs.Scan(&r.Contact, &r.User); e != nil {
			return out, e
		}
		r.Ordinal = len(out)
		out = append(out, r)
	}
	if len(out) > broadcast.MaxRecipients {
		return nil, database.ErrInvalid
	}
	return out, rs.Err()
}
func (s *Service) PreviewAudience(ctx context.Context, actor string, scope sdk.Scope, a broadcast.Audience) (map[string]any, error) {
	scope.PluginID = broadcast.ID
	var n int
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error { r, e := s.audience(ctx, tx, scope, a); n = len(r); return e })
	return map[string]any{"eligible": n, "ceiling": broadcast.MaxRecipients, "snapshot_created": false}, e
}
func (s *Service) Campaign(ctx context.Context, actor string, scope sdk.Scope, id string) (broadcast.Campaign, error) {
	scope.PluginID = broadcast.ID
	var c broadcast.Campaign
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error { return campaign(ctx, tx, scope, id, &c) })
	return c, e
}
func campaign(ctx context.Context, tx pgx.Tx, scope sdk.Scope, id string, c *broadcast.Campaign) error {
	d, e := get(ctx, tx, scope, id)
	if e != nil {
		return e
	}
	if e = json.Unmarshal(d.Data, c); e != nil {
		return e
	}
	c.ID = d.ID
	c.Revision = d.Revision
	e = tx.QueryRow(ctx, "SELECT state FROM broadcast_campaigns WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3", scope.AccountID, scope.BotID, id).Scan(&c.State)
	if e != nil {
		return e
	}
	return tx.QueryRow(ctx, "SELECT count(*),count(*) FILTER(WHERE state='SENT'),count(*) FILTER(WHERE state='FAILED'),count(*) FILTER(WHERE state IN('SENDING','UNKNOWN')) FROM plugin_broadcast_recipients WHERE workspace_id=$1 AND bot_id=$2 AND campaign_id::text=$3", scope.AccountID, scope.BotID, id).Scan(&c.Recipients, &c.Sent, &c.Failed, &c.Unknown)
}

type Action struct {
	Action   string `json:"action"`
	Expected int64  `json:"expected_revision"`
	Key      string `json:"idempotency_key"`
}

func (s *Service) CampaignAction(ctx context.Context, actor string, scope sdk.Scope, id string, in Action) (broadcast.Campaign, error) {
	scope.PluginID = broadcast.ID
	var c broadcast.Campaign
	if !key(in.Key) || in.Expected < 1 {
		return c, database.ErrInvalid
	}
	fp := hash([]any{actor, id, in})
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		b, exists, e := receipt(ctx, tx, scope, in.Key, fp)
		if e != nil {
			return e
		}
		if exists {
			return json.Unmarshal(b, &c)
		}
		if e = campaign(ctx, tx, scope, id, &c); e != nil {
			return e
		}
		next, ok := broadcast.Transition(c.State, in.Action)
		if !ok || c.Revision != in.Expected {
			return database.ErrConflict
		}
		if in.Action == "START" {
			rs, e := s.audience(ctx, tx, scope, c.Audience)
			if e != nil {
				return e
			}
			if len(rs) == 0 {
				return database.ErrInvalid
			}
			if e = s.quota(ctx, tx, scope, "broadcast.recipients.count", int64(len(rs)), false); e != nil {
				return e
			}
			scheduled := s.now()
			if c.Scheduled != nil && c.Scheduled.After(scheduled) {
				scheduled = *c.Scheduled
			}
			for _, r := range rs {
				if _, e = tx.Exec(ctx, "INSERT INTO plugin_broadcast_recipients(workspace_id,bot_id,campaign_id,contact_id,telegram_id,ordinal,available_at) VALUES($1,$2,$3,$4,$5,$6,$7)", scope.AccountID, scope.BotID, id, r.Contact, r.User, r.Ordinal, scheduled); e != nil {
					return e
				}
			}
			if _, e = tx.Exec(ctx, "UPDATE broadcast_campaigns SET state=$4,scheduled_at=$5,snapshot_at=$6,revision=revision+1 WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3", scope.AccountID, scope.BotID, id, next, scheduled, s.now()); e != nil {
				return e
			}
		} else {
			if _, e = tx.Exec(ctx, "UPDATE broadcast_campaigns SET state=$4,revision=revision+1 WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3", scope.AccountID, scope.BotID, id, next); e != nil {
				return e
			}
			if in.Action == "CANCEL" {
				if _, e = tx.Exec(ctx, "UPDATE plugin_broadcast_recipients SET state='CANCELLED',code='CAMPAIGN_CANCELLED' WHERE workspace_id=$1 AND bot_id=$2 AND campaign_id::text=$3 AND state='QUEUED'", scope.AccountID, scope.BotID, id); e != nil {
					return e
				}
			}
		}
		kind := "broadcast.status_changed"
		if in.Action == "START" {
			kind = "broadcast.started"
		}
		if in.Action == "CANCEL" {
			kind = "broadcast.cancelled"
		}
		if e = emit(ctx, tx, actor, scope, kind, id, "", map[string][]string{"campaign.id": {id}, "campaign.status": {next}}); e != nil {
			return e
		}
		if e = campaign(ctx, tx, scope, id, &c); e != nil {
			return e
		}
		if e = s.campaignHistory(ctx, tx, scope, c.ID); e != nil {
			return e
		}
		return record(ctx, tx, scope, in.Key, fp, c)
	})
	return c, e
}

type RecipientPage struct {
	Items []Recipient `json:"items"`
	Next  string      `json:"next_cursor"`
}

func (s *Service) Recipients(ctx context.Context, actor string, scope sdk.Scope, id, after string) (RecipientPage, error) {
	scope.PluginID = broadcast.ID
	out := RecipientPage{Items: []Recipient{}}
	if len(after) > 36 {
		return out, database.ErrInvalid
	}
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		rs, e := tx.Query(ctx, "SELECT id::text,contact_id::text,telegram_id,state,code,ordinal,attempt FROM plugin_broadcast_recipients WHERE workspace_id=$1 AND bot_id=$2 AND campaign_id::text=$3 AND ($4='' OR id::text>$4) ORDER BY id LIMIT 51", scope.AccountID, scope.BotID, id, after)
		if e != nil {
			return e
		}
		defer rs.Close()
		for rs.Next() {
			var r Recipient
			if e = rs.Scan(&r.ID, &r.Contact, &r.User, &r.State, &r.Code, &r.Ordinal, &r.Attempt); e != nil {
				return e
			}
			out.Items = append(out.Items, r)
		}
		if len(out.Items) > 50 {
			out.Items = out.Items[:50]
			out.Next = out.Items[49].ID
		}
		return rs.Err()
	})
	return out, e
}

// BroadcastSender returns retry delay only for proven rejected requests (e.g. 429).
type BroadcastSender interface {
	BroadcastReply(context.Context, int64, string) (int64, string, time.Duration)
}

// DeliverOne reserves one side effect durably and never repeats an uncertain send.
func (s *Service) DeliverOne(ctx context.Context, actor string, scope sdk.Scope, id string) error {
	scope.PluginID = broadcast.ID
	var r Recipient
	var text string
	var tr contentplugins.Transport
	claimed := false
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, "UPDATE plugin_broadcast_recipients SET state='UNKNOWN',code='INTERRUPTED_SEND' WHERE workspace_id=$1 AND bot_id=$2 AND campaign_id::text=$3 AND state='SENDING' AND lease_until<=$4", scope.AccountID, scope.BotID, id, s.now()); e != nil {
			return e
		}
		var c broadcast.Campaign
		if e := campaign(ctx, tx, scope, id, &c); e != nil {
			return e
		}
		if c.State != "SCHEDULED" && c.State != "RUNNING" {
			return nil
		}
		var pending int
		if e := tx.QueryRow(ctx, "SELECT count(*) FROM plugin_broadcast_recipients WHERE workspace_id=$1 AND bot_id=$2 AND campaign_id::text=$3 AND state IN('QUEUED','SENDING')", scope.AccountID, scope.BotID, id).Scan(&pending); e != nil {
			return e
		}
		if pending == 0 {
			state := "COMPLETED"
			if c.Failed > 0 || c.Unknown > 0 {
				state = "FAILED"
			}
			if _, e := tx.Exec(ctx, "UPDATE broadcast_campaigns SET state=$4,completed_at=$5,revision=revision+1 WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3", scope.AccountID, scope.BotID, id, state, s.now()); e != nil {
				return e
			}
			if e := s.campaignHistory(ctx, tx, scope, id); e != nil {
				return e
			}
			return emit(ctx, tx, actor, scope, "broadcast.completed", id, "", map[string][]string{"campaign.id": {id}, "campaign.status": {state}})
		}
		if e := s.admit(ctx, tx, actor, scope, "telegram.send", true); e != nil {
			return e
		}
		if e := s.crm(ctx, tx, scope, false); e != nil {
			return e
		}
		if s.Content == nil || s.Content.Provider == nil {
			return sdk.ErrPermission
		}
		if _, e := tx.Exec(ctx, "INSERT INTO broadcast_rate(workspace_id,bot_id,next_at) VALUES($1,$2,$3) ON CONFLICT(bot_id) DO NOTHING", scope.AccountID, scope.BotID, s.now()); e != nil {
			return e
		}
		var next time.Time
		if e := tx.QueryRow(ctx, "SELECT next_at FROM broadcast_rate WHERE workspace_id=$1 AND bot_id=$2 FOR UPDATE", scope.AccountID, scope.BotID).Scan(&next); e != nil {
			return e
		}
		if next.After(s.now()) {
			return nil
		}
		e := tx.QueryRow(ctx, "SELECT id::text,contact_id::text,telegram_id,attempt FROM plugin_broadcast_recipients WHERE workspace_id=$1 AND bot_id=$2 AND campaign_id::text=$3 AND state='QUEUED' AND available_at<=$4 ORDER BY ordinal LIMIT 1 FOR UPDATE", scope.AccountID, scope.BotID, id, s.now()).Scan(&r.ID, &r.Contact, &r.User, &r.Attempt)
		if errors.Is(e, pgx.ErrNoRows) {
			return nil
		}
		if e != nil {
			return e
		}
		var eligible bool
		if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM crm_contacts c JOIN crm_identities i ON i.contact_id=c.id AND i.workspace_id=c.workspace_id AND i.bot_id=c.bot_id WHERE c.workspace_id=$1 AND c.bot_id=$2 AND c.id=$3 AND c.definition->>'status'='ACTIVE' AND i.provider='telegram' AND i.external_key=$4)", scope.AccountID, scope.BotID, r.Contact, fmtUser(r.User)).Scan(&eligible); e != nil {
			return e
		}
		if !eligible {
			_, e = tx.Exec(ctx, "UPDATE plugin_broadcast_recipients SET state='SKIPPED',code='CONTACT_INELIGIBLE' WHERE id=$1 AND workspace_id=$2 AND bot_id=$3", r.ID, scope.AccountID, scope.BotID)
			return e
		}
		tr, e = s.Content.Provider.ForBot(ctx, tx, scope)
		if e != nil {
			return e
		}
		if _, ok := tr.(BroadcastSender); !ok {
			return sdk.ErrPermission
		}
		if e = s.quota(ctx, tx, scope, "broadcast.messages.monthly", 1, true); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "UPDATE plugin_broadcast_recipients SET state='SENDING',attempt=attempt+1,lease_until=$4 WHERE id=$1 AND workspace_id=$2 AND bot_id=$3", r.ID, scope.AccountID, scope.BotID, s.now().Add(30*time.Second)); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "UPDATE broadcast_rate SET next_at=$3 WHERE workspace_id=$1 AND bot_id=$2", scope.AccountID, scope.BotID, s.now().Add(time.Second)); e != nil {
			return e
		}
		if c.State == "SCHEDULED" {
			if _, e = tx.Exec(ctx, "UPDATE broadcast_campaigns SET state='RUNNING',revision=revision+1 WHERE id::text=$3 AND workspace_id=$1 AND bot_id=$2", scope.AccountID, scope.BotID, id); e != nil {
				return e
			}
			if e = s.campaignHistory(ctx, tx, scope, id); e != nil {
				return e
			}
		}
		text = c.Text
		claimed = true
		return nil
	})
	if e != nil || !claimed {
		return e
	}
	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	message, code, delay := tr.(BroadcastSender).BroadcastReply(sendCtx, r.User, text)
	cancel()
	state := "UNKNOWN"
	switch code {
	case "DELIVERY_ACKNOWLEDGED":
		if message > 0 {
			state = "SENT"
		}
	case "DELIVERY_FAILED":
		state = "FAILED"
	case "RATE_LIMITED":
		state = "FAILED"
		if r.Attempt+1 < 3 {
			state = "QUEUED"
		}
	}
	if delay < time.Second {
		delay = time.Second
	}
	if delay > 24*time.Hour {
		delay = 24 * time.Hour
	}
	final, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return s.DB.Tenant(final, actor, scope.AccountID, false, func(tx pgx.Tx) error {
		if _, e := tx.Exec(final, "SELECT pg_advisory_xact_lock(hashtextextended($1,7100213))", scope.AccountID); e != nil {
			return e
		}
		if state == "QUEUED" {
			var active bool
			if e := tx.QueryRow(final, "SELECT state IN('RUNNING','SCHEDULED','PAUSED') FROM broadcast_campaigns WHERE id::text=$3 AND workspace_id=$1 AND bot_id=$2", scope.AccountID, scope.BotID, id).Scan(&active); e != nil {
				return e
			}
			if !active {
				state = "CANCELLED"
			}
		}
		if _, e := tx.Exec(final, "UPDATE plugin_broadcast_recipients SET state=$4,code=$5,message_id=$6,available_at=$7 WHERE id=$1 AND workspace_id=$2 AND bot_id=$3 AND state='SENDING'", r.ID, scope.AccountID, scope.BotID, state, code, message, s.now().Add(delay)); e != nil {
			return e
		}
		if code == "RATE_LIMITED" {
			_, e := tx.Exec(final, "UPDATE broadcast_rate SET next_at=greatest(next_at,$3) WHERE workspace_id=$1 AND bot_id=$2", scope.AccountID, scope.BotID, s.now().Add(delay))
			return e
		}
		return nil
	})
}
