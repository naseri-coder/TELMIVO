package testfixtures

import (
	platform "github.com/naseri-coder/TELMIVO/internal/domains/pluginplatform"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
)

// FactoryRegistry is injected only behind the test+synthetic configuration guard.
// It defines metadata-only acceptance fixtures, not production business plugins.
func FactoryRegistry() (*platform.Registry, error) {
	ids := []string{"factory-basic", "factory-dependent", "factory-paid", "factory-conflict", "factory-account"}
	ms := []sdk.Manifest{}
	for _, id := range ids {
		m := sdk.Manifest{SchemaVersion: 1, PluginID: id, Name: "آزمایش " + id, Description: "پلاگین آزمایشی برای پذیرش ایزوله", Version: "1.0.0", CoreVersions: sdk.VersionRange{Minimum: "0.1.0", MaximumExclusive: "1.0.0"}, DataVersion: 1, Permissions: []string{"telegram.send"}, Settings: sdk.SettingsSchema{Properties: map[string]sdk.Setting{}}}
		if id == "factory-dependent" {
			m.Dependencies = []sdk.Dependency{{PluginID: "factory-basic", Versions: sdk.VersionRange{Minimum: "1.0.0"}}}
		}
		if id == "factory-conflict" {
			m.Conflicts = []string{"factory-basic"}
		}
		ms = append(ms, m)
	}
	return platform.NewRegistry("0.3.0", ms)
}
