package groupbot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/identity"
	"github.com/naseri-coder/TELMIVO/internal/domains/policies"
	"github.com/naseri-coder/TELMIVO/internal/platform/auth"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"log/slog"
	"strconv"
	"time"
	"unicode/utf8"
)

var ErrAuthority = errors.New("GROUP_AUTHORITY_UNAVAILABLE")
var ErrRights = errors.New("GROUP_BOT_RIGHT_MISSING")
var ErrProtected = errors.New("GROUP_TARGET_PROTECTED")

type Member struct {
	User struct {
		IsBot bool `json:"is_bot"`
	} `json:"user"`
	Status   string `json:"status"`
	Delete   bool   `json:"can_delete_messages"`
	Restrict bool   `json:"can_restrict_members"`
	Invite   bool   `json:"can_invite_users"`
	Manage   bool   `json:"can_manage_chat"`
}

func (m Member) Admin() bool { return m.Status == "creator" || m.Status == "administrator" }

type Authority interface {
	Inspect(context.Context, int64, int64) (Member, error)
	Bot(context.Context, int64) (int64, Member, error)
}
type Transport interface {
	Execute(context.Context, int64, int64, int64, Action, int64) error
}
type Service struct {
	DB         *database.Store
	Identity   *identity.Service
	Authority  Authority
	Transport  Transport
	MiniAppURL string
	Now        func() time.Time
}
type View struct {
	ID              string            `json:"id"`
	Title           string            `json:"title"`
	Status          string            `json:"status"`
	Revision        int64             `json:"revision"`
	Policy          Policy            `json:"policy"`
	Rights          Member            `json:"rights"`
	AuthorityHealth string            `json:"authority_health"`
	Access          policies.Decision `json:"access"`
}
type claimAuthority struct{ s *Service }

