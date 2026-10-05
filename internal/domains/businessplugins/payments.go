package businessplugins

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/ledger"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/plugins/shop"
)

const (
	RailTelegramStars = "TELEGRAM_STARS"
	RailExternal      = "EXTERNAL"

	PaymentCreating      = "CREATING"
	PaymentAwaiting      = "AWAITING_PAYMENT"
	PaymentPaid          = "PAID"
	PaymentFailed        = "FAILED"
	PaymentUnknown       = "UNKNOWN"
	PaymentCancelled     = "CANCELLED"
	PaymentRefundPending = "REFUND_PENDING"
	PaymentRefundUnknown = "REFUND_UNKNOWN"
	PaymentRefunded      = "REFUNDED"

	OutcomeAcknowledged = "ACKNOWLEDGED"
	OutcomeRejected     = "REJECTED"
	OutcomeUnknown      = "UNKNOWN"
)

type Payment struct {
	ID                string       `json:"id"`
	OrderID           string       `json:"order_id"`
	ContactID         string       `json:"contact_id"`
	Rail              string       `json:"rail"`
	Provider          string       `json:"provider"`
	State             string       `json:"state"`
	Amount            ledger.Money `json:"amount"`
	CheckoutReference string       `json:"checkout_reference,omitempty"`
	CheckoutURL       string       `json:"checkout_url,omitempty"`
	ProviderPaymentID string       `json:"provider_payment_id,omitempty"`
	OutcomeCode       string       `json:"outcome_code,omitempty"`
	Revision          int64        `json:"revision"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
	PaidAt            *time.Time   `json:"paid_at,omitempty"`
	RefundedAt        *time.Time   `json:"refunded_at,omitempty"`
}

type CheckoutRequest struct {
	Key      string `json:"idempotency_key"`
	Rail     string `json:"rail"`
	Provider string `json:"provider,omitempty"`
}

type Refund struct {
	ID                string       `json:"id"`
	PaymentID         string       `json:"payment_id"`
	State             string       `json:"state"`
	Amount            ledger.Money `json:"amount"`
	Reason            string       `json:"reason"`
	ProviderReference string       `json:"provider_reference,omitempty"`
	OutcomeCode       string       `json:"outcome_code,omitempty"`
	Revision          int64        `json:"revision"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
	CompletedAt       *time.Time   `json:"completed_at,omitempty"`
}

type RefundRequest struct {
	Key    string `json:"idempotency_key"`
	Reason string `json:"reason"`
}

type ExternalCheckoutRequest struct {
	Key       string
	OrderID   string
	PaymentID string
	Amount    ledger.Money
}

type ExternalRefundRequest struct {
	Key               string
	OrderID           string
	PaymentID         string
	ProviderPaymentID string
	Amount            ledger.Money
	Reason            string
}

type ExternalReconcileRequest struct {
	PaymentID         string
	OrderID           string
	ProviderPaymentID string
	Amount            ledger.Money
}

type ExternalResult struct {
	Outcome   string
	Status    string
	Reference string
	URL       string
	Amount    ledger.Money
}

type ExternalPaymentProvider interface {
	Create(context.Context, string, ExternalCheckoutRequest) (ExternalResult, error)
	Refund(context.Context, string, ExternalRefundRequest) (ExternalResult, error)
	Reconcile(context.Context, string, ExternalReconcileRequest) (ExternalResult, error)
}

type VerifiedPayment struct {
	Provider          string
	EventKey          string
	PaymentID         string
	ExternalReference string
	Amount            ledger.Money
	PayerTelegramID   int64
}

func validMoney(m ledger.Money) bool {
	_, ok := ledger.Scales[m.Asset]
	return ok && m.AmountAtomic > 0
}

func validReconciledPayment(p Payment, result ExternalResult) bool {
	return result.Outcome == OutcomeAcknowledged && validMoney(result.Amount) &&
		result.Amount == p.Amount && strings.TrimSpace(result.Reference) != "" && len(result.Reference) <= 512
}

func validProvider(v string) bool {
	if len(v) < 1 || len(v) > 64 {
		return false
	}
	for i, r := range v {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || (i > 0 && (r == '.' || r == '_' || r == '-')) {
			continue
		}
		return false
	}
	return true
}

const paymentColumns = `id::text,order_id::text,contact_id::text,rail,provider_key,state,amount,checkout_reference,checkout_url,provider_payment_id,outcome_code,revision,created_at,updated_at,paid_at,refunded_at`

