package botfactory

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/commerce"
	platform "github.com/naseri-coder/TELMIVO/internal/domains/pluginplatform"
	cred "github.com/naseri-coder/TELMIVO/internal/platform/crypto"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
)

type Service struct {
	DB          *database.Store
	Registry    *platform.Registry
	Commercial  *commerce.Service
	Credentials cred.CredentialStore
	Hooks       sdk.LifecycleValidator
}

func permission(ctx context.Context, tx pgx.Tx, actor, account string) error {
	if !database.Permission(ctx, tx, actor, account, "bots.manage") {
		return database.ErrForbidden
	}
	return nil
}
func commercialLocks(ctx context.Context, tx pgx.Tx, account string, write bool) error {
	lock := "pg_advisory_xact_lock_shared"
	if write {
		lock = "pg_advisory_xact_lock"
	}
	if _, e := tx.Exec(ctx, "SELECT "+lock+"(7100213)"); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, "SELECT "+lock+"(hashtextextended($1,7100213))", account)
	return e
}
func (s *Service) snapshot(ctx context.Context, tx pgx.Tx, account, bot string) (commerce.Snapshot, error) {
	return s.Commercial.SnapshotTx(ctx, tx, account, bot)
}
func (s *Service) catalog(ctx context.Context, tx pgx.Tx, account, bot string, snap commerce.Snapshot) ([]Item, error) {
	out := []Item{}
	states := map[string]struct {
		state         sdk.State
		revision      int64
		configuration json.RawMessage
	}{}
	if bot != "" {
		rs, e := tx.Query(ctx, `SELECT plugin_id,state,revision,configuration FROM plugin_installations WHERE workspace_id=$1 AND bot_id=$2`, account, bot)
		if e != nil {
			return nil, e
		}
		for rs.Next() {
			var id string
			var st sdk.State
			var rev int64
			var config []byte
			if e = rs.Scan(&id, &st, &rev, &config); e != nil {
				rs.Close()
				return nil, e
			}
			states[id] = struct {
				state         sdk.State
				revision      int64
				configuration json.RawMessage
			}{st, rev, config}
		}
		e = rs.Err()
		rs.Close()
		if e != nil {
			return nil, e
		}
	}
	for _, id := range s.Registry.IDs() {
		m, _ := s.Registry.Manifest(id)
		d := commerce.Evaluate(snap.Catalog, snap.Agreements, snap.Plan, account, bot, id, snap.Now)
		item := Item{ID: id, Name: m.Name, Description: m.Description, Version: m.Version, Scope: d.Scope, Status: "UNAVAILABLE", Entitled: d.Allowed, Reason: d.Code, Plan: snap.Plan, Dependencies: []string{}, Conflicts: append([]string{}, m.Conflicts...), State: sdk.Available, Settings: m.Settings, PolicyRevision: d.Revision}
		for _, dep := range m.Dependencies {
			item.Dependencies = append(item.Dependencies, dep.PluginID)
		}
		sort.Strings(item.Dependencies)
		if st, ok := states[id]; ok {
			item.State = st.state
			item.Revision = st.revision
			item.Configuration = st.configuration
		}
		if p, ok := snap.Catalog.Get(commerce.PluginKind, id); ok {
			def := p.Definition.Plugin
			item.Prices = &def.Prices
			item.Scope = def.Scope
			item.TrialSeconds = def.Trial.DurationSeconds
			item.TrialEligible = def.Trial.Enabled && !snap.Consumed[id]
			plan, yes := snap.Catalog.Get(commerce.PlanKind, snap.Plan)
			policy := commerce.Unavailable
			if yes {
				policy = plan.Definition.Plan.Plugins[id]
			}
			item.Included = policy == commerce.Included
			switch {
			case item.Included && d.Allowed:
				item.Status = "INCLUDED"
			case d.Allowed:
				item.Status = "ALREADY_ENTITLED"
			case policy != commerce.PaidAddon && policy != commerce.Included:
				item.Status = "UNAVAILABLE"
			case !def.SaleEnabled:
				item.Status = "SALE_DISABLED"
			case item.TrialEligible:
				item.Status = "TRIAL_AVAILABLE"
			default:
				item.Status = "PAID"
			}
			item.TrialEligible = item.TrialEligible && (policy == commerce.Included || policy == commerce.PaidAddon)
			item.Installable = d.Allowed
			for _, cap := range m.Permissions {
				if !sdk.Contains(def.Capabilities, cap) {
					item.Installable = false
					item.Reason = "CAPABILITY_DENIED"
				}
			}
		}
		item.Store = commerce.ProjectPolicy(snap, account, bot, id)
		if item.Store.Metadata.Operational != "OPERATIONAL" {
			item.Installable = false
			item.Reason = "OPERATIONAL_UNAVAILABLE"
		}
		for _, dep := range item.Dependencies {
			st, ok := states[dep]
			if !ok || st.state != sdk.Active {
				item.DependencyBlocked = true
			}
		}
		for _, other := range item.Conflicts {
			if states[other].state == sdk.Active {
				item.ConflictBlocked = true
			}
		}
		out = append(out, item)
	}
	return out, nil
}
func (s *Service) Catalog(ctx context.Context, actor, account, bot string) ([]Item, error) {
	var out []Item
	e := s.DB.Tenant(ctx, actor, account, false, func(tx pgx.Tx) error {
		if e := permission(ctx, tx, actor, account); e != nil {
			return e
		}
		if e := commercialLocks(ctx, tx, account, false); e != nil {
			return e
		}
		snap, e := s.snapshot(ctx, tx, account, bot)
		if e != nil {
			return e
		}
		out, e = s.catalog(ctx, tx, account, bot, snap)
		return e
	})
	return out, e
}
func (s *Service) preview(ctx context.Context, tx pgx.Tx, account, bot string, in Selection) (Review, error) {
	r := Review{Requested: append([]string{}, in.Requested...), Required: []string{}, Order: []string{}, Plugins: []Item{}, Issues: []Issue{}, Ready: true}
	if len(in.Requested) > 64 {
		return r, database.ErrInvalid
	}
	sort.Strings(r.Requested)
	seen := map[string]bool{}
	for _, id := range r.Requested {
		if seen[id] {
			return r, database.ErrInvalid
		}
		seen[id] = true
		if _, e := s.Registry.Manifest(id); e != nil {
			r.Issues = append(r.Issues, Issue{Code: "UNKNOWN_PLUGIN", Plugin: id})
		}
	}

	if len(r.Issues) > 0 {
		r.Ready = false
		r.Digest = digest(struct {
			Review    Review
			Selection Selection
		}{r, in})
		return r, nil
	}
	order, e := s.Registry.Resolve(r.Requested)
	if errors.Is(e, sdk.ErrConflict) {
		// Resolve intentionally rejects conflicting closures. Reconstruct only for
		// read-only review so customers see both sides; commit remains rejected.
		visited := map[string]bool{}
		var walk func(string)
		walk = func(id string) {
			if visited[id] {
				return
			}
			visited[id] = true
			m, _ := s.Registry.Manifest(id) // Immutable registry validates dependencies.
			deps := append([]sdk.Dependency(nil), m.Dependencies...)
			sort.Slice(deps, func(i, j int) bool { return deps[i].PluginID < deps[j].PluginID })
			for _, d := range deps {
				walk(d.PluginID)
			}
			order = append(order, id)
		}
		for _, id := range r.Requested {
			walk(id)
		}
	} else if e != nil {
		return r, e
	}
	r.Order = append(r.Order, order...)
	chosen := map[string]bool{}
	for _, id := range order {
		chosen[id] = true
		if !seen[id] {
			r.Required = append(r.Required, id)
		}
	}
	for id := range in.Configurations {
		if !chosen[id] {
			return r, database.ErrInvalid
		}
	}
	snap, e := s.snapshot(ctx, tx, account, bot)
	if e != nil {
		return r, e
	}
	items, e := s.catalog(ctx, tx, account, bot, snap)
	if e != nil {
		return r, e
	}
	for _, item := range items {
		if !chosen[item.ID] {
			continue
		}
		r.Plugins = append(r.Plugins, item)
		if item.State == sdk.Uninstalled || item.State == sdk.UninstallPending {
			r.Issues = append(r.Issues, Issue{Code: "LIFECYCLE_INVALID", Plugin: item.ID})
		}
		if !item.Installable {
			code := item.Reason
			if item.Status == "PAID" || item.Status == "SALE_DISABLED" || item.Status == "TRIAL_AVAILABLE" {
				code = "ACCESS_REQUIRED"
			}
			r.Issues = append(r.Issues, Issue{Code: code, Plugin: item.ID})
		}
		m, _ := s.Registry.Manifest(item.ID)
		config := in.Configurations[item.ID]
		if len(config) == 0 {
			config = json.RawMessage(`{}`)
		}
		if item.State != sdk.Available && item.State != sdk.Uninstalled {
			config = item.Configuration
		}
		if e = m.Settings.ValidateConfiguration(config); e != nil {
			r.Issues = append(r.Issues, Issue{Code: "CONFIGURATION_REQUIRED", Plugin: item.ID})
		}
		for _, other := range item.Conflicts {
			if chosen[other] {
				r.Issues = append(r.Issues, Issue{Code: "PLUGIN_CONFLICT", Plugin: item.ID, Other: other})
			}
		}
	}
	r.Ready = len(r.Issues) == 0
	r.Digest = digest(struct {
		Review    Review
		Selection Selection
	}{r, in})
	return r, nil
}
func (s *Service) Preview(ctx context.Context, actor, account, bot string, in Selection) (Review, error) {
	var r Review
	e := s.DB.Tenant(ctx, actor, account, false, func(tx pgx.Tx) error {
		if e := permission(ctx, tx, actor, account); e != nil {
			return e
		}
		if e := commercialLocks(ctx, tx, account, false); e != nil {
			return e
		}
		var e error
		r, e = s.preview(ctx, tx, account, bot, in)
		return e
	})
	return r, e
}
