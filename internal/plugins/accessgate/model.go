// Package accessgate is the reusable membership-policy plugin contract. No DB or credentials.
package accessgate

import (
	"context"
	"encoding/json"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const ID = "access-gate"

type Requirement struct {
	Channel int64  `json:"channel_id"`
	JoinURL string `json:"join_url,omitempty"`
}
type Policy struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Enabled      bool          `json:"enabled"`
	Mode         string        `json:"mode"`
	Requirements []Requirement `json:"requirements"`
	Revision     int64         `json:"revision"`
	Created      time.Time     `json:"created_at"`
	Updated      time.Time     `json:"updated_at"`
}
type Check struct {
	Requirement
	State string `json:"state"`
	Code  string `json:"code,omitempty"`
}
type Decision struct {
	Allowed bool    `json:"allowed"`
	Code    string  `json:"code"`
	Checks  []Check `json:"checks"`
}
type Membership interface {
	Check(context.Context, int64, int64) (string, string)
}
type Request struct {
	Policy string `json:"policy"`
	User   int64  `json:"user"`
}
type Plugin struct{ Runtime sdk.Invoker }

func (p Plugin) Authorize(ctx context.Context, policy string, user int64) (Decision, error) {
	var d Decision
	b, e := p.Runtime.Invoke(ctx, "telegram.read_member_status", mustJSON(Request{policy, user}))
	if e == nil {
		e = json.Unmarshal(b, &d)
	}
	return d, e
}
func mustJSON(v any) json.RawMessage { b, _ := json.Marshal(v); return b }

var channelName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{4,31}$`)

func (p Policy) Valid() bool {
	if strings.TrimSpace(p.Name) == "" || len(p.Name) > 200 || (p.Mode != "ALL" && p.Mode != "ANY") || len(p.Requirements) == 0 || len(p.Requirements) > 20 {
		return false
	}
	seen := map[int64]bool{}
	for _, r := range p.Requirements {
		if r.Channel >= -1000000000000 || r.Channel < -1999999999999 || seen[r.Channel] {
			return false
		}
		seen[r.Channel] = true
		if r.JoinURL != "" {
			u, e := url.Parse(r.JoinURL)
			if e != nil || u.Scheme != "https" || u.Host != "t.me" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || !channelName.MatchString(strings.TrimPrefix(u.Path, "/")) {
				return false
			}
		}
	}
	return true
}
func Evaluate(ctx context.Context, p Policy, user int64, m Membership) Decision {
	d := Decision{Code: "POLICY_DISABLED", Checks: []Check{}}
	if !p.Enabled {
		return d
	}
	if !p.Valid() || user <= 0 {
		d.Code = "POLICY_INVALID"
		return d
	}
	members := 0
	failed := false
	for _, r := range p.Requirements {
		state, code := "CHECK_FAILED", "CHECK_UNAVAILABLE"
		if m != nil {
			state, code = m.Check(ctx, r.Channel, user)
		}
		d.Checks = append(d.Checks, Check{r, state, code})
		if state == "MEMBER" {
			members++
		} else if state != "NOT_MEMBER" {
			failed = true
		}
	}
	if failed {
		d.Code = "MEMBERSHIP_CHECK_FAILED"
		return d
	}
	d.Allowed = (p.Mode == "ALL" && members == len(p.Requirements)) || (p.Mode == "ANY" && members > 0)
	d.Code = "DENIED_MISSING_MEMBERSHIP"
	if d.Allowed {
		d.Code = "GRANTED"
	}
	return d
}
func Manifest() sdk.Manifest {
	return sdk.Manifest{SchemaVersion: 1, PluginID: ID, Name: "کنترل دسترسی", Description: "سیاست عضویت کانال برای منابع محافظت‌شده", Version: "1.0.0", CoreVersions: sdk.VersionRange{Minimum: "0.3.0", MaximumExclusive: "1.0.0"}, DataVersion: 1, Permissions: []string{"telegram.read_member_status"}, Settings: sdk.SettingsSchema{Properties: map[string]sdk.Setting{}}, Extensions: sdk.Extensions{PricingPolicy: ID, QuotaPolicy: "membership_checks.monthly"}}
}
