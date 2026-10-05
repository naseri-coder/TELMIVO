package contentplugins

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"

	files "github.com/naseri-coder/TELMIVO/internal/plugins/filemanager"
	"net/url"
	"strings"
)

func (s *Service) WebhookSecret(account, bot string) string {
	h := hmac.New(sha256.New, s.DigestKey)
	h.Write([]byte("telmivo-customer-webhook:" + account + ":" + bot))
	return hex.EncodeToString(h.Sum(nil))
}

var ErrRuntimeNotRunning = errors.New("customer bot runtime not running")

type RuntimeStatus struct {
	BotID       string `json:"bot_id"`
	Username    string `json:"username"`
	Status      string `json:"status"`
	IngressPath string `json:"ingress_path"`
	Connected   bool   `json:"connected"`
}

func ingressPath(account, bot string) string {
	return "/api/v1/customer-bot-updates/" + account + "/" + bot
}

func (s *Service) RuntimeStatus(ctx context.Context, actor string, scope sdk.Scope) (RuntimeStatus, error) {
	out := RuntimeStatus{BotID: scope.BotID, IngressPath: ingressPath(scope.AccountID, scope.BotID)}
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		if !database.Permission(ctx, tx, actor, scope.AccountID, "bots.manage") {
			return database.ErrForbidden
		}
		if e := tx.QueryRow(ctx, "SELECT username,status FROM customer_bots WHERE workspace_id=$1 AND id::text=$2", scope.AccountID, scope.BotID).Scan(&out.Username, &out.Status); e != nil {
			return e
		}
		out.Connected = out.Status == "RUNNING"
		return nil
	})
	return out, e
}

func (s *Service) AdmitWebhook(ctx context.Context, scope sdk.Scope) error {
	tx, e := s.DB.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if e = database.Scope(ctx, tx, scope.AccountID); e != nil {
		return e
	}
	var status string
	if e = tx.QueryRow(ctx, "SELECT status FROM customer_bots WHERE workspace_id=$1 AND id::text=$2", scope.AccountID, scope.BotID).Scan(&status); e != nil {
		return e
	}
	if status != "RUNNING" {
		return ErrRuntimeNotRunning
	}
	return nil
}

type disconnectTransport interface {
	Disconnect(context.Context) error
}

func (s *Service) Connect(ctx context.Context, actor string, scope sdk.Scope, origin, corr string) error {
	u, e := url.Parse(origin)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(s.DigestKey) < 32 {
		return database.ErrInvalid
	}
	return s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		if !database.Permission(ctx, tx, actor, scope.AccountID, "bots.manage") {
			return database.ErrForbidden
		}
		var status string
		if e := tx.QueryRow(ctx, "SELECT status FROM customer_bots WHERE workspace_id=$1 AND id::text=$2 FOR UPDATE", scope.AccountID, scope.BotID).Scan(&status); e != nil {
			return e
		}
		tr, e := s.transport(ctx, tx, scope)
		if e != nil {
			return e
		}
		name, e := tr.Connect(ctx, u.Scheme+"://"+u.Host+ingressPath(scope.AccountID, scope.BotID), s.WebhookSecret(scope.AccountID, scope.BotID))
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "UPDATE customer_bots SET username=$3,status='RUNNING' WHERE workspace_id=$1 AND id::text=$2", scope.AccountID, scope.BotID, name); e != nil {
			return e
		}
		return database.Audit(ctx, tx, actor, scope.AccountID, "bot.telegram.connect", "customer_bot", scope.BotID, corr, map[string]any{"status": status}, map[string]any{"status": "RUNNING", "username": name})
	})
}