func (a claimAuthority) CanAdmin(ctx context.Context, chat, user int64) (bool, error) {
	if a.s.Authority == nil {
		return false, ErrAuthority
	}
	m, e := a.s.Authority.Inspect(ctx, chat, user)
	if e != nil {
		return false, ErrAuthority
	}
	return m.Admin(), nil
}
func (s *Service) Claim(ctx context.Context, tx pgx.Tx, user, ws, corr string, tg, chat int64, title string) (string, error) {
	if s.Authority == nil {
		return "", ErrAuthority
	}
	_, bot, e := s.Authority.Bot(ctx, chat)
	if e != nil {
		return "", ErrAuthority
	}
	if !bot.Admin() {
		return "", ErrRights
	}
	id, e := identity.ClaimGroup(ctx, tx, user, ws, corr, tg, chat, title, claimAuthority{s})
	if e != nil {
		return "", e
	}
	_, e = tx.Exec(ctx, "INSERT INTO group_settings(workspace_id,group_id,moderation,security) VALUES($1,$2,$3,$4) ON CONFLICT(group_id) DO UPDATE SET security=excluded.security", ws, id, DefaultPolicy(), map[string]any{"bot_rights": bot, "checked_at": time.Now().UTC()})
	if e == nil {
		_, e = tx.Exec(ctx, "UPDATE groups SET status='CONNECTED' WHERE id=$1 AND workspace_id=$2", id, ws)
	}
	return id, e
}
func (s *Service) read(ctx context.Context, tx pgx.Tx, ws, id string) (View, int64, error) {
	v := View{Policy: DefaultPolicy(), AuthorityHealth: "REVALIDATION_REQUIRED"}
	var chat int64
	var p, b []byte
	e := tx.QueryRow(ctx, "SELECT g.id::text,g.title,g.status,g.telegram_chat_id,s.revision,s.moderation,s.security FROM groups g JOIN group_settings s ON s.group_id=g.id AND s.workspace_id=g.workspace_id WHERE g.workspace_id=$1 AND g.id::text=$2", ws, id).Scan(&v.ID, &v.Title, &v.Status, &chat, &v.Revision, &p, &b)
	if e != nil {
		return v, chat, e
	}
	if json.Unmarshal(p, &v.Policy) != nil {
		return v, chat, database.ErrInvalid
	}
	var sec struct {
		Rights Member `json:"bot_rights"`
	}
	_ = json.Unmarshal(b, &sec)
	v.Rights = sec.Rights
	return v, chat, nil
}
func Access(ctx context.Context, tx pgx.Tx, user, ws, id string) (policies.Decision, error) {
	var role string
	if e := tx.QueryRow(ctx, "SELECT role_key FROM workspace_members WHERE user_id=$1 AND workspace_id=$2", user, ws).Scan(&role); e != nil {
		return policies.Decision{}, e
	}
	return policies.Resolve(ctx, tx, policies.Request{User: user, Workspace: ws, Role: role, Group: id, Feature: "groups", Permission: "groups.moderate"})
}
func (s *Service) View(ctx context.Context, tx pgx.Tx, user, ws, id string) (View, error) {
	v, _, e := s.read(ctx, tx, ws, id)
	if e == nil {
		v.Access, e = s.resolve(ctx, tx, user, ws, id)
	}
	return v, e
}
func (s *Service) authorize(ctx context.Context, tx pgx.Tx, user, ws string, chat int64) (Member, int64, error) {
	if !database.Permission(ctx, tx, user, ws, "groups.moderate") {
		return Member{}, 0, database.ErrForbidden
	}
	tg, e := identity.TelegramID(ctx, tx, user)
	if e != nil {
		return Member{}, 0, e
	}
	if s.Authority == nil {
		return Member{}, 0, ErrAuthority
	}
	m, e := s.Authority.Inspect(ctx, chat, tg)
	if e != nil {
		return Member{}, 0, ErrAuthority
	}
	if !m.Admin() {
		return Member{}, 0, database.ErrForbidden
	}
	botID, bot, e := s.Authority.Bot(ctx, chat)
	if e != nil {
		return Member{}, 0, ErrAuthority
	}
	if !bot.Admin() {
		return bot, botID, ErrRights
	}
	return bot, botID, nil
}
func (s *Service) Configure(ctx context.Context, tx pgx.Tx, user, ws, id, corr string, revision int64, p Policy) (View, error) {
	if e := p.Validate(); e != nil {
		return View{}, e
	}
	old, chat, e := s.read(ctx, tx, ws, id)
	if e != nil {
		return old, e
	}
	bot, _, e := s.authorize(ctx, tx, user, ws, chat)
	if e != nil {
		return old, e
	}
	d, e := s.resolve(ctx, tx, user, ws, id)
	if e != nil {
		return old, e
	}
	if !d.Allowed {
		return old, &policies.Denied{Decision: d}
	}
	tag, e := tx.Exec(ctx, "UPDATE group_settings SET moderation=$1,revision=revision+1,security=$2,updated_at=now() WHERE workspace_id=$3 AND group_id=$4 AND revision=$5", p, map[string]any{"bot_rights": bot, "checked_at": time.Now().UTC()}, ws, id, revision)
	if e != nil {
		return old, e
	}
	if tag.RowsAffected() != 1 {
		return old, database.ErrConflict
	}
	if e = database.Audit(ctx, tx, user, ws, "group.policy.update", "group", id, corr, old.Policy, p); e != nil {
		return old, e
	}
	return s.View(ctx, tx, user, ws, id)
}
func record(ctx context.Context, tx pgx.Tx, user, ws, id, key, kind string, target, msg int64, action Action, reason, outcome, corr, digest string) (bool, error) {
	tag, e := tx.Exec(ctx, `INSERT INTO group_moderation_events(workspace_id,group_id,actor_id,target_id,message_id,operation_key,kind,action,reason,outcome,correlation_id,message_digest) VALUES($1,$2,NULLIF($3,'')::uuid,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT DO NOTHING`, ws, id, user, target, msg, key, kind, action, reason, outcome, corr, digest)
	if e != nil {
		return false, e
	}
	if tag.RowsAffected() == 0 {
		var same bool
		e = tx.QueryRow(ctx, "SELECT target_id=$4 AND message_id=$5 AND action=$6 AND reason=$7 FROM group_moderation_events WHERE workspace_id=$1 AND group_id=$2 AND operation_key=$3 AND kind=$8", ws, id, key, target, msg, action, reason, kind).Scan(&same)
		if e != nil {
			return false, e
		}
		if !same {
			return false, database.ErrConflict
		}
	}
	return tag.RowsAffected() == 1, nil
}
func activeWarnings(ctx context.Context, tx pgx.Tx, ws, id string, target int64) (int, error) {
	var n int
	e := tx.QueryRow(ctx, `SELECT count(*) FROM group_moderation_events WHERE workspace_id=$1 AND group_id=$2 AND target_id=$3 AND kind='WARNING' AND sequence>COALESCE((SELECT max(sequence) FROM group_moderation_events WHERE workspace_id=$1 AND group_id=$2 AND target_id=$3 AND kind='CLEAR_WARNINGS'),0)`, ws, id, target).Scan(&n)
	return n, e
}
func rightsFor(m Member, a Action) bool {
	switch a {
	case Delete:
		return m.Admin() && m.Delete
	case Restrict, Ban, Unmute, Unban:
		return m.Admin() && m.Restrict
	default:
		return true
	}
}
func (s *Service) request(ctx context.Context, tx pgx.Tx, user, ws, id, key, corr string, chat, target, msg int64, a Action, reason string, p Policy, bot Member, botID int64) error {
	if target <= 0 || target == botID {
		return ErrProtected
	}
	if s.Authority == nil {
		return ErrAuthority
	}
	member, e := s.Authority.Inspect(ctx, chat, target)
	if e != nil {
		return ErrAuthority
	}
	if member.Admin() || member.User.IsBot {
		return ErrProtected
	}
	if !rightsFor(bot, a) {
		return ErrRights
	}
	var priorAction, priorReason string
	var priorTarget, priorMsg int64
	err := tx.QueryRow(ctx, "SELECT action,reason,target_id,message_id FROM group_moderation_events WHERE workspace_id=$1 AND group_id=$2 AND operation_key=$3 AND kind IN('DECISION','WARNING','CLEAR_WARNINGS')", ws, id, key).Scan(&priorAction, &priorReason, &priorTarget, &priorMsg)
	if err == nil {
		if priorAction != string(a) || priorReason != reason || priorTarget != target || priorMsg != msg {
			return database.ErrConflict
		}
		return nil
	}
	if err != pgx.ErrNoRows {
		return err
	}
	kind := "DECISION"
	outcome := "RECORDED"
	if a == Warn {
		kind = "WARNING"
	}
	if a == Clear {
		kind = "CLEAR_WARNINGS"
	}
	if a == Delete || a == Restrict || a == Ban || a == Unmute || a == Unban {
		outcome = "ACTION_REQUESTED"
	}
	fresh, e := record(ctx, tx, user, ws, id, key, kind, target, msg, a, reason, outcome, corr, "")
	if e != nil || !fresh {
		return e
	}
	if e = database.Audit(ctx, tx, user, ws, "group."+kind, "group", id, corr, map[string]any{}, map[string]any{"action": a, "reason": reason, "target_id": target}); e != nil {
		return e
	}
	if a == Warn && p.WarningThreshold > 0 {
		n, e := activeWarnings(ctx, tx, ws, id, target)
		if e != nil {
			return e
		}
		if n >= p.WarningThreshold && p.WarningAction != Warn && p.WarningAction != Log {
			return s.request(ctx, tx, user, ws, id, key+":threshold", corr, chat, target, msg, p.WarningAction, "WARNING_THRESHOLD", p, bot, botID)
		}
	}
	if outcome != "ACTION_REQUESTED" {
		return nil
	}
	return database.Event(ctx, tx, ws, "telmivo.jobs.notification", "group.action", corr, map[string]any{"group_id": id, "chat_id": chat, "target_id": target, "message_id": msg, "action": a, "reason": reason, "duration": p.RestrictSeconds, "operation_key": key, "actor_id": user})
}
func (s *Service) Moderate(ctx context.Context, tx pgx.Tx, user, ws, id, key, corr string, target, msg int64, a Action, reason string) error {
	if len(key) < 1 || len(key) > 150 || utf8.RuneCountInString(reason) < 1 || utf8.RuneCountInString(reason) > 500 || (!actionValid(a) && a != Clear && a != Unmute && a != Unban) {
		return database.ErrInvalid
	}
	v, chat, e := s.read(ctx, tx, ws, id)
	if e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", id+":"+strconv.FormatInt(target, 10)); e != nil {
		return e
	}
	bot, botID, e := s.authorize(ctx, tx, user, ws, chat)
	if e != nil {
		return e
	}
	tg, err := identity.TelegramID(ctx, tx, user)
	if err != nil {
		return err
	}
	moderator, err := s.Authority.Inspect(ctx, chat, tg)
	if err != nil {
		return ErrAuthority
	}
	if moderator.Status != "creator" && !rightsFor(moderator, a) {
		return database.ErrForbidden
	}
	d, e := s.resolve(ctx, tx, user, ws, id)
	if e != nil {
		return e
	}
	if !d.Allowed {
		return &policies.Denied{Decision: d}
	}
	return s.request(ctx, tx, user, ws, id, key, corr, chat, target, msg, a, reason, v.Policy, bot, botID)
}
func (s *Service) Message(ctx context.Context, tx pgx.Tx, user, ws, id, corr string, update int64, m Message) (Decision, error) {
	if update < 0 || m.ID <= 0 || m.User <= 0 || utf8.RuneCountInString(m.Text) > 4096 || len(m.Text) > 16384 {
		return Decision{}, database.ErrInvalid
	}
	if len(m.Links) > 50 {
		return Decision{}, database.ErrInvalid
	}
	for _, link := range m.Links {
		if len(link) > 4096 {
			return Decision{}, database.ErrInvalid
		}
	}
	v, chat, e := s.read(ctx, tx, ws, id)
	if e != nil {
		return Decision{}, e
	}
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", id+":"+strconv.FormatInt(m.User, 10)); e != nil {
		return Decision{}, e
	}
	tag, e := tx.Exec(ctx, "INSERT INTO telegram_updates(bot_key,update_id) VALUES('group-bot',$1) ON CONFLICT DO NOTHING", update)
	if e != nil {
		return Decision{}, e
	}
	if tag.RowsAffected() == 0 {
		return Decision{Log, "DUPLICATE_UPDATE"}, nil
	}
	d, e := s.resolve(ctx, tx, user, ws, id)
	if e != nil {
		return Decision{}, e
	}
	if !d.Allowed || v.Status != "CONNECTED" {
		_, e = record(ctx, tx, user, ws, id, fmt.Sprintf("update:%d", update), "DECISION", m.User, m.ID, Log, "ACCESS_DISABLED", "RECORDED", corr, "")
		return Decision{Log, "ACCESS_DISABLED"}, e
	}
	if s.Authority == nil {
		return Decision{}, ErrAuthority
	}
	member, e := s.Authority.Inspect(ctx, chat, m.User)
	if e != nil {
		return Decision{}, ErrAuthority
	}
	m.Trusted = member.Admin()
	botID, bot, e := s.Authority.Bot(ctx, chat)
	if e != nil {
		return Decision{}, ErrAuthority
	}
	if _, _, e = s.authorize(ctx, tx, user, ws, chat); e != nil {
		return Decision{}, e
	}
	key := fmt.Sprintf("update:%d", update)
	digest := Fingerprint(m.Text)
	if _, e = record(ctx, tx, user, ws, id, key, "MESSAGE", m.User, m.ID, Log, "MESSAGE_OBSERVED", "RECORDED", corr, digest); e != nil {
		return Decision{}, e
	}
	var count, duplicates int
	e = tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE message_digest=$4) FROM (SELECT message_digest FROM group_moderation_events WHERE workspace_id=$1 AND group_id=$2 AND target_id=$3 AND kind='MESSAGE' AND created_at>now()-make_interval(secs=>$5) ORDER BY created_at DESC LIMIT 1001) recent`, ws, id, m.User, digest, v.Policy.WindowSeconds).Scan(&count, &duplicates)
	if e != nil {
		return Decision{}, e
	}
	decision := Evaluate(v.Policy, m, count, duplicates)
	slog.Info("group moderation decision", "decision", decision.Action, "reason", decision.Reason)
	if member.Admin() || member.User.IsBot || m.User == botID {
		decision = Decision{Log, "PROTECTED_TARGET"}
		_, e = record(ctx, tx, user, ws, id, key, "DECISION", m.User, m.ID, Log, decision.Reason, "RECORDED", corr, "")
		return decision, e
	}
	e = s.request(ctx, tx, user, ws, id, key, corr, chat, m.User, m.ID, decision.Action, decision.Reason, v.Policy, bot, botID)
	return decision, e
}
func (s *Service) Enter(ctx context.Context, u auth.TelegramUser, event string) (identity.Result, error) {
	tx, e := s.DB.Pool.Begin(ctx)
	if e != nil {
		return identity.Result{}, e
	}
	defer tx.Rollback(ctx)
	r, e := identity.Enter(ctx, tx, u, s.Identity.Admins, "GROUP_BOT", event)
	if e == nil {
		e = tx.Commit(ctx)
	}
	return r, e
}

// Deliver revalidates access, Telegram authority/rights and target immediately before transport.
// Unknown outcomes are durable and never blindly retried by this consumer.
func (s *Service) Deliver(ctx context.Context, tx pgx.Tx, ws, corr string, raw []byte) error {
	var in struct {
		Group    string `json:"group_id"`
		Chat     int64  `json:"chat_id"`
		Target   int64  `json:"target_id"`
		Message  int64  `json:"message_id"`
		Action   Action `json:"action"`
		Reason   string `json:"reason"`
		Duration int64  `json:"duration"`
		Key      string `json:"operation_key"`
		Actor    string `json:"actor_id"`
	}
	if json.Unmarshal(raw, &in) != nil || in.Key == "" || in.Target <= 0 || in.Duration < 30 || in.Duration > 86400 {
		return database.ErrInvalid
	}
	var exists bool
	if e := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM group_moderation_events WHERE workspace_id=$1 AND group_id=$2 AND operation_key=$3 AND kind='ACTION_RESULT')", ws, in.Group, in.Key).Scan(&exists); e != nil {
		return e
	}
	if exists {
		return nil
	}
	var valid bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM group_moderation_events WHERE workspace_id=$1 AND group_id=$2 AND operation_key=$3 AND action=$4 AND target_id=$5 AND outcome='ACTION_REQUESTED')", ws, in.Group, in.Key, in.Action, in.Target).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return database.ErrForbidden
	}
	v, chat, e := s.read(ctx, tx, ws, in.Group)
	if e != nil {
		return e
	}
	if chat != in.Chat {
		return database.ErrInvalid
	}
	bot, botID, e := s.authorize(ctx, tx, in.Actor, ws, chat)
	if e == nil {
		tg, err := identity.TelegramID(ctx, tx, in.Actor)
		if err != nil {
			e = err
		} else {
			m, err := s.Authority.Inspect(ctx, chat, tg)
			if err != nil {
				e = ErrAuthority
			} else if m.Status != "creator" && !rightsFor(m, in.Action) {
				e = database.ErrForbidden
			}
		}
	}
	if e == nil && !rightsFor(bot, in.Action) {
		e = ErrRights
	}
	if e == nil {
		var m Member
		m, e = s.Authority.Inspect(ctx, chat, in.Target)
		if e == nil && (m.Admin() || m.User.IsBot || in.Target == botID) {
			e = ErrProtected
		}
	}
	if e == nil {
		var d policies.Decision
		d, e = s.resolve(ctx, tx, in.Actor, ws, in.Group)
		if e == nil && (!d.Allowed || v.Status != "CONNECTED") {
			e = database.ErrForbidden
		}
	}
	outcome := "ACTION_FAILED"
	if e == nil {
		if s.Transport == nil {
			e = ErrAuthority
		} else {
			e = s.Transport.Execute(ctx, chat, in.Target, in.Message, in.Action, time.Now().Unix()+in.Duration)
		}
		if e == nil {
			outcome = "ACTION_ACKNOWLEDGED"
		} else if e.Error() == "TELEGRAM_DELIVERY_UNKNOWN" {
			outcome = "ACTION_OUTCOME_UNKNOWN"
		}
	}
	slog.Info("group action result", "outcome", outcome)
	_, err := record(ctx, tx, in.Actor, ws, in.Group, in.Key, "ACTION_RESULT", in.Target, in.Message, in.Action, "TRANSPORT_RESULT", outcome, corr, "")
	if err != nil {
		return err
	}
	return database.Audit(ctx, tx, in.Actor, ws, "group.action.result", "group", in.Group, corr, map[string]any{}, map[string]any{"action": in.Action, "outcome": outcome})
}

func (s *Service) resolve(ctx context.Context, tx pgx.Tx, user, ws, id string) (policies.Decision, error) {
	if s.Now == nil {
		return Access(ctx, tx, user, ws, id)
	}
	var role string
	if e := tx.QueryRow(ctx, "SELECT role_key FROM workspace_members WHERE user_id=$1 AND workspace_id=$2", user, ws).Scan(&role); e != nil {
		return policies.Decision{}, e
	}
	return policies.Resolve(ctx, tx, policies.Request{User: user, Workspace: ws, Role: role, Group: id, Feature: "groups", Permission: "groups.moderate", Now: s.Now()})
}
