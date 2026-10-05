//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/businessplugins"
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/ledger"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/plugins/shop"
)

type r4dTransport struct {
	*r3d7Transport
	invoices         atomic.Int64
	refunds          atomic.Int64
	prechecks        atomic.Int64
	precheckAccepted atomic.Bool
}

func (t *r4dTransport) CreateStarsInvoice(context.Context, string, string, string, int64) (string, string) {
	t.invoices.Add(1)
	return "https://t.me/$r4d_synthetic_invoice", "CHECKOUT_CREATED"
}
func (t *r4dTransport) AnswerStarsPreCheckout(_ context.Context, _ string, ok bool) string {
	t.prechecks.Add(1)
	t.precheckAccepted.Store(ok)
	return "PRECHECKOUT_ACKNOWLEDGED"
}
func (t *r4dTransport) RefundStars(context.Context, int64, string) string {
	t.refunds.Add(1)
	return "REFUND_ACKNOWLEDGED"
}

type r4dProvider struct{ tr *r4dTransport }

func (p r4dProvider) ForBot(context.Context, pgx.Tx, sdk.Scope) (contentplugins.Transport, error) {
	return p.tr, nil
}

func r4dFixture(t *testing.T) (r3d7Data, *r4dTransport) {
	t.Helper()
	f := r3d7Fixture(t)
	tr := &r4dTransport{r3d7Transport: f.tr}
	f.biz.Content.Provider = r4dProvider{tr}
	return f, tr
}

func TestR4DStarsPreCheckoutWrongCurrencyRejected(t *testing.T) {
	f, tr := r4dFixture(t)
	d, e := f.biz.SaveProduct(ctx, f.u.UserID, f.scope(shop.ID), "", businessplugins.Change[shop.Product]{
		Definition: shop.Product{Name: "Wrong currency Stars", Enabled: true, Digital: true, Price: ledger.Money{Asset: ledger.XTR, AmountAtomic: 7}},
		Key:        "r4d-currency-product",
	})
	must(t, e)
	o, e := f.biz.PlaceTrusted(ctx, f.u.UserID, f.scope(shop.ID), "r4d-currency-order", businessplugins.Customer{User: 10101004, Name: "Currency Customer"}, []shop.Item{{Product: d.ID, Quantity: 1}})
	must(t, e)
	p, e := f.biz.Checkout(ctx, f.u.UserID, f.scope(shop.ID), o.ID, "r4d-currency-checkout", businessplugins.CheckoutRequest{Key: "r4d-currency-checkout", Rail: businessplugins.RailTelegramStars})
	must(t, e)
	if p.Amount.Asset != ledger.XTR || p.State != businessplugins.PaymentAwaiting {
		t.Fatal("fixture must expect XTR", p)
	}
	for i := 0; i < 2; i++ {
		must(t, f.biz.PreCheckoutStars(ctx, f.u.UserID, f.scope(shop.ID), "r4d-wrong-currency", "telmivo:"+p.ID, "USD", 7, 10101004))
		if tr.prechecks.Load() != int64(i+1) || tr.precheckAccepted.Load() {
			t.Fatal("wrong currency pre-checkout accepted")
		}
	}
	payments, e := f.biz.Payments(ctx, f.u.UserID, f.scope(shop.ID), o.ID)
	must(t, e)
	if len(payments) != 1 || payments[0].ID != p.ID || payments[0].State != businessplugins.PaymentAwaiting || payments[0].Revision != p.Revision || payments[0].PaidAt != nil || payments[0].ProviderPaymentID != "" {
		t.Fatal("rejected currency changed or duplicated payment", payments)
	}
	events, e := f.biz.PaymentHistory(ctx, f.u.UserID, f.scope(shop.ID), p.ID)
	must(t, e)
	if len(events) != 0 {
		t.Fatal("rejected currency produced misleading payment history", events)
	}
	history, e := f.biz.OrderHistory(ctx, f.u.UserID, f.scope(shop.ID), o.ID, "")
	must(t, e)
	if len(history) != 1 || history[0]["state"] != "PLACED" {
		t.Fatal("rejected currency confirmed or fulfilled order", history)
	}
	must(t, f.biz.PreCheckoutStars(ctx, f.u.UserID, f.scope(shop.ID), "r4d-correct-currency", "telmivo:"+p.ID, "XTR", 7, 10101004))
	if tr.prechecks.Load() != 3 || !tr.precheckAccepted.Load() || tr.invoices.Load() != 1 || tr.refunds.Load() != 0 {
		t.Fatal("correct XTR path affected or duplicate external effect")
	}
	payments, e = f.biz.Payments(ctx, f.u.UserID, f.scope(shop.ID), o.ID)
	must(t, e)
	if len(payments) != 1 || payments[0].State != businessplugins.PaymentAwaiting || payments[0].PaidAt != nil {
		t.Fatal("pre-checkout alone must never grant purchase", payments)
	}
}

