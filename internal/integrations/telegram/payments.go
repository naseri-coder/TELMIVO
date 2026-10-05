package telegram

import (
	"context"
	"errors"
)

func deterministicTelegramError(e error) bool {
	var api *APIError
	return errors.As(e, &api) || errors.Is(e, ErrDisabled)
}

func (c *Client) CreateStarsInvoice(ctx context.Context, title, description, payload string, amount int64) (string, string) {
	if title == "" || description == "" || payload == "" || amount <= 0 {
		return "", "CHECKOUT_REJECTED"
	}
	var link string
	e := c.call(ctx, "createInvoiceLink", map[string]any{
		"title":          title,
		"description":    description,
		"payload":        payload,
		"provider_token": "",
		"currency":       "XTR",
		"prices":         []map[string]any{{"label": title, "amount": amount}},
	}, &link)
	if e != nil {
		if deterministicTelegramError(e) {
			return "", "CHECKOUT_REJECTED"
		}
		return "", "CHECKOUT_UNKNOWN"
	}
	if link == "" {
		return "", "CHECKOUT_UNKNOWN"
	}
	return link, "CHECKOUT_CREATED"
}

func (c *Client) AnswerStarsPreCheckout(ctx context.Context, query string, ok bool) string {
	if query == "" {
		return "PRECHECKOUT_REJECTED"
	}
	payload := map[string]any{"pre_checkout_query_id": query, "ok": ok}
	if !ok {
		payload["error_message"] = "اطلاعات پرداخت با سفارش مطابقت ندارد."
	}
	var accepted bool
	e := c.call(ctx, "answerPreCheckoutQuery", payload, &accepted)
	if e != nil {
		if deterministicTelegramError(e) {
			return "PRECHECKOUT_REJECTED"
		}
		return "PRECHECKOUT_UNKNOWN"
	}
	if !accepted {
		return "PRECHECKOUT_UNKNOWN"
	}
	return "PRECHECKOUT_ACKNOWLEDGED"
}

func (c *Client) RefundStars(ctx context.Context, user int64, charge string) string {
	if user <= 0 || charge == "" {
		return "REFUND_REJECTED"
	}
	var ok bool
	e := c.call(ctx, "refundStarPayment", map[string]any{"user_id": user, "telegram_payment_charge_id": charge}, &ok)
	if e != nil {
		if deterministicTelegramError(e) {
			return "REFUND_REJECTED"
		}
		return "REFUND_UNKNOWN"
	}
	if !ok {
		return "REFUND_UNKNOWN"
	}
	return "REFUND_ACKNOWLEDGED"
}
