package contract_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/naseri-coder/TELMIVO/internal/domains/accessgates"
	"github.com/naseri-coder/TELMIVO/internal/domains/botmodules"
	"github.com/naseri-coder/TELMIVO/internal/domains/broadcasts"
	"github.com/naseri-coder/TELMIVO/internal/domains/downloads"
	"github.com/naseri-coder/TELMIVO/internal/domains/identity"
	"github.com/naseri-coder/TELMIVO/internal/domains/tickets"
	"github.com/naseri-coder/TELMIVO/internal/domains/uploads"
	"github.com/naseri-coder/TELMIVO/internal/platform/auth"
	"github.com/naseri-coder/TELMIVO/internal/platform/config"
	"github.com/naseri-coder/TELMIVO/internal/platform/crypto"
	"github.com/naseri-coder/TELMIVO/internal/platform/entitlement"
	"github.com/naseri-coder/TELMIVO/internal/platform/ledger"
	"github.com/naseri-coder/TELMIVO/internal/platform/localization"
	"github.com/naseri-coder/TELMIVO/internal/platform/payments"
	"github.com/naseri-coder/TELMIVO/internal/platform/rbac"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func ptr(v int64) *int64 { return &v }
func TestEntitlementMatrix(t *testing.T) {
	now := time.Unix(1800000000, 0)
	past := now.Add(-time.Second)
	future := now.Add(time.Second)
	base := entitlement.Input{Global: true, Plan: entitlement.Rule{Enabled: true, Limit: ptr(10)}, Permission: true, PluginAllowed: true, DependenciesValid: true, Now: now, Requested: 1}
	cases := []struct {
		name   string
		mutate func(*entitlement.Input)
		reason string
	}{
		{"free-disabled", func(i *entitlement.Input) { i.Plan.Enabled = false }, "FEATURE_DISABLED"},
		{"pro-allowed", func(i *entitlement.Input) {}, "ALLOWED"},
		{"business-allowed", func(i *entitlement.Input) { i.Plan.Limit = ptr(1000) }, "ALLOWED"},
		{"global-disabled", func(i *entitlement.Input) { i.Global = false; i.Override = &entitlement.Rule{Enabled: true} }, "GLOBAL_DISABLED"},
		{"plugin-disabled", func(i *entitlement.Input) { i.PluginAllowed = false }, "PLUGIN_DISABLED"},
		{"override-enable", func(i *entitlement.Input) { i.Plan.Enabled = false; i.Override = &entitlement.Rule{Enabled: true} }, "ALLOWED"},
		{"override-disable", func(i *entitlement.Input) { i.Override = &entitlement.Rule{Enabled: false} }, "FEATURE_DISABLED"},
		{"active-grant", func(i *entitlement.Input) {
			i.Plan.Enabled = false
			i.Grant = &entitlement.Rule{Enabled: true, ExpiresAt: &future}
		}, "ALLOWED"},
		{"expired-grant", func(i *entitlement.Input) {
			i.Plan.Enabled = false
			i.Grant = &entitlement.Rule{Enabled: true, ExpiresAt: &past}
		}, "FEATURE_DISABLED"},
		{"expiry-boundary", func(i *entitlement.Input) {
			i.Plan.Enabled = false
			i.Grant = &entitlement.Rule{Enabled: true, ExpiresAt: &now}
		}, "FEATURE_DISABLED"},
		{"override-beats-grant", func(i *entitlement.Input) {
			i.Grant = &entitlement.Rule{Enabled: true}
			i.Override = &entitlement.Rule{Enabled: false}
		}, "FEATURE_DISABLED"},
		{"role-denied", func(i *entitlement.Input) { i.Permission = false }, "PERMISSION_DENIED"},
		{"quota-last-slot", func(i *entitlement.Input) { i.Usage = 9 }, "ALLOWED"},
		{"quota-exceeded", func(i *entitlement.Input) { i.Usage = 10 }, "QUOTA_EXCEEDED"},
		{"quota-overflow", func(i *entitlement.Input) { i.Usage = math.MaxInt64; i.Requested = math.MaxInt64 }, "QUOTA_EXCEEDED"},
		{"negative-usage", func(i *entitlement.Input) { i.Usage = -1 }, "INVALID_USAGE"},
		{"dependency-invalid", func(i *entitlement.Input) { i.DependenciesValid = false }, "DEPENDENCY_INVALID"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			i := base
			c.mutate(&i)
			d := entitlement.Resolve(i)
			if d.Reason != c.reason || d.Allowed != (c.reason == "ALLOWED") {
				t.Fatalf("got %+v", d)
			}
		})
	}
}
func signed(token string, v url.Values) string {
	keys := []string{}
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := []string{}
	for _, k := range keys {
		lines = append(lines, k+"="+v.Get(k))
	}
	s := hmac.New(sha256.New, []byte("WebAppData"))
	s.Write([]byte(token))
	h := hmac.New(sha256.New, s.Sum(nil))
	h.Write([]byte(strings.Join(lines, "\n")))
	v.Set("hash", hex.EncodeToString(h.Sum(nil)))
	return v.Encode()
}
func TestTelegramTrustBoundary(t *testing.T) {
	now := time.Unix(1800000000, 0)
	token := "synthetic-only-test-token"
	v := url.Values{"auth_date": {strconv.FormatInt(now.Unix(), 10)}, "user": {`{"id":9007199254740000,"first_name":"کاربر"}`}, "signature": {"signed-field"}}
	raw := signed(token, v)
	u, e := auth.ValidateInitData(raw, token, now, 10*time.Minute)
	if e != nil || u.ID != 9007199254740000 {
		t.Fatal("valid signed identity rejected", e)
	}
	for name, raw := range map[string]string{"tampered": strings.Replace(raw, "signed-field", "tampered", 1), "duplicate": raw + "&auth_date=1", "wrong-user": strings.Replace(raw, "9007199254740000", "1", 1)} {
		t.Run(name, func(t *testing.T) {
			if _, e := auth.ValidateInitData(raw, token, now, 10*time.Minute); e == nil {
				t.Fatal("invalid identity accepted")
			}
		})
	}
	if _, e = auth.ValidateInitData(raw, token, now.Add(11*time.Minute), 10*time.Minute); e == nil {
		t.Fatal("stale identity accepted")
	}
	if _, e = auth.ValidateInitData(raw, token, now.Add(-time.Minute), 10*time.Minute); e == nil {
		t.Fatal("future identity accepted")
	}
}
func TestConfiguration(t *testing.T) {
	base := map[string]string{"DATABASE_URL": "postgres://test:test@localhost/test", "REDIS_URL": "redis://localhost:6379", "NATS_URL": "nats://localhost:4222"}
	get := func(k string) string { return base[k] }
	if _, e := config.Load(get); e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{"0", "-1", "9223372036854775808", "oops"} {
		base["ADMIN_TELEGRAM_IDS"] = id
		if _, e := config.Load(get); e == nil {
			t.Fatal("invalid ID accepted")
		}
	}
	base["ADMIN_TELEGRAM_IDS"] = ""
	base["TELMIVO_ENV"] = "production"
	base["SYNTHETIC_MODE"] = "true"
	if _, e := config.Load(get); e == nil {
		t.Fatal("production synthetic mode accepted")
	}
}
func TestCredentialIsolation(t *testing.T) {
	s, e := crypto.NewAESStore("test-key", make([]byte, 32))
	if e != nil {
		t.Fatal(e)
	}
	encrypted, e := s.Seal(context.Background(), "workspace-a:bot-1", []byte("synthetic-secret"))
	if e != nil {
		t.Fatal(e)
	}
	plain, e := s.Open(context.Background(), "workspace-a:bot-1", encrypted)
	if e != nil || string(plain) != "synthetic-secret" {
		t.Fatal(e)
	}
	if _, e = s.Open(context.Background(), "workspace-b:bot-1", encrypted); e == nil {
		t.Fatal("cross-workspace credential decryption")
	}
	encrypted.Ciphertext[len(encrypted.Ciphertext)-1] ^= 1
	if _, e = s.Open(context.Background(), "workspace-a:bot-1", encrypted); e == nil {
		t.Fatal("tamper accepted")
	}
}
func TestLedgerInvariants(t *testing.T) {
	entry := func(a ledger.Asset, n int64) ledger.Entry {
		return ledger.Entry{AccountID: "account", Money: ledger.Money{Asset: a, AmountAtomic: n}}
	}
	for _, c := range []struct {
		name    string
		entries []ledger.Entry
		valid   bool
	}{{"balanced", []ledger.Entry{entry(ledger.USD, 100), entry(ledger.USD, -100)}, true}, {"unbalanced", []ledger.Entry{entry(ledger.USD, 100), entry(ledger.USD, -99)}, false}, {"cross-asset", []ledger.Entry{entry(ledger.USD, 100), entry(ledger.IRR, -100)}, false}, {"zero", []ledger.Entry{entry(ledger.USD, 0), entry(ledger.USD, 0)}, false}, {"overflow", []ledger.Entry{entry(ledger.USD, math.MaxInt64), entry(ledger.USD, 1), entry(ledger.USD, -1)}, false}} {
		t.Run(c.name, func(t *testing.T) {
			if (ledger.Validate(c.entries) == nil) != c.valid {
				t.Fatal("incorrect balance validation")
			}
		})
	}
}
func TestPluginDependencies(t *testing.T) {
	r := botmodules.Defaults()
	if e := r.Validate(map[string]bool{"subscription": true}); e == nil {
		t.Fatal("missing payment dependency")
	}
	if e := r.Validate(map[string]bool{"subscription": true, "payments": true}); e != nil {
		t.Fatal(e)
	}
	if _, e := botmodules.New([]botmodules.Module{{Key: "a", Version: "1", Dependencies: []string{"b"}}, {Key: "b", Version: "1", Dependencies: []string{"a"}}}); e == nil {
		t.Fatal("cycle accepted")
	}
}
func TestAccessGateMatrix(t *testing.T) {
	member := accessgates.Membership{Status: "member"}
	left := accessgates.Membership{Status: "left"}
	for _, c := range []struct {
		name       string
		mode       accessgates.Mode
		n          int
		ms         []accessgates.Membership
		allow, err bool
	}{{"zero", accessgates.All, 1, nil, false, true}, {"single", accessgates.All, 1, []accessgates.Membership{member}, true, false}, {"all-mixed", accessgates.All, 1, []accessgates.Membership{member, left}, false, false}, {"any", accessgates.Any, 1, []accessgates.Membership{member, left}, true, false}, {"minimum", accessgates.Minimum, 2, []accessgates.Membership{member, member, left}, true, false}, {"kicked", accessgates.All, 1, []accessgates.Membership{{Status: "kicked"}}, false, false}, {"restricted-member", accessgates.All, 1, []accessgates.Membership{{Status: "restricted", IsMember: true}}, true, false}, {"API-error", accessgates.Any, 1, []accessgates.Membership{member, {Err: errors.New("timeout")}}, false, true}} {
		t.Run(c.name, func(t *testing.T) {
			ok, e := accessgates.Evaluate(true, false, c.mode, c.n, c.ms)
			if ok != c.allow || (e != nil) != c.err {
				t.Fatal(ok, e)
			}
		})
	}
	if ok, _ := accessgates.Evaluate(false, false, accessgates.All, 0, nil); !ok {
		t.Fatal("disabled gate")
	}
	if ok, _ := accessgates.Evaluate(true, true, accessgates.All, 0, nil); !ok {
		t.Fatal("bypass")
	}
}
func TestWorkflowPolicies(t *testing.T) {
	if tickets.Transition("CLOSED", "OPEN") != nil || tickets.Transition("CLOSED", "IN_PROGRESS") == nil {
		t.Fatal("ticket reopening")
	}
	if downloads.Transition("UPLOADING", "COMPLETED") != nil || downloads.Transition("QUEUED", "COMPLETED") == nil {
		t.Fatal("download transitions")
	}
	for _, raw := range []string{"https://youtube.com.evil.test/v", "http://youtube.com/v", "https://127.0.0.1/v", "https://user@youtube.com/v", "https://youtube.com:443/v"} {
		if _, e := downloads.Provider(raw); e == nil {
			t.Fatal("unsafe URL accepted", raw)
		}
	}
	if _, e := downloads.Provider("https://www.youtube.com/watch?v=fixture"); e != nil {
		t.Fatal(e)
	}
	if _, e := downloads.Select(downloads.FixtureFormats(), "1080p"); e == nil {
		t.Fatal("invented quality")
	}
	if uploads.Transition("STORED", "RECEIVED") == nil {
		t.Fatal("invalid upload transition")
	}
	p := uploads.Policy{Enabled: true, MaxBytes: 10, AllowedTypes: map[string]bool{"document": true}}
	if p.Validate(11, "document") == nil || p.Validate(1, "exe") == nil || p.Validate(10, "document") != nil {
		t.Fatal("upload constraints")
	}
}
func TestBroadcastAndRBAC(t *testing.T) {
	for _, plan := range []string{"FREE", "PRO", "BUSINESS"} {
		if !broadcasts.Matches(plan, broadcasts.Audience{Plan: plan}) || broadcasts.Matches(plan, broadcasts.Audience{Plan: plan, Blocked: true}) {
			t.Fatal("segmentation")
		}
	}
	if broadcasts.Matches("CUSTOM", broadcasts.Audience{}) {
		t.Fatal("unknown audience must fail closed")
	}
	for _, mode := range []string{"SEND", "COPY", "FORWARD"} {
		if !broadcasts.ValidMode(mode) {
			t.Fatal(mode)
		}
	}
	if broadcasts.RetryableDelivery("UNKNOWN") || broadcasts.RetryableDelivery("DELIVERED") {
		t.Fatal("uncertain delivery retried")
	}
	if broadcasts.Transition("RUNNING", "PAUSED") != nil || broadcasts.Transition("COMPLETED", "RUNNING") == nil {
		t.Fatal("broadcast states")
	}
	if rbac.Allows(map[string]bool{"tickets.read": true}, "wallet.adjust") || rbac.CanRemoveOwner(1, true) {
		t.Fatal("privilege escalation/self lockout")
	}
}
func TestPaymentVerification(t *testing.T) {
	now := time.Now()
	i := payments.Intent{ID: "intent", Recipient: "merchant", QuoteID: "quote", Money: ledger.Money{Asset: ledger.TON, AmountAtomic: 100}, ExpiresAt: now.Add(time.Minute)}
	c := payments.Confirmation{Provider: "ton", Reference: "reference", Recipient: i.Recipient, Status: "CONFIRMED", QuoteID: i.QuoteID, Money: i.Money, Confirmed: true, ObservedAt: now}
	if e := payments.Validate(i, c, now); e != nil {
		t.Fatal(e)
	}
	c.Recipient = "attacker"
	if payments.Validate(i, c, now) == nil {
		t.Fatal("wrong recipient credited")
	}
	c.Recipient = i.Recipient
	c.Money.Asset = ledger.USDT
	if payments.Validate(i, c, now) == nil {
		t.Fatal("wrong asset credited")
	}
	if payments.RailAllowed("TELEGRAM", "DIGITAL", ledger.TON) || !payments.RailAllowed("TELEGRAM", "DIGITAL", ledger.XTR) {
		t.Fatal("Telegram Stars policy")
	}
}
func TestPersianMainMenu(t *testing.T) {
	b, _ := json.Marshal(identity.MainMenu(false, "https://example.test"))
	if !strings.Contains(string(b), "داشبورد") || strings.Contains(string(b), "پنل مدیریت") || strings.Contains(string(b), "web_app") {
		t.Fatal("main menu authorization/localization")
	}
	b, _ = json.Marshal(identity.MainMenu(true, ""))
	if !strings.Contains(string(b), "پنل مدیریت") {
		t.Fatal("admin menu")
	}
	b, _ = json.Marshal(identity.PanelMarkup("https://example.test"))
	if !strings.Contains(string(b), localization.Text("menu.open")) || !strings.Contains(string(b), "web_app") || !strings.Contains(string(b), "https://example.test") {
		t.Fatal("inline panel entry")
	}
}