func scanPayment(row pgx.Row) (Payment, error) {
	var p Payment
	var amount []byte
	e := row.Scan(&p.ID, &p.OrderID, &p.ContactID, &p.Rail, &p.Provider, &p.State, &amount, &p.CheckoutReference, &p.CheckoutURL, &p.ProviderPaymentID, &p.OutcomeCode, &p.Revision, &p.CreatedAt, &p.UpdatedAt, &p.PaidAt, &p.RefundedAt)
	if e == nil {
		e = json.Unmarshal(amount, &p.Amount)
	}
	return p, e
}

func payment(ctx context.Context, tx pgx.Tx, scope sdk.Scope, id string, lock bool) (Payment, int64, error) {
	q := "SELECT " + paymentColumns + ",COALESCE(payer_telegram_id,0) FROM commerce_payment_intents WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3"
	if lock {
		q += " FOR UPDATE"
	}
	var p Payment
	var amount []byte
	var payer int64
	e := tx.QueryRow(ctx, q, scope.AccountID, scope.BotID, id).Scan(&p.ID, &p.OrderID, &p.ContactID, &p.Rail, &p.Provider, &p.State, &amount, &p.CheckoutReference, &p.CheckoutURL, &p.ProviderPaymentID, &p.OutcomeCode, &p.Revision, &p.CreatedAt, &p.UpdatedAt, &p.PaidAt, &p.RefundedAt, &payer)
	if e == nil {
		e = json.Unmarshal(amount, &p.Amount)
	}
	return p, payer, e
}

func payerTelegram(ctx context.Context, tx pgx.Tx, scope sdk.Scope, contact string) (int64, error) {
	var raw string
	if e := tx.QueryRow(ctx, "SELECT external_key FROM crm_identities WHERE workspace_id=$1 AND bot_id=$2 AND contact_id=$3 AND provider='telegram'", scope.AccountID, scope.BotID, contact).Scan(&raw); e != nil {
		return 0, e
	}
	n, e := strconv.ParseInt(raw, 10, 64)
	if e != nil || n <= 0 {
		return 0, database.ErrInvalid
	}
	return n, nil
}

func starsEligible(o shop.Order) bool {
	if o.Total.Asset != ledger.XTR || o.Total.AmountAtomic <= 0 || len(o.Lines) == 0 {
		return false
	}
	for _, line := range o.Lines {
		if !line.Digital {
			return false
		}
	}
	return true
}

