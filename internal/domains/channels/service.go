package channels

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/entitlements"
	"github.com/naseri-coder/TELMIVO/internal/domains/groupbot"
	"github.com/naseri-coder/TELMIVO/internal/domains/identity"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	telegramtransport "github.com/naseri-coder/TELMIVO/internal/platform/telegram"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrAuthority = errors.New("CHANNEL_AUTHORITY_UNAVAILABLE")
var ErrBotRights = errors.New("CHANNEL_BOT_RIGHT_MISSING")

type Authority interface {
	Inspect(context.Context, int64, int64) (groupbot.Member, error)
	Bot(context.Context, int64) (int64, groupbot.Member, error)
	Channel(context.Context, int64) (string, error)
}
type Publisher interface {
	Send(context.Context, telegramtransport.Message) (int64, error)
}
type Service struct {
	DB        *database.Store
	Authority Authority
	Publisher Publisher
	Now       func() time.Time
}
type Channel struct {
	ID              string    `json:"id"`
	TelegramChatID  int64     `json:"telegram_chat_id"`
	Title           string    `json:"title"`
	Status          string    `json:"status"`
	AuthorityHealth string    `json:"authority_health"`
	BotCanPost      bool      `json:"bot_can_post"`
	CheckedAt       time.Time `json:"checked_at,omitempty"`
}
type Post struct {
	ID                string     `json:"id"`
	ChannelID         string     `json:"channel_id"`
	Body              string     `json:"body"`
	State             string     `json:"state"`
	ScheduledAt       *time.Time `json:"scheduled_at,omitempty"`
	AttemptedAt       *time.Time `json:"attempted_at,omitempty"`
	DeliveredAt       *time.Time `json:"delivered_at,omitempty"`
	TelegramMessageID *int64     `json:"telegram_message_id,omitempty"`
	OutcomeCode       string     `json:"outcome_code"`
	Revision          int64      `json:"revision"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}
type CreatePost struct {
	Body         string     `json:"body"`
	ScheduledAt  *time.Time `json:"scheduled_at,omitempty"`
	OperationKey string     `json:"idempotency_key"`
}
type EditPost struct {
	Body             string `json:"body"`
	ExpectedRevision int64  `json:"expected_revision"`
}
type Action struct {
	Action           string     `json:"action"`
	ExpectedRevision int64      `json:"expected_revision"`
	ScheduledAt      *time.Time `json:"scheduled_at,omitempty"`
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func botCanPost(m groupbot.Member) bool {
	return m.Status == "creator" || (m.Status == "administrator" && m.Post)
}
func postBody(body string) bool {
	return strings.TrimSpace(body) != "" && utf8.ValidString(body) && utf8.RuneCountInString(body) <= 4096 && len(body) <= 16384
}
func fingerprint(body string, scheduled *time.Time) string {
	b, _ := json.Marshal(struct {
		Body      string     `json:"body"`
		Scheduled *time.Time `json:"scheduled_at,omitempty"`
	}{body, scheduled})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (s *Service) verify(ctx context.Context, tx pgx.Tx, actor, ws string, chat int64) (string, groupbot.Member, error) {
	if s.Authority == nil || chat >= 0 {
		return "", groupbot.Member{}, ErrAuthority
	}
	tg, e := identity.TelegramID(ctx, tx, actor)
	if e != nil {
		return "", groupbot.Member{}, e
	}
	title, e := s.Authority.Channel(ctx, chat)
	if e != nil || strings.TrimSpace(title) == "" {
		return "", groupbot.Member{}, ErrAuthority
	}
	member, e := s.Authority.Inspect(ctx, chat, tg)
	if e != nil {
		return "", groupbot.Member{}, ErrAuthority
	}
	if !member.Admin() {
		return "", groupbot.Member{}, database.ErrForbidden
	}
	_, bot, e := s.Authority.Bot(ctx, chat)
	if e != nil {
		return "", groupbot.Member{}, ErrAuthority
	}
	if !botCanPost(bot) {
		return "", bot, ErrBotRights
	}
	return title, bot, nil
}
func security(bot groupbot.Member, at time.Time) map[string]any {
	return map[string]any{"bot_rights": bot, "checked_at": at}
}
func (s *Service) Connect(ctx context.Context, tx pgx.Tx, actor, ws, corr string, chat int64) (Channel, error) {
	var out Channel
	if chat >= 0 || !database.Permission(ctx, tx, actor, ws, "channels.manage") {
		return out, database.ErrForbidden
	}
	// Match the entitlement reservation lock before counting resources.
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", ws+":channels"); err != nil {
		return out, err
	}
	var existing string
	e := tx.QueryRow(ctx, "SELECT id::text FROM channels WHERE workspace_id=$1 AND telegram_chat_id=$2", ws, chat).Scan(&existing)
	requested := int64(1)
	if e == nil {
		requested = 0
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	if e = entitlements.Require(ctx, tx, actor, ws, "channels", "channels.manage", requested); e != nil {
		return out, e
	}
	title, bot, e := s.verify(ctx, tx, actor, ws, chat)
	if e != nil {
		return out, e
	}
	if existing == "" {
		e = tx.QueryRow(ctx, "INSERT INTO channels(workspace_id,telegram_chat_id,title,status) VALUES($1,$2,$3,'CONNECTED') RETURNING id::text", ws, chat, title).Scan(&existing)
	} else {
		_, e = tx.Exec(ctx, "UPDATE channels SET title=$3,status='CONNECTED' WHERE workspace_id=$1 AND id::text=$2", ws, existing, title)
	}
	if e != nil {
		return out, e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO channel_settings(workspace_id,channel_id,settings) VALUES($1,$2,$3) ON CONFLICT(channel_id) DO UPDATE SET settings=channel_settings.settings||EXCLUDED.settings,updated_at=now()", ws, existing, security(bot, s.now())); e != nil {
		return out, e
	}
	if e = database.Audit(ctx, tx, actor, ws, "channel.connect", "channel", existing, corr, map[string]any{}, map[string]any{"chat_id": chat, "status": "CONNECTED"}); e != nil {
		return out, e
	}
	return s.read(ctx, tx, ws, existing)
}
func (s *Service) read(ctx context.Context, tx pgx.Tx, ws, id string) (Channel, error) {
	var out Channel
	var checked *time.Time
	e := tx.QueryRow(ctx, `SELECT c.id::text,c.telegram_chat_id,c.title,c.status,
		COALESCE((s.settings->'bot_rights'->>'can_post_messages')::boolean,false),
		NULLIF(s.settings->>'checked_at','')::timestamptz
		FROM channels c LEFT JOIN channel_settings s ON s.channel_id=c.id AND s.workspace_id=c.workspace_id
		WHERE c.workspace_id=$1 AND c.id::text=$2`, ws, id).Scan(&out.ID, &out.TelegramChatID, &out.Title, &out.Status, &out.BotCanPost, &checked)
	if e != nil {
		return out, e
	}
	if checked != nil {
		out.CheckedAt = checked.UTC()
		out.AuthorityHealth = "VERIFIED"
	} else {
		out.AuthorityHealth = "REVALIDATION_REQUIRED"
	}
	if out.Status != "CONNECTED" {
		out.AuthorityHealth = "DISCONNECTED"
	}
	return out, nil
}
func (s *Service) List(ctx context.Context, tx pgx.Tx, actor, ws string) ([]Channel, error) {
	if e := entitlements.Require(ctx, tx, actor, ws, "channels", "channels.read", 0); e != nil {
		return nil, e
	}
	rows, e := tx.Query(ctx, "SELECT id::text FROM channels WHERE workspace_id=$1 ORDER BY created_at,id LIMIT 200", ws)
	if e != nil {
		return nil, e
	}
	var ids []string
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			break
		}
		ids = append(ids, id)
	}
	rows.Close()
	if e != nil {
		return nil, e
	}
	out := make([]Channel, 0, len(ids))
	for _, id := range ids {
		v, e := s.read(ctx, tx, ws, id)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Service) Verify(ctx context.Context, tx pgx.Tx, actor, ws, id, corr string) (Channel, error) {
	if e := entitlements.Require(ctx, tx, actor, ws, "channels", "channels.manage", 0); e != nil {
		return Channel{}, e
	}
	v, e := s.read(ctx, tx, ws, id)
	if e != nil {
		return v, e
	}
	title, bot, e := s.verify(ctx, tx, actor, ws, v.TelegramChatID)
	if e != nil {
		return v, e
	}
	if _, e = tx.Exec(ctx, "UPDATE channels SET title=$3,status='CONNECTED' WHERE workspace_id=$1 AND id::text=$2", ws, id, title); e != nil {
		return v, e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO channel_settings(workspace_id,channel_id,settings) VALUES($1,$2,$3) ON CONFLICT(channel_id) DO UPDATE SET settings=channel_settings.settings||EXCLUDED.settings,updated_at=now()", ws, id, security(bot, s.now())); e != nil {
		return v, e
	}
	if e = database.Audit(ctx, tx, actor, ws, "channel.verify", "channel", id, corr, map[string]any{}, map[string]any{"status": "CONNECTED"}); e != nil {
		return v, e
	}
	return s.read(ctx, tx, ws, id)
}
func (s *Service) Disconnect(ctx context.Context, tx pgx.Tx, actor, ws, id, corr string) (Channel, error) {
	if e := entitlements.Require(ctx, tx, actor, ws, "channels", "channels.manage", 0); e != nil {
		return Channel{}, e
	}
	v, e := s.read(ctx, tx, ws, id)
	if e != nil {
		return v, e
	}
	if v.Status != "DISCONNECTED" {
		if _, e = tx.Exec(ctx, "UPDATE channels SET status='DISCONNECTED' WHERE workspace_id=$1 AND id::text=$2", ws, id); e != nil {
			return v, e
		}
		if e = database.Audit(ctx, tx, actor, ws, "channel.disconnect", "channel", id, corr, map[string]any{"status": v.Status}, map[string]any{"status": "DISCONNECTED"}); e != nil {
			return v, e
		}
	}
	return s.read(ctx, tx, ws, id)
}
func scanPost(row pgx.Row) (Post, error) {
	var p Post
	e := row.Scan(&p.ID, &p.ChannelID, &p.Body, &p.State, &p.ScheduledAt, &p.AttemptedAt, &p.DeliveredAt, &p.TelegramMessageID, &p.OutcomeCode, &p.Revision, &p.CreatedAt, &p.UpdatedAt)
	return p, e
}

const postSelect = "SELECT id::text,channel_id::text,body,state,scheduled_at,attempted_at,delivered_at,telegram_message_id,outcome_code,revision,created_at,updated_at FROM channel_posts"

func (s *Service) post(ctx context.Context, tx pgx.Tx, ws, channel, id string) (Post, error) {
	return scanPost(tx.QueryRow(ctx, postSelect+" WHERE workspace_id=$1 AND channel_id::text=$2 AND id::text=$3", ws, channel, id))
}
func (s *Service) ListPosts(ctx context.Context, tx pgx.Tx, actor, ws, channel string) ([]Post, error) {
	if e := entitlements.Require(ctx, tx, actor, ws, "channels", "channels.read", 0); e != nil {
		return nil, e
	}
	rows, e := tx.Query(ctx, postSelect+" WHERE workspace_id=$1 AND channel_id::text=$2 ORDER BY created_at DESC,id DESC LIMIT 200", ws, channel)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Post{}
	for rows.Next() {
		p, e := scanPost(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func addHistory(ctx context.Context, tx pgx.Tx, ws string, p Post) error {
	_, e := tx.Exec(ctx, "INSERT INTO channel_post_history(workspace_id,channel_id,post_id,revision,state,outcome_code) VALUES($1,$2,$3,$4,$5,$6)", ws, p.ChannelID, p.ID, p.Revision, p.State, p.OutcomeCode)
	return e
}
func (s *Service) CreatePost(ctx context.Context, tx pgx.Tx, actor, ws, channel, corr string, in CreatePost) (Post, error) {
	var zero Post
	if !postBody(in.Body) || len(in.OperationKey) < 1 || len(in.OperationKey) > 100 || strings.TrimSpace(in.OperationKey) != in.OperationKey {
		return zero, database.ErrInvalid
	}
	if e := entitlements.Require(ctx, tx, actor, ws, "channels", "channels.publish", 0); e != nil {
		return zero, e
	}
	v, e := s.read(ctx, tx, ws, channel)
	if e != nil {
		return zero, e
	}
	var scheduled *time.Time
	state := "DRAFT"
	if in.ScheduledAt != nil {
		t := in.ScheduledAt.UTC()
		if t.Before(s.now().Add(-5*time.Second)) || t.After(s.now().Add(365*24*time.Hour)) || v.Status != "CONNECTED" {
			return zero, database.ErrInvalid
		}
		scheduled = &t
		state = "SCHEDULED"
	}
	fp := fingerprint(in.Body, scheduled)
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", channel+":"+in.OperationKey); e != nil {
		return zero, e
	}
	var prior, priorFP string
	e = tx.QueryRow(ctx, "SELECT id::text,fingerprint FROM channel_posts WHERE channel_id=$1 AND operation_key=$2", channel, in.OperationKey).Scan(&prior, &priorFP)
	if e == nil {
		if priorFP != fp {
			return zero, database.ErrConflict
		}
		return s.post(ctx, tx, ws, channel, prior)
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return zero, e
	}
	var id string
	e = tx.QueryRow(ctx, "INSERT INTO channel_posts(workspace_id,channel_id,actor_id,body,state,scheduled_at,operation_key,fingerprint) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text", ws, channel, actor, in.Body, state, scheduled, in.OperationKey, fp).Scan(&id)
	if e != nil {
		return zero, e
	}
	p, e := s.post(ctx, tx, ws, channel, id)
	if e != nil {
		return zero, e
	}
	if e = addHistory(ctx, tx, ws, p); e != nil {
		return zero, e
	}
	if e = database.Audit(ctx, tx, actor, ws, "channel.post.create", "channel_post", id, corr, map[string]any{}, map[string]any{"state": p.State, "channel": channel}); e != nil {
		return zero, e
	}
	return p, nil
}
func (s *Service) EditPost(ctx context.Context, tx pgx.Tx, actor, ws, channel, id, corr string, in EditPost) (Post, error) {
	if !postBody(in.Body) || in.ExpectedRevision < 1 {
		return Post{}, database.ErrInvalid
	}
	if e := entitlements.Require(ctx, tx, actor, ws, "channels", "channels.publish", 0); e != nil {
		return Post{}, e
	}
	old, e := s.post(ctx, tx, ws, channel, id)
	if e != nil {
		return old, e
	}
	if old.State != "DRAFT" {
		return old, database.ErrConflict
	}
	tag, e := tx.Exec(ctx, "UPDATE channel_posts SET body=$1,revision=revision+1,updated_at=now() WHERE workspace_id=$2 AND channel_id=$3 AND id=$4 AND revision=$5 AND state='DRAFT'", in.Body, ws, channel, id, in.ExpectedRevision)
	if e != nil {
		return old, e
	}
	if tag.RowsAffected() != 1 {
		return old, database.ErrConflict
	}
	p, e := s.post(ctx, tx, ws, channel, id)
	if e != nil {
		return old, e
	}
	if e = addHistory(ctx, tx, ws, p); e != nil {
		return old, e
	}
	e = database.Audit(ctx, tx, actor, ws, "channel.post.edit", "channel_post", id, corr, map[string]any{"revision": old.Revision}, map[string]any{"revision": p.Revision})
	return p, e
}
func (s *Service) Act(ctx context.Context, tx pgx.Tx, actor, ws, channel, id, corr string, in Action) (Post, error) {
	if in.ExpectedRevision < 1 {
		return Post{}, database.ErrInvalid
	}
	if e := entitlements.Require(ctx, tx, actor, ws, "channels", "channels.publish", 0); e != nil {
		return Post{}, e
	}
	v, e := s.read(ctx, tx, ws, channel)
	if e != nil {
		return Post{}, e
	}
	old, e := s.post(ctx, tx, ws, channel, id)
	if e != nil {
		return old, e
	}
	if old.Revision != in.ExpectedRevision {
		return old, database.ErrConflict
	}
	next := old.State
	var scheduled *time.Time = old.ScheduledAt
	switch in.Action {
	case "SCHEDULE":
		if old.State != "DRAFT" && old.State != "SCHEDULED" || in.ScheduledAt == nil || v.Status != "CONNECTED" {
			return old, database.ErrConflict
		}
		t := in.ScheduledAt.UTC()
		if t.Before(s.now().Add(-5*time.Second)) || t.After(s.now().Add(365*24*time.Hour)) {
			return old, database.ErrInvalid
		}
		scheduled, next = &t, "SCHEDULED"
	case "PUBLISH_NOW":
		if old.State != "DRAFT" && old.State != "SCHEDULED" || v.Status != "CONNECTED" {
			return old, database.ErrConflict
		}
		t := s.now()
		scheduled, next = &t, "SCHEDULED"
	case "CANCEL":
		if old.State != "DRAFT" && old.State != "SCHEDULED" {
			return old, database.ErrConflict
		}
		next = "CANCELLED"
		scheduled = old.ScheduledAt
	default:
		return old, database.ErrInvalid
	}
	tag, e := tx.Exec(ctx, "UPDATE channel_posts SET state=$1,scheduled_at=$2,revision=revision+1,updated_at=now() WHERE workspace_id=$3 AND channel_id=$4 AND id=$5 AND revision=$6", next, scheduled, ws, channel, id, old.Revision)
	if e != nil {
		return old, e
	}
	if tag.RowsAffected() != 1 {
		return old, database.ErrConflict
	}
	p, e := s.post(ctx, tx, ws, channel, id)
	if e != nil {
		return old, e
	}
	if e = addHistory(ctx, tx, ws, p); e != nil {
		return old, e
	}
	e = database.Audit(ctx, tx, actor, ws, "channel.post."+strings.ToLower(in.Action), "channel_post", id, corr, map[string]any{"state": old.State, "revision": old.Revision}, map[string]any{"state": p.State, "revision": p.Revision})
	return p, e
}
func (s *Service) History(ctx context.Context, tx pgx.Tx, actor, ws, channel, id string) ([]map[string]any, error) {
	if e := entitlements.Require(ctx, tx, actor, ws, "channels", "channels.read", 0); e != nil {
		return nil, e
	}
	rows, e := tx.Query(ctx, "SELECT revision,state,outcome_code,created_at FROM channel_post_history WHERE workspace_id=$1 AND channel_id=$2 AND post_id=$3 ORDER BY revision", ws, channel, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var revision int64
		var state, code string
		var at time.Time
		if e = rows.Scan(&revision, &state, &code, &at); e != nil {
			return nil, e
		}
		out = append(out, map[string]any{"revision": revision, "state": state, "outcome_code": code, "created_at": at})
	}
	return out, rows.Err()
}

type delivery struct {
	post, channel, actor, body string
	chat                       int64
}

func terminal(ctx context.Context, tx pgx.Tx, ws, id, state, code string, message *int64, at time.Time) error {
	var channel string
	var revision int64
	e := tx.QueryRow(ctx, "UPDATE channel_posts SET state=$2,outcome_code=$3,telegram_message_id=$4,delivered_at=$5,revision=revision+1,updated_at=now() WHERE workspace_id=$1 AND id=$6 AND state='SENDING' RETURNING channel_id::text,revision", ws, state, code, message, at, id).Scan(&channel, &revision)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, "INSERT INTO channel_post_history(workspace_id,channel_id,post_id,revision,state,outcome_code) VALUES($1,$2,$3,$4,$5,$6)", ws, channel, id, revision, state, code)
	return e
}
func (s *Service) claim(ctx context.Context, operator, ws, id string) (delivery, bool, error) {
	var out delivery
	claimed := false
	e := s.DB.Tenant(ctx, operator, ws, false, func(tx pgx.Tx) error {
		var state, channelStatus string
		var scheduled, attempted *time.Time
		var revision int64
		e := tx.QueryRow(ctx, `SELECT p.channel_id::text,p.actor_id::text,p.body,p.state,p.scheduled_at,p.attempted_at,p.revision,c.telegram_chat_id,c.status
			FROM channel_posts p JOIN channels c ON c.id=p.channel_id AND c.workspace_id=p.workspace_id
			WHERE p.workspace_id=$1 AND p.id::text=$2 FOR UPDATE OF p`, ws, id).Scan(&out.channel, &out.actor, &out.body, &state, &scheduled, &attempted, &revision, &out.chat, &channelStatus)
		if e != nil {
			return e
		}
		now := s.now()
		if state == "SENDING" {
			if attempted != nil && attempted.Add(30*time.Second).Before(now) {
				return terminal(ctx, tx, ws, id, "UNKNOWN", "DELIVERY_OUTCOME_UNKNOWN", nil, now)
			}
			return database.ErrConflict
		}
		if state != "SCHEDULED" || scheduled == nil || scheduled.After(now) {
			return database.ErrConflict
		}
		if channelStatus != "CONNECTED" {
			tag, e := tx.Exec(ctx, "UPDATE channel_posts SET state='CANCELLED',outcome_code='CHANNEL_DISCONNECTED',revision=revision+1,updated_at=now() WHERE workspace_id=$1 AND id=$2 AND state='SCHEDULED'", ws, id)
			if e != nil || tag.RowsAffected() != 1 {
				return e
			}
			p, e := s.post(ctx, tx, ws, out.channel, id)
			if e != nil {
				return e
			}
			return addHistory(ctx, tx, ws, p)
		}
		if e := entitlements.Require(ctx, tx, out.actor, ws, "channels", "channels.publish", 0); e != nil {
			var denied *entitlements.Denied
			if errors.As(e, &denied) || errors.Is(e, pgx.ErrNoRows) {
				tag, x := tx.Exec(ctx, "UPDATE channel_posts SET state='CANCELLED',outcome_code='ACCESS_REVOKED',revision=revision+1,updated_at=now() WHERE workspace_id=$1 AND id=$2 AND state='SCHEDULED'", ws, id)
				if x != nil || tag.RowsAffected() != 1 {
					return x
				}
				p, x := s.post(ctx, tx, ws, out.channel, id)
				if x != nil {
					return x
				}
				return addHistory(ctx, tx, ws, p)
			}
			return e
		}
		tag, e := tx.Exec(ctx, "UPDATE channel_posts SET state='SENDING',attempted_at=$1,revision=revision+1,updated_at=now() WHERE workspace_id=$2 AND id=$3 AND state='SCHEDULED' AND revision=$4", now, ws, id, revision)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return database.ErrConflict
		}
		p, e := s.post(ctx, tx, ws, out.channel, id)
		if e != nil {
			return e
		}
		if e = addHistory(ctx, tx, ws, p); e != nil {
			return e
		}
		out.post = id
		claimed = true
		return nil
	})
	return out, claimed, e
}
func (s *Service) finish(ctx context.Context, operator, ws, id, state, code string, message *int64) error {
	return s.DB.Tenant(ctx, operator, ws, false, func(tx pgx.Tx) error {
		return terminal(ctx, tx, ws, id, state, code, message, s.now())
	})
}
func (s *Service) DeliverOne(ctx context.Context, operator, ws, id string) error {
	d, claimed, e := s.claim(ctx, operator, ws, id)
	if e != nil || !claimed {
		return e
	}
	fail := func(code string) error { return s.finish(ctx, operator, ws, id, "FAILED", code, nil) }
	if s.Authority == nil || s.Publisher == nil {
		return fail("TRANSPORT_UNAVAILABLE")
	}
	var tg int64
	if e = s.DB.Pool.QueryRow(ctx, "SELECT telegram_id FROM telegram_accounts WHERE user_id=$1", d.actor).Scan(&tg); e != nil {
		return fail("ACTOR_IDENTITY_UNAVAILABLE")
	}
	title, e := s.Authority.Channel(ctx, d.chat)
	if e != nil || title == "" {
		return fail("CHANNEL_AUTHORITY_UNAVAILABLE")
	}
	member, e := s.Authority.Inspect(ctx, d.chat, tg)
	if e != nil || !member.Admin() {
		return fail("ACTOR_NOT_ADMIN")
	}
	_, bot, e := s.Authority.Bot(ctx, d.chat)
	if e != nil || !botCanPost(bot) {
		return fail("BOT_RIGHT_MISSING")
	}
	message, e := s.Publisher.Send(ctx, telegramtransport.Message{ChatID: d.chat, Text: d.body})
	if e == nil && message > 0 {
		return s.finish(ctx, operator, ws, id, "SENT", "DELIVERY_ACKNOWLEDGED", &message)
	}
	if e != nil && e.Error() == "TELEGRAM_DELIVERY_UNKNOWN" {
		return s.finish(ctx, operator, ws, id, "UNKNOWN", "DELIVERY_OUTCOME_UNKNOWN", nil)
	}
	return fail("DELIVERY_FAILED")
}
func (s *Service) Poll(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	cursor := ""
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			rows, e := s.DB.Pool.Query(ctx, "SELECT user_id::text,workspace_id::text FROM workspace_members WHERE role_key='OWNER' AND workspace_id::text>$1 ORDER BY workspace_id LIMIT 20", cursor)
			if e != nil {
				return e
			}
			type owner struct{ actor, ws string }
			var owners []owner
			for rows.Next() {
				var o owner
				if e = rows.Scan(&o.actor, &o.ws); e != nil {
					break
				}
				owners = append(owners, o)
			}
			rowsErr := rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
			if rowsErr != nil {
				return rowsErr
			}
			if len(owners) == 0 {
				cursor = ""
				continue
			}
			for _, o := range owners {
				cursor = o.ws
				var ids []string
				e = s.DB.Tenant(ctx, o.actor, o.ws, false, func(tx pgx.Tx) error {
					rs, e := tx.Query(ctx, `SELECT id::text FROM channel_posts
						WHERE workspace_id=$1 AND ((state='SCHEDULED' AND scheduled_at<=clock_timestamp()) OR (state='SENDING' AND attempted_at<=clock_timestamp()-interval '30 seconds'))
						ORDER BY COALESCE(scheduled_at,attempted_at),id LIMIT 8 FOR UPDATE SKIP LOCKED`, o.ws)
					if e != nil {
						return e
					}
					defer rs.Close()
					for rs.Next() {
						var id string
						if e = rs.Scan(&id); e != nil {
							return e
						}
						ids = append(ids, id)
					}
					return rs.Err()
				})
				if e != nil {
					return e
				}
				for _, id := range ids {
					if e = s.DeliverOne(ctx, o.actor, o.ws, id); e != nil && !errors.Is(e, database.ErrConflict) {
						slog.WarnContext(ctx, "channel_delivery_blocked", "post_id", id, "error_code", "CHANNEL_DELIVERY_FAILURE")
					}
				}
			}
		}
	}
}
