package commerce

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"strings"
	"unicode/utf8"
)

// Store metadata is versioned alongside commercial policy, never executable code.
type StoreMetadata struct {
	Visible     bool   `json:"visible"`
	Category    string `json:"category"`
	Summary     string `json:"summary"`
	Features    string `json:"features"`
	Icon        string `json:"icon"`
	Operational string `json:"operational"`
}

func Metadata(d Definition) StoreMetadata {
	m := StoreMetadata{Visible: true, Category: "general", Operational: "OPERATIONAL", Icon: "plugin"}
	if d.Metadata["store.visible"] == "false" {
		m.Visible = false
	}
	for k, p := range map[string]*string{"category": &m.Category, "summary": &m.Summary, "features": &m.Features, "icon": &m.Icon, "operational": &m.Operational} {
		if v := d.Metadata["store."+k]; v != "" {
			*p = v
		}
	}
	return m
}
func validMetadata(d Definition) bool {
	if len(d.Metadata) > 32 {
		return false
	}
	for k, v := range d.Metadata {
		if len(k) > 100 || !utf8.ValidString(v) || len(v) > 4000 {
			return false
		}
		if !strings.HasPrefix(k, "store.") {
			continue
		}
		if d.Plugin == nil {
			return false
		}
		switch k {
		case "store.visible":
			if v != "true" && v != "false" {
				return false
			}
		case "store.operational":
			if v != "OPERATIONAL" && v != "MAINTENANCE" && v != "UNAVAILABLE" {
				return false
			}
		case "store.category", "store.icon":
			if !identifier.MatchString(v) {
				return false
			}
		case "store.summary":
			if len(v) > 500 {
				return false
			}
		case "store.features":
			if len(v) > 4000 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

type Contribution struct {
	ID     string `json:"id"`
	Source Source `json:"source"`
	State  State  `json:"state"`
	Scope  string `json:"scope"`
}
type EffectivePolicy struct {
	Decision      Decision               `json:"decision"`
	Plan          string                 `json:"plan"`
	Inclusion     Inclusion              `json:"inclusion"`
	Prices        *Prices                `json:"prices,omitempty"`
	SaleEnabled   bool                   `json:"sale_enabled"`
	Metadata      StoreMetadata          `json:"metadata"`
	Trial         TrialPolicy            `json:"trial"`
	TrialState    string                 `json:"trial_state"`
	GraceSeconds  int64                  `json:"grace_seconds"`
	Contributions []Contribution         `json:"contributions"`
	Quotas        map[string]QuotaResult `json:"quotas"`
}

func ProjectPolicy(s Snapshot, account, bot, plugin string) EffectivePolicy {
	p := EffectivePolicy{Decision: Evaluate(s.Catalog, s.Agreements, s.Plan, account, bot, plugin, s.Now), Plan: s.Plan, Inclusion: Unavailable, TrialState: "INELIGIBLE", Contributions: []Contribution{}, Quotas: map[string]QuotaResult{}}
	if v, ok := s.Catalog.Get(PluginKind, plugin); ok {
		d := v.Definition.Plugin
		p.Prices = &d.Prices
		p.SaleEnabled = d.SaleEnabled
		p.Metadata = Metadata(v.Definition)
		p.Trial = d.Trial
		p.GraceSeconds = d.GraceSeconds
		if plan, ok := s.Catalog.Get(PlanKind, s.Plan); ok {
			if x, exists := plan.Definition.Plan.Plugins[plugin]; exists {
				p.Inclusion = x
			}
		}
		if d.Trial.Enabled && !s.Consumed[plugin] && (p.Inclusion == Included || p.Inclusion == PaidAddon) {
			p.TrialState = "ELIGIBLE"
		}
		if s.Consumed[plugin] {
			p.TrialState = "CONSUMED"
		}
	}
	for _, a := range s.Agreements {
		if a.Account != account || !contains(a.Terms.Effects.Plugins, plugin) || (a.Scope == "PER_BOT" && a.Bot != bot) {
			continue
		}
		state := CommercialState(a, s.Now)
		p.Contributions = append(p.Contributions, Contribution{a.ID, a.Source, state, string(a.Scope)})
		if a.Source == TrialSource {
			p.TrialState = string(state)
		}
	}
	return p
}

// Effective is a read-only Super Admin view using the same evaluator and meter.
func (s *Service) Effective(ctx context.Context, actor, account, bot, plugin string) (EffectivePolicy, error) {
	var out EffectivePolicy
	e := s.DB.Tenant(ctx, actor, account, true, func(tx pgx.Tx) error {
		if !database.PlatformPermission(ctx, tx, actor, "platform.commercial.read") {
			return database.ErrForbidden
		}
		if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock_shared(7100213)"); e != nil {
			return e
		}
		snap, e := s.SnapshotTx(ctx, tx, account, bot)
		if e != nil {
			return e
		}
		out = ProjectPolicy(snap, account, bot, plugin)
		for metric := range snap.Catalog[QuotaKind] {
			q, e := s.QuotaTx(ctx, tx, actor, account, bot, metric, 1, false)
			if e != nil {
				return e
			}
			out.Quotas[metric] = q
		}
		return nil
	})
	return out, e
}
