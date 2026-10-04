package plugincontract

import "encoding/json"

// KnownCapability is an explicit allowlist; callers cannot expand it through manifests.
func KnownCapability(s string) bool {
	switch s {
	case "telegram.send", "telegram.read_member_status", "files.read", "files.write", "tickets.read", "tickets.write", "ai.invoke", "workflow.execute":
		return true
	}
	return false
}

func (m Manifest) Validate(coreVersion string) error {
	if m.SchemaVersion != 1 || !ValidID(m.PluginID) || m.Name == "" || len(m.Name) > 200 || len(m.Description) > 2000 || m.DataVersion < 1 {
		return ErrManifest
	}
	if !ValidVersion(m.Version) || !m.CoreVersions.Allows(coreVersion) {
		return ErrVersion
	}
	if err := m.Settings.Validate(); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, list := range [][]Dependency{m.Dependencies, m.OptionalDependencies} {
		for _, d := range list {
			if !ValidID(d.PluginID) || seen[d.PluginID] || d.Versions.Validate() != nil {
				return ErrDependency
			}
			seen[d.PluginID] = true
		}
	}
	seen = map[string]bool{}
	for _, c := range m.Conflicts {
		if !ValidID(c) || c == m.PluginID || seen[c] {
			return ErrManifest
		}
		seen[c] = true
	}
	seen = map[string]bool{}
	for _, c := range m.Permissions {
		if !KnownCapability(c) || seen[c] {
			return ErrPermission
		}
		seen[c] = true
	}
	hooks := map[Action]bool{}
	for _, h := range m.LifecycleHooks {
		if hooks[h] {
			return ErrManifest
		}
		switch h {
		case Install, Activate, Disable, RequestUninstall, CompleteUninstall:
		default:
			return ErrManifest
		}
		hooks[h] = true
	}
	seen = map[string]bool{}
	for _, k := range m.MigrationKeys {
		if !ValidID(k) || seen[k] {
			return ErrManifest
		}
		seen[k] = true
	}
	for _, k := range []string{m.Extensions.PricingPolicy, m.Extensions.PlanPolicy, m.Extensions.TrialPolicy, m.Extensions.QuotaPolicy, m.Extensions.BillingScopePolicy} {
		if k != "" && !ValidID(k) {
			return ErrManifest
		}
	}
	for _, k := range m.Extensions.BundleKeys {
		if !ValidID(k) {
			return ErrManifest
		}
	}
	if x := m.Extensions.Marketplace; x != nil && (!ValidID(x.DeveloperID) || !ValidID(x.ListingID)) {
		return ErrManifest
	}
	return nil
}

// Clone prevents a caller or plugin from mutating registered metadata through shared maps.
func (m Manifest) Clone() Manifest {
	b, _ := json.Marshal(m)
	var out Manifest
	_ = json.Unmarshal(b, &out)
	return out
}

func ValidateGrant(m Manifest, g Grant) error {
	if g.LicenseScope != PerBot && g.LicenseScope != PerAccount {
		return ErrPermission
	}
	seen := map[string]bool{}
	for _, c := range g.Capabilities {
		if !KnownCapability(c) || !Contains(m.Permissions, c) || seen[c] {
			return ErrPermission
		}
		seen[c] = true
	}
	return nil
}
