package commerce

import (
	"encoding/json"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"math"
	"testing"
	"time"
)

func testNow() time.Time { return time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC) }
func testPrices(n int64) Prices {
	return Prices{Daily: Money{Unit: "USD", Scale: 2, Amount: n}, Monthly: Money{Unit: "USD", Scale: 2, Amount: n * 20}}
}
func testCatalog() Catalog {
	return Catalog{
		PluginKind: {"one": {Kind: PluginKind, Key: "one", Version: 1, Enabled: true, EffectiveAt: testNow(), Definition: Definition{Plugin: &Plugin{Scope: sdk.PerBot, Prices: testPrices(10)}}}, "two": {Kind: PluginKind, Key: "two", Version: 1, Enabled: true, EffectiveAt: testNow(), Definition: Definition{Plugin: &Plugin{Scope: sdk.PerAccount, Prices: testPrices(0)}}}},
		PlanKind:   {"CUSTOM": {Kind: PlanKind, Key: "CUSTOM", Version: 1, Enabled: true, EffectiveAt: testNow(), Definition: Definition{Plan: &Plan{Name: "Custom", Plugins: map[string]Inclusion{"one": PaidAddon, "two": Included}, Limits: map[string]Limit{"requests": {Value: 10}}}}}},
		QuotaKind:  {"requests": {Kind: QuotaKind, Key: "requests", Version: 1, Enabled: true, EffectiveAt: testNow(), Definition: Definition{Quota: &Quota{Scope: sdk.PerBot, Reset: Daily, Default: Limit{Value: 5}}}}},
	}
}
func TestCommercialValidation(t *testing.T) {
	base := testCatalog()[PluginKind]["one"]
	for _, tc := range []struct {
		name   string
		change func(*Version)
		valid  bool
	}{
		{"valid", func(*Version) {}, true}, {"negative", func(v *Version) { v.Definition.Plugin.Prices.Daily.Amount = -1 }, false},
		{"unit", func(v *Version) { v.Definition.Plugin.Prices.Monthly.Unit = "IRR" }, false}, {"scope", func(v *Version) { v.Definition.Plugin.Scope = "GLOBAL" }, false},
		{"zero_trial", func(v *Version) { v.Definition.Plugin.Trial.Enabled = true }, false}, {"permission", func(v *Version) { v.Definition.Plugin.Capabilities = []string{"root"} }, false},
		{"free", func(v *Version) { v.Definition.Plugin.Prices = testPrices(0) }, true}, {"duplicate_permission", func(v *Version) { v.Definition.Plugin.Capabilities = []string{"files.read", "files.read"} }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := json.Marshal(base)
			var v Version
			_ = json.Unmarshal(b, &v)
			tc.change(&v)
			if v.Validate() != tc.valid {
				t.Fatal(v)
			}
		})
	}
	for _, raw := range []string{`{"unit":"USD","scale":2,"amount":"1.5"}`, `{"unit":"USD","scale":2,"amount":2}`, `{"unit":"USD","scale":2,"amount":"9223372036854775808"}`, `{"unit":"USD","scale":2,"amount":"0","unknown":0}`} {
		var m Money
		if decodeStrict([]byte(raw), &m) == nil {
			t.Fatal("malformed money", raw)
		}
	}
	var p Prices
	if json.Unmarshal([]byte(`{"daily":{"unit":"USD","scale":2,"amount":"-1"},"monthly":{"unit":"USD","scale":2,"amount":"0"}}`), &p) != nil || p.Validate() {
		t.Fatal("negative money")
	}
}
func TestCommercialEntitlementSourcesAndIsolation(t *testing.T) {
	now := testNow()
	end := now.Add(time.Hour)
	c := testCatalog()
	for _, source := range []Source{Standalone, TrialSource, BundleSource, AddonSource, AdminSource} {
		t.Run(string(source), func(t *testing.T) {
			a := Agreement{ID: "grant", Account: "a", Bot: "b", Scope: sdk.PerBot, Source: source, State: Active, StartsAt: now, EndsAt: &end, Terms: Terms{Effects: Effects{Plugins: []string{"one", "two"}}}}
			if d := Evaluate(c, []Agreement{a}, "CUSTOM", "a", "b", "one", now); !d.Allowed || len(d.Sources) != 1 {
				t.Fatal(d)
			}
			if d := Evaluate(c, []Agreement{a}, "CUSTOM", "a", "other", "one", now); d.Allowed || d.Code != "NO_ENTITLEMENT" {
				t.Fatal(d)
			}
			if d := Evaluate(c, []Agreement{a}, "CUSTOM", "other", "b", "one", now); d.Allowed {
				t.Fatal(d)
			}
			a.Scope = sdk.PerAccount
			a.Bot = ""
			if !Evaluate(c, []Agreement{a}, "CUSTOM", "a", "other", "one", now).Allowed {
				t.Fatal("account scope")
			}
			a.State = Cancelled
			if Evaluate(c, []Agreement{a}, "CUSTOM", "a", "b", "one", now).Allowed {
				t.Fatal("cancelled")
			}
		})
	}
	if d := Evaluate(c, nil, "CUSTOM", "a", "b", "two", now); !d.Allowed || d.Sources[0] != "PLAN:CUSTOM" {
		t.Fatal(d)
	}
	v := c[PlanKind]["CUSTOM"]
	v.Definition.Plan.Plugins["one"] = Unavailable
	if d := Evaluate(c, nil, "CUSTOM", "a", "b", "one", now); d.Code != "PLAN_UNAVAILABLE" {
		t.Fatal(d)
	}
	v = c[PluginKind]["one"]
	v.Enabled = false
	c[PluginKind]["one"] = v
	if d := Evaluate(c, nil, "CUSTOM", "a", "b", "one", now); d.Code != "PLUGIN_UNAVAILABLE" {
		t.Fatal(d)
	}
}
func TestCommercialTimeBoundaries(t *testing.T) {
	now := testNow()
	end := now.Add(time.Hour)
	grace := end.Add(time.Hour)
	a := Agreement{State: Active, StartsAt: now, EndsAt: &end, GraceUntil: &grace}
	for _, tc := range []struct {
		at    time.Time
		state State
	}{{now.Add(-time.Nanosecond), Suspended}, {now, Active}, {end, Grace}, {grace, Expired}} {
		if got := CommercialState(a, tc.at); got != tc.state {
			t.Fatal(got, tc.state)
		}
	}
	a.State = Cancelled
	if CommercialState(a, now) != Cancelled {
		t.Fatal("cancelled")
	}
	for _, tc := range []struct {
		at  string
		end string
	}{{"2026-01-31T12:00:00Z", "2026-02-28T12:00:00Z"}, {"2024-01-31T12:00:00Z", "2024-02-29T12:00:00Z"}, {"2026-12-15T00:00:00Z", "2027-01-15T00:00:00Z"}} {
		at, _ := time.Parse(time.RFC3339, tc.at)
		if monthEnd(at).Format(time.RFC3339) != tc.end {
			t.Fatal(monthEnd(at))
		}
	}
	if Bucket(Daily, now) != "2026-01-31" || Bucket(Monthly, now) != "2026-01" || Bucket(NoReset, now) != "lifetime" {
		t.Fatal("UTC buckets")
	}
}
func TestCommercialQuotas(t *testing.T) {
	for _, tc := range []struct {
		name         string
		limit        Limit
		used, amount int64
		allowed      bool
		code         string
	}{
		{"below", Limit{Value: 10}, 8, 1, true, "QUOTA_AVAILABLE"}, {"reaches", Limit{Value: 10}, 8, 2, true, "QUOTA_AVAILABLE"}, {"exceeds", Limit{Value: 10}, 10, 1, false, "QUOTA_EXCEEDED"}, {"unlimited", Limit{Unlimited: true}, 100, 100, true, "QUOTA_AVAILABLE"}, {"overflow", Limit{Unlimited: true}, math.MaxInt64, 1, false, "INVALID_QUOTA"}, {"negative", Limit{Value: 10}, 0, -1, false, "INVALID_QUOTA"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := CheckQuota(tc.limit, tc.used, tc.amount, "bucket")
			if r.Allowed != tc.allowed || r.Code != tc.code {
				t.Fatal(r)
			}
		})
	}
	c := testCatalog()
	a := Agreement{Account: "a", Bot: "b", Scope: sdk.PerBot, Source: AddonSource, State: Active, StartsAt: testNow(), Terms: Terms{Effects: Effects{Quotas: map[string]int64{"requests": 5}}}}
	l, ok := EffectiveLimit(c, []Agreement{a}, "CUSTOM", "a", "b", "requests", testNow())
	if !ok || l.Value != 15 {
		t.Fatal(l, ok)
	}
	l, _ = EffectiveLimit(c, []Agreement{a}, "CUSTOM", "a", "other", "requests", testNow())
	if l.Value != 10 {
		t.Fatal(l)
	}
}
