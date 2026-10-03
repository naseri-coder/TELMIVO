package downloads

import (
	"context"
	"fmt"
	"github.com/naseri-coder/TELMIVO/internal/domains/identity"
	"github.com/naseri-coder/TELMIVO/internal/platform/auth"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"strings"
)

type Bot struct {
	Identity   *identity.Service
	Product    *Product
	MiniAppURL string
}
type BotMessage struct {
	From auth.TelegramUser `json:"from"`
	Chat struct {
		ID   int64  `json:"id"`
		Type string `json:"type"`
	} `json:"chat"`
	Text string `json:"text"`
}
type BotUpdate struct {
	ID       int64       `json:"update_id"`
	Message  *BotMessage `json:"message"`
	Callback *struct {
		From    auth.TelegramUser `json:"from"`
		Data    string            `json:"data"`
		Message *BotMessage       `json:"message"`
	} `json:"callback_query"`
}

func PersianError(code string) string {
	switch code {
	case "INVALID_URL", "UNSUPPORTED_PROVIDER":
		return "لینک پشتیبانی نمی‌شود؛ لینک عمومی YouTube، Instagram یا Facebook بفرستید."
	case "ACCESS_RESTRICTED", "MEDIA_NOT_FOUND":
		return "محتوا عمومی یا قابل دسترسی نیست."
	case "MEDIA_TOO_LARGE":
		return "حجم فایل از حد مجاز بیشتر است."
	case "FORMAT_UNAVAILABLE":
		return "فرمت دیگر در دسترس نیست؛ لینک را دوباره ارسال کنید."
	case "QUOTA_EXCEEDED", "FEATURE_DISABLED", "TRIAL_EXPIRED":
		return "دسترسی یا سهمیه شما کافی نیست."
	case "DELIVERY_OUTCOME_UNKNOWN":
		return "نتیجه ارسال مشخص نیست؛ برای جلوگیری از ارسال تکراری، خودکار تکرار نمی‌شود."
	case "DELIVERY_FAILED":
		return "ارسال به تلگرام ناموفق بود."
	default:
		return "عملیات انجام نشد؛ وضعیت و سهمیه را بررسی کنید."
	}
}
func (b *Bot) Handle(ctx context.Context, corr string, u BotUpdate) error {
	var from auth.TelegramUser
	chat := int64(0)
	text := ""
	if u.Message != nil {
		from = u.Message.From
		chat = u.Message.Chat.ID
		text = u.Message.Text
		if u.Message.Chat.Type != "private" {
			return auth.ErrInvalid
		}
	} else if u.Callback != nil && u.Callback.Message != nil {
		from = u.Callback.From
		chat = u.Callback.Message.Chat.ID
		text = u.Callback.Data
		if u.Callback.Message.Chat.Type != "private" {
			return auth.ErrInvalid
		}
	} else {
		return auth.ErrInvalid
	}
	if u.ID < 0 || from.ID <= 0 || chat != from.ID || len(text) > 2048 {
		return auth.ErrInvalid
	}
	tx, e := b.Identity.DB.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	tag, e := tx.Exec(ctx, "INSERT INTO telegram_updates(bot_key,update_id) VALUES('downloader-bot',$1) ON CONFLICT DO NOTHING", u.ID)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	ident, e := identity.Enter(ctx, tx, from, b.Identity.Admins, "DOWNLOADER_BOT", fmt.Sprint("downloader-bot:", u.ID))
	if e != nil {
		return e
	}
	if e = database.Scope(ctx, tx, ident.WorkspaceID); e != nil {
		return e
	}
	body := "لینک عمومی YouTube، Instagram یا Facebook را ارسال کنید.\n/help راهنما • /status وضعیت • /history تاریخچه"
	var markup any
	operation, err := tx.Begin(ctx)
	if err != nil {
		return err
	}
	if u.Callback != nil {
		parts := strings.Split(text, ":")
		if len(parts) != 3 || parts[0] != "f" || len(text) > 64 {
			return auth.ErrInvalid
		}
		e = b.Product.Select(ctx, operation, ident.UserID, ident.WorkspaceID, parts[1], parts[2])
		body = "فرمت انتخاب شد؛ دانلود در صف قرار گرفت."
	} else {
		fields := strings.Fields(text)
		command := ""
		if len(fields) > 0 {
			command = fields[0]
		}
		switch command {
		case "/start", "/help":
		case "/history", "/status":
			var views []View
			views, _, e = b.Product.History(ctx, operation, ident.UserID, ident.WorkspaceID, "")
			body = "هنوز دانلودی ندارید."
			if len(views) > 0 {
				body = "دانلودهای اخیر:"
				for i, v := range views {
					if i >= 5 {
						break
					}
					body += "\n" + v.ID + " — " + v.State
					if v.Error != "" {
						body += "\n" + PersianError(v.Error)
					}
				}
			}
		case "/cancel":
			if len(fields) != 2 {
				return auth.ErrInvalid
			}
			e = b.Product.Cancel(ctx, operation, ident.UserID, ident.WorkspaceID, fields[1], corr)
			body = "درخواست لغو ثبت شد؛ ارسال آغازشده قابل لغو نیست."
		default:
			var id string
			id, e = b.Product.Submit(ctx, operation, ident.UserID, ident.WorkspaceID, corr, text, fmt.Sprint("downloader-bot:", u.ID))
			body = "در حال بررسی لینک؛ شناسه: " + id
		}
	}
	if e != nil {
		operation.Rollback(ctx)
		body = PersianError(category(e, e.Error()))
	} else if e = operation.Commit(ctx); e != nil {
		return e
	}
	if b.MiniAppURL != "" {
		markup = map[string]any{"inline_keyboard": [][]map[string]any{{{"text": "وضعیت و تاریخچه", "web_app": map[string]string{"url": strings.TrimRight(b.MiniAppURL, "/") + "/downloader"}}}}}
	}
	e = database.Event(ctx, tx, ident.WorkspaceID, "telmivo.jobs.notification", "bot.menu", corr, map[string]any{"user_id": ident.UserID, "body": body, "markup": markup, "idempotency_key": fmt.Sprint("downloader-bot:", u.ID)})
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
