package plugincontract

import (
	"encoding/json"
	"testing"
)

func manifest() Manifest {
	return Manifest{SchemaVersion: 1, PluginID: "demo", Name: "Demo", Version: "1.2.0", CoreVersions: VersionRange{Minimum: "0.1.0", MaximumExclusive: "1.0.0"}, DataVersion: 1, Permissions: []string{"telegram.send"}, Settings: SettingsSchema{Properties: map[string]Setting{"title": {Type: "string", MaxLength: 20}}, Required: []string{"title"}}}
}

func TestManifestValidation(t *testing.T) {
	for _, c := range []struct {
		name   string
		mutate func(*Manifest)
		core   string
		valid  bool
	}{
		{"valid", func(*Manifest) {}, "0.3.0", true},
		{"future-schema", func(m *Manifest) { m.SchemaVersion = 2 }, "0.3.0", false},
		{"identity", func(m *Manifest) { m.PluginID = "../demo" }, "0.3.0", false},
		{"core-too-old", func(*Manifest) {}, "0.0.9", false},
		{"core-upper-bound", func(*Manifest) {}, "1.0.0", false},
		{"malformed-version", func(m *Manifest) { m.Version = "latest" }, "0.3.0", false},
		{"undeclared-catalog-capability", func(m *Manifest) { m.Permissions = []string{"database.root"} }, "0.3.0", false},
		{"duplicate-permission", func(m *Manifest) { m.Permissions = []string{"telegram.send", "telegram.send"} }, "0.3.0", false},
		{"unsupported-hook", func(m *Manifest) { m.LifecycleHooks = []Action{"CHARGE"} }, "0.3.0", false},
		{"migration-version", func(m *Manifest) { m.DataVersion = 0 }, "0.3.0", false},
		{"commercial-references", func(m *Manifest) {
			m.Extensions = Extensions{PricingPolicy: "price.demo", PlanPolicy: "plans.demo", TrialPolicy: "trial.demo", QuotaPolicy: "quota.demo", BillingScopePolicy: "licensing.demo", BundleKeys: []string{"bundle.demo"}, Marketplace: &Marketplace{DeveloperID: "developer.demo", ListingID: "listing.demo"}}
		}, "0.3.0", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := manifest()
			c.mutate(&m)
			if (m.Validate(c.core) == nil) != c.valid {
				t.Fatal("incorrect validation")
			}
		})
	}
}

func TestVersionRanges(t *testing.T) {
	r := VersionRange{Minimum: "1.2.3", MaximumExclusive: "2.0.0"}
	for _, raw := range []string{"1.2.3", "1.10.0", "1.99.99"} {
		if !r.Allows(raw) {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{"1.2.2", "2.0.0", "01.2.3", "1.2", "1.2.3-beta", "+1.2.3", "4294967296.0.0"} {
		if r.Allows(raw) {
			t.Fatal(raw)
		}
	}
	for _, r := range []VersionRange{{}, {Minimum: "2.0.0", MaximumExclusive: "1.0.0"}, {Minimum: "1.0.0", MaximumExclusive: "1.0.0"}} {
		if r.Validate() == nil {
			t.Fatal("invalid range")
		}
	}
}

func TestSettingsSchema(t *testing.T) {
	lo, hi := int64(1), int64(10)
	s := SettingsSchema{Properties: map[string]Setting{"title": {Type: "string", MaxLength: 4, Enum: []string{"سلام"}}, "enabled": {Type: "boolean"}, "count": {Type: "integer", Minimum: &lo, Maximum: &hi}}, Required: []string{"title"}}
	for _, raw := range []string{`{"title":"سلام"}`, `{"title":"سلام","count":10,"enabled":true}`} {
		if s.ValidateConfiguration(json.RawMessage(raw)) != nil {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{`{}`, `null`, `[]`, `{"title":null}`, `{"title":"other"}`, `{"title":"سلام","count":11}`, `{"title":"سلام","count":1.5}`, `{"title":"سلام","enabled":"true"}`, `{"title":"سلام","root":true}`, `{"title":"سلام"} {}`} {
		if s.ValidateConfiguration(json.RawMessage(raw)) == nil {
			t.Fatal(raw)
		}
	}
	s.Properties["unknown"] = Setting{Type: "object"}
	if s.Validate() == nil {
		t.Fatal("unsupported schema dialect")
	}
}

func TestGrantDeclarationBoundary(t *testing.T) {
	m := manifest()
	for _, g := range []Grant{{Capabilities: []string{"telegram.send"}, LicenseScope: PerBot}, {LicenseScope: PerAccount}} {
		if ValidateGrant(m, g) != nil {
			t.Fatal(g)
		}
	}
	for _, g := range []Grant{{Capabilities: []string{"files.write"}, LicenseScope: PerBot}, {Capabilities: []string{"database.root"}, LicenseScope: PerBot}, {Capabilities: []string{"telegram.send", "telegram.send"}, LicenseScope: PerAccount}, {LicenseScope: "GLOBAL"}} {
		if ValidateGrant(m, g) == nil {
			t.Fatal("unauthorized grant")
		}
	}
}

func TestManifestDefensiveCopy(t *testing.T) {
	m := manifest()
	copy := m.Clone()
	copy.Settings.Properties["title"] = Setting{Type: "integer"}
	copy.Permissions[0] = "files.write"
	if m.Settings.Properties["title"].Type != "string" || m.Permissions[0] != "telegram.send" {
		t.Fatal("shared mutable metadata")
	}
}

func TestStrictManifestDecoding(t *testing.T) {
	raw, _ := json.Marshal(manifest())
	if _, err := DecodeManifest(raw, "0.3.0"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`{"schema_version":1,"schema_version":2}`, `{"unknown_policy":"grant-all"}`, string(raw) + `{}`, `null`} {
		if _, err := DecodeManifest([]byte(bad), "0.3.0"); err == nil {
			t.Fatal(bad)
		}
	}
	if manifest().Settings.ValidateConfiguration(json.RawMessage(`{"title":"first","title":"last"}`)) == nil {
		t.Fatal("ambiguous configuration")
	}
}
