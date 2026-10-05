// Package shop defines the customer commerce plugin, separate from platform billing.
package shop

import (
	"github.com/naseri-coder/TELMIVO/internal/platform/ledger"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/plugins/crm"
	"math"
	"strings"
)

const ID = "commerce"

type Product struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Enabled     bool         `json:"enabled"`
	Digital     bool         `json:"digital"`
	Price       ledger.Money `json:"price"`
	Revision    int64        `json:"revision"`
}

func (p Product) Valid() bool {
	_, ok := ledger.Scales[p.Price.Asset]
	return ok && p.Price.AmountAtomic >= 0 && strings.TrimSpace(p.Name) != "" && len(p.Name) <= 100 && len(p.Description) <= 1000
}

type Item struct {
	Product  string `json:"product"`
	Quantity int64  `json:"quantity"`
}
type Line struct {
	Product  string       `json:"product"`
	Name     string       `json:"name"`
	Revision int64        `json:"product_revision"`
	Quantity int64        `json:"quantity"`
	Digital  bool         `json:"digital"`
	Price    ledger.Money `json:"price"`
	Total    ledger.Money `json:"total"`
}
type Order struct {
	ID       string       `json:"id"`
	Contact  string       `json:"contact"`
	State    string       `json:"state"`
	Lines    []Line       `json:"lines"`
	Total    ledger.Money `json:"total"`
	Revision int64        `json:"revision"`
}

func Snapshot(items []Item, products map[string]Product) ([]Line, ledger.Money, bool) {
	lines := []Line{}
	total := ledger.Money{}
	seen := map[string]bool{}
	if len(items) < 1 || len(items) > 20 {
		return nil, total, false
	}
	for _, i := range items {
		p, ok := products[i.Product]
		if !ok || !p.Valid() || !p.Enabled || seen[i.Product] || i.Quantity < 1 || i.Quantity > 1000 {
			return nil, total, false
		}
		seen[i.Product] = true
		if total.Asset == "" {
			total.Asset = p.Price.Asset
		}
		if total.Asset != p.Price.Asset || p.Price.AmountAtomic > math.MaxInt64/i.Quantity {
			return nil, total, false
		}
		n := p.Price.AmountAtomic * i.Quantity
		if total.AmountAtomic > math.MaxInt64-n {
			return nil, total, false
		}
		total.AmountAtomic += n
		lines = append(lines, Line{p.ID, p.Name, p.Revision, i.Quantity, p.Digital, p.Price, ledger.Money{Asset: p.Price.Asset, AmountAtomic: n}})
	}
	return lines, total, true
}
func Transition(from, to string) bool {
	switch from {
	case "DRAFT":
		return to == "PLACED" || to == "CANCELLED"
	case "PLACED":
		return to == "CONFIRMED" || to == "CANCELLED"
	case "CONFIRMED":
		return to == "FULFILLED" || to == "CANCELLED"
	}
	return false
}
func Manifest() sdk.Manifest {
	return sdk.Manifest{SchemaVersion: 1, PluginID: ID, Name: "فروشگاه", Description: "محصول، سفارش و checkout با قیمت تاریخی و rail پرداخت قابل‌اعتبارسنجی", Version: "1.0.0", CoreVersions: sdk.VersionRange{Minimum: "0.3.0", MaximumExclusive: "1.0.0"}, DataVersion: 1, Permissions: []string{"commerce.manage", "commerce.order", "crm.write"}, Dependencies: []sdk.Dependency{{PluginID: crm.ID, Versions: sdk.VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"}}}, Settings: sdk.SettingsSchema{Properties: map[string]sdk.Setting{}}, Extensions: sdk.Extensions{PricingPolicy: ID, QuotaPolicy: "commerce.orders.monthly"}}
}