func TestR4DStarsCheckoutVerificationRefundAndReplay(t *testing.T) {
	f, tr := r4dFixture(t)
	d, e := f.biz.SaveProduct(ctx, f.u.UserID, f.scope(shop.ID), "", businessplugins.Change[shop.Product]{
		Definition: shop.Product{Name: "Digital Stars", Enabled: true, Digital: true, Price: ledger.Money{Asset: ledger.XTR, AmountAtomic: 25}},
		Key:        "r4d-stars-product",
	})
	must(t, e)
	o, e := f.biz.PlaceTrusted(ctx, f.u.UserID, f.scope(shop.ID), "r4d-stars-order", businessplugins.Customer{User: 10101001, Name: "Stars Customer"}, []shop.Item{{Product: d.ID, Quantity: 2}})
	must(t, e)
	p, e := f.biz.Checkout(ctx, f.u.UserID, f.scope(shop.ID), o.ID, "r4d-stars-checkout", businessplugins.CheckoutRequest{Key: "r4d-stars-checkout", Rail: businessplugins.RailTelegramStars, Provider: "telegram"})
	must(t, e)
	if p.State != businessplugins.PaymentAwaiting || p.Amount.Asset != ledger.XTR || p.Amount.AmountAtomic != 50 || p.CheckoutURL == "" || tr.invoices.Load() != 1 {
		t.Fatal("stars checkout", p, tr.invoices.Load())
	}
	replay, e := f.biz.Checkout(ctx, f.u.UserID, f.scope(shop.ID), o.ID, "r4d-stars-checkout-replay", businessplugins.CheckoutRequest{Key: "r4d-stars-checkout", Rail: businessplugins.RailTelegramStars, Provider: "telegram"})
	must(t, e)
	if replay.ID != p.ID || tr.invoices.Load() != 1 {
		t.Fatal("checkout replay duplicated external effect", replay, tr.invoices.Load())
	}
	must(t, f.biz.PreCheckoutStars(ctx, f.u.UserID, f.scope(shop.ID), "pre-r4d-mismatch", "telmivo:"+p.ID, "XTR", 49, 10101001))
	if tr.prechecks.Load() != 1 || tr.precheckAccepted.Load() {
		t.Fatal("mismatched pre-checkout was accepted")
	}
	must(t, f.biz.PreCheckoutStars(ctx, f.u.UserID, f.scope(shop.ID), "pre-r4d-1", "telmivo:"+p.ID, "XTR", 50, 10101001))
	if tr.prechecks.Load() != 2 || !tr.precheckAccepted.Load() {
		t.Fatal("matching pre-checkout was not accepted")
	}

	u := contentplugins.Update{ID: 777}
	u.Message = &contentplugins.Message{}
	u.Message.From.ID = 10101001
	u.Message.Chat.ID = 10101001
	u.Message.Chat.Type = "private"
	u.Message.SuccessfulPayment = &contentplugins.SuccessfulPayment{Currency: "XTR", TotalAmount: 50, InvoicePayload: "telmivo:" + p.ID, TelegramPaymentChargeID: "tg-charge-r4d-1"}
	handled, e := f.biz.HandlePayment(ctx, f.scope(shop.ID), "r4d-stars-webhook", u)
	must(t, e)
	if !handled {
		t.Fatal("successful payment was not routed")
	}
	payments, e := f.biz.Payments(ctx, f.u.UserID, f.scope(shop.ID), o.ID)
	must(t, e)
	if len(payments) != 1 || payments[0].State != businessplugins.PaymentPaid || payments[0].ProviderPaymentID != "tg-charge-r4d-1" {
		t.Fatal(payments)
	}
	orders, e := f.biz.Orders(ctx, f.u.UserID, f.scope(shop.ID), "", "", "")
	must(t, e)
	var confirmed bool
	for _, got := range orders.Items {
		if got.ID == o.ID {
			confirmed = got.State == "CONFIRMED"
		}
	}
	if !confirmed {
		t.Fatal("verified payment did not confirm order")
	}
	_, e = f.biz.HandlePayment(ctx, f.scope(shop.ID), "r4d-stars-webhook-replay", u)
	must(t, e)

	refund, e := f.biz.Refund(ctx, f.u.UserID, f.scope(shop.ID), p.ID, "r4d-stars-refund", businessplugins.RefundRequest{Key: "r4d-stars-refund", Reason: "customer request"})
	must(t, e)
	if refund.State != "REFUNDED" || tr.refunds.Load() != 1 {
		t.Fatal(refund, tr.refunds.Load())
	}
	refundReplay, e := f.biz.Refund(ctx, f.u.UserID, f.scope(shop.ID), p.ID, "r4d-stars-refund-replay", businessplugins.RefundRequest{Key: "r4d-stars-refund", Reason: "customer request"})
	must(t, e)
	if refundReplay.ID != refund.ID || tr.refunds.Load() != 1 {
		t.Fatal("refund replay duplicated effect", refundReplay, tr.refunds.Load())
	}
	history, e := f.biz.PaymentHistory(ctx, f.u.UserID, f.scope(shop.ID), p.ID)
	must(t, e)
	if len(history) != 3 {
		t.Fatal("payment history", history)
	}
	e = db.Tenant(ctx, f.u.UserID, f.u.WorkspaceID, false, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE commerce_payment_events SET kind='PAYMENT_REJECTED' WHERE payment_id=$1", p.ID)
		return e
	})
	if e == nil {
		t.Fatal("immutable payment event changed")
	}
}

