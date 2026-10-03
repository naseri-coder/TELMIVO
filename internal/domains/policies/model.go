package policies

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"time"

	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/ledger"
)

// Typed version bodies share immutable version identity, not mutable commercial state.
type Definition struct {
	Scope              string                `json:"scope,omitempty"`
	Duration           int64                 `json:"duration_seconds,omitempty"`
	Eligibility        string                `json:"eligibility,omitempty"`
	Trigger            string                `json:"trigger,omitempty"`
	Once               bool                  `json:"once,omitempty"`
	Stacking           string                `json:"stacking,omitempty"`
	Fallback           string                `json:"fallback,omitempty"`
	GrantKind          string                `json:"grant_kind,omitempty"`
	MaxSubjects        int64                 `json:"max_subjects_per_user,omitempty"`
	Features           []string              `json:"features,omitempty"`
	Plugins            []string              `json:"plugins,omitempty"`
	Capabilities       []string              `json:"capabilities,omitempty"`
	Limits             map[string]LimitGrant `json:"limits,omitempty"`
	Metric             string                `json:"metric,omitempty"`
	Unit               string                `json:"unit,omitempty"`
	Window             string                `json:"window,omitempty"`
	Timezone           string                `json:"timezone,omitempty"`
	WindowSeconds      int64                 `json:"window_seconds,omitempty"`
	Limit              int64                 `json:"limit,omitempty"`
	Mode               string                `json:"mode,omitempty"`
	Strategy           string                `json:"strategy,omitempty"`
	Source             string                `json:"source,omitempty"`
	ReservationSeconds int64                 `json:"reservation_seconds,omitempty"`
	CountFailure       bool                  `json:"count_failure,omitempty"`
	Asset              string                `json:"asset,omitempty"`
	AmountAtomic       string                `json:"amount_atomic,omitempty"`
	Scale              int                   `json:"scale,omitempty"`
	Destination        string                `json:"destination,omitempty"`
	PriceKey           string                `json:"price_key,omitempty"`
	PriceVersion       int64                 `json:"price_version,omitempty"`
	Campaign           string                `json:"campaign,omitempty"`
	Reasons            []string              `json:"reasons,omitempty"`
	Offers             []string              `json:"offers,omitempty"`
}
type LimitGrant struct {
	Amount   int64  `json:"amount"`
	Strategy string `json:"strategy"`
}
type Version struct {
	Key            string     `json:"key"`
	Version        int64      `json:"version"`
	Kind           string     `json:"kind"`
	Feature        string     `json:"feature,omitempty"`
	Plugin         string     `json:"plugin,omitempty"`
	Plan           string     `json:"plan,omitempty"`
	Workspace      string     `json:"workspace,omitempty"`
	Enabled        bool       `json:"enabled"`
	Definition     Definition `json:"definition"`
	EffectiveFrom  time.Time  `json:"effective_from"`
	EffectiveUntil *time.Time `json:"effective_until,omitempty"`
	Reason         string     `json:"reason,omitempty"`
}
type Change struct {
	Version
	ExpectedVersion int64 `json:"expected_version"`
}
type Request struct {
	Capability   string           `json:"capability,omitempty"`
	User         string           `json:"user"`
	Workspace    string           `json:"workspace"`
	Role         string           `json:"role"`
	Plan         string           `json:"plan,omitempty"`
	Feature      string           `json:"feature"`
	Plugin       string           `json:"plugin,omitempty"`
	Permission   string           `json:"permission"`
	Group        string           `json:"group,omitempty"`
	Installation string           `json:"installation,omitempty"`
	Requested    int64            `json:"requested"`
	Amounts      map[string]int64 `json:"amounts,omitempty"`
	Now          time.Time        `json:"-"`
}
type Resource struct {
	Primary       bool       `json:"-"`
	Limit         int64      `json:"limit"`
	Used          int64      `json:"used"`
	Reserved      int64      `json:"reserved"`
	Remaining     int64      `json:"remaining"`
	Unit          string     `json:"unit"`
	Window        string     `json:"window"`
	ResetAt       *time.Time `json:"reset_at,omitempty"`
	SafetyCap     *int64     `json:"safety_cap,omitempty"`
	PolicyKey     string     `json:"policy_key"`
	PolicyVersion int64      `json:"policy_version"`
	Bucket        string     `json:"-"`
	Mode          string     `json:"-"`
	TTL           int64      `json:"-"`
	CountFailure  bool       `json:"-"`
}
type Trial struct {
	Key      string    `json:"key"`
	Version  int64     `json:"version"`
	State    string    `json:"state"`
	EndsAt   time.Time `json:"ends_at"`
	Fallback string    `json:"fallback"`
}
type Decision struct {
	Allowed      bool                `json:"allowed"`
	Reason       string              `json:"reason"`
	Feature      string              `json:"feature"`
	Plugin       string              `json:"plugin,omitempty"`
	Limit        *int64              `json:"limit,omitempty"`
	Usage        int64               `json:"usage"`
	Reserved     int64               `json:"reserved"`
	Remaining    *int64              `json:"remaining,omitempty"`
	ResetAt      *time.Time          `json:"reset_at,omitempty"`
	Trials       []Trial             `json:"trials"`
	Resources    map[string]Resource `json:"resources"`
	Capabilities []string            `json:"capabilities"`
	Sources      []string            `json:"sources"`
	Paywall      string              `json:"paywall,omitempty"`
	Offers       []Version           `json:"offers"`
}

var keyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,100}$`)

func one(s string, choices ...string) bool {
	for _, c := range choices {
		if s == c {
			return true
		}
	}
	return false
}
func Validate(v Version) error {
	d := v.Definition
	if !keyPattern.MatchString(v.Key) || v.EffectiveFrom.IsZero() || (v.EffectiveUntil != nil && !v.EffectiveUntil.After(v.EffectiveFrom)) {
		return database.ErrInvalid
	}
	for _, k := range append(append(append([]string{}, d.Features...), d.Plugins...), d.Capabilities...) {
		if !keyPattern.MatchString(k) {
			return database.ErrInvalid
		}
	}
	for _, g := range d.Limits {
		if g.Amount < 0 || !one(g.Strategy, "ADD", "MAX", "MIN", "REPLACE") {
			return database.ErrInvalid
		}
	}
	switch v.Kind {
	case "TRIAL":
		if !one(d.Scope, "PER_USER", "PER_WORKSPACE", "PER_GROUP", "PER_INSTALLATION") || d.Duration <= 0 || d.Duration > 315360000 || !one(d.Eligibility, "NEW_USER", "ANY") || !one(d.Trigger, "DISCOVERY", "GROUP_CLAIM", "INSTALLATION") || !one(d.Stacking, "KEEP_FIRST", "PARALLEL") || !one(d.Fallback, "FREE_LIMITED", "ENTITLEMENT_SUSPENDED") || !one(d.GrantKind, "GLOBAL_TRIAL", "PRODUCT_TRIAL", "TRIAL_EXTENSION", "TEMPORARY_PASS") || d.MaxSubjects < 0 {
			return database.ErrInvalid
		}
	case "GRANT":
		if v.Workspace == "" || !one(d.GrantKind, "TEMPORARY_GRANT", "PROMOTIONAL_GRANT", "TRIAL_EXTENSION", "TEMPORARY_PASS", "ADDON") || v.EffectiveUntil == nil {
			return database.ErrInvalid
		}
	case "QUOTA", "RESOURCE":
		if v.Feature == "" || !keyPattern.MatchString(d.Metric) || d.Limit < 0 || d.Limit > 9007199254740991 || !one(d.Unit, "COUNT", "BYTES", "STORAGE_BYTES", "SECONDS", "JOBS", "CONCURRENT_JOBS", "REQUESTS", "TOKENS", "CREDITS") || !one(d.Window, "NONE", "CALENDAR_DAY", "CALENDAR_WEEK", "CALENDAR_MONTH", "ROLLING_WINDOW") || !one(d.Mode, "USAGE", "CAPACITY", "ITEM", "CONCURRENT") || d.ReservationSeconds <= 0 || d.ReservationSeconds > 86400 {
			return database.ErrInvalid
		}
		if _, e := time.LoadLocation(d.Timezone); e != nil {
			return database.ErrInvalid
		}
		if d.Window == "ROLLING_WINDOW" && (d.WindowSeconds <= 0 || d.WindowSeconds > 315360000) {
			return database.ErrInvalid
		}
		if one(d.Mode, "CAPACITY", "ITEM", "CONCURRENT") && d.Window != "NONE" {
			return database.ErrInvalid
		}
		if v.Kind == "RESOURCE" && (!one(d.Strategy, "ADD", "MAX", "MIN", "REPLACE") || !one(d.Source, "PLAN", "WORKSPACE_OVERRIDE") || (d.Source == "WORKSPACE_OVERRIDE" && v.Workspace == "")) {
			return database.ErrInvalid
		}
	case "PRICE":
		n, e := strconv.ParseInt(d.AmountAtomic, 10, 64)
		scale, ok := ledger.Scales[ledger.Asset(d.Asset)]
		if e != nil || n < 0 || !ok || d.Scale != scale {
			return database.ErrInvalid
		}
	case "OFFER":
		if d.Destination == "" || d.PriceKey == "" || d.PriceVersion < 1 {
			return database.ErrInvalid
		}
	case "PAYWALL":
		if v.Feature == "" || len(d.Reasons) == 0 {
			return database.ErrInvalid
		}
	default:
		return database.ErrInvalid
	}
	return nil
}
func decodeDefinition(raw []byte) (Definition, error) {
	var d Definition
	x := json.NewDecoder(bytes.NewReader(raw))
	x.DisallowUnknownFields()
	e := x.Decode(&d)
	return d, e
}
func Merge(a, b int64, strategy string) (int64, error) {
	if a < 0 || b < 0 {
		return 0, database.ErrInvalid
	}
	switch strategy {
	case "ADD":
		if b > math.MaxInt64-a {
			return 0, database.ErrInvalid
		}
		return a + b, nil
	case "MAX":
		return max(a, b), nil
	case "MIN":
		return min(a, b), nil
	case "REPLACE":
		return b, nil
	}
	return 0, database.ErrInvalid
}

// Calendar periods use local midnight and Monday weeks; DST follows the configured IANA zone.
func Bucket(d Definition, now time.Time) (string, *time.Time, error) {
	if d.Window == "NONE" {
		return "NONE", nil, nil
	}
	if d.Window == "ROLLING_WINDOW" {
		start := now.Add(-time.Duration(d.WindowSeconds) * time.Second)
		return "ROLLING:" + start.UTC().Format(time.RFC3339Nano), nil, nil
	}
	loc, e := time.LoadLocation(d.Timezone)
	if e != nil {
		return "", nil, e
	}
	n := now.In(loc)
	start := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)
	var end time.Time
	switch d.Window {
	case "CALENDAR_DAY":
		end = start.AddDate(0, 0, 1)
	case "CALENDAR_WEEK":
		start = start.AddDate(0, 0, -(int(start.Weekday())+6)%7)
		end = start.AddDate(0, 0, 7)
	case "CALENDAR_MONTH":
		start = time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, loc)
		end = start.AddDate(0, 1, 0)
	default:
		return "", nil, fmt.Errorf("invalid window")
	}
	return d.Window + ":" + d.Timezone + ":" + start.UTC().Format(time.RFC3339), &end, nil
}
