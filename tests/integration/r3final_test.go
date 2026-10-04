//go:build integration

package integration

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/botfactory"
	"github.com/naseri-coder/TELMIVO/internal/domains/commerce"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/platform/queue"
)

func TestR3FinalRegistryCommercialContinuity(t *testing.T) {
	f := r3d7Fixture(t)
	factory := &botfactory.Service{DB: db, Registry: f.core.Registry, Commercial: f.s}
	catalog, e := f.s.Catalog(ctx, f.admin.UserID, f.u.WorkspaceID)
	must(t, e)
	for _, id := range f.core.Registry.IDs() {
		m, e := f.core.Registry.Manifest(id)
		must(t, e)
		v, ok := catalog.Get(commerce.PluginKind, m.Extensions.PricingPolicy)
		if !ok || m.Extensions.PricingPolicy != id {
			t.Fatal("missing canonical commercial identity", id)
		}
		for _, capability := range m.Permissions {
			if !sdk.Contains(v.Definition.Plugin.Capabilities, capability) {
				t.Fatal("commercial capability mismatch", id, capability)
			}
		}
		if _, ok := catalog.Get(commerce.QuotaKind, m.Extensions.QuotaPolicy); !ok {
			t.Fatal("missing declared quota", id)
		}
		detail, e := factory.StoreDetail(ctx, f.u.UserID, f.u.WorkspaceID, f.bot, id)
		must(t, e)
		if _, ok := detail.Store.Quotas[m.Extensions.QuotaPolicy]; !ok {
			t.Fatal("declared quota omitted from Store", id)
		}
		if len(detail.Configuration) != 0 {
			t.Fatal("Store leaked saved configuration", id)
		}
	}
}

// Exercise the same processor used by JetStream, without any real transport.
func TestR3FinalExternalEffectFence(t *testing.T) {
	u := fixture(t, false)
	var calls atomic.Int64
	makeEvent := func() queue.Envelope {
		var id string
		must(t, db.Pool.QueryRow(ctx, "SELECT gen_random_uuid()::text").Scan(&id))
		return queue.Envelope{ID: id, WorkspaceID: u.WorkspaceID, Type: "bot.menu", Payload: []byte(`{"body":"synthetic sensitive text"}`)}
	}
	unconfirmed := makeEvent()
	handler := func(c context.Context, tx pgx.Tx, e queue.Envelope) error {
		calls.Add(1) // Represents an externally acknowledged effect.
		// Simulate loss of the business/result transaction after the effect.
		return tx.Rollback(c)
	}
	if queue.Process(ctx, db, "r3-final-external", unconfirmed, true, handler) == nil {
		t.Fatal("expected failed result commit")
	}
	must(t, queue.Process(ctx, db, "r3-final-external", unconfirmed, true, handler))
	if calls.Load() != 1 {
		t.Fatal("unconfirmed external effect replayed")
	}
	var code, payload string
	must(t, db.Pool.QueryRow(ctx, "SELECT error_code,payload::text FROM dead_letter_events WHERE consumer='r3-final-external' AND event_id=$1", unconfirmed.ID).Scan(&code, &payload))
	if code != "EXTERNAL_OUTCOME_UNCONFIRMED" || strings.Contains(payload, "sensitive") {
		t.Fatal("missing or unsanitized uncertainty evidence")
	}

	// Handler errors after reservation are also fenced, even before result commit.
	failed := makeEvent()
	handler = func(context.Context, pgx.Tx, queue.Envelope) error {
		calls.Add(1)
		return errors.New("synthetic unknown external outcome")
	}
	if queue.Process(ctx, db, "r3-final-external", failed, true, handler) == nil {
		t.Fatal("expected unknown outcome")
	}
	must(t, queue.Process(ctx, db, "r3-final-external", failed, true, handler))
	if calls.Load() != 2 {
		t.Fatal("failed external attempt replayed")
	}

	// Concurrent deliveries race on the durable fence; exactly one may send.
	success := makeEvent()
	var sent atomic.Int64
	handler = func(context.Context, pgx.Tx, queue.Envelope) error { sent.Add(1); return nil }
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- queue.Process(ctx, db, "r3-final-external", success, true, handler) }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		must(t, e)
	}
	if sent.Load() != 1 {
		t.Fatal("concurrent external attempt duplicated")
	}
	var n int
	must(t, db.Pool.QueryRow(ctx, "SELECT count(*) FROM dead_letter_events WHERE consumer='r3-final-external' AND event_id=$1", success.ID).Scan(&n))
	if n != 0 {
		t.Fatal("confirmed attempt remains unconfirmed")
	}

	// No nested connection is required while the result transaction is active.
	dsn := os.Getenv("TEST_DATABASE_URL")
	if strings.Contains(dsn, "?") {
		dsn += "&"
	} else {
		dsn += "?"
	}
	dsn += "pool_max_conns=1"
	one, e := database.Open(ctx, dsn, true)
	must(t, e)
	defer one.Pool.Close()
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	must(t, queue.Process(bounded, one, "r3-final-external", makeEvent(), true, handler))

	// Pure database handlers keep their established rollback-and-retry contract.
	ordinary := makeEvent()
	var attempts int
	handler = func(context.Context, pgx.Tx, queue.Envelope) error {
		attempts++
		if attempts == 1 {
			return errors.New("safe transactional retry")
		}
		return nil
	}
	if queue.Process(ctx, db, "r3-final-transactional", ordinary, false, handler) == nil {
		t.Fatal("expected rollback")
	}
	must(t, queue.Process(ctx, db, "r3-final-transactional", ordinary, false, handler))
	if attempts != 2 {
		t.Fatal("transactional retry suppressed")
	}
}
