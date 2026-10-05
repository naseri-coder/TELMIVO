package pluginplatform

import (
	"errors"
	"reflect"
	"testing"

	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
)

func demo(id string, deps ...string) sdk.Manifest {
	m := sdk.Manifest{SchemaVersion: 1, PluginID: id, Name: id, Version: "1.0.0", CoreVersions: sdk.VersionRange{Minimum: "0.1.0", MaximumExclusive: "1.0.0"}, DataVersion: 1, Settings: sdk.SettingsSchema{Properties: map[string]sdk.Setting{}}}
	for _, d := range deps {
		m.Dependencies = append(m.Dependencies, sdk.Dependency{PluginID: d, Versions: sdk.VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"}})
	}
	return m
}

func TestRegistryRegistrationAndResolution(t *testing.T) {
	a, b, c := demo("a"), demo("b", "a"), demo("c", "b", "a")
	r, err := NewRegistry("0.3.0", []sdk.Manifest{c, a, b})
	if err != nil {
		t.Fatal(err)
	}
	order, err := r.Resolve([]string{"c", "a"})
	if err != nil || !reflect.DeepEqual(order, []string{"a", "b", "c"}) {
		t.Fatal(order, err)
	}
	if !reflect.DeepEqual(r.IDs(), order) {
		t.Fatal(r.IDs())
	}
	a.Settings.Properties["mutable"] = sdk.Setting{Type: "boolean"}
	copy, _ := r.Manifest("a")
	if len(copy.Settings.Properties) != 0 {
		t.Fatal("registration alias")
	}
	copy.Settings.Properties["mutable"] = sdk.Setting{Type: "boolean"}
	again, _ := r.Manifest("a")
	if len(again.Settings.Properties) != 0 {
		t.Fatal("retrieval alias")
	}
}

func TestRegistryRejectsInvalidGraphs(t *testing.T) {
	badVersion := demo("b", "a")
	badVersion.Dependencies[0].Versions.Minimum = "1.1.0"
	optionalVersion := demo("b")
	optionalVersion.OptionalDependencies = badVersion.Dependencies
	for _, c := range []struct {
		name      string
		core      string
		manifests []sdk.Manifest
		want      error
	}{
		{"duplicate", "0.3.0", []sdk.Manifest{demo("a"), demo("a")}, sdk.ErrManifest},
		{"missing", "0.3.0", []sdk.Manifest{demo("b", "a")}, sdk.ErrDependency},
		{"cycle", "0.3.0", []sdk.Manifest{demo("a", "b"), demo("b", "a")}, sdk.ErrCycle},
		{"self-cycle", "0.3.0", []sdk.Manifest{demo("a", "a")}, sdk.ErrCycle},
		{"dependency-version", "0.3.0", []sdk.Manifest{demo("a"), badVersion}, sdk.ErrVersion},
		{"optional-version", "0.3.0", []sdk.Manifest{demo("a"), optionalVersion}, sdk.ErrVersion},
		{"core-version", "1.0.0", []sdk.Manifest{demo("a")}, sdk.ErrVersion},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewRegistry(c.core, c.manifests); !errors.Is(err, c.want) {
				t.Fatal(err)
			}
		})
	}
}

func TestOptionalDependenciesAndConflicts(t *testing.T) {
	a := demo("a")
	a.OptionalDependencies = []sdk.Dependency{{PluginID: "absent", Versions: sdk.VersionRange{Minimum: "1.0.0"}}}
	a.Conflicts = []string{"b"}
	r, err := NewRegistry("0.3.0", []sdk.Manifest{a, demo("b")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Resolve([]string{"a"}); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Resolve([]string{"b", "a"}); !errors.Is(err, sdk.ErrConflict) {
		t.Fatal(err)
	}
	if err = r.ValidateActive(map[string]bool{"a": true, "b": true}); !errors.Is(err, sdk.ErrConflict) {
		t.Fatal(err)
	}
	if err = r.ValidateActive(map[string]bool{"missing": true}); !errors.Is(err, sdk.ErrDependency) {
		t.Fatal(err)
	}
}

func TestDependencyActivationAndDisableOrdering(t *testing.T) {
	r, err := NewRegistry("0.3.0", []sdk.Manifest{demo("a"), demo("b", "a"), demo("c", "b")})
	if err != nil {
		t.Fatal(err)
	}
	for _, active := range []map[string]bool{{"b": true}, {"a": true, "c": true}, {"a": false, "b": true}} {
		if !errors.Is(r.ValidateActive(active), sdk.ErrDependency) {
			t.Fatal("unsafe order")
		}
	}
	if r.ValidateActive(map[string]bool{"a": true, "b": true, "c": true}) != nil || r.ValidateActive(map[string]bool{"a": true}) != nil {
		t.Fatal("safe order rejected")
	}
}

func TestLifecycleTransitions(t *testing.T) {
	for _, c := range []struct {
		from   sdk.State
		action sdk.Action
		to     sdk.State
	}{
		{sdk.Available, sdk.Install, sdk.Installed}, {sdk.Installed, sdk.Activate, sdk.Active}, {sdk.Active, sdk.Disable, sdk.Disabled}, {sdk.Disabled, sdk.Activate, sdk.Active}, {sdk.Installed, sdk.Disable, sdk.Disabled}, {sdk.Installed, sdk.RequestUninstall, sdk.UninstallPending}, {sdk.Disabled, sdk.RequestUninstall, sdk.UninstallPending}, {sdk.UninstallPending, sdk.CompleteUninstall, sdk.Uninstalled},
	} {
		to, err := Transition(c.from, c.action)
		if err != nil || to != c.to {
			t.Fatal(c, to, err)
		}
	}
	for _, c := range []struct {
		from   sdk.State
		action sdk.Action
	}{{sdk.Available, sdk.Activate}, {sdk.Active, sdk.RequestUninstall}, {sdk.Active, sdk.CompleteUninstall}, {sdk.Disabled, sdk.CompleteUninstall}, {sdk.Uninstalled, sdk.Install}, {sdk.UninstallPending, sdk.Activate}} {
		if _, err := Transition(c.from, c.action); !errors.Is(err, sdk.ErrState) {
			t.Fatal(c, err)
		}
	}
}