func (s *Service) Checkout(ctx context.Context, actor string, scope sdk.Scope, orderID, correlation string, in CheckoutRequest) (Payment, error) {
	scope.PluginID = shop.ID
	var out Payment
	if !key(in.Key) || (in.Rail != RailTelegramStars && in.Rail != RailExternal) {
		return out, database.ErrInvalid
	}
	if in.Rail == RailTelegramStars {
		if in.Provider != "" && in.Provider != "telegram" {
			return out, database.ErrInvalid
		}
		in.Provider = "telegram"
	} else if !validProvider(in.Provider) || in.Provider == "telegram" {
		return out, database.ErrInvalid
	}
	fp := hash([]any{"checkout", actor, orderID, in})
	var payer int64
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		var saved string
		e := tx.QueryRow(ctx, "SELECT id::text,fingerprint FROM commerce_payment_intents WHERE workspace_id=$1 AND bot_id=$2 AND operation_key=$3", scope.AccountID, scope.BotID, in.Key).Scan(&out.ID, &saved)
		if e == nil {
			if saved != fp {
				return database.ErrConflict
			}
			out, payer, e = payment(ctx, tx, scope, out.ID, true)
			if e != nil {
				return e
			}
			if out.State == PaymentCreating {
				if _, e = tx.Exec(ctx, "UPDATE commerce_payment_intents SET state='UNKNOWN',outcome_code='INTERRUPTED_CHECKOUT',revision=revision+1,updated_at=$4 WHERE workspace_id=$1 AND bot_id=$2 AND id=$3", scope.AccountID, scope.BotID, out.ID, s.now()); e != nil {
					return e
				}
				out, payer, e = payment(ctx, tx, scope, out.ID, false)
			}
			return e
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		o, e := order(ctx, tx, scope, orderID)
		if e != nil {
			return e
		}
		if o.State != "PLACED" || !validMoney(o.Total) {
			return database.ErrConflict
		}
		payer, e = payerTelegram(ctx, tx, scope, o.Contact)
		if e != nil {
			return e
		}
		if in.Rail == RailTelegramStars && !starsEligible(o) {
			return database.ErrInvalid
		}
		var active bool
		if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM commerce_payment_intents WHERE workspace_id=$1 AND bot_id=$2 AND order_id=$3 AND state IN('CREATING','AWAITING_PAYMENT','PAID','UNKNOWN','REFUND_PENDING','REFUND_UNKNOWN'))", scope.AccountID, scope.BotID, orderID).Scan(&active); e != nil {
			return e
		}
		if active {
			return database.ErrConflict
		}
		e = tx.QueryRow(ctx, `INSERT INTO commerce_payment_intents(workspace_id,bot_id,order_id,contact_id,rail,provider_key,state,amount,operation_key,fingerprint,payer_telegram_id)
			VALUES($1,$2,$3,$4,$5,$6,'CREATING',$7,$8,$9,$10) RETURNING id::text`,
			scope.AccountID, scope.BotID, orderID, o.Contact, in.Rail, in.Provider, body(o.Total), in.Key, fp, payer).Scan(&out.ID)
		if e != nil {
			return e
		}
		out, payer, e = payment(ctx, tx, scope, out.ID, false)
		return e
	})
	if e != nil || out.State != PaymentCreating {
		return out, e
	}

	result := ExternalResult{Outcome: OutcomeRejected}
	if in.Rail == RailTelegramStars {
		if s.Content == nil {
			result = ExternalResult{Outcome: OutcomeRejected, Status: "TRANSPORT_UNAVAILABLE"}
		} else {
			url, code := s.Content.CreateStarsInvoice(ctx, actor, scope, "TELMIVO", "پرداخت سفارش "+orderID, "telmivo:"+out.ID, out.Amount.AmountAtomic)
			result.URL, result.Reference, result.Status = url, out.ID, code
			switch code {
			case "CHECKOUT_CREATED":
				result.Outcome = OutcomeAcknowledged
			case "CHECKOUT_UNKNOWN":
				result.Outcome = OutcomeUnknown
			default:
				result.Outcome = OutcomeRejected
			}
		}
	} else if s.PaymentProvider == nil {
		result = ExternalResult{Outcome: OutcomeRejected, Status: "PROVIDER_UNAVAILABLE"}
	} else {
		result, e = s.PaymentProvider.Create(ctx, in.Provider, ExternalCheckoutRequest{Key: in.Key, OrderID: orderID, PaymentID: out.ID, Amount: out.Amount})
		if e != nil {
			result = ExternalResult{Outcome: OutcomeUnknown, Status: "PROVIDER_OUTCOME_UNKNOWN"}
		}
	}

	e = s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		current, _, e := payment(ctx, tx, scope, out.ID, true)
		if e != nil {
			return e
		}
		if current.State != PaymentCreating {
			out = current
			return nil
		}
		state, code := PaymentFailed, result.Status
		switch result.Outcome {
		case OutcomeAcknowledged:
			if strings.TrimSpace(result.URL) == "" {
				state, code = PaymentUnknown, "CHECKOUT_REFERENCE_UNKNOWN"
			} else {
				state = PaymentAwaiting
				if code == "" {
					code = "CHECKOUT_CREATED"
				}
			}
		case OutcomeUnknown:
			state = PaymentUnknown
			if code == "" {
				code = "CHECKOUT_OUTCOME_UNKNOWN"
			}
		case OutcomeRejected:
			if code == "" {
				code = "CHECKOUT_REJECTED"
			}
		default:
			state, code = PaymentUnknown, "CHECKOUT_OUTCOME_UNKNOWN"
		}
		tag, e := tx.Exec(ctx, `UPDATE commerce_payment_intents SET state=$4,checkout_reference=$5,checkout_url=$6,outcome_code=$7,revision=revision+1,updated_at=$8
			WHERE workspace_id=$1 AND bot_id=$2 AND id=$3 AND revision=$9`, scope.AccountID, scope.BotID, out.ID, state, result.Reference, result.URL, code, s.now(), current.Revision)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return database.ErrConflict
		}
		out, _, e = payment(ctx, tx, scope, out.ID, false)
		if e != nil {
			return e
		}
		return database.Audit(ctx, tx, actor, scope.AccountID, "commerce.checkout", "commerce_payment", out.ID, correlation, map[string]any{"state": PaymentCreating}, map[string]any{"state": out.State, "rail": out.Rail, "provider": out.Provider})
	})
	return out, e
}

