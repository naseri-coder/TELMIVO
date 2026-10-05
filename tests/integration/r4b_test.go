//go:build integration

package integration

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/channels"
	"github.com/naseri-coder/TELMIVO/internal/domains/groupbot"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	telegramtransport "github.com/naseri-coder/TELMIVO/internal/platform/telegram"
)

type r4bTransport struct {
	sends   atomic.Int64
	unknown atomic.Bool
}

func (*r4bTransport) Channel(context.Context, int64) (string, error) {
	return "کانال آزمایشی", nil
}
func (*r4bTransport) Inspect(context.Context, int64, int64) (groupbot.Member, error) {
	var m groupbot.Member
	m.Status = "administrator"
	m.Post = true
	return m, nil
}
func (*r4bTransport) Bot(context.Context, int64) (int64, groupbot.Member, error) {
	var m groupbot.Member
	m.Status = "administrator"
	m.Post = true
	return 990001, m, nil
}
func (t *r4bTransport) Send(context.Context, telegramtransport.Message) (int64, error) {
	n := t.sends.Add(1)
	if t.unknown.Load() {
		return 0, errors.New("TELEGRAM_DELIVERY_UNKNOWN")
	}
	return 7000 + n, nil
}

func TestR4BChannelConnectionAndSafePublishing(t *testing.T) {
	u := fixture(t, false)
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE subscriptions SET plan_key='PRO',previous_plan_key=NULL,expires_at=NULL WHERE workspace_id=$1", u.WorkspaceID)
		return e
	})
	tr := &r4bTransport{}
	now := time.Date(2027, 2, 1, 12, 0, 0, 0, time.UTC)
	s := &channels.Service{DB: db, Authority: tr, Publisher: tr, Now: func() time.Time { return now }}
	var channel channels.Channel
	tenant(t, u, func(tx pgx.Tx) error {
		var e error
		channel, e = s.Connect(ctx, tx, u.UserID, u.WorkspaceID, "r4b-connect", -1000000000401)
		return e
	})
	if channel.Status != "CONNECTED" || channel.Title != "کانال آزمایشی" || !channel.BotCanPost {
		t.Fatal(channel)
	}

	var post channels.Post
	tenant(t, u, func(tx pgx.Tx) error {
		var e error
		post, e = s.CreatePost(ctx, tx, u.UserID, u.WorkspaceID, channel.ID, "r4b-create", channels.CreatePost{Body: "پیام زمان‌بندی‌شده", OperationKey: "r4b-first"})
		return e
	})
	if post.State != "DRAFT" || post.Revision != 1 {
		t.Fatal(post)
	}
	tenant(t, u, func(tx pgx.Tx) error {
		var e error
		post, e = s.Act(ctx, tx, u.UserID, u.WorkspaceID, channel.ID, post.ID, "r4b-publish", channels.Action{Action: "PUBLISH_NOW", ExpectedRevision: post.Revision})
		return e
	})
	if post.State != "SCHEDULED" {
		t.Fatal(post)
	}
	must(t, s.DeliverOne(ctx, u.UserID, u.WorkspaceID, post.ID))
	tenant(t, u, func(tx pgx.Tx) error {
		items, e := s.ListPosts(ctx, tx, u.UserID, u.WorkspaceID, channel.ID)
		if e != nil {
			return e
		}
		if len(items) != 1 || items[0].State != "SENT" || items[0].OutcomeCode != "DELIVERY_ACKNOWLEDGED" || items[0].TelegramMessageID == nil {
			t.Fatal(items)
		}
		return nil
	})
	if tr.sends.Load() != 1 {
		t.Fatal("unexpected send count", tr.sends.Load())
	}

	tr.unknown.Store(true)
	var uncertain channels.Post
	tenant(t, u, func(tx pgx.Tx) error {
		var e error
		at := now
		uncertain, e = s.CreatePost(ctx, tx, u.UserID, u.WorkspaceID, channel.ID, "r4b-unknown", channels.CreatePost{Body: "پیام مبهم", ScheduledAt: &at, OperationKey: "r4b-unknown"})
		return e
	})
	must(t, s.DeliverOne(ctx, u.UserID, u.WorkspaceID, uncertain.ID))
	if e := s.DeliverOne(ctx, u.UserID, u.WorkspaceID, uncertain.ID); !errors.Is(e, database.ErrConflict) {
		t.Fatal("terminal unknown was eligible for retry", e)
	}
	tenant(t, u, func(tx pgx.Tx) error {
		items, e := s.ListPosts(ctx, tx, u.UserID, u.WorkspaceID, channel.ID)
		if e != nil {
			return e
		}
		for _, item := range items {
			if item.ID == uncertain.ID && (item.State != "UNKNOWN" || item.OutcomeCode != "DELIVERY_OUTCOME_UNKNOWN") {
				t.Fatal(item)
			}
		}
		return nil
	})
	if tr.sends.Load() != 2 {
		t.Fatal("unknown delivery blindly retried", tr.sends.Load())
	}

	tenant(t, u, func(tx pgx.Tx) error {
		var e error
		channel, e = s.Disconnect(ctx, tx, u.UserID, u.WorkspaceID, channel.ID, "r4b-disconnect")
		return e
	})
	if channel.Status != "DISCONNECTED" {
		t.Fatal(channel)
	}
}

func TestR4BBrowserFixture(t *testing.T) {
	u, _ := r3aLogin(t, 98009001, false)
	tenant(t, u, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, "UPDATE subscriptions SET plan_key='PRO',previous_plan_key=NULL,expires_at=NULL WHERE workspace_id=$1", u.WorkspaceID); e != nil {
			return e
		}
		var id string
		if e := tx.QueryRow(ctx, "INSERT INTO channels(workspace_id,telegram_chat_id,title,status) VALUES($1,$2,'کانال مرورگر','CONNECTED') RETURNING id::text", u.WorkspaceID, int64(-1000000090001)).Scan(&id); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, "INSERT INTO channel_settings(workspace_id,channel_id,settings) VALUES($1,$2,$3)", u.WorkspaceID, id, map[string]any{"checked_at": time.Now().UTC(), "bot_rights": map[string]any{"status": "administrator", "can_post_messages": true}})
		return e
	})
}

func TestR4BRecoveryPreparation(t *testing.T) {
	u := fixture(t, false)
	tenant(t, u, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, "UPDATE subscriptions SET plan_key='PRO',previous_plan_key=NULL,expires_at=NULL WHERE workspace_id=$1", u.WorkspaceID); e != nil {
			return e
		}
		var channel string
		if e := tx.QueryRow(ctx, "INSERT INTO channels(workspace_id,telegram_chat_id,title,status) VALUES($1,$2,'Recovery Channel','CONNECTED') RETURNING id::text", u.WorkspaceID, int64(-1000000000499)).Scan(&channel); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, "INSERT INTO channel_posts(workspace_id,channel_id,actor_id,body,state,operation_key,fingerprint) VALUES($1,$2,$3,'recovery','DRAFT','r4b-recovery',$4)", u.WorkspaceID, channel, u.UserID, strings.Repeat("a", 64))
		return e
	})
}

func TestR4BRecovered(t *testing.T) {
	store, e := database.Open(ctx, os.Getenv("TEST_RESTORE_MIGRATION_DATABASE_URL"), false)
	must(t, e)
	defer store.Pool.Close()
	var n int
	must(t, store.Pool.QueryRow(ctx, "SELECT count(*) FROM channel_posts WHERE operation_key='r4b-recovery'").Scan(&n))
	if n != 1 {
		t.Fatal("channel post restore lost", n)
	}
}
