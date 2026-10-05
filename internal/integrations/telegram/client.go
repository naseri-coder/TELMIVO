package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/naseri-coder/TELMIVO/internal/domains/accessgates"
	"github.com/naseri-coder/TELMIVO/internal/platform/telegram"
	"net/http"
	"time"
)

var ErrDisabled = errors.New("Telegram transport disabled")

type Client struct {
	token   string
	http    *http.Client
	Enabled bool
}

func New(token string, enabled bool) *Client {
	return &Client{token: token, Enabled: enabled, http: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (c *Client) call(ctx context.Context, method string, payload any, result any) error {
	if !c.Enabled || c.token == "" {
		return ErrDisabled
	}
	b, e := json.Marshal(payload)
	if e != nil {
		return e
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+c.token+"/"+method, bytes.NewReader(b))
	if e != nil {
		return errors.New("Telegram request invalid")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, e := c.http.Do(req)
	if e != nil {
		return errors.New("TELEGRAM_DELIVERY_UNKNOWN")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return errors.New("TELEGRAM_DELIVERY_UNKNOWN")
	}
	var envelope struct {
		OK         bool            `json:"ok"`
		Result     json.RawMessage `json:"result"`
		ErrorCode  int             `json:"error_code"`
		Parameters struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}
	if e = json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 1<<20)).Decode(&envelope); e != nil {
		return errors.New("TELEGRAM_DELIVERY_UNKNOWN")
	}
	if !envelope.OK {
		return &APIError{Code: envelope.ErrorCode, RetryAfter: envelope.Parameters.RetryAfter}
	}
	if result != nil {
		if json.Unmarshal(envelope.Result, result) != nil {
			return errors.New("TELEGRAM_DELIVERY_UNKNOWN")
		}
		return nil
	}
	return nil
}
func (c *Client) Send(ctx context.Context, m telegram.Message) (int64, error) {
	var out struct {
		ID int64 `json:"message_id"`
	}
	e := c.call(ctx, "sendMessage", map[string]any{"chat_id": m.ChatID, "text": m.Text, "reply_markup": m.Markup}, &out)
	if e == nil && out.ID <= 0 {
		e = errors.New("TELEGRAM_DELIVERY_UNKNOWN")
	}
	return out.ID, e
}
func (c *Client) Copy(ctx context.Context, to, from, id int64) (int64, error) {
	var out struct {
		ID int64 `json:"message_id"`
	}
	e := c.call(ctx, "copyMessage", map[string]int64{"chat_id": to, "from_chat_id": from, "message_id": id}, &out)
	if e == nil && out.ID <= 0 {
		e = errors.New("TELEGRAM_DELIVERY_UNKNOWN")
	}
	return out.ID, e
}
func (c *Client) Forward(ctx context.Context, to, from, id int64) (int64, error) {
	var out struct {
		ID int64 `json:"message_id"`
	}
	e := c.call(ctx, "forwardMessage", map[string]int64{"chat_id": to, "from_chat_id": from, "message_id": id}, &out)
	if e == nil && out.ID <= 0 {
		e = errors.New("TELEGRAM_DELIVERY_UNKNOWN")
	}
	return out.ID, e
}
func (c *Client) Membership(ctx context.Context, chat, user int64) (accessgates.Membership, error) {
	var out struct {
		Status   string `json:"status"`
		IsMember bool   `json:"is_member"`
	}
	e := c.call(ctx, "getChatMember", map[string]int64{"chat_id": chat, "user_id": user}, &out)
	return accessgates.Membership{Status: out.Status, IsMember: out.IsMember, Err: e}, e
}