func (s *Service) paymentEvent(ctx context.Context, tx pgx.Tx, scope sdk.Scope, p Payment, key, kind, ref string, amount ledger.Money, value any) error {
	d := hash(value)
	var old string
	e := tx.QueryRow(ctx, "SELECT digest FROM commerce_payment_events WHERE payment_id=$1 AND event_key=$2", p.ID, key).Scan(&old)
	if e == nil {
		if old != d {
			return database.ErrConflict
		}
		return nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO commerce_payment_events(workspace_id,bot_id,payment_id,event_key,kind,amount,external_reference,digest,created_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, scope.AccountID, scope.BotID, p.ID, key, kind, body(amount), ref, d, s.now())
	return e
}

func (s *Service) applyVerified(ctx context.Context, actor string, scope sdk.Scope, correlation string, v VerifiedPayment) (Payment, error) {
	scope.PluginID = shop.ID
	var out Payment
	if !key(v.EventKey) || v.PaymentID == "" || !validProvider(v.Provider) || !validMoney(v.Amount) || strings.TrimSpace(v.ExternalReference) == "" || len(v.ExternalReference) > 512 {
		return out, database.ErrInvalid
	}
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		p, payer, e := payment(ctx, tx, scope, v.PaymentID, true)
		if e != nil {
			return e
		}
		out = p
		eventValue := []any{v.Provider, v.EventKey, v.PaymentID, v.ExternalReference, v.Amount, v.PayerTelegramID}
		var existing string
		e = tx.QueryRow(ctx, "SELECT digest FROM commerce_payment_events WHERE payment_id=$1 AND event_key=$2", p.ID, v.EventKey).Scan(&existing)
		if e == nil {
			if existing != hash(eventValue) {
				return database.ErrConflict
			}
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		valid := p.Provider == v.Provider && p.Amount == v.Amount && (p.Rail != RailTelegramStars || (v.PayerTelegramID == payer && v.PayerTelegramID > 0))
		if !valid {
			if e = s.paymentEvent(ctx, tx, scope, p, v.EventKey, "PAYMENT_REJECTED", v.ExternalReference, v.Amount, eventValue); e != nil {
				return e
			}
			if p.State == PaymentAwaiting {
				if _, e = tx.Exec(ctx, "UPDATE commerce_payment_intents SET state='UNKNOWN',outcome_code='PAYMENT_VERIFICATION_MISMATCH',revision=revision+1,updated_at=$4 WHERE workspace_id=$1 AND bot_id=$2 AND id=$3", scope.AccountID, scope.BotID, p.ID, s.now()); e != nil {
					return e
				}
				out, _, e = payment(ctx, tx, scope, p.ID, false)
			}
			return e
		}
		if p.State == PaymentPaid || p.State == PaymentRefundPending || p.State == PaymentRefundUnknown || p.State == PaymentRefunded {
			if p.ProviderPaymentID != v.ExternalReference {
				return database.ErrConflict
			}
			return s.paymentEvent(ctx, tx, scope, p, v.EventKey, "PAYMENT_VERIFIED", v.ExternalReference, v.Amount, eventValue)
		}
		if p.State != PaymentAwaiting && p.State != PaymentUnknown {
			return database.ErrConflict
		}
		if e = s.paymentEvent(ctx, tx, scope, p, v.EventKey, "PAYMENT_VERIFIED", v.ExternalReference, v.Amount, eventValue); e != nil {
			return e
		}
		now := s.now()
		if _, e = tx.Exec(ctx, `UPDATE commerce_payment_intents SET state='PAID',provider_payment_id=$4,outcome_code='PAYMENT_VERIFIED',paid_at=$5,revision=revision+1,updated_at=$5
			WHERE workspace_id=$1 AND bot_id=$2 AND id=$3`, scope.AccountID, scope.BotID, p.ID, v.ExternalReference, now); e != nil {
			return e
		}
		o, e := order(ctx, tx, scope, p.OrderID)
		if e != nil {
			return e
		}
		if o.State == "PLACED" {
			if _, e = tx.Exec(ctx, "UPDATE commerce_orders SET state='CONFIRMED',revision=revision+1,updated_at=$4 WHERE workspace_id=$1 AND bot_id=$2 AND id=$3", scope.AccountID, scope.BotID, o.ID, now); e != nil {
				return e
			}
			o.State = "CONFIRMED"
			o.Revision++
			if e = s.history(ctx, tx, actor, scope, o); e != nil {
				return e
			}
		}
		out, _, e = payment(ctx, tx, scope, p.ID, false)
		if e != nil {
			return e
		}
		if e = emit(ctx, tx, actor, scope, "commerce.payment.paid", p.ID, p.ContactID, map[string][]string{"order.id": {p.OrderID}, "payment.id": {p.ID}}); e != nil {
			return e
		}
		return database.Audit(ctx, tx, actor, scope.AccountID, "commerce.payment.verified", "commerce_payment", p.ID, correlation, map[string]any{"state": p.State}, map[string]any{"state": out.State})
	})
	return out, e
}

