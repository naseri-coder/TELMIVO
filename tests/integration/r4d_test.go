//go:build integration

package integration

import (
	"context"
	"errors"
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
	invoices atomic.Int64
	refunds  atomic.Int64
}

func (t *r4dTransport) CreateStarsInvoice(context.Context, string, string, string, int64) (string, string) {
	t.invoices.Add(1)
	return "https://t.me/$r4d_synthetic_invoice", "CHECKOUT_CREATED"
}
func (*r4dTransport) AnswerStarsPreCheckout(context.Context, string, bool) string {
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

func TestR4DStarsCheckoutVerificationRefundAndReplay(t *testing.T) {
	f, tr := r4dFixture(t)
	d, e := f.biz.SaveProduct(ctx, f.u.UserID, f.scope(shop.ID), "", businessplugins.Change[shop.Product]{
		Definition: shop.Product{Name: "Digital Stars", Enabled: true, Digital: true, Price: ledger.Money{Asset: ledger.XTR, AmountAtomic: 25}},
		Key: "r4d-stars-product",
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
	must(t, f.biz.PreCheckoutStars(ctx, f.u.UserID, f.scope(shop.ID), "pre-r4d-1", "telmivo:"+p.ID, "XTR", 50, 10101001))

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
	creates    atomic.Int64
	refunds    atomic.Int64
	createErr  atomic.Bool
	refundMode atomic.Int64
	status     string
}

func (p *r4dExternal) Create(context.Context, string, businessplugins.ExternalCheckoutRequest) (businessplugins.ExternalResult, error) {
	p.creates.Add(1)
	if p.createErr.Load() {
		return businessplugins.ExternalResult{}, errors.New("provider transport interrupted")
	}
	return businessplugins.ExternalResult{Outcome: businessplugins.OutcomeAcknowledged, Status: "CHECKOUT_CREATED", Reference: "ext-session", URL: "https://payments.example.test/session"}, nil
}
func (p *r4dExternal) Refund(context.Context, string, businessplugins.ExternalRefundRequest) (businessplugins.ExternalResult, error) {
	p.refunds.Add(1)
	if p.refundMode.Load() == 1 {
		return businessplugins.ExternalResult{}, errors.New("refund transport interrupted")
	}
	return businessplugins.ExternalResult{Outcome: businessplugins.OutcomeAcknowledged, Status: "REFUND_VERIFIED", Reference: "ext-refund"}, nil
}
func (p *r4dExternal) Reconcile(context.Context, string, businessplugins.ExternalReconcileRequest) (businessplugins.ExternalResult, error) {
	return businessplugins.ExternalResult{Status: p.status, Reference: "ext-reconciled"}, nil
}

func TestR4DExternalProviderNeutralUnknownAndReconciliation(t *testing.T) {
	f, _ := r4dFixture(t)
	ext := &r4dExternal{}
	f.biz.Payments = ext
	d, e := f.biz.SaveProduct(ctx, f.u.UserID, f.scope(shop.ID), "", businessplugins.Change[shop.Product]{
		Definition: shop.Product{Name: "External Product", Enabled: true, Price: ledger.Money{Asset: ledger.USD, AmountAtomic: 500}},
		Key: "r4d-external-product",
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
		i, e := f.core.Change(ctx, u.UserID, scope, sdk.Install, 0, nil, "r4d-browser")
		must(t, e)
		_, e = f.core.Change(ctx, u.UserID, scope, sdk.Activate, i.Revision, nil, "r4d-browser")
		must(t, e)
	}
	biz := &businessplugins.Service{DB: db, Core: f.core, Commercial: f.s, Support: f.srv, Content: &contentplugins.Service{DB: db, Core: f.core, Commercial: f.s, Provider: r4dProvider{tr}}, Clock: f.clock}
	scope.PluginID = shop.ID
	d, e := biz.SaveProduct(ctx, u.UserID, scope, "", businessplugins.Change[shop.Product]{
		Definition: shop.Product{Name: "محصول ستاره‌ای مرورگر", Enabled: true, Digital: true, Price: ledger.Money{Asset: ledger.XTR, AmountAtomic: 7}},
		Key: "r4d-browser-product",
	})
	must(t, e)
	_, e = biz.PlaceTrusted(ctx, u.UserID, scope, "r4d-browser-order", businessplugins.Customer{User: 98012002, Name: "مشتری مرورگر"}, []shop.Item{{Product: d.ID, Quantity: 1}})
	must(t, e)
}

func TestR4DRecoveryPreparation(t *testing.T) {
	f, _ := r4dFixture(t)
	d, e := f.biz.SaveProduct(ctx, f.u.UserID, f.scope(shop.ID), "", businessplugins.Change[shop.Product]{
		Definition: shop.Product{Name: "Recovery Stars", Enabled: true, Digital: true, Price: ledger.Money{Asset: ledger.XTR, AmountAtomic: 3}},
		Key: "r4d-recovery-product",
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
	store, e := database.Open(ctx, getenv("TEST_RESTORE_MIGRATION_DATABASE_URL"), false)
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