type r4dExternal struct {
	creates         atomic.Int64
	refunds         atomic.Int64
	createErr       atomic.Bool
	refundMode      atomic.Int64
	status          string
	reconcileResult *businessplugins.ExternalResult
	createResult    *businessplugins.ExternalResult
	refundResult    *businessplugins.ExternalResult
}

func (p *r4dExternal) Create(context.Context, string, businessplugins.ExternalCheckoutRequest) (businessplugins.ExternalResult, error) {
	p.creates.Add(1)
	if p.createResult != nil {
		return *p.createResult, nil
	}
	if p.createErr.Load() {
		return businessplugins.ExternalResult{}, errors.New("provider transport interrupted")
	}
	return businessplugins.ExternalResult{Outcome: businessplugins.OutcomeAcknowledged, Status: "CHECKOUT_CREATED", Reference: "ext-session", URL: "https://payments.example.test/session"}, nil
}
func (p *r4dExternal) Refund(context.Context, string, businessplugins.ExternalRefundRequest) (businessplugins.ExternalResult, error) {
	p.refunds.Add(1)
	if p.refundResult != nil {
		return *p.refundResult, nil
	}
	if p.refundMode.Load() == 1 {
		return businessplugins.ExternalResult{}, errors.New("refund transport interrupted")
	}
	return businessplugins.ExternalResult{Outcome: businessplugins.OutcomeAcknowledged, Status: "REFUND_VERIFIED", Reference: "ext-refund"}, nil
}
func (p *r4dExternal) Reconcile(_ context.Context, _ string, in businessplugins.ExternalReconcileRequest) (businessplugins.ExternalResult, error) {
	if p.reconcileResult != nil {
		return *p.reconcileResult, nil
	}
	return businessplugins.ExternalResult{Outcome: businessplugins.OutcomeAcknowledged, Status: p.status, Reference: "ext-reconciled", Amount: in.Amount}, nil
}

