//go:build integration

package integration

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/businessplugins"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/ledger"
	"github.com/naseri-coder/TELMIVO/internal/plugins/shop"
	"sync/atomic"
	"testing"
	"time"
)

type postAuditPaymentBarrier struct {
	*r4dExternal
	entered, release chan struct{}
	calls            atomic.Int64
}

func (p *postAuditPaymentBarrier) wait(ctx context.Context) error {
	p.calls.Add(1)
	select {
	case p.entered <- struct{}{}:
	default:
	}
	select {
	case <-p.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (p *postAuditPaymentBarrier) Create(ctx context.Context, key string, in businessplugins.ExternalCheckoutRequest) (businessplugins.ExternalResult, error) {
	if err := p.wait(ctx); err != nil {
		return businessplugins.ExternalResult{}, err
	}
	return p.r4dExternal.Create(ctx, key, in)
}
func (p *postAuditPaymentBarrier) Refund(ctx context.Context, key string, in businessplugins.ExternalRefundRequest) (businessplugins.ExternalResult, error) {
	if err := p.wait(ctx); err != nil {
		return businessplugins.ExternalResult{}, err
	}
	return p.r4dExternal.Refund(ctx, key, in)
}
func TestPostAuditPaymentInFlightReplay(t *testing.T) {
	f, _ := r4dFixture(t)
	p := &postAuditPaymentBarrier{r4dExternal: &r4dExternal{}, entered: make(chan struct{}, 2), release: make(chan struct{})}
	f.biz.PaymentProvider = p
	product, err := f.biz.SaveProduct(ctx, f.u.UserID, f.scope(shop.ID), "", businessplugins.Change[shop.Product]{Definition: shop.Product{Name: "Barrier", Enabled: true, Price: ledger.Money{Asset: ledger.USD, AmountAtomic: 500}}, Key: "post-audit-barrier-product"})
	must(t, err)
	order, err := f.biz.PlaceTrusted(ctx, f.u.UserID, f.scope(shop.ID), "post-audit-barrier-order", businessplugins.Customer{User: 10101002, Name: "Synthetic"}, []shop.Item{{Product: product.ID, Quantity: 1}})
	must(t, err)
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	payments := make(chan businessplugins.Payment, 1)
	request := businessplugins.CheckoutRequest{Key: "post-audit-barrier-checkout", Rail: businessplugins.RailExternal, Provider: "bridge.test"}
	go func() {
		v, e := f.biz.Checkout(callCtx, f.u.UserID, f.scope(shop.ID), order.ID, "post-audit", request)
		payments <- v
		done <- e
	}()
	select {
	case <-p.entered:
	case <-callCtx.Done():
		t.Fatal("provider never started")
	}
	receipt, err := f.biz.Checkout(callCtx, f.u.UserID, f.scope(shop.ID), order.ID, "post-audit", request)
	must(t, err)
	if receipt.State != businessplugins.PaymentCreating || p.calls.Load() != 1 {
		t.Fatal("replay stole/duplicated live effect", receipt, p.calls.Load())
	}
	close(p.release)
	must(t, <-done)
	payment := <-payments
	if payment.State != businessplugins.PaymentAwaiting || payment.CheckoutURL == "" || payment.ID != receipt.ID || p.creates.Load() != 1 {
		t.Fatal("original result lost", payment)
	}
	payment, err = f.biz.VerifyExternal(ctx, f.u.UserID, f.scope(shop.ID), "post-audit", businessplugins.VerifiedPayment{Provider: "bridge.test", PaymentID: payment.ID, EventKey: "post-audit-barrier-paid", ExternalReference: "post-audit-charge", Amount: payment.Amount})
	must(t, err)
	p.release = make(chan struct{})
	refunds := make(chan businessplugins.Refund, 1)
	refundRequest := businessplugins.RefundRequest{Key: "post-audit-barrier-refund", Reason: "synthetic refund"}
	go func() {
		v, e := f.biz.Refund(callCtx, f.u.UserID, f.scope(shop.ID), payment.ID, "post-audit", refundRequest)
		refunds <- v
		done <- e
	}()
	select {
	case <-p.entered:
	case <-callCtx.Done():
		t.Fatal("refund never started")
	}
	refundReceipt, err := f.biz.Refund(callCtx, f.u.UserID, f.scope(shop.ID), payment.ID, "post-audit", refundRequest)
	must(t, err)
	if refundReceipt.State != "PROCESSING" || p.calls.Load() != 2 {
		t.Fatal("refund replay stole/duplicated attempt", refundReceipt)
	}
	close(p.release)
	must(t, <-done)
	refunded := <-refunds
	if refunded.State != "REFUNDED" || refunded.ID != refundReceipt.ID || p.refunds.Load() != 1 {
		t.Fatal("acknowledged refund lost", refunded)
	}
}
func TestPostAuditOrderPaymentCancellation(t *testing.T) {
	f, tr := r4dFixture(t)
	product, err := f.biz.SaveProduct(ctx, f.u.UserID, f.scope(shop.ID), "", businessplugins.Change[shop.Product]{Definition: shop.Product{Name: "Cancellation", Enabled: true, Digital: true, Price: ledger.Money{Asset: ledger.XTR, AmountAtomic: 3}}, Key: "post-audit-cancel-product"})
	must(t, err)
	order, err := f.biz.PlaceTrusted(ctx, f.u.UserID, f.scope(shop.ID), "post-audit-cancel-order", businessplugins.Customer{User: 10101002, Name: "Synthetic"}, []shop.Item{{Product: product.ID, Quantity: 1}})
	must(t, err)
	payment, err := f.biz.Checkout(ctx, f.u.UserID, f.scope(shop.ID), order.ID, "post-audit", businessplugins.CheckoutRequest{Key: "post-audit-cancel-checkout", Rail: businessplugins.RailTelegramStars})
	must(t, err)
	if _, err = f.biz.OrderAction(ctx, f.u.UserID, f.scope(shop.ID), order.ID, businessplugins.Action{Key: "post-audit-cancel-action", Expected: order.Revision, Action: "CANCELLED"}); !errors.Is(err, database.ErrConflict) {
		t.Fatal("active checkout cancellation admitted", err)
	}
	// Simulate a legacy cancelled order, preserving its real late payment evidence.
	tenant(t, f.u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE commerce_orders SET state='CANCELLED',revision=revision+1 WHERE id=$1", order.ID)
		return e
	})
	must(t, f.biz.PreCheckoutStars(ctx, f.u.UserID, f.scope(shop.ID), "post-audit-precheck", "telmivo:"+payment.ID, "XTR", 3, 10101002))
	if tr.precheckAccepted.Load() {
		t.Fatal("cancelled order remained payable")
	}
	payment, err = f.biz.VerifyStars(ctx, f.u.UserID, f.scope(shop.ID), "post-audit", "telmivo:"+payment.ID, "post-audit-late-charge", 3, "XTR", 10101002)
	must(t, err)
	if payment.State != businessplugins.PaymentPaid || payment.OutcomeCode != "PAYMENT_VERIFIED_REFUND_REQUIRED" || number(t, f.u, "SELECT count(*) FROM commerce_orders WHERE id=$1 AND state='CANCELLED'", order.ID) != 1 {
		t.Fatal("late charge lost or fulfilled", payment)
	}
}