func (s *Service) VerifyExternal(ctx context.Context, actor string, scope sdk.Scope, correlation string, v VerifiedPayment) (Payment, error) {
	if v.Provider == "telegram" {
		return Payment{}, database.ErrInvalid
	}
	return s.applyVerified(ctx, actor, scope, correlation, v)
}

func (s *Service) PreCheckoutStars(ctx context.Context, actor string, scope sdk.Scope, queryID, payload, currency string, amount, user int64) error {
	scope.PluginID = shop.ID
	id := strings.TrimPrefix(payload, "telmivo:")
	ok := false
	if strings.HasPrefix(payload, "telmivo:") && currency == string(ledger.XTR) && amount > 0 && user > 0 {
		e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
			p, payer, e := payment(ctx, tx, scope, id, true)
			if e != nil {
				return e
			}
			ok = p.Rail == RailTelegramStars && p.Provider == "telegram" && (p.State == PaymentAwaiting || p.State == PaymentUnknown) && p.Amount.Asset == ledger.XTR && p.Amount.AmountAtomic == amount && payer == user
			if ok {
				value := []any{queryID, payload, currency, amount, user}
				if e = s.paymentEvent(ctx, tx, scope, p, "precheckout:"+queryID, "PRECHECKOUT_ACCEPTED", queryID, p.Amount, value); e != nil {
					return e
				}
				if p.State == PaymentUnknown {
					_, e = tx.Exec(ctx, "UPDATE commerce_payment_intents SET state='AWAITING_PAYMENT',outcome_code='PRECHECKOUT_VERIFIED',revision=revision+1,updated_at=$4 WHERE workspace_id=$1 AND bot_id=$2 AND id=$3", scope.AccountID, scope.BotID, p.ID, s.now())
				}
			}
			return e
		})
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
	}
	if s.Content == nil {
		return database.ErrForbidden
	}
	code := s.Content.AnswerStarsPreCheckout(ctx, actor, scope, queryID, ok)
	if code == "PRECHECKOUT_UNKNOWN" {
		return errors.New("PRECHECKOUT_OUTCOME_UNKNOWN")
	}
	return nil
}

func (s *Service) VerifyStars(ctx context.Context, actor string, scope sdk.Scope, correlation, payload, charge string, amount int64, currency string, user int64) (Payment, error) {
	if !strings.HasPrefix(payload, "telmivo:") || charge == "" || currency != string(ledger.XTR) || amount <= 0 || user <= 0 {
		return Payment{}, database.ErrInvalid
	}
	return s.applyVerified(ctx, actor, scope, correlation, VerifiedPayment{
		Provider: "telegram", EventKey: "telegram:" + charge, PaymentID: strings.TrimPrefix(payload, "telmivo:"),
		ExternalReference: charge, Amount: ledger.Money{Asset: ledger.XTR, AmountAtomic: amount}, PayerTelegramID: user,
	})
}

func scanRefund(row pgx.Row) (Refund, error) {
	var r Refund
	var amount []byte
	e := row.Scan(&r.ID, &r.PaymentID, &r.State, &amount, &r.Reason, &r.ProviderReference, &r.OutcomeCode, &r.Revision, &r.CreatedAt, &r.UpdatedAt, &r.CompletedAt)
	if e == nil {
		e = json.Unmarshal(amount, &r.Amount)
	}
	return r, e
}

const refundColumns = `id::text,payment_id::text,state,amount,reason,provider_reference,outcome_code,revision,created_at,updated_at,completed_at`

