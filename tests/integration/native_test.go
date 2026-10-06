//go:build integration

package integration

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/naseri-coder/TELMIVO/internal/domains/downloads"
	"github.com/naseri-coder/TELMIVO/internal/domains/entitlements"
	"github.com/naseri-coder/TELMIVO/internal/domains/identity"
	"github.com/naseri-coder/TELMIVO/internal/domains/subscriptions"
	"github.com/naseri-coder/TELMIVO/internal/domains/tickets"
	"github.com/naseri-coder/TELMIVO/internal/domains/wallets"
	"github.com/naseri-coder/TELMIVO/internal/platform/auth"
	"github.com/naseri-coder/TELMIVO/internal/platform/cache"
	"github.com/naseri-coder/TELMIVO/internal/platform/config"
	cred "github.com/naseri-coder/TELMIVO/internal/platform/crypto"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/httpapi"
	"github.com/naseri-coder/TELMIVO/internal/platform/ledger"
	"github.com/naseri-coder/TELMIVO/internal/platform/queue"
	"github.com/nats-io/nats.go"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var ctx = context.Background()
var db, owner *database.Store
var sequence atomic.Int64
var authSequence atomic.Int64

func TestMain(m *testing.M) {
	for _, key := range []string{"TEST_DATABASE_URL", "TEST_MIGRATION_DATABASE_URL"} {
		u, e := url.Parse(os.Getenv(key))
		if e != nil || u.Path != "/telmivo_test" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" && u.Hostname() != "postgres") {
			fmt.Fprintln(os.Stderr, "isolated telmivo_test URLs required")
			os.Exit(1)
		}
	}
	var e error
	db, e = database.Open(ctx, os.Getenv("TEST_DATABASE_URL"), true)
	if e != nil {
		panic(e)
	}
	owner, e = database.Open(ctx, os.Getenv("TEST_MIGRATION_DATABASE_URL"), false)
	if e != nil {
		panic(e)
	}
	code := m.Run()
	db.Pool.Close()
	owner.Pool.Close()
	os.Exit(code)
}
func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func fixture(t *testing.T, admin bool) identity.Result {
	t.Helper()
	id := int64(90000000) + sequence.Add(1)
	var admins []int64
	if admin {
		admins = []int64{id}
	}
	tx, e := db.Pool.Begin(ctx)
	must(t, e)
	defer tx.Rollback(ctx)
	out, e := identity.Bootstrap(ctx, tx, auth.TelegramUser{ID: id, FirstName: "کاربر آزمایشی"}, admins)
	must(t, e)
	must(t, tx.Commit(ctx))
	return out
}
func tenant(t *testing.T, u identity.Result, fn func(pgx.Tx) error) {
	t.Helper()
	must(t, db.Tenant(ctx, u.UserID, u.WorkspaceID, false, fn))
}
func number(t *testing.T, u identity.Result, q string, args ...any) int64 {
	t.Helper()
	var n int64
	tenant(t, u, func(tx pgx.Tx) error { return tx.QueryRow(ctx, q, args...).Scan(&n) })
	return n
}
func pgCode(t *testing.T, e error, code string) {
	t.Helper()
	var pe *pgconn.PgError
	if !errors.As(e, &pe) || pe.Code != code {
		t.Fatalf("expected SQLSTATE %s, got %v", code, e)
	}
}
func TestMigration(t *testing.T) {
	t.Run("version_and_reapply", func(t *testing.T) {
		var v string
		must(t, db.Pool.QueryRow(ctx, "SHOW server_version").Scan(&v))
		t.Log("PostgreSQL", v)
		must(t, owner.Migrate(ctx))
		must(t, owner.Migrate(ctx))
		var n int
		must(t, owner.Pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&n))
		if n != 24 {
			t.Fatalf("migration count %d", n)
		}
	})
	t.Run("checksum_drift", func(t *testing.T) {
		var original string
		must(t, owner.Pool.QueryRow(ctx, "SELECT checksum FROM schema_migrations WHERE name='0001_core.sql'").Scan(&original))
		_, e := owner.Pool.Exec(ctx, "UPDATE schema_migrations SET checksum='changed' WHERE name='0001_core.sql'")
		must(t, e)
		defer func() {
			_, e := owner.Pool.Exec(ctx, "UPDATE schema_migrations SET checksum=$1 WHERE name='0001_core.sql'", original)
			must(t, e)
		}()
		if e = owner.Migrate(ctx); e == nil || !strings.Contains(e.Error(), "released migration drift") {
			t.Fatalf("expected checksum drift, got %v", e)
		}
	})
	t.Run("role_separation_force_rls", func(t *testing.T) {
		var dangerous bool
		must(t, db.Pool.QueryRow(ctx, "SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname=current_user").Scan(&dangerous))
		if dangerous {
			t.Fatal("dangerous runtime")
		}
		var n int
		must(t, db.Pool.QueryRow(ctx, "SELECT count(*) FROM pg_class WHERE relname IN('tickets','ledger_entries','subscriptions','customer_bots') AND relrowsecurity AND relforcerowsecurity AND relowner<>(SELECT oid FROM pg_roles WHERE rolname=current_user)").Scan(&n))
		if n != 4 {
			t.Fatal("FORCE RLS/ownership invalid")
		}
		_, e := db.Pool.Exec(ctx, "SET ROLE telmivo_migration")
		pgCode(t, e, "42501")
		_, e = db.Pool.Exec(ctx, "SELECT * FROM schema_migrations")
		pgCode(t, e, "42501")
		if e = db.Migrate(ctx); e == nil {
			t.Fatal("runtime migrated schema")
		}
	})
}
func signed(id int64) string {
	q := url.Values{
		"auth_date": {strconv.FormatInt(time.Now().Unix(), 10)},
		"query_id":  {fmt.Sprintf("integration-%d-%d", id, authSequence.Add(1))},
		"user":      {fmt.Sprintf(`{"id":%d,"first_name":"آزمایش"}`, id)},
	}
	var lines []string
	for k := range q {
		lines = append(lines, k+"="+q.Get(k))
	}
	sort.Strings(lines)
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte("synthetic-signing-only"))
	h := hmac.New(sha256.New, secret.Sum(nil))
	h.Write([]byte(strings.Join(lines, "\n")))
	q.Set("hash", hex.EncodeToString(h.Sum(nil)))
	return q.Encode()
}
func TestIdentityWorkspace(t *testing.T) {
	raw := signed(80000000 + sequence.Add(1))
	u, e := auth.ValidateInitData(raw, "synthetic-signing-only", time.Now(), time.Minute)
	must(t, e)
	s := identity.Service{DB: db, SessionTTL: time.Hour}
	out, token, e := s.Login(ctx, u, raw)
	must(t, e)
	id, e := s.Session(ctx, token)
	must(t, e)
	if id != out.UserID {
		t.Fatal("session identity")
	}
	if _, _, e = s.Login(ctx, u, raw); e == nil {
		t.Fatal("login replay accepted")
	}
	tx, e := db.Pool.Begin(ctx)
	must(t, e)
	same, e := identity.Bootstrap(ctx, tx, u, nil)
	must(t, e)
	must(t, tx.Commit(ctx))
	if same != out {
		t.Fatal("registration duplicated")
	}
	if n := number(t, out, "SELECT count(*) FROM subscriptions WHERE workspace_id=$1 AND plan_key='FREE'", out.WorkspaceID); n != 1 {
		t.Fatal("FREE subscription")
	}
	if n := number(t, out, "SELECT count(*) FROM wallet_accounts WHERE workspace_id=$1", out.WorkspaceID); n != 4 {
		t.Fatalf("wallet accounts %d", n)
	}
	tenant(t, out, func(tx pgx.Tx) error {
		d, e := entitlements.Resolve(ctx, tx, out.WorkspaceID, "OWNER", "tickets", "", "tickets.read", 0)
		if e == nil && !d.Allowed {
			t.Fatal(d)
		}
		return e
	})
	_, e = s.Session(ctx, strings.Repeat("x", 64))
	if e == nil {
		t.Fatal("forged session")
	}
	_, e = db.Pool.Exec(ctx, "UPDATE sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1", auth.Digest(token))
	must(t, e)
	if _, e = s.Session(ctx, token); e == nil {
		t.Fatal("expired session")
	}
}
func createTicket(t *testing.T, u identity.Result) string {
	t.Helper()
	var id string
	tenant(t, u, func(tx pgx.Tx) error {
		var e error
		id, e = tickets.Create(ctx, tx, u.UserID, u.WorkspaceID, "r1-ticket", tickets.CreateInput{Subject: "آزمایش", Body: "متن", Priority: "NORMAL", Key: "create-ticket-" + u.UserID})
		return e
	})
	return id
}
func TestWorkspaceIsolation(t *testing.T) {
	a, b := fixture(t, false), fixture(t, false)
	bid := createTicket(t, b)
	t.Run("read_and_modify", func(t *testing.T) {
		tenant(t, a, func(tx pgx.Tx) error {
			var n int
			must(t, tx.QueryRow(ctx, "SELECT count(*) FROM tickets WHERE id=$1", bid).Scan(&n))
			if n != 0 {
				t.Fatal("ticket leaked")
			}
			tag, e := tx.Exec(ctx, "UPDATE tickets SET subject='attack' WHERE id=$1", bid)
			if e == nil && tag.RowsAffected() != 0 {
				t.Fatal("cross-workspace mutation")
			}
			return e
		})
		if e := db.Tenant(ctx, a.UserID, b.WorkspaceID, false, func(tx pgx.Tx) error { return nil }); !errors.Is(e, database.ErrForbidden) {
			t.Fatal("app scope bypass", e)
		}
	})
	t.Run("composite_fk", func(t *testing.T) {
		e := db.Tenant(ctx, a.UserID, a.WorkspaceID, false, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, "INSERT INTO ticket_messages(workspace_id,ticket_id,actor_id,body,idempotency_key) VALUES($1,$2,$3,'attack','cross-fk')", a.WorkspaceID, bid, a.UserID)
			return e
		})
		pgCode(t, e, "23503")
	})
	t.Run("ordinary_no_bypass", func(t *testing.T) {
		tx, e := db.Pool.Begin(ctx)
		must(t, e)
		defer tx.Rollback(ctx)
		must(t, database.Scope(ctx, tx, a.WorkspaceID))
		_, e = tx.Exec(ctx, "SET LOCAL row_security=off; SELECT * FROM tickets")
		pgCode(t, e, "42501")
	})
	t.Run("credential_context", func(t *testing.T) {
		store, e := cred.NewAESStore("test", make([]byte, 32))
		must(t, e)
		secret, e := store.Seal(ctx, b.WorkspaceID+":bot", []byte("synthetic-credential"))
		must(t, e)
		var bot string
		tenant(t, b, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "INSERT INTO customer_bots(workspace_id,name,username,key_id,encrypted_token,status,idempotency_key) VALUES($1,'fixture',$2,$3,$4,'DRAFT','credential-test') RETURNING id::text", b.WorkspaceID, "fixture_"+b.UserID, secret.KeyID, secret.Ciphertext).Scan(&bot)
		})
		if n := number(t, a, "SELECT count(*) FROM customer_bots WHERE id=$1", bot); n != 0 {
			t.Fatal("credential storage leaked")
		}
		var stored cred.EncryptedSecret
		tenant(t, b, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT key_id,encrypted_token FROM customer_bots WHERE id=$1", bot).Scan(&stored.KeyID, &stored.Ciphertext)
		})
		plain, e := store.Open(ctx, b.WorkspaceID+":bot", stored)
		must(t, e)
		if string(plain) != "synthetic-credential" {
			t.Fatal("wrong credential")
		}
		if _, e = store.Open(ctx, a.WorkspaceID+":bot", secret); e == nil {
			t.Fatal("credential context leak")
		}
	})
	t.Run("no_scope_no_financial_rows", func(t *testing.T) {
		var n int
		must(t, db.Pool.QueryRow(ctx, "SELECT count(*) FROM wallet_accounts").Scan(&n))
		if n != 0 {
			t.Fatal("unscoped accounts leaked")
		}
	})
}
func TestTicketing(t *testing.T) {
	u := fixture(t, false)
	agent := fixture(t, false)
	denied := fixture(t, false)
	id := createTicket(t, u)
	_, e := db.Pool.Exec(ctx, "INSERT INTO workspace_members(workspace_id,user_id,role_key) VALUES($1,$2,'SUPPORT_AGENT'),($1,$3,'ANALYST')", u.WorkspaceID, agent.UserID, denied.UserID)
	must(t, e)
	agent.WorkspaceID = u.WorkspaceID
	denied.WorkspaceID = u.WorkspaceID
	t.Run("support_read_reply_replay", func(t *testing.T) {
		tenant(t, agent, func(tx pgx.Tx) error {
			if !database.Permission(ctx, tx, agent.UserID, agent.WorkspaceID, "tickets.read") {
				t.Fatal("support cannot read")
			}
			var body string
			must(t, tx.QueryRow(ctx, "SELECT subject FROM tickets WHERE id=$1", id).Scan(&body))
			return tickets.Reply(ctx, tx, agent.UserID, u.WorkspaceID, id, "r1-reply", tickets.ReplyInput{Body: "پاسخ", Key: "reply-stable-key"})
		})
		tenant(t, agent, func(tx pgx.Tx) error {
			return tickets.Reply(ctx, tx, agent.UserID, u.WorkspaceID, id, "retry", tickets.ReplyInput{Body: "پاسخ", Key: "reply-stable-key"})
		})
		if n := number(t, u, "SELECT count(*) FROM ticket_messages WHERE ticket_id=$1", id); n != 2 {
			t.Fatalf("message count %d", n)
		}
		var n int
		must(t, db.Pool.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE workspace_id=$1 AND event_type='ticket.reply'", u.WorkspaceID).Scan(&n))
		if n != 1 {
			t.Fatal("duplicate notification")
		}
	})
	t.Run("unauthorized_reply", func(t *testing.T) {
		e := db.Tenant(ctx, denied.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
			return tickets.Reply(ctx, tx, denied.UserID, u.WorkspaceID, id, "deny", tickets.ReplyInput{Body: "attack", Key: "unauthorized-key"})
		})
		var d *entitlements.Denied
		if !errors.As(e, &d) {
			t.Fatal("unauthorized role accepted", e)
		}
	})
	t.Run("valid_invalid_transition", func(t *testing.T) {
		tenant(t, agent, func(tx pgx.Tx) error {
			return tickets.Act(ctx, tx, agent.UserID, u.WorkspaceID, id, "resolve", tickets.ActionInput{Action: "status", Value: "RESOLVED"})
		})
		e := db.Tenant(ctx, agent.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
			return tickets.Act(ctx, tx, agent.UserID, u.WorkspaceID, id, "invalid", tickets.ActionInput{Action: "status", Value: "IN_PROGRESS"})
		})
		if e == nil {
			t.Fatal("invalid transition")
		}
	})
}
func apiRequest(t *testing.T, u identity.Result, path, method, body string) int {
	t.Helper()
	token, e := auth.Token()
	must(t, e)
	_, e = db.Pool.Exec(ctx, "INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '1 hour')", auth.Digest(token), u.UserID)
	must(t, e)
	api := httpapi.API{DB: db, Identity: &identity.Service{DB: db}, Config: config.Config{MiniAppURL: "http://localhost:5173", Synthetic: true}}
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Origin", "http://localhost:5173")
	r.Header.Set("X-Workspace-ID", u.WorkspaceID)
	r.AddCookie(&http.Cookie{Name: "telmivo_session", Value: token})
	w := httptest.NewRecorder()
	api.Handler().ServeHTTP(w, r)
	return w.Code
}
func TestPlatformPermissionsAndGlobalPlans(t *testing.T) {
	planner, other := fixture(t, true), fixture(t, false)
	_, e := owner.Pool.Exec(ctx, "DELETE FROM platform_admin_permissions WHERE user_id=$1 AND permission_key<>'platform.plans.manage'", planner.UserID)
	must(t, e)
	if !db.HasPlatformPermission(ctx, planner.UserID, "platform.plans.manage") || db.HasPlatformPermission(ctx, planner.UserID, "platform.wallets.adjust") {
		t.Fatal("granularity")
	}
	if code := apiRequest(t, planner, "/api/v1/wallets/adjustments", "POST", `{"asset":"USD","amount_atomic":"100","reason":"test","idempotency_key":"admin-denied"}`); code != 403 {
		t.Fatalf("unrelated sensitive mutation status %d", code)
	}
	tenant(t, other, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "UPDATE subscriptions SET plan_key='PRO' WHERE workspace_id=$1", other.WorkspaceID)
		return e
	})
	tenant(t, planner, func(tx pgx.Tx) error {
		var n int
		must(t, tx.QueryRow(ctx, "SELECT count(*) FROM subscriptions WHERE workspace_id=$1", other.WorkspaceID).Scan(&n))
		if n != 0 {
			t.Fatal("global path tenant leak")
		}
		var used bool
		must(t, tx.QueryRow(ctx, "SELECT platform_plan_in_use($1,'PRO')", planner.UserID).Scan(&used))
		if !used {
			t.Fatal("missed other workspace subscription")
		}
		return nil
	})
	if code := apiRequest(t, planner, "/api/v1/plans/PRO", "PATCH", `{"name":"PRO","active":false,"version":1}`); code != 409 {
		t.Fatalf("global plan deactivation status %d", code)
	}
	e = db.Tenant(ctx, other.UserID, other.WorkspaceID, false, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "SELECT platform_plan_in_use($1,'PRO')", other.UserID)
		return e
	})
	pgCode(t, e, "42501")
}
func TestPlanChange(t *testing.T) {
	u := fixture(t, true)
	change := func(key, plan string, expiry *time.Time) {
		tenant(t, u, func(tx pgx.Tx) error {
			return subscriptions.ChangePlan(ctx, tx, u.UserID, u.WorkspaceID, "r1-plan", subscriptions.Change{Plan: plan, Reason: "synthetic R1", Key: key, ExpiresAt: expiry})
		})
	}
	change("upgrade-operation", "PRO", nil)
	change("upgrade-operation", "PRO", nil)
	tenant(t, u, func(tx pgx.Tx) error {
		d, e := entitlements.Resolve(ctx, tx, u.WorkspaceID, "OWNER", "downloads", "", "downloads.create", 1)
		if e == nil && !d.Allowed {
			t.Fatal("PRO unavailable", d)
		}
		return e
	})
	_, e := owner.Pool.Exec(ctx, "UPDATE feature_flags SET enabled=true,rollout_status='ON' WHERE feature_key='channels';UPDATE plan_entitlements SET enabled=true,limit_value=1 WHERE plan_key='FREE' AND feature_key='channels'")
	must(t, e)
	defer func() {
		_, e := owner.Pool.Exec(ctx, "UPDATE feature_flags SET enabled=false,rollout_status='OFF' WHERE feature_key='channels';UPDATE plan_entitlements SET enabled=false,limit_value=10 WHERE plan_key='FREE' AND feature_key='channels'")
		must(t, e)
	}()
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "INSERT INTO channels(workspace_id,telegram_chat_id,title,status) VALUES($1,-1001,'one','CONNECTED'),($1,-1002,'two','CONNECTED')", u.WorkspaceID)
		return e
	})
	change("downgrade-operation", "FREE", nil)
	if n := number(t, u, "SELECT count(*) FROM channels WHERE workspace_id=$1", u.WorkspaceID); n != 2 {
		t.Fatal("downgrade deleted resources")
	}
	tenant(t, u, func(tx pgx.Tx) error {
		d, e := entitlements.Resolve(ctx, tx, u.WorkspaceID, "OWNER", "channels", "", "channels.manage", 1)
		if e == nil && (d.Allowed || d.Reason != "QUOTA_EXCEEDED") {
			t.Fatal("resource count not enforced", d)
		}
		read, e := entitlements.Resolve(ctx, tx, u.WorkspaceID, "OWNER", "channels", "", "channels.read", 0)
		if e == nil && !read.Allowed {
			t.Fatal("existing resources inaccessible")
		}
		return e
	})
	expiry := time.Now().Add(100 * time.Millisecond)
	change("temporary-operation", "PRO", &expiry)
	time.Sleep(150 * time.Millisecond)
	tenant(t, u, func(tx pgx.Tx) error {
		plan, e := entitlements.EffectivePlan(ctx, tx, u.WorkspaceID)
		if e == nil && plan != "FREE" {
			t.Fatal("temporary plan did not expire")
		}
		return e
	})
	if n := number(t, u, "SELECT count(*) FROM subscription_events WHERE workspace_id=$1", u.WorkspaceID); n != 3 {
		t.Fatalf("subscription event count %d", n)
	}
	if n := number(t, u, "SELECT count(*) FROM audit_logs WHERE workspace_id=$1 AND action='subscription.change'", u.WorkspaceID); n != 3 {
		t.Fatal("plan audit")
	}
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "INSERT INTO temporary_grants(workspace_id,feature_key,enabled,expires_at,reason) VALUES($1,'analytics',true,now()-interval '1 second','expired')", u.WorkspaceID)
		if e != nil {
			return e
		}
		d, e := entitlements.Resolve(ctx, tx, u.WorkspaceID, "OWNER", "analytics", "", "analytics.read", 1)
		if e == nil && d.Allowed {
			t.Fatal("expired grant allowed")
		}
		return e
	})
}
func adjust(u identity.Result, key string, n int64) (string, error) {
	var id string
	e := db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
		var e error
		id, e = wallets.Adjust(ctx, tx, u.UserID, u.WorkspaceID, "r1-wallet", wallets.Adjustment{Asset: ledger.USD, AmountAtomic: n, Reason: "synthetic credits", Key: key})
		return e
	})
	return id, e
}
func TestWallet(t *testing.T) {
	u := fixture(t, true)
	id, e := adjust(u, "balanced-adjustment", 1000)
	must(t, e)
	same, e := adjust(u, "balanced-adjustment", 1000)
	must(t, e)
	if id != same {
		t.Fatal("idempotency duplicated posting")
	}
	if n := number(t, u, "SELECT balance_atomic::bigint FROM wallet_balances WHERE workspace_id=$1 AND asset='USD'", u.WorkspaceID); n != 1000 {
		t.Fatalf("balance %d", n)
	}
	t.Run("audit_and_outbox", func(t *testing.T) {
		if n := number(t, u, "SELECT count(*) FROM financial_adjustments WHERE ledger_transaction_id=$1", id); n != 1 {
			t.Fatal("adjustment record")
		}
		if n := number(t, u, "SELECT count(*) FROM audit_logs WHERE workspace_id=$1 AND action='wallet.adjust'", u.WorkspaceID); n != 1 {
			t.Fatal("financial audit")
		}
		var n int
		must(t, db.Pool.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE workspace_id=$1 AND event_type='wallet.adjusted'", u.WorkspaceID).Scan(&n))
		if n != 1 {
			t.Fatal("wallet outbox")
		}
	})
	t.Run("negative_balance", func(t *testing.T) {
		if _, e := adjust(u, "overdraft-operation", -1001); e == nil {
			t.Fatal("negative balance accepted")
		}
		_, e := adjust(u, "debit-operation", -250)
		must(t, e)
	})
	for _, sql := range []string{"UPDATE ledger_entries SET amount_atomic=1 WHERE transaction_id=$1", "DELETE FROM ledger_transactions WHERE id=$1"} {
		t.Run("append_only", func(t *testing.T) {
			e := db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error { _, e := tx.Exec(ctx, sql, id); return e })
			if e == nil {
				t.Fatal("financial history mutated")
			}
		})
	}
	t.Run("sealed_entry_set", func(t *testing.T) {
		e := db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, "INSERT INTO ledger_entries(workspace_id,transaction_id,account_id,asset,amount_atomic) SELECT workspace_id,$1,id,asset,1 FROM wallet_accounts WHERE workspace_id=$2 AND asset='USD' AND kind='WALLET'", id, u.WorkspaceID)
			return e
		})
		if e == nil {
			t.Fatal("old posting amended")
		}
	})
	t.Run("unbalanced_and_negative_raw_postings", func(t *testing.T) {
		for i, amounts := range [][]int64{{1, -2}, {-100000, 100000}} {
			e := db.Tenant(ctx, u.UserID, u.WorkspaceID, false, func(tx pgx.Tx) error {
				var tid string
				if e := tx.QueryRow(ctx, "INSERT INTO ledger_transactions(workspace_id,asset,transaction_type,reference,idempotency_key,fingerprint,actor_id,reason,correlation_id) VALUES($1,'USD','ADMIN_ADJUSTMENT','raw',$2,'raw',$3,'raw test','raw') RETURNING id::text", u.WorkspaceID, fmt.Sprintf("invalid-raw-%d", i), u.UserID).Scan(&tid); e != nil {
					return e
				}
				for k, kind := range []string{"WALLET", "CLEARING"} {
					_, e := tx.Exec(ctx, "INSERT INTO ledger_entries(workspace_id,transaction_id,account_id,asset,amount_atomic) SELECT workspace_id,$1,id,asset,$2 FROM wallet_accounts WHERE workspace_id=$3 AND asset='USD' AND kind=$4", tid, amounts[k], u.WorkspaceID, kind)
					if e != nil {
						return e
					}
				}
				return nil
			})
			expected := []string{"unbalanced ledger transaction", "insufficient wallet balance"}[i]
			if e == nil || !strings.Contains(e.Error(), expected) {
				t.Fatalf("expected %s, got %v", expected, e)
			}
		}
	})
	t.Run("cross_workspace", func(t *testing.T) {
		other := fixture(t, false)
		if n := number(t, other, "SELECT count(*) FROM ledger_entries WHERE transaction_id=$1", id); n != 0 {
			t.Fatal("financial leak")
		}
	})
}
func TestFinancialConcurrency(t *testing.T) {
	u := fixture(t, true)
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	ids := make(chan string, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); id, e := adjust(u, "concurrent-duplicate", 100); errs <- e; ids <- id }()
	}
	wg.Wait()
	close(errs)
	close(ids)
	for e := range errs {
		must(t, e)
	}
	var first string
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatal("duplicate posting")
		}
	}
	if n := number(t, u, "SELECT balance_atomic::bigint FROM wallet_balances WHERE workspace_id=$1 AND asset='USD'", u.WorkspaceID); n != 100 {
		t.Fatalf("concurrent balance %d", n)
	}
	errs = make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); _, e := adjust(u, fmt.Sprintf("concurrent-debit-%d", i), -75); errs <- e }(i)
	}
	wg.Wait()
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("concurrent debit successes %d", success)
	}
	if n := number(t, u, "SELECT balance_atomic::bigint FROM wallet_balances WHERE workspace_id=$1 AND asset='USD'", u.WorkspaceID); n != 25 {
		t.Fatal("overdraft race")
	}
}
func TestRedis(t *testing.T) {
	c, e := cache.Open(os.Getenv("TEST_REDIS_URL"))
	must(t, e)
	defer c.Redis.Close()
	must(t, c.Redis.Ping(ctx).Err())
	info, e := c.Redis.Info(ctx, "server").Result()
	must(t, e)
	for _, line := range strings.Split(info, "\n") {
		if strings.HasPrefix(line, "redis_version:") {
			t.Log(strings.TrimSpace(line))
		}
	}
	key := "r1-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	for i := 0; i < 3; i++ {
		ok, e := c.Allow(ctx, key, 2, 100*time.Millisecond)
		must(t, e)
		if ok != (i < 2) {
			t.Fatal("rate primitive")
		}
	}
	time.Sleep(150 * time.Millisecond)
	ok, e := c.Allow(ctx, key, 2, 100*time.Millisecond)
	must(t, e)
	if !ok {
		t.Fatal("rate expiry")
	}
	must(t, c.Redis.Set(ctx, key, "temporary", 100*time.Millisecond).Err())
	value, e := c.Redis.Get(ctx, key).Result()
	must(t, e)
	if value != "temporary" {
		t.Fatal("cache read")
	}
	time.Sleep(150 * time.Millisecond)
	if c.Redis.Exists(ctx, key).Val() != 0 {
		t.Fatal("cache did not expire")
	}
	must(t, c.Redis.Close())
	ok, e = c.Allow(ctx, key, 2, time.Second)
	if e == nil || ok {
		t.Fatal("Redis failure did not fail closed")
	}
}
func wait(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for durable effect")
}
func consumer(t *testing.T, q *queue.Client, subject, durable string, h queue.Handler) (context.CancelFunc, chan error) {
	t.Helper()
	c, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- q.Consume(c, db, subject, durable, h) }()
	return cancel, done
}
func stop(t *testing.T, cancel context.CancelFunc, done chan error) {
	t.Helper()
	cancel()
	select {
	case e := <-done:
		if e != nil && !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("consumer did not stop")
	}
}
func TestDownloadQueue(t *testing.T) {
	u := fixture(t, true)
	tenant(t, u, func(tx pgx.Tx) error {
		return subscriptions.ChangePlan(ctx, tx, u.UserID, u.WorkspaceID, "download-plan", subscriptions.Change{Plan: "PRO", Reason: "fixture", Key: "download-plan-key"})
	})
	q, e := queue.Open(os.Getenv("TEST_NATS_URL"))
	must(t, e)
	defer q.Conn.Close()
	must(t, q.Ensure())
	t.Log("NATS", q.Conn.ConnectedServerVersion())
	var job string
	tenant(t, u, func(tx pgx.Tx) error {
		var e error
		job, e = downloads.CreateSynthetic(ctx, tx, u.UserID, u.WorkspaceID, "fixture", downloads.CreateInput{URL: "https://www.youtube.com/watch?v=controlled-fixture", FormatID: "fixture-360", Key: "durable-fixture-key"})
		return e
	})
	handler := func(ctx context.Context, tx pgx.Tx, e queue.Envelope) error {
		var payload struct {
			ID string `json:"download_job_id"`
		}
		if err := json.Unmarshal(e.Payload, &payload); err != nil {
			return err
		}
		return downloads.CompleteFixture(ctx, tx, e.WorkspaceID, payload.ID)
	}
	// Create durable, stop before publishing, then reconnect a fresh client.
	cancel, done := consumer(t, q, "telmivo.jobs.download", "r1-download", handler)
	time.Sleep(100 * time.Millisecond)
	stop(t, cancel, done)
	q.Conn.Close()
	q, e = queue.Open(os.Getenv("TEST_NATS_URL"))
	must(t, e)
	defer q.Conn.Close()
	must(t, q.Ensure())
	must(t, q.Relay(ctx, db))
	cancel, done = consumer(t, q, "telmivo.jobs.download", "r1-download", handler)
	wait(t, func() bool {
		select {
		case err := <-done:
			t.Fatalf("download consumer stopped: %v", err)
		default:
		}
		must(t, q.Relay(ctx, db))
		return number(t, u, "SELECT count(*) FROM download_jobs WHERE id=$1 AND state='COMPLETED'", job) == 1
	})
	stop(t, cancel, done)
	var event queue.Envelope
	must(t, db.Pool.QueryRow(ctx, "SELECT id::text,workspace_id::text,event_type,correlation_id,payload FROM outbox_events WHERE workspace_id=$1 AND event_type='download.queued'", u.WorkspaceID).Scan(&event.ID, &event.WorkspaceID, &event.Type, &event.CorrelationID, &event.Payload))
	must(t, q.Publish(ctx, "telmivo.jobs.download", event))
	info, e := q.JS.ConsumerInfo("TELMIVO", "r1-download")
	must(t, e)
	if info.NumAckPending != 0 {
		t.Fatal("explicit ack not persisted")
	}
	// Duplicate business event without broker Msg-ID must still be deduplicated by inbox.
	raw, e := json.Marshal(event)
	must(t, e)
	_, e = q.JS.Publish("telmivo.jobs.download", raw)
	must(t, e)
	cancel, done = consumer(t, q, "telmivo.jobs.download", "r1-download", handler)
	wait(t, func() bool {
		info, e := q.JS.ConsumerInfo("TELMIVO", "r1-download")
		return e == nil && info.NumPending == 0 && info.NumAckPending == 0
	})
	stop(t, cancel, done)
	var n int
	must(t, db.Pool.QueryRow(ctx, "SELECT count(*) FROM inbox_events WHERE consumer='r1-download' AND event_id=$1", event.ID).Scan(&n))
	if n != 1 {
		t.Fatal("inbox duplicated")
	}
}
func TestJetStreamRecovery(t *testing.T) {
	q, e := queue.Open(os.Getenv("TEST_NATS_URL"))
	must(t, e)
	defer q.Conn.Close()
	must(t, q.Ensure())
	u := fixture(t, false)
	// Raw explicit-ack delivery, subscription restart, then server redelivery.
	_, e = q.JS.AddConsumer("TELMIVO", &nats.ConsumerConfig{Durable: "r1-redelivery", FilterSubject: "telmivo.jobs.redelivery", AckPolicy: nats.AckExplicitPolicy, AckWait: 200 * time.Millisecond, MaxDeliver: 6})
	must(t, e)
	sub, e := q.JS.PullSubscribe("telmivo.jobs.redelivery", "r1-redelivery", nats.Bind("TELMIVO", "r1-redelivery"))
	must(t, e)
	_, e = q.JS.Publish("telmivo.jobs.redelivery", []byte("fixture"))
	must(t, e)
	msgs, e := sub.Fetch(1, nats.MaxWait(time.Second))
	must(t, e)
	meta, e := msgs[0].Metadata()
	must(t, e)
	must(t, sub.Unsubscribe())
	time.Sleep(250 * time.Millisecond)
	sub, e = q.JS.PullSubscribe("telmivo.jobs.redelivery", "r1-redelivery", nats.Bind("TELMIVO", "r1-redelivery"))
	must(t, e)
	msgs, e = sub.Fetch(1, nats.MaxWait(time.Second))
	must(t, e)
	again, e := msgs[0].Metadata()
	must(t, e)
	if again.NumDelivered < 2 || again.Sequence.Stream != meta.Sequence.Stream {
		t.Fatal("no redelivery")
	}
	must(t, msgs[0].AckSync())
	must(t, sub.Unsubscribe())
	var attempts atomic.Int64
	cancel, done := consumer(t, q, "telmivo.jobs.retry", "r1-retry", func(_ context.Context, _ pgx.Tx, _ queue.Envelope) error {
		if attempts.Add(1) == 1 {
			return errors.New("synthetic transient")
		}
		return nil
	})
	event := queue.Envelope{ID: "11111111-1111-4111-8111-111111111111", WorkspaceID: u.WorkspaceID, Type: "fixture.retry", Payload: json.RawMessage(`{}`)}
	must(t, q.Publish(ctx, "telmivo.jobs.retry", event))
	wait(t, func() bool {
		var n int
		e := db.Pool.QueryRow(ctx, "SELECT count(*) FROM inbox_events WHERE consumer='r1-retry' AND event_id=$1", event.ID).Scan(&n)
		return e == nil && n == 1
	})
	if attempts.Load() != 2 {
		t.Fatal("unexpected retry count")
	}
	stop(t, cancel, done)
	cancel, done = consumer(t, q, "telmivo.jobs.failure", "r1-failure", func(_ context.Context, _ pgx.Tx, _ queue.Envelope) error { return errors.New("synthetic permanent") })
	event.ID = "22222222-2222-4222-8222-222222222222"
	must(t, q.Publish(ctx, "telmivo.jobs.failure", event))
	_, e = q.JS.Publish("telmivo.jobs.failure", []byte(`{"id":"bad-id","workspace_id":"bad"}`))
	must(t, e)
	wait(t, func() bool {
		var n int
		e := db.Pool.QueryRow(ctx, "SELECT count(*) FROM dead_letter_events WHERE consumer='r1-failure'").Scan(&n)
		return e == nil && n == 2
	})
	stop(t, cancel, done)
	var n int
	must(t, db.Pool.QueryRow(ctx, "SELECT count(*) FROM inbox_events WHERE consumer='r1-failure'").Scan(&n))
	if n != 0 {
		t.Fatal("failed handler committed inbox")
	}
}

