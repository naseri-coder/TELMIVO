package groupbot

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/identity"
	"github.com/naseri-coder/TELMIVO/internal/platform/auth"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"strconv"
	"strings"
)

type Chat struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
}
type Update struct {
	ID      *int64 `json:"update_id"`
	Message *struct {
		Entities []struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"entities"`
		ID          int64             `json:"message_id"`
		From        auth.TelegramUser `json:"from"`
		Chat        Chat              `json:"chat"`
		Text        string            `json:"text"`
		MigrateTo   int64             `json:"migrate_to_chat_id"`
		MigrateFrom int64             `json:"migrate_from_chat_id"`
		Reply       *struct {
			ID   int64             `json:"message_id"`
			From auth.TelegramUser `json:"from"`
		} `json:"reply_to_message"`
	} `json:"message"`
	Membership *struct {
		Chat Chat              `json:"chat"`
		From auth.TelegramUser `json:"from"`
		New  struct {
			Status string `json:"status"`
		} `json:"new_chat_member"`
	} `json:"my_chat_member"`
	MemberChange *struct {
		Chat Chat              `json:"chat"`
		From auth.TelegramUser `json:"from"`
	} `json:"chat_member"`
}

func claimableMembership(u Update) bool {
	return u.Membership != nil && u.Membership.New.Status == "administrator"
}

func (s *Service) Handle(ctx context.Context, u Update, corr string) error {
	if u.ID == nil || *u.ID < 0 {
		return auth.ErrInvalid
	}
	key := fmt.Sprintf("update:%d", *u.ID)
	if u.Message != nil && u.Message.Chat.Type == "private" {
		if u.Message.Chat.ID != u.Message.From.ID {
			return auth.ErrInvalid
		}
		fields := strings.Fields(u.Message.Text)
		if len(fields) == 0 || strings.Split(fields[0], "@")[0] != "/start" {
			// Unsupported private messages are valid Telegram updates. ACK them
			// without creating state so Telegram does not retry user input as a
			// webhook transport failure.
			return nil
		}
		update, e := identity.DecodeUpdate([]byte(fmt.Sprintf(`{"update_id":%d,"message":{"chat":{"id":%d,"type":"private"},"from":{"id":%d,"first_name":%q},"text":"/start"}}`, *u.ID, u.Message.Chat.ID, u.Message.From.ID, u.Message.From.FirstName)))
		if e != nil {
			return e
		}
		_, e = s.Identity.Start(ctx, "group-bot", s.MiniAppURL+"/groups", corr, update, nil)
		return e
	}
	var chat Chat
	var from auth.TelegramUser
	switch {
	case u.Membership != nil:
		chat = u.Membership.Chat
		from = u.Membership.From
	case u.MemberChange != nil:
		chat = u.MemberChange.Chat
		from = u.MemberChange.From
	case u.Message != nil:
		chat = u.Message.Chat
		from = u.Message.From
	default:
		return database.ErrInvalid
	}
	if chat.ID >= 0 || (chat.Type != "group" && chat.Type != "supergroup") || from.ID <= 0 {
		return auth.ErrInvalid
	}
	var ws, id string
	e := s.DB.Pool.QueryRow(ctx, "SELECT workspace_id::text,group_id::text FROM telegram_group_route($1)", chat.ID).Scan(&ws, &id)
	if e == pgx.ErrNoRows {
		if claimableMembership(u) {
			r, enterErr := s.Enter(ctx, from, key)
			if enterErr != nil {
				return enterErr
			}
			return s.DB.Tenant(ctx, r.UserID, r.WorkspaceID, false, func(tx pgx.Tx) error {
				tag, txErr := tx.Exec(ctx, "INSERT INTO telegram_updates(bot_key,update_id) VALUES('group-bot',$1) ON CONFLICT DO NOTHING", *u.ID)
				if txErr != nil || tag.RowsAffected() == 0 {
					return txErr
				}
				id, txErr = s.Claim(ctx, tx, r.UserID, r.WorkspaceID, corr, from.ID, chat.ID, chat.Title)
				if txErr != nil {
					return txErr
				}
				return database.Event(ctx, tx, r.WorkspaceID, "telmivo.jobs.notification", "group.reply", corr, map[string]any{"chat_id": chat.ID, "user_id": r.UserID, "body": "گروه به فضای کاری متصل شد. پنل گروه: " + s.MiniAppURL + "/groups/" + id, "idempotency_key": key})
			})
		}
		// Telegram sends non-actionable membership/chat updates before the bot is
		// promoted to administrator. A 5xx here causes Telegram to retry the old
		// update and can block the later administrator update that performs the
		// actual claim. Unclaimed groups therefore ACK all non-claim updates
		// without creating state.
		return nil
	}
	if e != nil {
		return e
	}
	tx, e := s.DB.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if e = database.Scope(ctx, tx, ws); e != nil {
		return e
	}
	var owner string
	if e = tx.QueryRow(ctx, "SELECT claimed_by::text FROM group_claims WHERE workspace_id=$1 AND group_id=$2 ORDER BY created_at LIMIT 1", ws, id).Scan(&owner); e != nil {
		return e
	}
	if u.Message != nil && u.Message.MigrateTo == 0 && u.Message.MigrateFrom == 0 && !strings.HasPrefix(u.Message.Text, "/") {
		links := []string{}
		if len(u.Message.Entities) > 100 {
			return database.ErrInvalid
		}
		for _, entity := range u.Message.Entities {
			if entity.Type == "text_link" {
				links = append(links, entity.URL)
			}
		}
		_, e = s.Message(ctx, tx, owner, ws, id, corr, *u.ID, Message{ID: u.Message.ID, User: from.ID, Text: u.Message.Text, Links: links})
		if e != nil {
			return e
		}
		return tx.Commit(ctx)
	}
	tag, e := tx.Exec(ctx, "INSERT INTO telegram_updates(bot_key,update_id) VALUES('group-bot',$1) ON CONFLICT DO NOTHING", *u.ID)
	if e != nil || tag.RowsAffected() == 0 {
		return e
	}
	if u.Membership != nil || u.MemberChange != nil {
		status := "CONNECTED"
		if u.Membership != nil && (u.Membership.New.Status == "left" || u.Membership.New.Status == "kicked") {
			status = "DISCONNECTED"
		}
		if u.Membership != nil {
			_, e = tx.Exec(ctx, "UPDATE groups SET status=$1 WHERE workspace_id=$2 AND id=$3", status, ws, id)
		}
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, "UPDATE group_settings SET security='{}',updated_at=now() WHERE workspace_id=$1 AND group_id=$2", ws, id)
		if e != nil {
			return e
		}
		_, e = record(ctx, tx, owner, ws, id, key, "AUTHORITY", from.ID, 0, Log, "AUTHORITY_REVALIDATION_REQUIRED", "RECORDED", corr, "")
	} else if u.Message.MigrateTo != 0 || u.Message.MigrateFrom != 0 {
		// Preserve original claim and group/trial identity; add a verified new chat alias.
		newChat := u.Message.MigrateTo
		if newChat == 0 {
			var priorWS, priorID string
			if e = tx.QueryRow(ctx, "SELECT workspace_id::text,group_id::text FROM telegram_group_route($1)", u.Message.MigrateFrom).Scan(&priorWS, &priorID); e != nil {
				return e
			}
			if priorWS != ws || priorID != id {
				return database.ErrConflict
			}
			_, e = record(ctx, tx, owner, ws, id, key, "AUTHORITY", from.ID, 0, Log, "MIGRATION_CONFIRMED", "RECORDED", corr, "")
			if e != nil {
				return e
			}
			return tx.Commit(ctx)
		}
		if newChat >= 0 {
			return database.ErrInvalid
		}
		if s.Authority == nil {
			return ErrAuthority
		}
		_, bot, err := s.Authority.Bot(ctx, newChat)
		if err != nil || !bot.Admin() {
			return ErrAuthority
		}
		if _, e = tx.Exec(ctx, "INSERT INTO group_claims(telegram_chat_id,workspace_id,group_id,claimed_by) VALUES($1,$2,$3,$4)", newChat, ws, id, owner); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, "UPDATE groups SET telegram_chat_id=$1 WHERE workspace_id=$2 AND id=$3", newChat, ws, id)
		if e != nil {
			return e
		}
		e = database.Audit(ctx, tx, owner, ws, "group.migrate", "group", id, corr, map[string]any{"chat_id": chat.ID}, map[string]any{"chat_id": newChat})
	} else {
		var actor string
		if e = tx.QueryRow(ctx, "SELECT user_id::text FROM telegram_accounts WHERE telegram_id=$1", from.ID).Scan(&actor); e != nil {
			return database.ErrForbidden
		}
		if _, _, e = s.authorize(ctx, tx, actor, ws, chat.ID); e != nil {
			return e
		}
		fields := strings.Fields(u.Message.Text)
		command := strings.Split(fields[0], "@")[0]
		v, _, err := s.read(ctx, tx, ws, id)
		if err != nil {
			return err
		}
		body := ""
		switch command {
		case "/status":
			d, err := Access(ctx, tx, actor, ws, id)
			if err != nil {
				return err
			}
			body = "وضعیت گروه: " + v.Status + "\nدسترسی: " + d.Reason
		case "/rules":
			if v.Policy.RulesEnabled {
				body = strings.Join(v.Policy.Rules, "\n")
			} else {
				body = "قوانین گروه غیرفعال است"
			}
		case "/settings":
			// A raw HTTPS link opens Telegram's ordinary in-app browser and does
			// not provide WebApp initData. The official Mini App entry for the
			// Group Bot is the private-chat /start flow, which returns the
			// WebApp button signed by Telegram.
			body = "مدیریت گروه فقط از Mini App رسمی انجام می‌شود.\nدر چت خصوصی همین ربات /start را بزنید و سپس «ورود به پنل حرفه‌ای» را انتخاب کنید."
		case "/help":
			body = "/status /rules /settings /warnings\nبرای هشدار به پیام عضو پاسخ دهید: /warn دلیل"
		case "/warnings":
			target := from.ID
			if u.Message.Reply != nil {
				target = u.Message.Reply.From.ID
			}
			n, err := activeWarnings(ctx, tx, ws, id, target)
			if err != nil {
				return err
			}
			body = "هشدارهای فعال: " + strconv.Itoa(n)
		case "/warn", "/mute", "/unmute", "/ban", "/unban", "/clearwarnings":
			if u.Message.Reply == nil {
				body = "برای اجرای دستور، روی پیام عضو Reply کنید و سپس دستور را بفرستید. مثال: /warn دلیل"
				break
			}
			if len(fields) < 2 {
				body = "برای این دستور یک دلیل کوتاه بنویسید. مثال: /warn اسپم"
				break
			}
			a := map[string]Action{"/warn": Warn, "/mute": Restrict, "/unmute": Unmute, "/ban": Ban, "/unban": Unban, "/clearwarnings": Clear}[command]
			if e = s.Moderate(ctx, tx, actor, ws, id, key, corr, u.Message.Reply.From.ID, u.Message.Reply.ID, a, strings.Join(fields[1:], " ")); e != nil {
				return e
			}
			body = "درخواست مدیریت ثبت شد"
		default:
			// Unknown user commands are valid Telegram updates. Reply with usage
			// instead of returning 4xx, otherwise Telegram retries the same
			// update and can create webhook backpressure.
			body = "دستور شناخته نشد. برای راهنما /help را بفرستید."
		}
		if body == "" {
			body = "هنوز قانونی ثبت نشده است"
		}
		e = database.Event(ctx, tx, ws, "telmivo.jobs.notification", "group.reply", corr, map[string]any{"chat_id": chat.ID, "body": body, "user_id": actor, "idempotency_key": key})
	}
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
