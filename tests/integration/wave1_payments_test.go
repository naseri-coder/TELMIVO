//go:build integration

package integration

import (
	"errors"
	"fmt"
	"testing"

	"github.com/naseri-coder/TELMIVO/internal/domains/businessplugins"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/ledger"
	"github.com/naseri-coder/TELMIVO/internal/plugins/shop"
)

func TestWave1PaymentReconciliationMatrix(t *testing.T) {
	f, _ := r4dFixture(t)
	ext := &r4dExternal{}
	ext.createErr.Store(true)
	f.biz.PaymentProvider = ext
	product, err := f.biz.SaveProduct(ctx, f.u.UserID, f.scope(shop.ID), "", businessplugins.Change[shop.Product]{
		Definition: shop.Product{Name: "Synthetic reconciliation matrix", Enabled: true, Price: ledger.Money{Asset: ledger.USD, AmountAtomic: 500}}, Key: "wave1-product",
	})
	must(t, err)
	for _, outcome := range []string{businessplugins.OutcomeUnknown, "", "UNRECOGNIZED"} {
		for _, status := range []string{businessplugins.PaymentFailed, businessplugins.PaymentAwaiting} {
			t.Run(outcome+"/"+status, func(t *testing.T) {
				key := fmt.Sprintf("wave1-order-%d", ext.creates.Load())
				order, err := f.biz.PlaceTrusted(ctx, f.u.UserID, f.scope(shop.ID), key, businessplugins.Customer{User: 10101002, Name: "Synthetic customer"}, []shop.Item{{Product: product.ID, Quantity: 1}})
				must(t, err)
				payment, err := f.biz.Checkout(ctx, f.u.UserID, f.scope(shop.ID), order.ID, key, businessplugins.CheckoutRequest{Key: key, Rail: businessplugins.RailExternal, Provider: "bridge.test"})
				must(t, err)
				if payment.State != businessplugins.PaymentUnknown {
					t.Fatal("fixture did not produce ambiguous payment", payment.State)
				}
				before := ext.creates.Load()
				beforeEvents := number(t, f.u, "SELECT count(*) FROM commerce_payment_events WHERE payment_id=$1", payment.ID)
				ext.reconcileResult = &businessplugins.ExternalResult{Outcome: outcome, Status: status, Reference: "synthetic-record", Amount: payment.Amount}
				got, err := f.biz.Reconcile(ctx, f.u.UserID, f.scope(shop.ID), payment.ID, key)
				if !errors.Is(err, database.ErrConflict) || got.State != businessplugins.PaymentUnknown {
					t.Fatal("ambiguous evidence resolved", got, err)
				}
				storedPayments, err := f.biz.Payments(ctx, f.u.UserID, f.scope(shop.ID), order.ID)
				must(t, err)
				if len(storedPayments) != 1 || storedPayments[0].ID != payment.ID {
					t.Fatal("unexpected durable payment set", storedPayments)
				}
				stored := storedPayments[0]
				if stored.State != businessplugins.PaymentUnknown || stored.Revision != payment.Revision ||
					number(t, f.u, "SELECT count(*) FROM commerce_payment_events WHERE payment_id=$1", payment.ID) != beforeEvents {
					t.Fatal("ambiguous reconciliation changed durable state/history", stored)
				}
				_, err = f.biz.Checkout(ctx, f.u.UserID, f.scope(shop.ID), order.ID, key, businessplugins.CheckoutRequest{Key: key + "-second", Rail: businessplugins.RailExternal, Provider: "bridge.test"})
				if !errors.Is(err, database.ErrConflict) || ext.creates.Load() != before || ext.refunds.Load() != 0 {
					t.Fatal("active fence released or second provider effect invoked", err)
				}
				// An exact deterministic rejection can safely resolve this checkout.
				ext.reconcileResult = &businessplugins.ExternalResult{Outcome: businessplugins.OutcomeRejected, Status: businessplugins.PaymentFailed, Reference: "synthetic-rejection", Amount: payment.Amount}
				got, err = f.biz.Reconcile(ctx, f.u.UserID, f.scope(shop.ID), payment.ID, key)
				must(t, err)
				if got.State != businessplugins.PaymentFailed || ext.creates.Load() != before || ext.refunds.Load() != 0 {
					t.Fatal("deterministic rejection not recorded safely", got)
				}
			})
		}
	}
}