func (s *Service) Refund(ctx context.Context, actor string, scope sdk.Scope, paymentID, correlation string, in RefundRequest) (Refund, error) {
	scope.PluginID = shop.ID
	var out Refund
	if !key(in.Key) || strings.TrimSpace(in.Reason) == "" || len(in.Reason) > 500 {
		return out, database.ErrInvalid
	}
	in.Reason = strings.TrimSpace(in.Reason)
	fp := hash([]any{"refund", actor, paymentID, in})
	var p Payment
	var payer int64
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		var saved string
		e := tx.QueryRow(ctx, "SELECT id::text,fingerprint FROM commerce_refunds WHERE payment_id=$1 AND operation_key=$2", paymentID, in.Key).Scan(&out.ID, &saved)
		if e == nil {
			if saved != fp {
				return database.ErrConflict
			}
			out, e = scanRefund(tx.QueryRow(ctx, "SELECT "+refundColumns+" FROM commerce_refunds WHERE id=$1 FOR UPDATE", out.ID))
			if e != nil {
				return e
			}
			if out.State == "PROCESSING" {
				p, _, e = payment(ctx, tx, scope, paymentID, true)
				if e != nil {
					return e
				}
				if _, e = tx.Exec(ctx, "UPDATE commerce_refunds SET state='UNKNOWN',outcome_code='INTERRUPTED_REFUND',revision=revision+1,updated_at=$2,completed_at=$2 WHERE id=$1", out.ID, s.now()); e != nil {
					return e
				}
				if p.State == PaymentRefundPending {
					if _, e = tx.Exec(ctx, "UPDATE commerce_payment_intents SET state='REFUND_UNKNOWN',outcome_code='INTERRUPTED_REFUND',revision=revision+1,updated_at=$2 WHERE id=$1", p.ID, s.now()); e != nil {
						return e
					}
				}
				out, e = scanRefund(tx.QueryRow(ctx, "SELECT "+refundColumns+" FROM commerce_refunds WHERE id=$1", out.ID))
			}
			return e
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		p, payer, e = payment(ctx, tx, scope, paymentID, true)
		if e != nil {
			return e
		}
		if p.State != PaymentPaid {
			return database.ErrConflict
		}
		e = tx.QueryRow(ctx, `INSERT INTO commerce_refunds(workspace_id,bot_id,payment_id,state,amount,reason,operation_key,fingerprint)
			VALUES($1,$2,$3,'REQUESTED',$4,$5,$6,$7) RETURNING id::text`, scope.AccountID, scope.BotID, p.ID, body(p.Amount), in.Reason, in.Key, fp).Scan(&out.ID)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "UPDATE commerce_refunds SET state='PROCESSING',revision=revision+1,updated_at=$2 WHERE id=$1", out.ID, s.now()); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "UPDATE commerce_payment_intents SET state='REFUND_PENDING',revision=revision+1,updated_at=$2 WHERE id=$1", p.ID, s.now()); e != nil {
			return e
		}
		out, e = scanRefund(tx.QueryRow(ctx, "SELECT "+refundColumns+" FROM commerce_refunds WHERE id=$1", out.ID))
		return e
	})
	if e != nil || out.State != "PROCESSING" {
		return out, e
	}

	result := ExternalResult{Outcome: OutcomeRejected}
	if p.Rail == RailTelegramStars {
		if s.Content == nil || p.ProviderPaymentID == "" {
			result.Status = "REFUND_UNAVAILABLE"
		} else {
			code := s.Content.RefundStars(ctx, actor, scope, payer, p.ProviderPaymentID)
			result.Status = code
			switch code {
			case "REFUND_ACKNOWLEDGED":
				result.Outcome = OutcomeAcknowledged
				result.Reference = p.ProviderPaymentID
			case "REFUND_UNKNOWN":
				result.Outcome = OutcomeUnknown
			default:
				result.Outcome = OutcomeRejected
			}
		}
	} else if s.PaymentProvider == nil {
		result.Status = "PROVIDER_UNAVAILABLE"
	} else {
		result, e = s.PaymentProvider.Refund(ctx, p.Provider, ExternalRefundRequest{Key: in.Key, OrderID: p.OrderID, PaymentID: p.ID, ProviderPaymentID: p.ProviderPaymentID, Amount: p.Amount, Reason: in.Reason})
		if e != nil {
			result = ExternalResult{Outcome: OutcomeUnknown, Status: "REFUND_OUTCOME_UNKNOWN"}
		}
	}

	e = s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		current, _, e := payment(ctx, tx, scope, p.ID, true)
		if e != nil {
			return e
		}
		if current.State != PaymentRefundPending {
			return database.ErrConflict
		}
		r, e := scanRefund(tx.QueryRow(ctx, "SELECT "+refundColumns+" FROM commerce_refunds WHERE id=$1 FOR UPDATE", out.ID))
		if e != nil || r.State != "PROCESSING" {
			return e
		}
		rState, pState, code := "FAILED", PaymentPaid, result.Status
		var completed *time.Time
		now := s.now()
		switch result.Outcome {
		case OutcomeAcknowledged:
			rState, pState = "REFUNDED", PaymentRefunded
			completed = &now
			if code == "" {
				code = "REFUND_VERIFIED"
			}
		case OutcomeUnknown:
			rState, pState = "UNKNOWN", PaymentRefundUnknown
			completed = &now
			if code == "" {
				code = "REFUND_OUTCOME_UNKNOWN"
			}
		case OutcomeRejected:
			completed = &now
			if code == "" {
				code = "REFUND_REJECTED"
			}
		default:
			rState, pState, code = "UNKNOWN", PaymentRefundUnknown, "REFUND_OUTCOME_UNKNOWN"
			completed = &now
		}
		if _, e = tx.Exec(ctx, "UPDATE commerce_refunds SET state=$2,provider_reference=$3,outcome_code=$4,revision=revision+1,updated_at=$5,completed_at=$6 WHERE id=$1", r.ID, rState, result.Reference, code, now, completed); e != nil {
			return e
		}
		refundedAt := any(nil)
		if pState == PaymentRefunded {
			refundedAt = now
		}
		if _, e = tx.Exec(ctx, "UPDATE commerce_payment_intents SET state=$2,outcome_code=$3,refunded_at=$4,revision=revision+1,updated_at=$5 WHERE id=$1", p.ID, pState, code, refundedAt, now); e != nil {
			return e
		}
		kind := "REFUND_REJECTED"
		if pState == PaymentRefunded {
			kind = "REFUND_VERIFIED"
		} else if pState == PaymentRefundUnknown {
			kind = "REFUND_UNKNOWN"
		}
		if e = s.paymentEvent(ctx, tx, scope, p, "refund:"+r.ID, kind, result.Reference, p.Amount, []any{r.ID, result}); e != nil {
			return e
		}
		out, e = scanRefund(tx.QueryRow(ctx, "SELECT "+refundColumns+" FROM commerce_refunds WHERE id=$1", r.ID))
		if e != nil {
			return e
		}
		return database.Audit(ctx, tx, actor, scope.AccountID, "commerce.refund", "commerce_refund", r.ID, correlation, map[string]any{"payment_state": PaymentRefundPending}, map[string]any{"payment_state": pState, "refund_state": rState})
	})
	return out, e
}

