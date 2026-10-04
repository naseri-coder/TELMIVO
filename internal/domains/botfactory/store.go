package botfactory

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/commerce"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"strings"
)

type StoreFilter struct{ Search, Category, Status, After string }
type StorePage struct {
	Items []Item `json:"items"`
	Next  string `json:"next_cursor"`
}

func FilterStore(items []Item, f StoreFilter) (StorePage, error) {
	out := StorePage{Items: []Item{}}
	if len(f.Search) > 100 || len(f.Category) > 100 || len(f.Status) > 30 || len(f.After) > 100 {
		return out, database.ErrInvalid
	}
	for _, p := range items {
		if !p.Store.Metadata.Visible || p.ID <= f.After || f.Category != "" && p.Store.Metadata.Category != f.Category || f.Status != "" && p.Status != f.Status {
			continue
		}
		if !strings.Contains(strings.ToLower(p.Name+" "+p.ID+" "+p.Description+" "+p.Store.Metadata.Summary), strings.ToLower(f.Search)) {
			continue
		}
		// A Store response never transports saved plugin configuration or private settings.
		p.Configuration = nil
		out.Items = append(out.Items, p)
		if len(out.Items) == 25 {
			out.Next = p.ID
			break
		}
	}
	return out, nil
}
func (s *Service) Store(ctx context.Context, actor, account, bot string, f StoreFilter) (StorePage, error) {
	var out StorePage
	if bot == "" {
		return out, database.ErrInvalid
	}
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
		out, e = FilterStore(items, f)
		return e
	})
	return out, e
}
func (s *Service) StoreDetail(ctx context.Context, actor, account, bot, id string) (Item, error) {
	var out Item
	if bot == "" {
		return out, database.ErrInvalid
	}
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
		found := false
		for _, p := range items {
			if p.ID == id && p.Store.Metadata.Visible {
				out = p
				out.Configuration = nil
				found = true
				break
			}
		}
		if !found {
			return database.ErrForbidden
		}
		manifest, e := s.Registry.Manifest(id)
		if e != nil {
			return e
		}
		for metric := range snap.Catalog[commerce.QuotaKind] {
			if metric == manifest.Extensions.QuotaPolicy || strings.HasPrefix(metric, id+".") {
				q, e := s.Commercial.QuotaTx(ctx, tx, actor, account, bot, metric, 1, false)
				if e != nil {
					return e
				}
				out.Store.Quotas[metric] = q
			}
		}
		return nil
	})
	return out, e
}

// Registered prevents catalog-only records becoming executable marketplace packages.
func (s *Service) Registered(id string) bool {
	_, e := s.Registry.Manifest(id)
	return sdk.ValidID(id) && e == nil
}
