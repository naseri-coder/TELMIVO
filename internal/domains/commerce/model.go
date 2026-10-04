// Package commerce owns commercial policy, never plugin lifecycle or payment charging.
package commerce

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"regexp"
	"time"

	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
)

type Kind string

const (
	PlanKind   Kind = "PLAN"
	PluginKind Kind = "PLUGIN"
	BundleKind Kind = "BUNDLE"
	AddonKind  Kind = "ADDON"
	QuotaKind  Kind = "QUOTA"
)

type Inclusion string

const (
	Unavailable Inclusion = "UNAVAILABLE"
	PaidAddon   Inclusion = "PAID_ADDON"
	Included    Inclusion = "INCLUDED"
)

type Period string

const (
	Daily   Period = "DAILY"
	Monthly Period = "MONTHLY"
	NoReset Period = "NONE"
)

type Source string

const (
	Standalone   Source = "SUBSCRIPTION"
	TrialSource  Source = "TRIAL"
	BundleSource Source = "BUNDLE"
	AddonSource  Source = "ADDON"
	AdminSource  Source = "ADMIN"
)

type State string

const (
	Active    State = "ACTIVE"
	Grace     State = "GRACE"
	Suspended State = "SUSPENDED"
	Cancelled State = "CANCELLED"
	Expired   State = "EXPIRED"
)

// Amount is a JSON string of atomic units: browser clients cannot lose integer precision.
// Unit identifies the denomination; Scale is its decimal exponent, not an FX conversion.
type Money struct {
	Unit   string `json:"unit"`
	Scale  int    `json:"scale"`
	Amount int64  `json:"amount,string"`
}
type Prices struct {
	Daily   Money `json:"daily"`
	Monthly Money `json:"monthly"`
}
type Limit struct {
	Value     int64 `json:"value,string"`
	Unlimited bool  `json:"unlimited"`
}
type TrialPolicy struct {
	Enabled         bool  `json:"enabled"`
	DurationSeconds int64 `json:"duration_seconds"`
}
type Plan struct {
	Name    string               `json:"name"`
	Order   int                  `json:"order"`
	Plugins map[string]Inclusion `json:"plugins"`
	Limits  map[string]Limit     `json:"limits"`
}
type Plugin struct {
	Capabilities []string         `json:"capabilities"`
	SaleEnabled  bool             `json:"sale_enabled"`
	Scope        sdk.LicenseScope `json:"scope"`
	Prices       Prices           `json:"prices"`
	Trial        TrialPolicy      `json:"trial"`
	GraceSeconds int64            `json:"grace_seconds"`
}
type Product struct {
	Name         string           `json:"name"`
	Scope        sdk.LicenseScope `json:"scope"`
	SaleEnabled  bool             `json:"sale_enabled"`
	Prices       Prices           `json:"prices"`
	Plugins      []string         `json:"plugins"`
	Quotas       map[string]int64 `json:"quotas"`
	GraceSeconds int64            `json:"grace_seconds"`
}
type Quota struct {
	Scope   sdk.LicenseScope `json:"scope"`
	Reset   Period           `json:"reset"`
	Default Limit            `json:"default"`
}
type Definition struct {
	Plan     *Plan             `json:"plan,omitempty"`
	Plugin   *Plugin           `json:"plugin,omitempty"`
	Product  *Product          `json:"product,omitempty"`
	Quota    *Quota            `json:"quota,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}
type Version struct {
	Kind        Kind       `json:"kind"`
	Key         string     `json:"key"`
	Version     int64      `json:"version"`
	Enabled     bool       `json:"enabled"`
	EffectiveAt time.Time  `json:"effective_at"`
	Definition  Definition `json:"definition"`
}
type CatalogChange struct {
	Version  Version `json:"catalog"`
	Expected int64   `json:"expected_version"`
	Reason   string  `json:"reason"`
}
type Catalog map[Kind]map[string]Version

func (c Catalog) Get(k Kind, key string) (Version, bool) {
	v, ok := c[k][key]
	return v, ok && v.Enabled
}

type Effects struct {
	Plugins []string         `json:"plugins"`
	Quotas  map[string]int64 `json:"quotas"`
}
type Terms struct {
	Catalog      Version `json:"catalog"`
	Period       Period  `json:"period"`
	Price        *Money  `json:"price,omitempty"`
	Effects      Effects `json:"effects"`
	GraceSeconds int64   `json:"grace_seconds"`
}
type Agreement struct {
	ID         string           `json:"id"`
	Account    string           `json:"account"`
	Bot        string           `json:"bot,omitempty"`
	Scope      sdk.LicenseScope `json:"scope"`
	Source     Source           `json:"source"`
	State      State            `json:"state"`
	StartsAt   time.Time        `json:"starts_at"`
	EndsAt     *time.Time       `json:"ends_at,omitempty"`
	GraceUntil *time.Time       `json:"grace_until,omitempty"`
	Terms      Terms            `json:"terms"`
	Revision   int64            `json:"revision"`
}

// Issue is a future payment-provider fulfillment boundary. Only commercial admins
// may call it today. No user-supplied price, duration or entitlement effects are accepted.
type Issue struct {
	Key         string     `json:"key"`
	Kind        Kind       `json:"kind"`
	CatalogKey  string     `json:"catalog_key"`
	Bot         string     `json:"bot,omitempty"`
	Period      Period     `json:"period"`
	Reason      string     `json:"reason"`
	AdminGrant  bool       `json:"admin_grant"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	RepeatTrial bool       `json:"repeat_trial"`
}
type Decision struct {
	Allowed  bool             `json:"allowed"`
	Code     string           `json:"code"`
	Sources  []string         `json:"sources"`
	Scope    sdk.LicenseScope `json:"scope,omitempty"`
	Revision string           `json:"policy_revision,omitempty"`
}
type Denied struct{ Decision Decision }

