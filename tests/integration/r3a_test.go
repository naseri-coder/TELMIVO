//go:build integration

package integration

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/identity"
	"github.com/naseri-coder/TELMIVO/internal/domains/mainbot"
	"github.com/naseri-coder/TELMIVO/internal/domains/policies"
	"github.com/naseri-coder/TELMIVO/internal/domains/wallets"
	telegramclient "github.com/naseri-coder/TELMIVO/internal/integrations/telegram"
	"github.com/naseri-coder/TELMIVO/internal/platform/auth"
	"github.com/naseri-coder/TELMIVO/internal/platform/config"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/httpapi"
	"github.com/naseri-coder/TELMIVO/internal/platform/ledger"
	"github.com/naseri-coder/TELMIVO/internal/platform/localization"
)

func r3aService() *identity.Service { return &identity.Service{DB: db, SessionTTL: time.Hour} }
func r3aUpdate(t *testing.T, id, update int64, text string) identity.Update {
	t.Helper()
	u, e := identity.DecodeUpdate([]byte(fmt.Sprintf(`{"update_id":%d,"message":{"from":{"id":%d,"first_name":"R3A"},"chat":{"id":%d,"type":"private"},"text":%q}}`, update, id, id, text)))
	must(t, e)
	return u
}
func r3aLogin(t *testing.T, id int64, admin bool) (identity.Result, string) {
	t.Helper()
	s := r3aService()
	if admin {
		s.Admins = []int64{id}
	}
	raw := signed(id)
	u, e := auth.ValidateInitData(raw, "synthetic-signing-only", time.Now(), time.Minute)
	must(t, e)
	out, token, e := s.Login(ctx, u, raw)
	must(t, e)
	return out, token
}
func r3aExtraWorkspace(t *testing.T, u identity.Result, name, role string) string {
	t.Helper()
	var id string
	tx, e := db.Pool.Begin(ctx)
	must(t, e)
	defer tx.Rollback(ctx)
	must(t, tx.QueryRow(ctx, "INSERT INTO workspaces(name,owner_id) VALUES($1,$2) RETURNING id::text", name, u.UserID).Scan(&id))
	_, e = tx.Exec(ctx, "INSERT INTO workspace_members(workspace_id,user_id,role_key) VALUES($1,$2,$3)", id, u.UserID, role)
	must(t, e)
	must(t, database.Scope(ctx, tx, id))
	_, e = tx.Exec(ctx, "INSERT INTO subscriptions(workspace_id,plan_key) VALUES($1,'FREE')", id)
	must(t, e)
	_, e = tx.Exec(ctx, "INSERT INTO wallet_accounts(workspace_id,asset,kind) SELECT $1,a,k FROM unnest(ARRAY['USD','IRR']) a CROSS JOIN unnest(ARRAY['WALLET','CLEARING']) k", id)
	must(t, e)
	must(t, tx.Commit(ctx))
	return id
}
func TestR3ASharedEntry(t *testing.T) {
	for index, first := range []string{"MAIN_BOT", "MINIAPP"} {
		t.Run(first+"_first", func(t *testing.T) {
			id := int64(96001001 + index)
			service := r3aService()
			bot := mainbot.Service{Identity: service, MiniAppURL: "https://mini.test"}
			var original identity.Result
			var e error
			if first == "MAIN_BOT" {
				original, e = bot.Handle(ctx, r3aUpdate(t, id, id, "/start"), "r3a-entry")
				must(t, e)
			} else {
				original, _ = r3aLogin(t, id, false)
			}
			var end time.Time
			var version int64
			tenant(t, original, func(tx pgx.Tx) error {
				return tx.QueryRow(ctx, "SELECT ends_at,policy_version FROM trial_enrollments WHERE user_id=$1 AND policy_key='global_customer_trial'", original.UserID).Scan(&end, &version)
			})
			later, e := bot.Handle(ctx, r3aUpdate(t, id, id+100, "/start product"), "r3a-repeat")
			must(t, e)
			if later.UserID != original.UserID || later.WorkspaceID != original.WorkspaceID {
				t.Fatal("cross-entry identity drift")
			}
			if first == "MAIN_BOT" {
				login, _ := r3aLogin(t, id, false)
				if login.UserID != original.UserID || login.WorkspaceID != original.WorkspaceID {
					t.Fatal("Mini App identity drift")
				}
			}
			_, e = bot.Handle(ctx, r3aUpdate(t, id, id+100, "/start product"), "r3a-retry")
			must(t, e)
			if number(t, original, "SELECT count(*) FROM workspaces WHERE owner_id=$1", original.UserID) != 1 || number(t, original, "SELECT count(*) FROM telegram_accounts WHERE user_id=$1", original.UserID) != 1 || number(t, original, "SELECT count(*) FROM identity_entries WHERE user_id=$1", original.UserID) != 2 {
				t.Fatal("duplicate identity/workspace/entries")
			}
			if number(t, original, "SELECT count(*) FROM trial_enrollments WHERE user_id=$1 AND ends_at=$2 AND policy_version=$3", original.UserID, end, version) != 1 {
				t.Fatal("trial restarted")
			}
			if number(t, original, "SELECT count(*) FROM outbox_events WHERE workspace_id=$1 AND payload->>'idempotency_key'=$2", original.WorkspaceID, "main-bot:"+identity.UpdateKey(id+100)) != 1 {
				t.Fatal("duplicate durable response")
			}
		})
	}
	t.Run("menu_and_thin_transport", func(t *testing.T) {
		id := int64(96001010)
		service := r3aService()
		bot := &mainbot.Service{Identity: service, MiniAppURL: "https://mini.test"}
		handler := telegramclient.MainBotWebhook("synthetic-webhook-fixture", bot)
		u := r3aUpdate(t, id, id, "/start")
		raw, _ := json.Marshal(u)
		req := func(secret string, body []byte) *httptest.ResponseRecorder {
			r := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
			r.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			return w
		}
		if req("wrong", raw).Code != 401 || req("synthetic-webhook-fixture", []byte("{")).Code != 400 {
			t.Fatal("invalid transport accepted")
		}
		if req("synthetic-webhook-fixture", raw).Code != 200 || req("synthetic-webhook-fixture", raw).Code != 200 {
			t.Fatal("transport retry")
		}
		var userID, ws string
		must(t, db.Pool.QueryRow(ctx, "SELECT user_id::text,w.id::text FROM telegram_accounts a JOIN workspaces w ON w.owner_id=a.user_id WHERE a.telegram_id=$1", id).Scan(&userID, &ws))
		out := identity.Result{UserID: userID, WorkspaceID: ws}
		var markup string
		tenant(t, out, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT (payload->'markup')::text FROM outbox_events WHERE workspace_id=$1 AND payload->>'idempotency_key'=$2", ws, "main-bot:"+identity.UpdateKey(id)).Scan(&markup)
		})
		for _, key := range []string{"dashboard", "channels", "groups", "bots", "tickets", "wallet", "plans", "settings"} {
			if !strings.Contains(markup, localization.Text("menu."+key)) {
				t.Fatal("missing menu", key)
			}
		}
		if strings.Contains(markup, localization.Text("menu.admin")) || strings.Contains(markup, "web_app") {
			t.Fatal("ordinary owner menu authority/transport")
		}
		var panel string
		tenant(t, out, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT (payload->'markup')::text FROM outbox_events WHERE workspace_id=$1 AND payload->>'idempotency_key'=$2", ws, "main-bot:panel:"+identity.UpdateKey(id)).Scan(&panel)
		})
		if !strings.Contains(panel, localization.Text("menu.open")) || !strings.Contains(panel, "web_app") {
			t.Fatal("inline panel entry missing")
		}
		service.Admins = []int64{id}
		_, e := bot.Handle(ctx, r3aUpdate(t, id, id+1, "/start"), "admin-existing-authority")
		must(t, e)
		tenant(t, out, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT (payload->'markup')::text FROM outbox_events WHERE workspace_id=$1 AND payload->>'idempotency_key'=$2", ws, "main-bot:"+identity.UpdateKey(id+1)).Scan(&markup)
		})
		if !strings.Contains(markup, localization.Text("menu.admin")) {
			t.Fatal("authorized admin menu missing")
		}
	})
}
func r3aAPI() http.Handler {
	return (&httpapi.API{DB: db, Identity: r3aService(), Config: config.Config{Environment: "test", Synthetic: true, BotToken: "synthetic-signing-only", MiniAppURL: "https://mini.test", SessionTTL: time.Hour}}).Handler()
}
func r3aRequest(handler http.Handler, token, workspace, method, path string, body any) *httptest.ResponseRecorder {
	var b bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&b).Encode(body)
	}
	r := httptest.NewRequest(method, path, &b)
	r.Header.Set("Origin", "https://mini.test")
	r.Header.Set("X-Workspace-ID", workspace)
	if token != "" {
		r.AddCookie(&http.Cookie{Name: "telmivo_session", Value: token})
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}
func decodeR3A[T any](t *testing.T, w *httptest.ResponseRecorder, status int) T {
	t.Helper()
	if w.Code != status {
		t.Fatalf("HTTP %d expected %d: %s", w.Code, status, w.Body.String())
	}
	var value T
	must(t, json.Unmarshal(w.Body.Bytes(), &value))
	return value
}
func TestR3ACustomerAPI(t *testing.T) {
	handler := r3aAPI()
	u, token := r3aLogin(t, 96002001, false)
	foreign, _ := r3aLogin(t, 96002002, false)
	admin, adminToken := r3aLogin(t, 96002003, true)
	second := r3aExtraWorkspace(t, u, "Second authorized workspace", "OWNER")
	tenant(t, u, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "INSERT INTO groups(workspace_id,telegram_chat_id,title,status) VALUES($1,-96002001,'Native group','CONNECTED')", u.WorkspaceID)
		return e
	})
	for i := 0; i < 3; i++ {
		must(t, db.Tenant(ctx, admin.UserID, u.WorkspaceID, true, func(tx pgx.Tx) error {
			_, e := wallets.Adjust(ctx, tx, admin.UserID, u.WorkspaceID, "r3a-seed", wallets.Adjustment{Asset: ledger.USD, AmountAtomic: 100, Reason: fmt.Sprintf("synthetic R3A ledger %d", i), Key: fmt.Sprintf("r3a-ledger-%d", i)})
			return e
		}))
	}
	t.Run("authorized_workspace_list", func(t *testing.T) {
		list := decodeR3A[[]map[string]any](t, r3aRequest(handler, token, "", "GET", "/api/v1/workspaces", nil), 200)
		if len(list) != 2 {
			t.Fatal("workspace list", list)
		}
		for _, w := range list {
			if w["id"] == foreign.WorkspaceID {
				t.Fatal("foreign workspace listed")
			}
		}
	})
	t.Run("real_dashboard_switch_and_isolation", func(t *testing.T) {
		for ws, count := range map[string]float64{u.WorkspaceID: 1, second: 0} {
			summary := decodeR3A[map[string]any](t, r3aRequest(handler, token, ws, "GET", "/api/v1/customer/overview", nil), 200)
			if summary["workspace_id"] != ws || summary["groups"] != count {
				t.Fatal("dashboard state", summary)
			}
		}
		decodeR3A[map[string]any](t, r3aRequest(handler, token, foreign.WorkspaceID, "GET", "/api/v1/customer/overview", nil), 403)
	})
	t.Run("resolver_equivalence_and_quota", func(t *testing.T) {
		tenant(t, u, func(tx pgx.Tx) error {
			request := policies.Request{User: u.UserID, Workspace: u.WorkspaceID, Role: "OWNER", Feature: "downloads", Permission: "downloads.create", Requested: 1}
			_, e := policies.Reserve(ctx, tx, request, "r3a-reserved")
			if e != nil {
				return e
			}
			_, e = policies.Reserve(ctx, tx, request, "r3a-success")
			if e != nil {
				return e
			}
			return policies.Settle(ctx, tx, u.WorkspaceID, "r3a-success", true)
		})
		actual := decodeR3A[policies.Decision](t, r3aRequest(handler, token, u.WorkspaceID, "GET", "/api/v1/customer/access", nil), 200)
		runtime := decodeR3A[policies.Decision](t, r3aRequest(handler, token, u.WorkspaceID, "POST", "/api/v1/access/decision", map[string]any{"feature": "downloads"}), 200)
		if !reflect.DeepEqual(actual, runtime) {
			t.Fatal("customer resolver differs from runtime")
		}
		resource, ok := actual.Resources["downloader.successes"]
		if !ok || resource.Used != 1 || resource.Reserved != 1 || resource.Remaining != resource.Limit-2 || resource.ResetAt == nil {
			t.Fatal("non-authoritative quota", actual)
		}
	})
	t.Run("plan_catalog", func(t *testing.T) {
		catalog := decodeR3A[map[string]json.RawMessage](t, r3aRequest(handler, token, u.WorkspaceID, "GET", "/api/v1/customer/catalog", nil), 200)
		var plans []any
		must(t, json.Unmarshal(catalog["plans"], &plans))
		if len(plans) == 0 {
			t.Fatal("empty seeded catalog")
		}
	})
	t.Run("wallet_matches_ledger", func(t *testing.T) {
		balance := decodeR3A[[]map[string]string](t, r3aRequest(handler, token, u.WorkspaceID, "GET", "/api/v1/wallets", nil), 200)
		for _, b := range balance {
			if b["asset"] == "USD" && b["balance_atomic"] != "300" {
				t.Fatal("ledger mismatch", b)
			}
		}
		decodeR3A[map[string]any](t, r3aRequest(handler, token, foreign.WorkspaceID, "GET", "/api/v1/wallets", nil), 403)
	})
	t.Run("wallet_stable_keyset_pagination", func(t *testing.T) {
		type page struct {
			Items []map[string]string `json:"items"`
			Next  string              `json:"next_cursor"`
		}
		one := decodeR3A[page](t, r3aRequest(handler, token, u.WorkspaceID, "GET", "/api/v1/customer/wallet/history?limit=2", nil), 200)
		two := decodeR3A[page](t, r3aRequest(handler, token, u.WorkspaceID, "GET", "/api/v1/customer/wallet/history?limit=2&cursor="+url.QueryEscape(one.Next), nil), 200)
		repeat := decodeR3A[page](t, r3aRequest(handler, token, u.WorkspaceID, "GET", "/api/v1/customer/wallet/history?limit=2", nil), 200)
		if len(one.Items) != 2 || one.Next == "" || len(two.Items) != 1 || two.Next != "" || !reflect.DeepEqual(one, repeat) {
			t.Fatal("pagination", one, two)
		}
		for _, item := range one.Items {
			if item["id"] == two.Items[0]["id"] {
				t.Fatal("duplicate page item")
			}
		}
		decodeR3A[map[string]any](t, r3aRequest(handler, token, u.WorkspaceID, "GET", "/api/v1/customer/wallet/history?limit=1000", nil), 400)
	})
	t.Run("read_only_customer_wallet", func(t *testing.T) {
		decodeR3A[map[string]any](t, r3aRequest(handler, token, u.WorkspaceID, "POST", "/api/v1/wallets/adjustments", map[string]any{"asset": "USD", "amount_atomic": "100", "reason": "forbidden", "idempotency_key": "r3a-denied"}), 403)
		if number(t, u, "SELECT count(*) FROM ledger_transactions WHERE workspace_id=$1", u.WorkspaceID) != 3 {
			t.Fatal("customer mutated ledger")
		}
	})
	t.Run("settings_authorized_audited_and_optimistic", func(t *testing.T) {
		old := decodeR3A[map[string]any](t, r3aRequest(handler, token, u.WorkspaceID, "GET", "/api/v1/customer/settings", nil), 200)
		value := map[string]any{"name": "نام جدید R3A", "expected_name": old["name"]}
		decodeR3A[map[string]any](t, r3aRequest(handler, token, u.WorkspaceID, "PATCH", "/api/v1/customer/settings", value), 200)
		decodeR3A[map[string]any](t, r3aRequest(handler, token, u.WorkspaceID, "PATCH", "/api/v1/customer/settings", value), 409)
		if number(t, u, "SELECT count(*) FROM audit_logs WHERE workspace_id=$1 AND action='workspace.name.update' AND actor_id=$2 AND length(correlation_id)>0", u.WorkspaceID, u.UserID) != 1 {
			t.Fatal("missing audit")
		}
	})
	t.Run("settings_cross_workspace_denied", func(t *testing.T) {
		decodeR3A[map[string]any](t, r3aRequest(handler, token, foreign.WorkspaceID, "PATCH", "/api/v1/customer/settings", map[string]any{"name": "forbidden", "expected_name": "R3A"}), 403)
	})
	t.Run("admin_authority_not_workspace_owner", func(t *testing.T) {
		me := decodeR3A[map[string]any](t, r3aRequest(handler, token, "", "GET", "/api/v1/auth/me", nil), 200)
		if me["can_admin_policies"] != false {
			t.Fatal("customer granted admin")
		}
		decodeR3A[map[string]any](t, r3aRequest(handler, token, u.WorkspaceID, "GET", "/api/v1/access-policies?kind=TRIAL", nil), 403)
		adminMe := decodeR3A[map[string]any](t, r3aRequest(handler, adminToken, "", "GET", "/api/v1/auth/me", nil), 200)
		if adminMe["can_admin_policies"] != true || adminMe["is_admin"] != true {
			t.Fatal("existing admin missing from customer context")
		}
		if !db.HasPlatformPermission(ctx, admin.UserID, "platform.access_policies.read") {
			t.Fatal("existing admin not recognized")
		}
	})
	t.Run("nonowner_member_cannot_rename", func(t *testing.T) {
		_, e := db.Pool.Exec(ctx, "INSERT INTO workspace_members(workspace_id,user_id,role_key) VALUES($1,$2,'ANALYST')", foreign.WorkspaceID, u.UserID)
		must(t, e)
		settings := decodeR3A[map[string]any](t, r3aRequest(handler, token, foreign.WorkspaceID, "GET", "/api/v1/customer/settings", nil), 200)
		if settings["can_edit"] != false {
			t.Fatal("member can edit")
		}
		decodeR3A[map[string]any](t, r3aRequest(handler, token, foreign.WorkspaceID, "PATCH", "/api/v1/customer/settings", map[string]any{"name": "forbidden", "expected_name": settings["name"]}), 403)
	})
}
func TestR3AAuth(t *testing.T) {
	handler := r3aAPI()
	raw := signed(96003001)
	t.Run("direct_signed_login_and_replay", func(t *testing.T) {
		decodeR3A[identity.Result](t, r3aRequest(handler, "", "", "POST", "/api/v1/auth/telegram", map[string]string{"init_data": raw}), 200)
		decodeR3A[map[string]any](t, r3aRequest(handler, "", "", "POST", "/api/v1/auth/telegram", map[string]string{"init_data": raw}), 401)
	})
	t.Run("tampered_and_malformed", func(t *testing.T) {
		for _, input := range []string{strings.Replace(raw, "96003001", "96003002", 1), "bad%encoding"} {
			decodeR3A[map[string]any](t, r3aRequest(handler, "", "", "POST", "/api/v1/auth/telegram", map[string]string{"init_data": input}), 401)
		}
	})
	t.Run("expired_signature", func(t *testing.T) {
		q, _ := url.ParseQuery(raw)
		q.Set("auth_date", strconv.FormatInt(time.Now().Add(-11*time.Minute).Unix(), 10))
		q.Del("hash")
		var keys []string
		for key := range q {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var lines []string
		for _, key := range keys {
			lines = append(lines, key+"="+q.Get(key))
		}
		secret := hmac.New(sha256.New, []byte("WebAppData"))
		secret.Write([]byte("synthetic-signing-only"))
		signature := hmac.New(sha256.New, secret.Sum(nil))
		signature.Write([]byte(strings.Join(lines, "\n")))
		q.Set("hash", hex.EncodeToString(signature.Sum(nil)))
		decodeR3A[map[string]any](t, r3aRequest(handler, "", "", "POST", "/api/v1/auth/telegram", map[string]string{"init_data": q.Encode()}), 401)
	})
	t.Run("restricted_runtime", func(t *testing.T) {
		var safe bool
		must(t, db.Pool.QueryRow(ctx, "SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolinherit FROM pg_roles WHERE rolname=current_user").Scan(&safe))
		if !safe {
			t.Fatal("privileged acceptance role")
		}
	})
}

// Synthetic fixtures for the actual browser/API/database journey; no customer behavior runs as migration role.
func TestR3ABrowserFixture(t *testing.T) {
	tx, e := db.Pool.Begin(ctx)
	must(t, e)
	u, e := identity.Enter(ctx, tx, auth.TelegramUser{ID: 95000001, FirstName: "مشتری R3A"}, nil, "MINIAPP", "browser-fixture")
	must(t, e)
	must(t, tx.Commit(ctx))
	second := r3aExtraWorkspace(t, u, "فضای دوم R3A", "OWNER")
	tenant(t, identity.Result{UserID: u.UserID, WorkspaceID: second}, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "INSERT INTO groups(workspace_id,telegram_chat_id,title,status) VALUES($1,-95000001,'Browser group','CONNECTED')", second)
		return e
	})
	admin, _ := r3aLogin(t, 95000099, true)
	for i := 0; i < 21; i++ {
		must(t, db.Tenant(ctx, admin.UserID, second, true, func(tx pgx.Tx) error {
			_, e := wallets.Adjust(ctx, tx, admin.UserID, second, "r3a-browser-fixture", wallets.Adjustment{Asset: ledger.USD, AmountAtomic: 100, Reason: fmt.Sprintf("synthetic browser ledger %02d", i), Key: fmt.Sprintf("r3a-browser-ledger-%02d", i)})
			return e
		}))
	}
	t.Log("isolated native browser customer fixtures prepared under restricted runtime role")
}