func TestR4DExternalProviderNeutralUnknownAndReconciliation(t *testing.T) {
	f, _ := r4dFixture(t)
	ext := &r4dExternal{}
	f.biz.PaymentProvider = ext
	d, e := f.biz.SaveProduct(ctx, f.u.UserID, f.scope(shop.ID), "", businessplugins.Change[shop.Product]{
		Definition: shop.Product{Name: "External Product", Enabled: true, Price: ledger.Money{Asset: ledger.USD, AmountAtomic: 500}},
		Key:        "r4d-external-product",
	})
	must(t, e)
	o, e := f.biz.PlaceTrusted(ctx, f.u.UserID, f.scope(shop.ID), "r4d-external-order", businessplugins.Customer{User: 10101002, Name: "External Customer"}, []shop.Item{{Product: d.ID, Quantity: 1}})
	must(t, e)
	ext.createErr.Store(true)
	p, e := f.biz.Checkout(ctx, f.u.UserID, f.scope(shop.ID), o.ID, "r4d-external-checkout", businessplugins.CheckoutRequest{Key: "r4d-external-checkout", Rail: businessplugins.RailExternal, Provider: "bridge.test"})
	must(t, e)
	if p.State != businessplugins.PaymentUnknown || ext.creates.Load() != 1 {
		t.Fatal("ambiguous checkout not sealed", p, ext.creates.Load())
	}
	_, e = f.biz.Checkout(ctx, f.u.UserID, f.scope(shop.ID), o.ID, "r4d-external-replay", businessplugins.CheckoutRequest{Key: "r4d-external-checkout", Rail: businessplugins.RailExternal, Provider: "bridge.test"})
	must(t, e)
	if ext.creates.Load() != 1 {
		t.Fatal("ambiguous checkout blindly retried")
	}
	for _, evidence := range []businessplugins.ExternalResult{
		{Outcome: businessplugins.OutcomeAcknowledged, Status: businessplugins.PaymentPaid, Reference: "unverified-charge"},
		{Outcome: businessplugins.OutcomeUnknown, Status: businessplugins.PaymentPaid, Reference: "ambiguous-charge", Amount: p.Amount},
		{Outcome: businessplugins.OutcomeAcknowledged, Status: businessplugins.PaymentPaid, Reference: "wrong-amount-charge", Amount: ledger.Money{Asset: ledger.USD, AmountAtomic: 499}},
	} {
		ext.reconcileResult = &evidence
		got, err := f.biz.Reconcile(ctx, f.u.UserID, f.scope(shop.ID), p.ID, "r4d-reconcile-unverified")
		if !errors.Is(err, database.ErrConflict) || got.State != businessplugins.PaymentUnknown {
			t.Fatal("incomplete reconciliation evidence accepted", got, err)
		}
	}
	ext.reconcileResult = nil
	ext.status = businessplugins.PaymentAwaiting
	p, e = f.biz.Reconcile(ctx, f.u.UserID, f.scope(shop.ID), p.ID, "r4d-reconcile-awaiting")
	must(t, e)
	if p.State != businessplugins.PaymentAwaiting {
		t.Fatal(p)
	}
	p, e = f.biz.VerifyExternal(ctx, f.u.UserID, f.scope(shop.ID), "r4d-external-paid", businessplugins.VerifiedPayment{
		Provider: "bridge.test", EventKey: "provider-event-paid", PaymentID: p.ID, ExternalReference: "provider-payment-1",
		Amount: ledger.Money{Asset: ledger.USD, AmountAtomic: 500},
	})
	must(t, e)
	if p.State != businessplugins.PaymentPaid {
		t.Fatal(p)
	}
	ext.refundMode.Store(1)
	rf, e := f.biz.Refund(ctx, f.u.UserID, f.scope(shop.ID), p.ID, "r4d-external-refund", businessplugins.RefundRequest{Key: "r4d-external-refund", Reason: "external refund"})
	must(t, e)
	if rf.State != "UNKNOWN" || ext.refunds.Load() != 1 {
		t.Fatal(rf, ext.refunds.Load())
	}
	_, e = f.biz.Refund(ctx, f.u.UserID, f.scope(shop.ID), p.ID, "r4d-external-refund-replay", businessplugins.RefundRequest{Key: "r4d-external-refund", Reason: "external refund"})
	must(t, e)
	if ext.refunds.Load() != 1 {
		t.Fatal("unknown refund blindly retried")
	}
	ext.status = businessplugins.PaymentRefunded
	p, e = f.biz.Reconcile(ctx, f.u.UserID, f.scope(shop.ID), p.ID, "r4d-reconcile-refund")
	must(t, e)
	if p.State != businessplugins.PaymentRefunded {
		t.Fatal(p)
	}
	var refundState string
	tenant(t, f.u, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT state FROM commerce_refunds WHERE payment_id=$1 AND operation_key='r4d-external-refund'", p.ID).Scan(&refundState)
	})
	if refundState != "REFUNDED" {
		t.Fatal("refund ledger not reconciled", refundState)
	}
}