func (s *Service) Reconcile(ctx context.Context, actor string, scope sdk.Scope, paymentID, correlation string) (Payment, error) {
	scope.PluginID = shop.ID
	var p Payment
	var e error
	e = s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		p, _, e = payment(ctx, tx, scope, paymentID, false)
		return e
	})
	if e != nil {
		return p, e
	}
	if p.Rail != RailExternal || s.PaymentProvider == nil || (p.State != PaymentUnknown && p.State != PaymentRefundUnknown) {
		return p, database.ErrConflict
	}
	result, e := s.PaymentProvider.Reconcile(ctx, p.Provider, ExternalReconcileRequest{PaymentID: p.ID, OrderID: p.OrderID, ProviderPaymentID: p.ProviderPaymentID, Amount: p.Amount})
	if e != nil {
		return p, e
	}
	if p.State == PaymentUnknown && result.Status == PaymentPaid {
		if !validReconciledPayment(p, result) {
			return p, database.ErrConflict
		}
		return s.applyVerified(ctx, actor, scope, correlation, VerifiedPayment{Provider: p.Provider, EventKey: "reconcile:paid:" + p.ID, PaymentID: p.ID, ExternalReference: result.Reference, Amount: result.Amount})
	}
	if (result.Status == PaymentPaid || result.Status == PaymentRefunded) && !validReconciledPayment(p, result) {
		return p, database.ErrConflict
	}

	e = s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		current, _, e := payment(ctx, tx, scope, p.ID, true)
		if e != nil {
			return e
		}
		next := ""
		refundState := ""
		switch {
		case current.State == PaymentUnknown && result.Status == PaymentAwaiting:
			next = PaymentAwaiting
		case current.State == PaymentUnknown && result.Status == PaymentFailed:
			next = PaymentFailed
		case current.State == PaymentUnknown && result.Status == PaymentRefunded:
			next = PaymentRefunded
		case current.State == PaymentRefundUnknown && result.Status == PaymentRefunded:
			next, refundState = PaymentRefunded, "REFUNDED"
		case current.State == PaymentRefundUnknown && result.Status == PaymentPaid:
			next, refundState = PaymentPaid, "FAILED"
		}
		if next == "" {
			return database.ErrConflict
		}
		now := s.now()
		refundedAt := any(nil)
		if next == PaymentRefunded {
			refundedAt = now
		}
		if _, e = tx.Exec(ctx, "UPDATE commerce_payment_intents SET state=$4,outcome_code='RECONCILED',refunded_at=$5,revision=revision+1,updated_at=$6 WHERE workspace_id=$1 AND bot_id=$2 AND id=$3", scope.AccountID, scope.BotID, p.ID, next, refundedAt, now); e != nil {
			return e
		}
		if refundState != "" {
			var rid string
			e = tx.QueryRow(ctx, "SELECT id::text FROM commerce_refunds WHERE workspace_id=$1 AND bot_id=$2 AND payment_id=$3 AND state='UNKNOWN' ORDER BY created_at DESC LIMIT 1 FOR UPDATE", scope.AccountID, scope.BotID, p.ID).Scan(&rid)
			if e != nil {
				return e
			}
			code := "RECONCILED_REFUND_FAILED"
			if refundState == "REFUNDED" {
				code = "RECONCILED_REFUND_VERIFIED"
			}
			if _, e = tx.Exec(ctx, "UPDATE commerce_refunds SET state=$2,outcome_code=$3,provider_reference=$4,revision=revision+1,updated_at=$5,completed_at=$5 WHERE id=$1", rid, refundState, code, result.Reference, now); e != nil {
				return e
			}
		}
		if e = s.paymentEvent(ctx, tx, scope, current, "reconcile:"+p.ID+":"+next, "CHECKOUT_RECONCILED", result.Reference, p.Amount, result); e != nil {
			return e
		}
		p, _, e = payment(ctx, tx, scope, p.ID, false)
		if e != nil {
			return e
		}
		return database.Audit(ctx, tx, actor, scope.AccountID, "commerce.payment.reconcile", "commerce_payment", p.ID, correlation, map[string]any{"state": current.State}, map[string]any{"state": p.State})
	})
	return p, e
}