func TestOutboxLease(t *testing.T) {
	u := fixture(t, false)
	q, e := queue.Open(os.Getenv("TEST_NATS_URL"))
	must(t, e)
	defer q.Conn.Close()
	must(t, q.Ensure())
	var id string
	must(t, db.Pool.QueryRow(ctx, "INSERT INTO outbox_events(workspace_id,subject,event_type,payload,correlation_id,lease_owner,lease_until) VALUES($1,'telmivo.events.lease','lease.fixture','{}','lease-test','old-owner',now()+interval '1 minute') RETURNING id::text", u.WorkspaceID).Scan(&id))
	must(t, q.Relay(ctx, db))
	var published bool
	must(t, db.Pool.QueryRow(ctx, "SELECT published_at IS NOT NULL FROM outbox_events WHERE id=$1", id).Scan(&published))
	if published {
		t.Fatal("live lease stolen")
	}
	_, e = db.Pool.Exec(ctx, "UPDATE outbox_events SET lease_until=now()-interval '1 second' WHERE id=$1", id)
	must(t, e)
	wait(t, func() bool {
		must(t, q.Relay(ctx, db))
		must(t, db.Pool.QueryRow(ctx, "SELECT published_at IS NOT NULL FROM outbox_events WHERE id=$1", id).Scan(&published))
		return published
	})
	tag, e := db.Pool.Exec(ctx, "UPDATE outbox_events SET published_at=NULL WHERE id=$1 AND lease_owner='old-owner'", id)
	must(t, e)
	if tag.RowsAffected() != 0 {
		t.Fatal("stale lease owner updated completion")
	}
	event := queue.Envelope{ID: id, WorkspaceID: u.WorkspaceID, Type: "lease.fixture", Payload: json.RawMessage(`{}`)}
	raw, e := json.Marshal(event)
	must(t, e)
	msg := nats.NewMsg("telmivo.events.lease")
	msg.Header.Set("Nats-Msg-Id", id)
	msg.Data = raw
	ack, e := q.JS.PublishMsg(msg)
	must(t, e)
	if !ack.Duplicate {
		t.Fatal("JetStream dedup failed")
	}
}
func TestDownloadCancel(t *testing.T) {
	u := fixture(t, true)
	tenant(t, u, func(tx pgx.Tx) error {
		return subscriptions.ChangePlan(ctx, tx, u.UserID, u.WorkspaceID, "cancel-plan", subscriptions.Change{Plan: "PRO", Reason: "fixture", Key: "cancel-plan-key"})
	})
	var id string
	tenant(t, u, func(tx pgx.Tx) error {
		var e error
		id, e = downloads.CreateSynthetic(ctx, tx, u.UserID, u.WorkspaceID, "cancel", downloads.CreateInput{URL: "https://youtube.com/watch?v=cancel-fixture", FormatID: "fixture-360", Key: "cancel-fixture-key"})
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, "UPDATE download_jobs SET state='CANCELLED' WHERE id=$1", id)
		return e
	})
	tenant(t, u, func(tx pgx.Tx) error { return downloads.CompleteFixture(ctx, tx, u.WorkspaceID, id) })
	if n := number(t, u, "SELECT count(*) FROM download_jobs WHERE id=$1 AND state='CANCELLED'", id); n != 1 {
		t.Fatal("cancel overwritten")
	}
}

// Run separately after restarting the same isolated NATS container/storage.
func TestBrokerRestart(t *testing.T) {
	if os.Getenv("TEST_BROKER_RESTART") != "true" {
		t.Skip("separate broker restart step")
	}
	q, e := queue.Open(os.Getenv("TEST_NATS_URL"))
	must(t, e)
	defer q.Conn.Close()
	info, e := q.JS.ConsumerInfo("TELMIVO", "r1-download")
	must(t, e)
	if info.AckFloor.Stream == 0 {
		t.Fatal("durable acknowledgement lost after broker restart")
	}
	stream, e := q.JS.StreamInfo("TELMIVO")
	must(t, e)
	if stream.State.Msgs == 0 {
		t.Fatal("stream lost after restart")
	}
}