func (s *Service) Disconnect(ctx context.Context, actor string, scope sdk.Scope, corr string) error {
	return s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		if !database.Permission(ctx, tx, actor, scope.AccountID, "bots.manage") {
			return database.ErrForbidden
		}
		var status string
		if e := tx.QueryRow(ctx, "SELECT status FROM customer_bots WHERE workspace_id=$1 AND id::text=$2 FOR UPDATE", scope.AccountID, scope.BotID).Scan(&status); e != nil {
			return e
		}
		if status == "DISABLED" {
			return nil
		}
		tr, e := s.transport(ctx, tx, scope)
		if e != nil {
			return e
		}
		d, ok := tr.(disconnectTransport)
		if !ok {
			return sdk.ErrPermission
		}
		if e = d.Disconnect(ctx); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "UPDATE customer_bots SET status='DISABLED' WHERE workspace_id=$1 AND id::text=$2", scope.AccountID, scope.BotID); e != nil {
			return e
		}
		return database.Audit(ctx, tx, actor, scope.AccountID, "bot.telegram.disconnect", "customer_bot", scope.BotID, corr, map[string]any{"status": status}, map[string]any{"status": "DISABLED"})
	})
}

type Media struct {
	FileID   string `json:"file_id"`
	UniqueID string `json:"file_unique_id"`
	Name     string `json:"file_name"`
}
type Message struct {
	From struct {
		ID       int64  `json:"id"`
		Name     string `json:"first_name"`
		Username string `json:"username"`
		Locale   string `json:"language_code"`
	} `json:"from"`
	Chat struct {
		ID   int64  `json:"id"`
		Type string `json:"type"`
	} `json:"chat"`
	Text     string  `json:"text"`
	Caption  string  `json:"caption"`
	Document *Media  `json:"document"`
	Video    *Media  `json:"video"`
	Audio    *Media  `json:"audio"`
	Photos   []Media `json:"photo"`
}
type Update struct {
	ID       int64    `json:"update_id"`
	Message  *Message `json:"message"`
	Callback *struct {
		From struct {
			ID int64 `json:"id"`
		} `json:"from"`
		Data    string   `json:"data"`
		Message *Message `json:"message"`
	} `json:"callback_query"`
}

