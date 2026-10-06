//go:build integration

package integration

import (
	"encoding/base64"
	"errors"
	apple "github.com/naseri-coder/TELMIVO/internal/domains/appleservices"
	"github.com/naseri-coder/TELMIVO/internal/domains/businessplugins"
	"github.com/naseri-coder/TELMIVO/internal/platform/config"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/httpapi"
	"github.com/naseri-coder/TELMIVO/internal/platform/ledger"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/plugins/shop"
	"os"
	"testing"
	"time"
)

func TestPostAuditPaymentRecoveryPreparation(t *testing.T) {
	f, _ := r4dFixture(t)
	fake := &r4dExternal{}
	f.biz.PaymentProvider = fake
	product, err := f.biz.SaveProduct(ctx, f.u.UserID, f.scope(shop.ID), "", businessplugins.Change[shop.Product]{Definition: shop.Product{Name: "Recovery behavior", Enabled: true, Price: ledger.Money{Asset: ledger.USD, AmountAtomic: 500}}, Key: "post-audit-recovery-product"})
	must(t, err)
	for _, state := range []string{"paid", "refunded", "unknown"} {
		key := "post-audit-recovery-" + state
		order, err := f.biz.PlaceTrusted(ctx, f.u.UserID, f.scope(shop.ID), key, businessplugins.Customer{User: 10101002, Name: "Synthetic recovery"}, []shop.Item{{Product: product.ID, Quantity: 1}})
		must(t, err)
		fake.createErr.Store(state == "unknown")
		payment, err := f.biz.Checkout(ctx, f.u.UserID, f.scope(shop.ID), order.ID, "post-audit", businessplugins.CheckoutRequest{Key: key, Rail: businessplugins.RailExternal, Provider: "bridge.test"})
		must(t, err)
		if state != "unknown" {
			payment, err = f.biz.VerifyExternal(ctx, f.u.UserID, f.scope(shop.ID), "post-audit", businessplugins.VerifiedPayment{Provider: "bridge.test", PaymentID: payment.ID, EventKey: key + "-paid", ExternalReference: key + "-charge", Amount: payment.Amount})
			must(t, err)
		}
		if state == "refunded" {
			_, err = f.biz.Refund(ctx, f.u.UserID, f.scope(shop.ID), payment.ID, "post-audit", businessplugins.RefundRequest{Key: key + "-refund", Reason: "synthetic recovered refund"})
			must(t, err)
		}
	}
}
func TestPostAuditPaymentsRecovered(t *testing.T) {
	migration, err := database.Open(ctx, os.Getenv("TEST_RESTORE_MIGRATION_DATABASE_URL"), false)
	must(t, err)
	defer migration.Pool.Close()
	runtime, err := database.Open(ctx, os.Getenv("TEST_RESTORE_DATABASE_URL"), true)
	must(t, err)
	defer runtime.Pool.Close()
	for _, state := range []string{"paid", "refunded", "unknown"} {
		key := "post-audit-recovery-" + state
		var workspace, bot, actor, order, id, expected string
		var at time.Time
		var beforeEvents, beforeOrders int
		must(t, migration.Pool.QueryRow(ctx, "SELECT p.workspace_id::text,p.bot_id::text,w.owner_id::text,p.order_id::text,p.id::text,p.state,p.updated_at FROM commerce_payment_intents p JOIN workspaces w ON w.id=p.workspace_id WHERE p.operation_key=$1", key).Scan(&workspace, &bot, &actor, &order, &id, &expected, &at))
		must(t, migration.Pool.QueryRow(ctx, "SELECT count(*) FROM commerce_payment_events WHERE payment_id=$1", id).Scan(&beforeEvents))
		must(t, migration.Pool.QueryRow(ctx, "SELECT count(*) FROM commerce_order_history WHERE order_id=$1", order).Scan(&beforeOrders))
		api := &httpapi.API{DB: runtime, Config: config.Config{Environment: "test", Synthetic: true, ContentTestFixture: true, CredentialKey: base64.StdEncoding.EncodeToString(make([]byte, 32))}}
		service := api.BusinessService()
		clock := &r3d2Clock{at: at}
		service.Clock = clock
		service.Commercial.Clock = clock
		fake := &r4dExternal{}
		service.PaymentProvider = fake
		scope := sdk.Scope{AccountID: workspace, BotID: bot, PluginID: shop.ID}
		receipt, err := service.Checkout(ctx, actor, scope, order, "post-audit", businessplugins.CheckoutRequest{Key: key, Rail: businessplugins.RailExternal, Provider: "bridge.test"})
		must(t, err)
		if receipt.ID != id || receipt.State != expected || fake.creates.Load() != 0 {
			t.Fatal("restored checkout replay changed effect", receipt)
		}
		if state == "refunded" {
			refund, err := service.Refund(ctx, actor, scope, id, "post-audit", businessplugins.RefundRequest{Key: key + "-refund", Reason: "synthetic recovered refund"})
			must(t, err)
			if refund.State != "REFUNDED" || fake.refunds.Load() != 0 {
				t.Fatal("duplicate recovered refund", refund)
			}
		}
		if state == "paid" {
			_, err = service.VerifyExternal(ctx, actor, scope, "post-audit", businessplugins.VerifiedPayment{Provider: "bridge.test", PaymentID: id, EventKey: key + "-paid", ExternalReference: key + "-charge", Amount: receipt.Amount})
			must(t, err)
		}
		if state == "unknown" {
			fake.reconcileResult = &businessplugins.ExternalResult{Outcome: businessplugins.OutcomeUnknown, Status: businessplugins.PaymentPaid, Reference: "ambiguous", Amount: receipt.Amount}
			unknown, err := service.Reconcile(ctx, actor, scope, id, "post-audit")
			if !errors.Is(err, database.ErrConflict) || unknown.State != businessplugins.PaymentUnknown {
				t.Fatal("restore converted UNKNOWN", unknown, err)
			}
			_, err = service.Checkout(ctx, actor, scope, order, "post-audit", businessplugins.CheckoutRequest{Key: key + "-second", Rail: businessplugins.RailExternal, Provider: "bridge.test"})
			if !errors.Is(err, database.ErrConflict) || fake.creates.Load() != 0 {
				t.Fatal("restore released active fence", err)
			}
		}
		var events, orders int
		must(t, migration.Pool.QueryRow(ctx, "SELECT count(*) FROM commerce_payment_events WHERE payment_id=$1", id).Scan(&events))
		must(t, migration.Pool.QueryRow(ctx, "SELECT count(*) FROM commerce_order_history WHERE order_id=$1", order).Scan(&orders))
		if events != beforeEvents || orders != beforeOrders {
			t.Fatal("restored replay duplicated history", events, orders)
		}
	}
}
func TestPostAuditConsumerRecovered(t *testing.T) {
	migration, err := database.Open(ctx, os.Getenv("TEST_RESTORE_MIGRATION_DATABASE_URL"), false)
	must(t, err)
	defer migration.Pool.Close()
	runtime, err := database.Open(ctx, os.Getenv("TEST_RESTORE_DATABASE_URL"), true)
	must(t, err)
	defer runtime.Pool.Close()
	var actor, workspace, id, subject string
	must(t, migration.Pool.QueryRow(ctx, "SELECT actor_id::text,workspace_id::text,id::text,subject_ref FROM apple_service_operations WHERE operation_key='r5b-recovery'").Scan(&actor, &workspace, &id, &subject))
	if subject != "telmivo-user:"+actor {
		t.Fatal("recovered consumer identity drift")
	}
	var beforeUsage int64
	must(t, migration.Pool.QueryRow(ctx, "SELECT COALESCE(sum(used),0) FROM usage_counters WHERE workspace_id=$1 AND feature_key='apple_services'", workspace).Scan(&beforeUsage))
	svc := &apple.Service{DB: runtime}
	request := apple.ConsumerStart{Goal: apple.ConsumerCheckExisting, Channel: apple.ConsumerAndroidWindows, Locale: "de-DE", Region: "DE", Key: "r5b-recovery"}
	flow, err := svc.StartConsumer(ctx, actor, workspace, "post-audit", request)
	must(t, err)
	if flow.Operation.ID != id || flow.Operation.State != apple.Ready || flow.CompletionSource != "USER_ATTESTED" || flow.Guidance.IdentityVerified {
		t.Fatal("recovered consumer replay drift", flow)
	}
	request.Region = "FR"
	if _, err = svc.StartConsumer(ctx, actor, workspace, "post-audit", request); !errors.Is(err, database.ErrConflict) {
		t.Fatal("restored conflicting consumer replay", err)
	}
	if _, err = svc.ConsumerAction(ctx, actor, workspace, id, "post-audit", apple.ConsumerActionRequest{Action: apple.ConsumerConfirmCompletion, Expected: 1}); !errors.Is(err, database.ErrConflict) {
		t.Fatal("restored stale revision admitted", err)
	}
	var events int
	must(t, migration.Pool.QueryRow(ctx, "SELECT count(*) FROM apple_service_operation_events WHERE operation_id=$1", id).Scan(&events))
	var afterUsage int64
	must(t, migration.Pool.QueryRow(ctx, "SELECT COALESCE(sum(used),0) FROM usage_counters WHERE workspace_id=$1 AND feature_key='apple_services'", workspace).Scan(&afterUsage))
	if afterUsage != beforeUsage {
		t.Fatal("recovered replay consumed quota", beforeUsage, afterUsage)
	}
	if events != 6 {
		t.Fatal("restored consumer replay duplicated events", events)
	}
}

