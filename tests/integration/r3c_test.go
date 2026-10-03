//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/downloads"
	"github.com/naseri-coder/TELMIVO/internal/domains/identity"
	"github.com/naseri-coder/TELMIVO/internal/domains/policies"
	"github.com/naseri-coder/TELMIVO/internal/integrations/media"
	"github.com/naseri-coder/TELMIVO/internal/platform/auth"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/storage"
	"github.com/naseri-coder/TELMIVO/internal/platform/testfixtures"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type r3cProvider struct {
	base     testfixtures.DownloaderProvider
	err      error
	oversize bool
}

func (p r3cProvider) Analyze(c context.Context, s media.Source) (media.Analysis, error) {
	if p.err != nil {
		return media.Analysis{}, p.err
	}
	return p.base.Analyze(c, s)
}
func (p r3cProvider) Fetch(c context.Context, s media.Source, f media.Format, w io.Writer, max int64) error {
	if p.oversize {
		_, e := media.CopyBounded(c, w, &r3cEndless{}, max)
		return e
	}
	return p.base.Fetch(c, s, f, w, max)
}

type r3cEndless struct{}

func (*r3cEndless) Read(b []byte) (int, error) {
	for i := range b {
		b[i] = 0
	}
	return len(b), nil
}

type r3cDelivery struct {
	mu     sync.Mutex
	calls  int
	chat   int64
	result media.DeliveryResult
}

