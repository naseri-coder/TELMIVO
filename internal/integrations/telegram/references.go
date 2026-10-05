package telegram

import (
	"context"
	"errors"
	"fmt"
	"github.com/naseri-coder/TELMIVO/internal/domains/groupbot"
)

// APIError carries an actionable category without Telegram's text or token-bearing URL.
type APIError struct {
	Code       int
	RetryAfter int
}

func (e *APIError) Error() string { return "Telegram request rejected" }
func (c *Client) Check(ctx context.Context, channel, user int64) (string, string) {
	_, bot, e := c.Bot(ctx, channel)
	if e == nil && !bot.Admin() {
		return "CHECK_FAILED", "BOT_NOT_AUTHORIZED"
	}
	var m struct {
		Status   string `json:"status"`
		IsMember bool   `json:"is_member"`
	}
	if e == nil {
		e = c.call(ctx, "getChatMember", map[string]int64{"chat_id": channel, "user_id": user}, &m)
	}
	if e != nil {
		var api *APIError
		if errors.As(e, &api) {
			if api.Code == 403 {
				return "CHECK_FAILED", "BOT_NOT_AUTHORIZED"
			}
			if api.Code == 400 {
				return "CHECK_FAILED", "CHANNEL_NOT_ACCESSIBLE"
			}
		}
		return "CHECK_FAILED", "CHECK_UNAVAILABLE"
	}
	switch m.Status {
	case "creator", "administrator", "member":
		return "MEMBER", ""
	case "restricted":
		if m.IsMember {
			return "MEMBER", ""
		}
		return "NOT_MEMBER", ""
	case "left", "kicked":
		return "NOT_MEMBER", ""
	}
	return "UNKNOWN", "MEMBERSHIP_UNKNOWN"
}
func (c *Client) SendFile(ctx context.Context, user int64, kind, id, title string) (int64, string) {
	methods := map[string]string{"document": "sendDocument", "video": "sendVideo", "audio": "sendAudio", "photo": "sendPhoto"}
	method, ok := methods[kind]
	if !ok || user <= 0 || id == "" {
		return 0, "INVALID_FILE_REFERENCE"
	}
	var out struct {
		ID int64 `json:"message_id"`
	}
	e := c.call(ctx, method, map[string]any{"chat_id": user, kind: id, "caption": title}, &out)
	if e != nil {
		var api *APIError
		if errors.As(e, &api) || errors.Is(e, ErrDisabled) || errors.Is(e, groupbot.ErrAuthority) {
			return 0, "DELIVERY_FAILED"
		}
		return 0, "DELIVERY_OUTCOME_UNKNOWN"
	}
	if out.ID <= 0 {
		return 0, "DELIVERY_OUTCOME_UNKNOWN"
	}
	return out.ID, "DELIVERY_ACKNOWLEDGED"
}
func (c *Client) Prompt(ctx context.Context, user int64, text string, markup any) error {
	var out struct {
		ID int64 `json:"message_id"`
	}
	return c.call(ctx, "sendMessage", map[string]any{"chat_id": user, "text": text, "reply_markup": markup}, &out)
}
func (c *Client) Connect(ctx context.Context, endpoint, secret string) (string, error) {
	var me struct {
		ID   int64  `json:"id"`
		Bot  bool   `json:"is_bot"`
		Name string `json:"username"`
	}
	if e := c.call(ctx, "getMe", map[string]any{}, &me); e != nil {
		return "", e
	}
	if me.ID <= 0 || !me.Bot || me.Name == "" {
		return "", errors.New("BOT_VERIFICATION_FAILED")
	}
	var ok bool
	if e := c.call(ctx, "setWebhook", map[string]any{"url": endpoint, "secret_token": secret, "allowed_updates": []string{"message", "callback_query", "pre_checkout_query"}}, &ok); e != nil {
		return "", e
	}
	if !ok {
		return "", fmt.Errorf("WEBHOOK_NOT_ACKNOWLEDGED")
	}
	return me.Name, nil
}

func (c *Client) Disconnect(ctx context.Context) error {
	var ok bool
	if e := c.call(ctx, "deleteWebhook", map[string]any{"drop_pending_updates": false}, &ok); e != nil {
		return e
	}
	if !ok {
		return fmt.Errorf("WEBHOOK_DELETE_NOT_ACKNOWLEDGED")
	}
	return nil
}