func TestPostAuditFoundationRecovered(t *testing.T) {
	migration, err := database.Open(ctx, os.Getenv("TEST_RESTORE_MIGRATION_DATABASE_URL"), false)
	must(t, err)
	defer migration.Pool.Close()
	runtime, err := database.Open(ctx, os.Getenv("TEST_RESTORE_DATABASE_URL"), true)
	must(t, err)
	defer runtime.Pool.Close()
	var actor, workspace, id string
	must(t, migration.Pool.QueryRow(ctx, "SELECT actor_id::text,workspace_id::text,id::text FROM apple_service_operations WHERE operation_key='r5a-recovery'").Scan(&actor, &workspace, &id))
	var beforeUsage int64
	must(t, migration.Pool.QueryRow(ctx, "SELECT COALESCE(sum(used),0) FROM usage_counters WHERE workspace_id=$1 AND feature_key='apple_services'", workspace).Scan(&beforeUsage))
	svc := &apple.Service{DB: runtime}
	request := apple.CreateRequest{Kind: apple.ManagedAccount, SubjectRef: "managed:recovery", Key: "r5a-recovery"}
	replay, err := svc.CreateOperation(ctx, actor, workspace, "post-audit", request)
	must(t, err)
	if replay.ID != id || replay.State != apple.AppleSyncPending || replay.Revision != 4 {
		t.Fatal("recovered Foundation replay drift", replay)
	}
	request.SubjectRef = "managed:changed"
	if _, err = svc.CreateOperation(ctx, actor, workspace, "post-audit", request); !errors.Is(err, database.ErrConflict) {
		t.Fatal("recovered conflicting Foundation replay", err)
	}
	if _, err = svc.Transition(ctx, actor, workspace, id, "post-audit", apple.TransitionRequest{State: apple.Active, Expected: 1, Reason: "stale recovery revision"}); !errors.Is(err, database.ErrConflict) {
		t.Fatal("restored stale Foundation transition", err)
	}
	var events int
	must(t, migration.Pool.QueryRow(ctx, "SELECT count(*) FROM apple_service_operation_events WHERE operation_id=$1", id).Scan(&events))
	var afterUsage int64
	must(t, migration.Pool.QueryRow(ctx, "SELECT COALESCE(sum(used),0) FROM usage_counters WHERE workspace_id=$1 AND feature_key='apple_services'", workspace).Scan(&afterUsage))
	if afterUsage != beforeUsage {
		t.Fatal("recovered replay consumed quota", beforeUsage, afterUsage)
	}
	if events != 4 {
		t.Fatal("restored Foundation replay duplicated history", events)
	}
}
