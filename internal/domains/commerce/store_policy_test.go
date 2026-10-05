package commerce

import (
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"testing"
	"time"
)

func TestStorePolicySeparation(t *testing.T) {
	now := time.Now().UTC()
	v := Version{Kind: PluginKind, Key: "forms", Enabled: true, EffectiveAt: now, Definition: Definition{Plugin: &Plugin{Scope: sdk.PerBot, SaleEnabled: false, Prices: Prices{Daily: Money{Unit: "USD", Amount: 9007199254740993}, Monthly: Money{Unit: "USD", Amount: 1}}, Trial: TrialPolicy{Enabled: true, DurationSeconds: 60}}, Metadata: map[string]string{"store.operational": "MAINTENANCE", "store.visible": "false"}}}
	c := Catalog{PluginKind: {"forms": v}, PlanKind: {"FREE": {Enabled: true, Definition: Definition{Plan: &Plan{Plugins: map[string]Inclusion{"forms": PaidAddon}}}}}}
	p := ProjectPolicy(Snapshot{Catalog: c, Plan: "FREE", Now: now, Consumed: map[string]bool{}}, "account", "bot", "forms")
	if p.Decision.Allowed || p.Metadata.Visible || p.Metadata.Operational != "MAINTENANCE" || p.SaleEnabled || p.Prices.Daily.Amount != 9007199254740993 {
		t.Fatal(p)
	}
	plan := c[PlanKind]["FREE"]
	plan.Definition.Plan.Plugins["forms"] = Included
	c[PlanKind]["FREE"] = plan
	p = ProjectPolicy(Snapshot{Catalog: c, Plan: "FREE", Now: now}, "account", "bot", "forms")
	if !p.Decision.Allowed || p.SaleEnabled || p.Metadata.Operational != "MAINTENANCE" {
		t.Fatal("entitlement must remain independent of sale/operations", p)
	}
	for _, meta := range []map[string]string{{"store.visible": "yes"}, {"store.operational": "RUN_CODE"}, {"store.icon": "https://evil"}, {"store.unknown": "x"}} {
		v.Definition.Metadata = meta
		if v.Validate() {
			t.Fatal("invalid metadata", meta)
		}
	}
	v.Definition.Metadata = nil
	v.Definition.Plugin.Prices.Daily.Amount = -1
	if v.Validate() {
		t.Fatal("negative money")
	}
}
func TestStoreTrialAndScopeProjection(t *testing.T) {
	now := time.Now()
	s := Snapshot{Now: now, Consumed: map[string]bool{"forms": true}, Catalog: Catalog{}, Agreements: []Agreement{{ID: "trial", Account: "a", Bot: "one", Scope: sdk.PerBot, Source: TrialSource, State: Active, StartsAt: now.Add(-time.Hour), EndsAt: ptrTime(now.Add(-time.Minute)), Terms: Terms{Effects: Effects{Plugins: []string{"forms"}}}}}}
	p := ProjectPolicy(s, "a", "two", "forms")
	if len(p.Contributions) != 0 {
		t.Fatal("cross bot leak")
	}
	p = ProjectPolicy(s, "a", "one", "forms")
	if p.TrialState != "EXPIRED" {
		t.Fatal(p)
	}
}
func ptrTime(t time.Time) *time.Time { return &t }
