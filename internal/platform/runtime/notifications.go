package runtime

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/queue"
)

type notificationOutcome struct {
	UserID string
	Key    string
	Body   string
	Markup any
	ChatID int64
}

func notificationStatus(code string) string {
	switch code {
	case "DELIVERY_ACKNOWLEDGED":
		return "DELIVERED"
	case "DELIVERY_FAILED", "RATE_LIMITED":
		return "FAILED"
	default:
		return "UNKNOWN"
	}
}

func persistNotificationOutcome(
	ctx context.Context,
	tx pgx.Tx,
	workspace, messageKey string,
	n notificationOutcome,
	messageID int64,
	code string,
	retryAfter time.Duration,
) error {
	if workspace == "" || messageKey == "" || n.UserID == "" || n.Key == "" {
		return database.ErrInvalid
	}
	status := notificationStatus(code)
	params := map[string]any{
		"body":          n.Body,
		"markup":        n.Markup,
		"delivery_code": code,
	}
	if n.ChatID != 0 {
		params["chat_id"] = n.ChatID
	}
	if messageID > 0 {
		params["message_id"] = messageID
	}
	if retryAfter > 0 {
		params["retry_after_seconds"] = int64(retryAfter / time.Second)
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO notifications(
			workspace_id,user_id,message_key,parameters,status,idempotency_key
		) VALUES($1,$2,$3,$4,$5,$6)
		ON CONFLICT(workspace_id,idempotency_key) DO UPDATE SET
			message_key=excluded.message_key,
			parameters=excluded.parameters,
			status=excluded.status
	`, workspace, n.UserID, messageKey, params, status, n.Key)
	if err != nil {
		return err
	}
	if status == "UNKNOWN" {
		return queue.ErrExternalOutcomeUnknown
	}
	return nil
}