func (d *Denied) Error() string   { return d.Decision.Code }
func (d *Denied) Is(e error) bool { return e == sdk.ErrPermission }

type Clock interface{ Now() time.Time }
type UTCClock struct{}

func (UTCClock) Now() time.Time { return time.Now().UTC() }

var identifier = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,99}$`)
var unit = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,23}$`)

func scopeValid(s sdk.LicenseScope) bool { return s == sdk.PerBot || s == sdk.PerAccount }
func secondsValid(n int64) bool          { return n >= 0 && n <= int64((365*24*time.Hour)/time.Second) }
func (p Prices) Validate() bool {
	return unit.MatchString(p.Daily.Unit) && p.Daily.Scale >= 0 && p.Daily.Scale <= 18 && p.Daily.Amount >= 0 && p.Monthly.Amount >= 0 && p.Daily.Unit == p.Monthly.Unit && p.Daily.Scale == p.Monthly.Scale
}
func validLimit(l Limit) bool { return l.Value >= 0 && (!l.Unlimited || l.Value == 0) }
func (v Version) Validate() bool {
	if !identifier.MatchString(v.Key) || v.EffectiveAt.IsZero() || v.Version < 0 {
		return false
	}
	d := v.Definition
	n := 0
	for _, b := range []bool{d.Plan != nil, d.Plugin != nil, d.Product != nil, d.Quota != nil} {
		if b {
			n++
		}
	}
	if n != 1 || !validMetadata(d) {
		return false
	}
	switch v.Kind {
	case PlanKind:
		if d.Plan == nil || d.Plan.Name == "" {
			return false
		}
		for k, x := range d.Plan.Plugins {
			if !identifier.MatchString(k) || (x != Unavailable && x != PaidAddon && x != Included) {
				return false
			}
		}
		for k, l := range d.Plan.Limits {
			if !identifier.MatchString(k) || !validLimit(l) {
				return false
			}
		}
	case PluginKind:
		if !sdk.ValidID(v.Key) {
			return false
		}
		p := d.Plugin
		if p == nil || !scopeValid(p.Scope) || !p.Prices.Validate() || !secondsValid(p.GraceSeconds) || !secondsValid(p.Trial.DurationSeconds) || (p.Trial.Enabled && p.Trial.DurationSeconds == 0) {
			return false
		}
		seen := map[string]bool{}
		for _, cap := range p.Capabilities {
			if !sdk.KnownCapability(cap) || seen[cap] {
				return false
			}
			seen[cap] = true
		}
	case BundleKind, AddonKind:
		p := d.Product
		if p == nil || p.Name == "" || !scopeValid(p.Scope) || !p.Prices.Validate() || !secondsValid(p.GraceSeconds) || (len(p.Plugins) == 0 && len(p.Quotas) == 0) {
			return false
		}
		seen := map[string]bool{}
		for _, k := range p.Plugins {
			if !identifier.MatchString(k) || seen[k] {
				return false
			}
			seen[k] = true
		}
		for k, n := range p.Quotas {
			if !identifier.MatchString(k) || n <= 0 {
				return false
			}
		}
	case QuotaKind:
		p := d.Quota
		if p == nil || !scopeValid(p.Scope) || !validLimit(p.Default) || (p.Reset != Daily && p.Reset != Monthly && p.Reset != NoReset) {
			return false
		}
	default:
		return false
	}
	return true
}
func decodeStrict(b []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e := d.Decode(out); e != nil {
		return e
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("extra JSON")
	}
	return nil
}
func add(a, b int64) (int64, bool) {
	if b < 0 || a < 0 || a > math.MaxInt64-b {
		return 0, false
	}
	return a + b, true
}