func TestR4DUnrecognizedProviderOutcomesRemainUnknown(t *testing.T) {
	f, _ := r4dFixture(t)
	ext := &r4dExternal{createResult: &businessplugins.ExternalResult{Outcome: "UNRECOGNIZED"}}
	f.biz.PaymentProvider = ext
	d, e := f.biz.SaveProduct(ctx, f.u.UserID, f.scope(shop.ID), "", businessplugins.Change[shop.Product]{
		Definition: shop.Product{Name: "Ambiguous protocol", Enabled: true, Price: ledger.Money{Asset: ledger.USD, AmountAtomic: 500}},
		Key:        "r4d-protocol-product",
	})
	must(t, e)
	o, e := f.biz.PlaceTrusted(ctx, f.u.UserID, f.scope(shop.ID), "r4d-protocol-order", businessplugins.Customer{User: 10101003, Name: "Protocol Customer"}, []shop.Item{{Product: d.ID, Quantity: 1}})
	must(t, e)
	in := businessplugins.CheckoutRequest{Key: "r4d-protocol-checkout", Rail: businessplugins.RailExternal, Provider: "bridge.test"}
	p, e := f.biz.Checkout(ctx, f.u.UserID, f.scope(shop.ID), o.ID, "r4d-protocol-checkout", in)
	must(t, e)
	if p.State != businessplugins.PaymentUnknown {
		t.Fatal("unrecognized provider outcome treated as deterministic rejection", p)
	}
	_, e = f.biz.Checkout(ctx, f.u.UserID, f.scope(shop.ID), o.ID, "r4d-protocol-checkout-replay", in)
	must(t, e)
	if ext.creates.Load() != 1 {
		t.Fatal("ambiguous checkout retried")
	}
	p, e = f.biz.VerifyExternal(ctx, f.u.UserID, f.scope(shop.ID), "r4d-protocol-paid", businessplugins.VerifiedPayment{Provider: "bridge.test", EventKey: "protocol-payment", PaymentID: p.ID, ExternalReference: "protocol-charge", Amount: p.Amount})
	must(t, e)
	ext.refundResult = &businessplugins.ExternalResult{}
	r := businessplugins.RefundRequest{Key: "r4d-protocol-refund", Reason: "protocol test"}
	rf, e := f.biz.Refund(ctx, f.u.UserID, f.scope(shop.ID), p.ID, "r4d-protocol-refund", r)
	must(t, e)
	if rf.State != "UNKNOWN" {
		t.Fatal("missing refund outcome treated as deterministic rejection", rf)
	}
	_, e = f.biz.Refund(ctx, f.u.UserID, f.scope(shop.ID), p.ID, "r4d-protocol-refund-replay", r)
	must(t, e)
	if ext.refunds.Load() != 1 {
		t.Fatal("ambiguous refund retried")
	}
}

