package telegram

import (
	"context"
	"errors"
	"github.com/naseri-coder/TELMIVO/internal/platform/installationlicense"
	"time"

	transport "github.com/naseri-coder/TELMIVO/internal/platform/telegram"
)

// SendClassified sends one Telegram message and classifies the transport result
// without exposing Telegram response text or token-bearing URLs.
func (c *Client) SendClassified(ctx context.Context, message transport.Message) (int64, string, time.Duration) {
	if message.ChatID == 0 || len(message.Text) == 0 || len(message.Text) > 4000 {
		return 0, "DELIVERY_FAILED", 0
	}
	var out struct {
		ID int64 `json:"message_id"`
	}
	e := c.call(ctx, "sendMessage", messagePayload(message), &out)
	if e != nil {
		if errors.Is(e, installationlicense.ErrRestricted) {
			return 0, "DELIVERY_FAILED", 0
		}
		var api *APIError
		if errors.As(e, &api) {
			if api.Code == 429 {
				delay := api.RetryAfter
				if delay < 1 {
					delay = 60
				}
				if delay > 86400 {
					delay = 86400
				}
				return 0, "RATE_LIMITED", time.Duration(delay) * time.Second
			}
			if api.Code >= 400 && api.Code < 500 {
				return 0, "DELIVERY_FAILED", 0
			}
		}
		if errors.Is(e, ErrDisabled) {
			return 0, "DELIVERY_FAILED", 0
		}
		return 0, "DELIVERY_OUTCOME_UNKNOWN", 0
	}
	if out.ID <= 0 {
		return 0, "DELIVERY_OUTCOME_UNKNOWN", 0
	}
	return out.ID, "DELIVERY_ACKNOWLEDGED", 0
}

// BroadcastReply classifies only explicit Telegram rejection as safely retryable.
func (c *Client) BroadcastReply(ctx context.Context, user int64, text string) (int64, string, time.Duration) {
	return c.SendClassified(ctx, transport.Message{ChatID: user, Text: text})
}