func (s *Service) Payments(ctx context.Context, actor string, scope sdk.Scope, orderID string) ([]Payment, error) {
	scope.PluginID = shop.ID
	out := []Payment{}
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, "SELECT "+paymentColumns+" FROM commerce_payment_intents WHERE workspace_id=$1 AND bot_id=$2 AND ($3='' OR order_id::text=$3) ORDER BY created_at DESC,id DESC LIMIT 200", scope.AccountID, scope.BotID, orderID)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			p, e := scanPayment(rows)
			if e != nil {
				return e
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, e
}

func (s *Service) PaymentHistory(ctx context.Context, actor string, scope sdk.Scope, id string) ([]map[string]any, error) {
	scope.PluginID = shop.ID
	out := []map[string]any{}
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, "SELECT event_key,kind,amount,external_reference,created_at FROM commerce_payment_events WHERE workspace_id=$1 AND bot_id=$2 AND payment_id=$3 ORDER BY created_at,id", scope.AccountID, scope.BotID, id)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var k, kind, ref string
			var amount []byte
			var at time.Time
			if e = rows.Scan(&k, &kind, &amount, &ref, &at); e != nil {
				return e
			}
			var m ledger.Money
			_ = json.Unmarshal(amount, &m)
			out = append(out, map[string]any{"event_key": k, "kind": kind, "amount": m, "external_reference": ref, "created_at": at})
		}
		return rows.Err()
	})
	return out, e
}

func (s *Service) HandlePayment(ctx context.Context, scope sdk.Scope, correlation string, u contentplugins.Update) (bool, error) {
	if u.PreCheckout == nil && (u.Message == nil || u.Message.SuccessfulPayment == nil) {
		return false, nil
	}
	tx, e := s.DB.Pool.Begin(ctx)
	if e != nil {
		return true, e
	}
	defer tx.Rollback(ctx)
	if e = database.Scope(ctx, tx, scope.AccountID); e != nil {
		return true, e
	}
	actor, e := s.principal(ctx, tx, scope)
	if e != nil {
		return true, e
	}
	if e = tx.Rollback(ctx); e != nil {
		return true, e
	}
	scope.PluginID = shop.ID
	if u.PreCheckout != nil {
		q := u.PreCheckout
		return true, s.PreCheckoutStars(ctx, actor, scope, q.ID, q.InvoicePayload, q.Currency, q.TotalAmount, q.From.ID)
	}
	m := u.Message
	if m == nil || m.SuccessfulPayment == nil || m.From.ID <= 0 || m.Chat.Type != "private" || m.Chat.ID != m.From.ID {
		return true, database.ErrInvalid
	}
	p := m.SuccessfulPayment
	_, e = s.VerifyStars(ctx, actor, scope, correlation, p.InvoicePayload, p.TelegramPaymentChargeID, p.TotalAmount, p.Currency, m.From.ID)
	return true, e
}