func TestR4DBrowserFixture(t *testing.T) {
	f, tr := r4dFixture(t)
	u, _ := r3aLogin(t, 98012001, false)
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE subscriptions SET plan_key=$1,expires_at=NULL WHERE workspace_id=$2", f.plan, u.WorkspaceID)
		return e
	})
	bot := r3dBot(t, u, "R4D Payments browser bot")
	scope := sdk.Scope{AccountID: u.WorkspaceID, BotID: bot}
	for _, id := range []string{"crm", shop.ID} {
		scope.PluginID = id
		i, e := f.core.Change(ctx, u.UserID, scope, sdk.Install, 0, json.RawMessage(`{}`), "r4d-browser")
		must(t, e)
		_, e = f.core.Change(ctx, u.UserID, scope, sdk.Activate, i.Revision, nil, "r4d-browser")
		must(t, e)
	}
	biz := &businessplugins.Service{DB: db, Core: f.core, Commercial: f.s, Support: f.srv, Content: &contentplugins.Service{DB: db, Core: f.core, Commercial: f.s, Provider: r4dProvider{tr}}, Clock: f.clock}
	scope.PluginID = shop.ID
	d, e := biz.SaveProduct(ctx, u.UserID, scope, "", businessplugins.Change[shop.Product]{
		Definition: shop.Product{Name: "محصول ستاره‌ای مرورگر", Enabled: true, Digital: true, Price: ledger.Money{Asset: ledger.XTR, AmountAtomic: 7}},
		Key:        "r4d-browser-product",
	})
	must(t, e)
	_, e = biz.PlaceTrusted(ctx, u.UserID, scope, "r4d-browser-order", businessplugins.Customer{User: 98012002, Name: "مشتری مرورگر"}, []shop.Item{{Product: d.ID, Quantity: 1}})
	must(t, e)
}

func TestR4DRecoveryPreparation(t *testing.T) {
	f, _ := r4dFixture(t)
	d, e := f.biz.SaveProduct(ctx, f.u.UserID, f.scope(shop.ID), "", businessplugins.Change[shop.Product]{
		Definition: shop.Product{Name: "Recovery Stars", Enabled: true, Digital: true, Price: ledger.Money{Asset: ledger.XTR, AmountAtomic: 3}},
		Key:        "r4d-recovery-product",
	})
	must(t, e)
	o, e := f.biz.PlaceTrusted(ctx, f.u.UserID, f.scope(shop.ID), "r4d-recovery-order", businessplugins.Customer{User: 10101999, Name: "Recovery Customer"}, []shop.Item{{Product: d.ID, Quantity: 1}})
	must(t, e)
	p, e := f.biz.Checkout(ctx, f.u.UserID, f.scope(shop.ID), o.ID, "r4d-recovery-checkout", businessplugins.CheckoutRequest{Key: "r4d-recovery-checkout", Rail: businessplugins.RailTelegramStars, Provider: "telegram"})
	must(t, e)
	_, e = f.biz.VerifyStars(ctx, f.u.UserID, f.scope(shop.ID), "r4d-recovery-paid", "telmivo:"+p.ID, "r4d-recovery-charge", 3, "XTR", 10101999)
	must(t, e)
}

func TestR4DRecovered(t *testing.T) {
	store, e := database.Open(ctx, os.Getenv("TEST_RESTORE_MIGRATION_DATABASE_URL"), false)
	must(t, e)
	defer store.Pool.Close()
	var state, orderState string
	var events int
	must(t, store.Pool.QueryRow(ctx, `SELECT p.state,o.state FROM commerce_payment_intents p JOIN commerce_orders o ON o.id=p.order_id AND o.workspace_id=p.workspace_id AND o.bot_id=p.bot_id WHERE p.operation_key='r4d-recovery-checkout'`).Scan(&state, &orderState))
	must(t, store.Pool.QueryRow(ctx, `SELECT count(*) FROM commerce_payment_events e JOIN commerce_payment_intents p ON p.id=e.payment_id WHERE p.operation_key='r4d-recovery-checkout'`).Scan(&events))
	if state != "PAID" || orderState != "CONFIRMED" || events != 1 {
		t.Fatal("R4D payment recovery lost", state, orderState, events)
	}
}
