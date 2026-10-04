package pluginplatform

import (
	"sort"

	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
)

// Registry is immutable after construction and contains one deployed version per ID.
// Conflicting alternatives may both be available; they may not both be active.
type Registry struct {
	coreVersion string
	manifests   map[string]sdk.Manifest
}

func NewRegistry(coreVersion string, manifests []sdk.Manifest) (*Registry, error) {
	if !sdk.ValidVersion(coreVersion) {
		return nil, sdk.ErrVersion
	}
	r := &Registry{coreVersion: coreVersion, manifests: map[string]sdk.Manifest{}}
	for _, m := range manifests {
		if err := m.Validate(coreVersion); err != nil {
			return nil, err
		}
		if _, exists := r.manifests[m.PluginID]; exists {
			return nil, sdk.ErrManifest
		}
		r.manifests[m.PluginID] = m.Clone()
	}
	// Validate all declared edges, including optional edges that are present, and
	// detect cycles even when no bot has activated any plugin yet.
	color := map[string]int{}
	var walk func(string) error
	walk = func(id string) error {
		if color[id] == 1 {
			return sdk.ErrCycle
		}
		if color[id] == 2 {
			return nil
		}
		color[id] = 1
		m := r.manifests[id]
		for i, ds := range [][]sdk.Dependency{m.Dependencies, m.OptionalDependencies} {
			for _, d := range ds {
				dep, ok := r.manifests[d.PluginID]
				if !ok {
					if i == 1 {
						continue
					}
					return sdk.ErrDependency
				}
				if !d.Versions.Allows(dep.Version) {
					return sdk.ErrVersion
				}
				if err := walk(d.PluginID); err != nil {
					return err
				}
			}
		}
		color[id] = 2
		return nil
	}
	for _, id := range r.IDs() {
		if err := walk(id); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *Registry) IDs() []string {
	ids := make([]string, 0, len(r.manifests))
	for id := range r.manifests {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (r *Registry) Manifest(id string) (sdk.Manifest, error) {
	m, ok := r.manifests[id]
	if !ok {
		return sdk.Manifest{}, sdk.ErrManifest
	}
	return m.Clone(), nil
}

// Resolve returns dependency-first, deterministic required closure. It never
// installs or activates dependencies, and optional dependencies are never implied.
func (r *Registry) Resolve(ids []string) ([]string, error) {
	roots := append([]string(nil), ids...)
	sort.Strings(roots)
	seen := map[string]bool{}
	var out []string
	var walk func(string) error
	walk = func(id string) error {
		if seen[id] {
			return nil
		}
		m, ok := r.manifests[id]
		if !ok {
			return sdk.ErrDependency
		}
		seen[id] = true
		ds := append([]sdk.Dependency(nil), m.Dependencies...)
		sort.Slice(ds, func(i, j int) bool { return ds[i].PluginID < ds[j].PluginID })
		for _, d := range ds {
			if err := walk(d.PluginID); err != nil {
				return err
			}
		}
		out = append(out, id)
		return nil
	}
	for _, id := range roots {
		if err := walk(id); err != nil {
			return nil, err
		}
	}
	set := map[string]bool{}
	for _, id := range out {
		set[id] = true
	}
	if err := r.ValidateActive(set); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Registry) ValidateActive(active map[string]bool) error {
	ids := make([]string, 0, len(active))
	for id, on := range active {
		if on {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		m, ok := r.manifests[id]
		if !ok {
			return sdk.ErrDependency
		}
		for _, d := range m.Dependencies {
			if !active[d.PluginID] {
				return sdk.ErrDependency
			}
		}
		for _, c := range m.Conflicts {
			if active[c] {
				return sdk.ErrConflict
			}
		}
	}
	return nil
}
