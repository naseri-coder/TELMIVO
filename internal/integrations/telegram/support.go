package telegram

import (
	"context"
	tickets "github.com/naseri-coder/TELMIVO/internal/plugins/ticketing"
)

// SupportReply shares the existing Telegram client and never sends internal notes.
func (c *Client) SupportReply(ctx context.Context, user int64, text string, a []tickets.Attachment) (int64, string) {
	if user <= 0 || len(text) == 0 || len(text) > 4000 || !tickets.AttachmentsValid(a) {
		return 0, "DELIVERY_FAILED"
	}
	var out struct {
		ID int64 `json:"message_id"`
	}
	if e := c.call(ctx, "sendMessage", map[string]any{"chat_id": user, "text": text}, &out); e != nil {
		return 0, "DELIVERY_OUTCOME_UNKNOWN"
	}
	if out.ID <= 0 {
		return 0, "DELIVERY_OUTCOME_UNKNOWN"
	}
	for _, x := range a {
		if _, code := c.SendFile(ctx, user, x.Kind, x.FileID, x.Title); code != "DELIVERY_ACKNOWLEDGED" {
			return out.ID, "DELIVERY_OUTCOME_UNKNOWN"
		}
	}
	return out.ID, "DELIVERY_ACKNOWLEDGED"
}