func (d *r3cDelivery) DeliverMedia(c context.Context, chat int64, a media.Asset) (media.DeliveryResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls++
	d.chat = chat
	return d.result, nil
}
func r3cProduct() *downloads.Product {
	return &downloads.Product{DB: db, Synthetic: true, MaxBytes: 1 << 20}
}
func r3cWorker(t *testing.T, provider media.Adapter, delivery media.Delivery) *downloads.Worker {
	t.Helper()
	local, e := storage.NewLocal(t.TempDir(), 1<<20)
	must(t, e)
	t.Cleanup(func() { local.Close() })
	return &downloads.Worker{Product: r3cProduct(), Adapter: provider, Delivery: delivery, Storage: storage.Scoped{Provider: local}, Root: t.TempDir(), Owner: "r3c-test-worker", Timeout: time.Minute, MaxBytes: 1 << 20}
}
func r3cSubmit(t *testing.T, u identity.Result, key string) string {
	var id string
	tenant(t, u, func(tx pgx.Tx) error {
		var e error
		id, e = r3cProduct().Submit(ctx, tx, u.UserID, u.WorkspaceID, "r3c-test", "https://youtu.be/dQw4w9WgXcQ", key)
		return e
	})
	return id
}
func r3cView(t *testing.T, u identity.Result, id string) downloads.View {
	var v downloads.View
	tenant(t, u, func(tx pgx.Tx) error {
		var e error
		v, e = r3cProduct().Detail(ctx, tx, u.UserID, u.WorkspaceID, id, false)
		return e
	})
	return v
}
func r3cSelect(t *testing.T, u identity.Result, id string) {
	tenant(t, u, func(tx pgx.Tx) error {
		return r3cProduct().Select(ctx, tx, u.UserID, u.WorkspaceID, id, "fixture-compatible")
	})
}
func TestR3CIdentityBot(t *testing.T) {
	s := r3aService()
	b := downloads.Bot{Identity: s, Product: r3cProduct()}
	u := downloads.BotUpdate{ID: 98000001, Message: &downloads.BotMessage{From: auth.TelegramUser{ID: 98000001, FirstName: "دانلودر"}, Text: "/start"}}
	u.Message.Chat.ID = 98000001
	u.Message.Chat.Type = "private"
	must(t, b.Handle(ctx, "bot-first", u))
	must(t, b.Handle(ctx, "duplicate", u))
	login, _ := r3aLogin(t, 98000001, false)
	u.ID++
	must(t, b.Handle(ctx, "repeat", u))
	tenant(t, login, func(tx pgx.Tx) error {
		var n int
		must(t, tx.QueryRow(ctx, "SELECT count(*) FROM identity_entries WHERE user_id=$1 AND source='DOWNLOADER_BOT'", login.UserID).Scan(&n))
		if n != 1 {
			t.Fatal(n)
		}
		must(t, tx.QueryRow(ctx, "SELECT count(*) FROM workspaces WHERE owner_id=$1", login.UserID).Scan(&n))
		if n != 1 {
			t.Fatal(n)
		}
		return nil
	})
	if number(t, login, "SELECT count(*) FROM trial_enrollments WHERE user_id=$1 AND policy_key='global_customer_trial'", login.UserID) != 1 {
		t.Fatal("trial restarted")
	}
	for _, text := range []string{"/help", "/status", "/history", "https://evil.test/a"} {
		u.ID++
		u.Message.Text = text
		must(t, b.Handle(ctx, "commands", u))
	}
	u.ID++
	u.Message.Text = "https://youtu.be/dQw4w9WgXcQ"
	must(t, b.Handle(ctx, "submission", u))
	must(t, b.Handle(ctx, "duplicate submission", u))
	if number(t, login, "SELECT count(*) FROM download_jobs WHERE creator_id=$1", login.UserID) != 1 {
		t.Fatal("duplicate")
	}
}
func TestR3CJourney(t *testing.T) {
	u := fixture(t, false)
	id := r3cSubmit(t, u, "r3c-journey-1")
	if other := r3cSubmit(t, u, "r3c-journey-1"); other != id {
		t.Fatal("duplicate")
	}
	d := &r3cDelivery{result: media.DeliveryResult{Outcome: "DELIVERY_ACKNOWLEDGED", MessageID: 123}}
	w := r3cWorker(t, r3cProvider{}, d)
	must(t, w.RunOne(ctx, u.UserID, u.WorkspaceID, id))
	v := r3cView(t, u, id)
	if v.State != "AWAITING_FORMAT" || len(v.Formats) != 2 {
		t.Fatal(v)
	}
	foreign := fixture(t, false)
	if e := db.Tenant(ctx, foreign.UserID, foreign.WorkspaceID, false, func(tx pgx.Tx) error {
		return r3cProduct().Select(ctx, tx, foreign.UserID, foreign.WorkspaceID, id, "fixture-compatible")
	}); e == nil {
		t.Fatal("foreign format")
	}
	if e := db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error { return r3cProduct().Select(ctx, tx, u.UserID, u.WorkspaceID, id, "tampered") }); e == nil {
		t.Fatal("tampered format")
	}
	r3cSelect(t, u, id)
	must(t, w.RunOne(ctx, u.UserID, u.WorkspaceID, id))
	v = r3cView(t, u, id)
	if v.State != "COMPLETED" || v.Bytes != 2553 || v.Delivery != "DELIVERY_ACKNOWLEDGED" {
		t.Fatal(v)
	}
	var expectedChat int64
	tenant(t, u, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT telegram_id FROM telegram_accounts WHERE user_id=$1", u.UserID).Scan(&expectedChat)
	})
	if d.chat != expectedChat {
		t.Fatal("untrusted delivery target")
	}
	if d.calls != 1 {
		t.Fatal(d.calls)
	}
	if e := w.RunOne(ctx, u.UserID, u.WorkspaceID, id); e == nil {
		t.Fatal("terminal replay")
	}
	if d.calls != 1 {
		t.Fatal("second delivery")
	}
	if number(t, u, "SELECT count(*) FROM quota_reservations WHERE workspace_id=$1 AND operation_key='r3c-journey-1' AND state='COMMITTED'", u.WorkspaceID) != 1 {
		t.Fatal("settlement")
	}
	tenant(t, u, func(tx pgx.Tx) error {
		views, next, e := r3cProduct().History(ctx, tx, u.UserID, u.WorkspaceID, "")
		if e != nil {
			return e
		}
		if len(views) != 1 || next != "" {
			t.Fatal(views, next)
		}
		return nil
	})
}
func TestR3CCancelAndDeliveryFailures(t *testing.T) {
	for _, outcome := range []string{"CANCELLED", "DELIVERY_FAILED", "DELIVERY_OUTCOME_UNKNOWN", "MEDIA_TOO_LARGE", "ACCESS_RESTRICTED"} {
		t.Run(outcome, func(t *testing.T) {
			u := fixture(t, false)
			id := r3cSubmit(t, u, "r3c-failure-"+outcome)
			d := &r3cDelivery{result: media.DeliveryResult{Outcome: outcome}}
			p := r3cProvider{}
			if outcome == "MEDIA_TOO_LARGE" {
				p.oversize = true
			}
			if outcome == "ACCESS_RESTRICTED" {
				p.err = media.Restricted
			}
			w := r3cWorker(t, p, d)
			if outcome == "CANCELLED" {
				tenant(t, u, func(tx pgx.Tx) error { return r3cProduct().Cancel(ctx, tx, u.UserID, u.WorkspaceID, id, "cancel") })
				must(t, w.RunOne(ctx, u.UserID, u.WorkspaceID, id))
			} else {
				must(t, w.RunOne(ctx, u.UserID, u.WorkspaceID, id))
				if outcome != "ACCESS_RESTRICTED" {
					r3cSelect(t, u, id)
					must(t, w.RunOne(ctx, u.UserID, u.WorkspaceID, id))
				}
			}
			v := r3cView(t, u, id)
			if outcome == "CANCELLED" && v.State != "CANCELLED" || outcome != "CANCELLED" && v.State != "FAILED" {
				t.Fatal(v)
			}
			must(t, w.Cleanup(ctx))
			if _, e := os.Stat(filepath.Join(w.Root, id)); !os.IsNotExist(e) {
				t.Fatal("terminal temporary artifacts retained", e)
			}
			if outcome == "MEDIA_TOO_LARGE" && d.calls != 0 {
				t.Fatal("oversize delivered")
			}
			if number(t, u, "SELECT count(*) FROM quota_reservations WHERE workspace_id=$1 AND operation_key=$2 AND state='RESERVED'", u.WorkspaceID, "r3c-failure-"+outcome) != 0 {
				t.Fatal("leaked quota")
			}
		})
	}
}
func TestR3CConcurrency(t *testing.T) {
	u := fixture(t, true)
	v := policies.Version{Key: "r3c-concurrency-quota-" + u.WorkspaceID, Kind: "QUOTA", Feature: "downloads", Workspace: u.WorkspaceID, Enabled: true, EffectiveFrom: time.Now().Add(-time.Second), Reason: "r3c isolated quota", Definition: policies.Definition{Metric: "downloader.successes", Unit: "JOBS", Window: "ROLLING_WINDOW", WindowSeconds: 3600, Timezone: "UTC", Limit: 5, Mode: "USAGE", ReservationSeconds: 900}}
	v = savePolicy(t, u, v)
	seed := r3cSubmit(t, u, "r3c-policy-before-change")
	v.Definition.Limit = 3
	savePolicy(t, u, v)
	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted := 0
	ids := []string{seed}
	for n := 0; n < 6; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			var id string
			e := db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
				var e error
				id, e = r3cProduct().Submit(ctx, tx, u.UserID, u.WorkspaceID, "concurrent", "https://youtu.be/dQw4w9WgXcQ", fmt.Sprintf("r3c-concurrent-%d", n))
				return e
			})
			mu.Lock()
			defer mu.Unlock()
			if e == nil {
				accepted++
				ids = append(ids, id)
			}
		}(n)
	}
	wg.Wait()
	if accepted != 2 {
		t.Fatal("admitted", accepted)
	}
	id := ids[0]
	d := &r3cDelivery{result: media.DeliveryResult{Outcome: "DELIVERY_ACKNOWLEDGED", MessageID: 1}}
	w := r3cWorker(t, r3cProvider{}, d)
	must(t, w.RunOne(ctx, u.UserID, u.WorkspaceID, id))
	r3cSelect(t, u, id)
	w2 := r3cWorker(t, r3cProvider{}, d)
	w2.Owner = "second-worker"
	errs := make(chan error, 2)
	go func() { errs <- w.RunOne(ctx, u.UserID, u.WorkspaceID, id) }()
	go func() { errs <- w2.RunOne(ctx, u.UserID, u.WorkspaceID, id) }()
	a, b := <-errs, <-errs
	if (a == nil) == (b == nil) {
		t.Fatal("two workers", a, b)
	}
	if d.calls != 1 {
		t.Fatal("double delivery")
	}
	for _, job := range ids[1:] {
		tenant(t, u, func(tx pgx.Tx) error { return r3cProduct().Cancel(ctx, tx, u.UserID, u.WorkspaceID, job, "cancel") })
		must(t, w.RunOne(ctx, u.UserID, u.WorkspaceID, job))
	}
}
func TestR3CStaleFencing(t *testing.T) {
	u := fixture(t, false)
	id := r3cSubmit(t, u, "r3c-stale-1")
	w := r3cWorker(t, r3cProvider{}, &r3cDelivery{})
	must(t, w.RunOne(ctx, u.UserID, u.WorkspaceID, id))
	e := db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE download_jobs SET state='DOWNLOADING' WHERE id=$1", id)
		return e
	})
	if e == nil {
		t.Fatal("unfenced transition")
	}
	foreign := fixture(t, false)
	e = db.Tenant(ctx, foreign.UserID, foreign.WorkspaceID, false, func(tx pgx.Tx) error {
		return r3cProduct().Cancel(ctx, tx, foreign.UserID, foreign.WorkspaceID, id, "foreign")
	})
	if e == nil {
		t.Fatal("foreign cancel")
	}
	if errors.Is(e, database.ErrInvalid) {
		t.Fatal("wrong denial")
	}
}
func TestR3CBrowserFixture(t *testing.T) {
	u, _ := r3aLogin(t, 98001001, false)
	tenant(t, u, func(tx pgx.Tx) error {
		var id string
		if e := tx.QueryRow(ctx, "INSERT INTO workspaces(name,owner_id) VALUES('فضای بدون دانلود R3C',$1) RETURNING id::text", u.UserID).Scan(&id); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, "INSERT INTO workspace_members(workspace_id,user_id,role_key) VALUES($1,$2,'OWNER')", id, u.UserID); e != nil {
			return e
		}
		if e := database.Scope(ctx, tx, id); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, "INSERT INTO subscriptions(workspace_id,plan_key) VALUES($1,'FREE')", id)
		return e
	})
}
func TestR3CRecoveryPreparation(t *testing.T) {
	u, _ := r3aLogin(t, 98002001, false)
	id := r3cSubmit(t, u, "r3c-recovery-completed")
	d := &r3cDelivery{result: media.DeliveryResult{Outcome: "DELIVERY_ACKNOWLEDGED", MessageID: 1}}
	w := r3cWorker(t, r3cProvider{}, d)
	must(t, w.RunOne(ctx, u.UserID, u.WorkspaceID, id))
	r3cSelect(t, u, id)
	must(t, w.RunOne(ctx, u.UserID, u.WorkspaceID, id))
	active := r3cSubmit(t, u, "r3c-recovery-active")
	must(t, w.RunOne(ctx, u.UserID, u.WorkspaceID, active))
	r3cSelect(t, u, active)
	cancelled := r3cSubmit(t, u, "r3c-recovery-cancel")
	tenant(t, u, func(tx pgx.Tx) error {
		return r3cProduct().Cancel(ctx, tx, u.UserID, u.WorkspaceID, cancelled, "before-backup")
	})
	unknown := r3cSubmit(t, u, "r3c-recovery-unknown")
	unknownWorker := r3cWorker(t, r3cProvider{}, &r3cDelivery{result: media.DeliveryResult{Outcome: "DELIVERY_OUTCOME_UNKNOWN"}})
	must(t, unknownWorker.RunOne(ctx, u.UserID, u.WorkspaceID, unknown))
	r3cSelect(t, u, unknown)
	must(t, unknownWorker.RunOne(ctx, u.UserID, u.WorkspaceID, unknown))
}