func (s *Service) webhookActor(ctx context.Context, scope sdk.Scope) (string, error) {
	tx, e := s.DB.Pool.Begin(ctx)
	if e != nil {
		return "", e
	}
	defer tx.Rollback(ctx)
	if e = database.Scope(ctx, tx, scope.AccountID); e != nil {
		return "", e
	}
	var actor string
	e = tx.QueryRow(ctx, `SELECT m.user_id::text FROM workspace_members m JOIN role_permissions p ON p.role_key=m.role_key AND p.permission_key='bots.manage' WHERE m.workspace_id=$1 AND m.role_key='OWNER' ORDER BY m.user_id LIMIT 1`, scope.AccountID).Scan(&actor)
	return actor, e
}
func (s *Service) HandleUpdate(ctx context.Context, scope sdk.Scope, corr string, u Update) error {
	m := u.Message
	text := ""
	user := int64(0)
	if m != nil {
		user = m.From.ID
		text = m.Text
	} else if u.Callback != nil {
		m = u.Callback.Message
		user = u.Callback.From.ID
		text = u.Callback.Data
	}
	if m == nil || u.ID < 0 || user <= 0 || m.Chat.Type != "private" || m.Chat.ID != user || len(text) > 4000 {
		return database.ErrInvalid
	}
	actor, e := s.webhookActor(ctx, scope)
	if e != nil {
		return e
	}
	key := fmt.Sprintf("tg:%d", u.ID)
	var media *Media
	kind := ""
	if m.Document != nil {
		media, kind = m.Document, "document"
	} else if m.Video != nil {
		media, kind = m.Video, "video"
	} else if m.Audio != nil {
		media, kind = m.Audio, "audio"
	} else if len(m.Photos) > 0 {
		media, kind = &m.Photos[len(m.Photos)-1], "photo"
	}
	if media != nil {
		var manager string
		e = s.DB.Tenant(ctx, actor, scope.AccountID, false, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT m.user_id::text FROM telegram_accounts u JOIN workspace_members m ON m.user_id=u.user_id JOIN role_permissions p ON p.role_key=m.role_key AND p.permission_key='bots.manage' WHERE u.telegram_id=$1 AND m.workspace_id=$2`, user, scope.AccountID).Scan(&manager)
		})
		if e != nil {
			if s.Support != nil {
				return s.Support.HandleSupport(ctx, scope, corr, u)
			}
			return database.ErrForbidden
		}
		scope.PluginID = files.ID
		title := media.Name
		if title == "" {
			title = "فایل تلگرام"
		}
		f, e := s.SaveFile(ctx, manager, scope, "", corr, FileChange{File: files.File{FileID: media.FileID, UniqueID: media.UniqueID, Kind: kind, Title: title, Enabled: false}, Key: key})
		if e != nil {
			if s.Support != nil && (e == sdk.ErrState) {
				return s.Support.HandleSupport(ctx, scope, corr, u)
			}
			return e
		}
		return s.prompt(ctx, actor, scope, user, "فایل ثبت شد و غیرفعال است؛ عنوان و سیاست دسترسی را در پنل تنظیم کنید.\n"+f.Reference, nil)
	}
	ref := ""
	if strings.HasPrefix(text, "/start file_") {
		ref = strings.TrimPrefix(text, "/start file_")
	} else if strings.HasPrefix(text, "file:") {
		ref = strings.TrimPrefix(text, "file:")
	}
	if !files.ReferencePattern.MatchString(ref) {
		if s.Support != nil && u.Message != nil && !strings.HasPrefix(text, "/start file_") {
			return s.Support.HandleSupport(ctx, scope, corr, u)
		}
		return database.ErrInvalid
	}
	scope.PluginID = files.ID
	r, e := s.Deliver(ctx, actor, scope, files.Request{Reference: ref, User: user, Key: key})
	if e != nil {
		return e
	}
	if r.Allowed || r.Replayed {
		return nil
	}
	rows := [][]map[string]string{}
	for _, c := range r.Access.Checks {
		if c.State != "MEMBER" && c.JoinURL != "" {
			rows = append(rows, []map[string]string{{"text": "باز کردن کانال", "url": c.JoinURL}})
		}
	}
	rows = append(rows, []map[string]string{{"text": "بررسی دوباره", "callback_data": "file:" + ref}})
	text = denialText(r.Code)
	for _, c := range r.Access.Checks {
		mark := "✗"
		if c.State == "MEMBER" {
			mark = "✓"
		}
		text += fmt.Sprintf("\n%s %d — %s", mark, c.Channel, c.Code)
	}
	return s.prompt(ctx, actor, scope, user, text, map[string]any{"inline_keyboard": rows})
}
func denialText(code string) string {
	switch code {
	case "DENIED_MISSING_MEMBERSHIP":
		return "عضویت کانال‌های لازم کامل نیست؛ پس از عضویت، بررسی دوباره را بزنید."
	case "MEMBERSHIP_CHECK_FAILED", "ACCESS_GATE_UNAVAILABLE", "CHECK_UNAVAILABLE":
		return "بررسی دسترسی ممکن نشد؛ دسترسی باز نشده است. مدیر باید تنظیمات و مجوز کانال را بررسی کند."
	case "FILE_DISABLED", "FILE_NOT_FOUND":
		return "فایل در دسترس نیست."
	case "QUOTA_EXCEEDED":
		return "سهمیهٔ این عملیات کافی نیست."
	}
	return "ارسال انجام نشد؛ وضعیت دسترسی و فایل را بررسی کنید."
}
func (s *Service) prompt(ctx context.Context, actor string, scope sdk.Scope, user int64, text string, markup any) error {
	return s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		if e := s.Core.ValidateExecutionTx(ctx, tx, actor, scope, "telegram.send", true); e != nil {
			return e
		}
		tr, e := s.transport(ctx, tx, scope)
		if e != nil {
			return e
		}
		return tr.Prompt(ctx, user, text, markup)
	})
}
