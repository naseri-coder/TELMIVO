package botfactory

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/commerce"
	platform "github.com/naseri-coder/TELMIVO/internal/domains/pluginplatform"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
)

type txPolicy struct {
	s  *commerce.Service
	tx pgx.Tx
}

func (p txPolicy) Authorize(ctx context.Context, in sdk.Admission) (sdk.Grant, error) {
	return p.s.AuthorizeTx(ctx, p.tx, in)
}
func (s *Service) pluginService(tx pgx.Tx) *platform.Service {
	return &platform.Service{DB: s.DB, Registry: s.Registry, Policy: txPolicy{s.Commercial, tx}, Hooks: s.Hooks}
}
func (s *Service) List(ctx context.Context, actor, account string) ([]Bot, error) {
	out := []Bot{}
	e := s.DB.Tenant(ctx, actor, account, false, func(tx pgx.Tx) error {
		if e := permission(ctx, tx, actor, account); e != nil {
			return e
		}
		rs, e := tx.Query(ctx, "SELECT id::text,name,status FROM customer_bots WHERE workspace_id=$1 ORDER BY created_at,id", account)
		if e != nil {
			return e
		}
		defer rs.Close()
		for rs.Next() {
			var b Bot
			if e = rs.Scan(&b.ID, &b.Name, &b.Status); e != nil {
				return e
			}
			out = append(out, b)
		}
		return rs.Err()
	})
	return out, e
}
func (s *Service) Create(ctx context.Context, actor, account, correlation string, in Create) (Bot, error) {
	var out Bot
	in.Name = strings.TrimSpace(in.Name)
	if !ValidInput(in) || len(in.Selection.Requested) > 64 {
		return out, database.ErrInvalid
	}
	if s.Credentials == nil {
		return out, database.ErrForbidden
	}
	// Hash only; raw tokens/fingerprints are never projected into responses or audit.
	fingerprint := digest(struct {
		Name, Token string
		Selection   Selection
		Accepted    []string
	}{in.Name, in.Token, in.Selection, in.Accepted})
	e := s.DB.Tenant(ctx, actor, account, false, func(tx pgx.Tx) error {
		if e := permission(ctx, tx, actor, account); e != nil {
			return e
		}
		if e := commercialLocks(ctx, tx, account, true); e != nil {
			return e
		}
		var saved string
		e := tx.QueryRow(ctx, `SELECT id::text,name,status,COALESCE(factory_fingerprint,'') FROM customer_bots WHERE workspace_id=$1 AND idempotency_key=$2`, account, in.Key).Scan(&out.ID, &out.Name, &out.Status, &saved)
		if e == nil {
			if saved != fingerprint {
				return database.ErrConflict
			}
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		review, e := s.preview(ctx, tx, account, "", in.Selection)
		if e != nil {
			return e
		}
		if !review.Ready {
			return &Rejected{Review: review}
		}
		accepted := append([]string{}, in.Accepted...)
		sort.Strings(accepted)
		order := append([]string{}, review.Order...)
		sort.Strings(order)
		if !reflect.DeepEqual(accepted, order) || in.ReviewDigest != review.Digest {
			return database.ErrConflict
		}
		if e = tx.QueryRow(ctx, "SELECT gen_random_uuid()::text").Scan(&out.ID); e != nil {
			return e
		}
		encrypted, e := s.Credentials.Seal(ctx, account+":"+out.ID, []byte(in.Token))
		if e != nil {
			return database.ErrInvalid
		}
		out.Name = in.Name
		out.Status = "DRAFT"
		if _, e = tx.Exec(ctx, `INSERT INTO customer_bots(id,workspace_id,name,username,key_id,encrypted_token,status,idempotency_key,factory_fingerprint,factory_token_digest) VALUES($1,$2,$3,$4,$5,$6,'DRAFT',$7,$8,$9)`, out.ID, account, in.Name, "pending_"+strings.ReplaceAll(out.ID, "-", ""), encrypted.KeyID, encrypted.Ciphertext, in.Key, fingerprint, digest(in.Token)); e != nil {
			return e
		}
		core := s.pluginService(tx)
		for _, id := range review.Order {
			config := in.Selection.Configurations[id]
			if len(config) == 0 {
				config = json.RawMessage(`{}`)
			}
			i, e := core.ChangeTx(ctx, tx, actor, sdk.Scope{AccountID: account, BotID: out.ID, PluginID: id}, sdk.Install, 0, config, correlation)
			if e != nil {
				return e
			}
			if in.Selection.Activate {
				if _, e = core.ChangeTx(ctx, tx, actor, i.Scope, sdk.Activate, i.Revision, nil, correlation); e != nil {
					return e
				}
			}
		}
		return database.Audit(ctx, tx, actor, account, "bot.factory.create", "customer_bot", out.ID, correlation, map[string]any{}, map[string]any{"bot": out, "plugins": review.Order, "activated": in.Selection.Activate})
	})
	if e != nil {
		return Bot{}, e
	}
	return out, nil
}

type Change struct {
	Action        sdk.Action      `json:"action"`
	Expected      int64           `json:"expected_revision"`
	Configuration json.RawMessage `json:"configuration,omitempty"`
}

// Impact is server-authoritative and non-mutating, including active dependents.
func (s *Service) Impact(ctx context.Context, actor, account, bot, id string, in Change) (Review, error) {
	var r Review
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
		items, e := s.catalog(ctx, tx, account, bot, snap)
		if e != nil {
			return e
		}
		r = Review{Requested: []string{id}, Required: []string{}, Order: []string{id}, Plugins: []Item{}, Issues: []Issue{}, Ready: true}
		active := map[string]bool{}
		found := false
		for _, item := range items {
			active[item.ID] = item.State == sdk.Active
			if item.ID == id {
				found = true
				r.Plugins = append(r.Plugins, item)
				if _, e = platform.Transition(item.State, in.Action); e != nil {
					r.Issues = append(r.Issues, Issue{Code: "LIFECYCLE_INVALID", Plugin: id})
				}
				if item.Revision != in.Expected {
					r.Issues = append(r.Issues, Issue{Code: "STALE_REVISION", Plugin: id})
				}
				if in.Action == sdk.Install || in.Action == sdk.Activate {
					if !item.Installable {
						r.Issues = append(r.Issues, Issue{Code: item.Reason, Plugin: id})
					}
					if in.Action == sdk.Install {
						m, _ := s.Registry.Manifest(id)
						if e = m.Settings.ValidateConfiguration(in.Configuration); e != nil {
							r.Issues = append(r.Issues, Issue{Code: "CONFIGURATION_REQUIRED", Plugin: id})
						}
					}
				}
			}
		}
		if in.Action == sdk.Install {
			target, _ := s.Registry.Manifest(id)
			for _, dep := range target.Dependencies {
				for _, item := range items {
					if item.ID == dep.PluginID && (item.State == sdk.Available || item.State == sdk.Uninstalled || item.State == sdk.UninstallPending) {
						r.Required = append(r.Required, item.ID)
						r.Plugins = append(r.Plugins, item)
						r.Issues = append(r.Issues, Issue{Code: "DEPENDENCY_NOT_INSTALLED", Plugin: id, Other: item.ID})
					}
				}
			}
		}
		if !found {
			return database.ErrInvalid
		}
		switch in.Action {
		case sdk.Activate:
			active[id] = true
		case sdk.Disable, sdk.RequestUninstall, sdk.CompleteUninstall:
			active[id] = false
		}
		for _, item := range items {
			if !active[item.ID] {
				continue
			}
			for _, dep := range item.Dependencies {
				if !active[dep] {
					r.Issues = append(r.Issues, Issue{Code: "ACTIVE_DEPENDENT", Plugin: item.ID, Other: dep})
				}
			}
			for _, other := range item.Conflicts {
				if active[other] {
					r.Issues = append(r.Issues, Issue{Code: "PLUGIN_CONFLICT", Plugin: item.ID, Other: other})
				}
			}
		}
		r.Ready = len(r.Issues) == 0
		r.Digest = digest(r)
		return nil
	})
	return r, e
}
func (s *Service) Change(ctx context.Context, actor, account, bot, id, correlation string, in Change) (Item, error) {
	var out Item
	e := s.DB.Tenant(ctx, actor, account, false, func(tx pgx.Tx) error {
		if e := permission(ctx, tx, actor, account); e != nil {
			return e
		}
		if e := commercialLocks(ctx, tx, account, true); e != nil {
			return e
		}
		if in.Action == sdk.Install {
			m, e := s.Registry.Manifest(id)
			if e != nil {
				return e
			}
			for _, dep := range m.Dependencies {
				var installed bool
				if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM plugin_installations WHERE workspace_id=$1 AND bot_id=$2 AND plugin_id=$3 AND state IN('INSTALLED','ACTIVE','DISABLED'))", account, bot, dep.PluginID).Scan(&installed); e != nil {
					return e
				}
				if !installed {
					return sdk.ErrDependency
				}
			}
		}
		if _, e := s.pluginService(tx).ChangeTx(ctx, tx, actor, sdk.Scope{AccountID: account, BotID: bot, PluginID: id}, in.Action, in.Expected, in.Configuration, correlation); e != nil {
			return e
		}
		snap, e := s.snapshot(ctx, tx, account, bot)
		if e != nil {
			return e
		}
		items, e := s.catalog(ctx, tx, account, bot, snap)
		if e != nil {
			return e
		}
		for _, item := range items {
			if item.ID == id {
				out = item
			}
		}
		return nil
	})
	return out, e
}
