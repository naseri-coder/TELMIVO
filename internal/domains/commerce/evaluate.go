package commerce

import (
	"fmt"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"sort"
	"time"
)

// CommercialState is derived at the supplied instant. End and grace boundaries
// are exclusive. Cancellation/suspension always revoke access immediately.
func CommercialState(a Agreement, now time.Time) State {
	if a.State == Cancelled || a.State == Suspended || a.State == Expired {
		return a.State
	}
	if now.Before(a.StartsAt) {
		return Suspended
	}
	if a.EndsAt == nil || now.Before(*a.EndsAt) {
		return Active
	}
	if a.GraceUntil != nil && now.Before(*a.GraceUntil) {
		return Grace
	}
	return Expired
}
func usable(a Agreement, account, bot string, now time.Time) bool {
	s := CommercialState(a, now)
	return a.Account == account && (a.Scope == sdk.PerAccount || (a.Scope == sdk.PerBot && a.Bot == bot && bot != "")) && (s == Active || s == Grace)
}
func contains(xs []string, k string) bool {
	for _, x := range xs {
		if x == k {
			return true
		}
	}
	return false
}

// Evaluate never changes installation state. Explicit historical agreements
// survive future price/sale/plan changes; disabled catalog plugins fail closed.
func Evaluate(c Catalog, agreements []Agreement, plan, account, bot, plugin string, now time.Time) Decision {
	out := Decision{Code: "NO_ENTITLEMENT", Sources: []string{}}
	pv, ok := c.Get(PluginKind, plugin)
	if !ok {
		out.Code = "PLUGIN_UNAVAILABLE"
		return out
	}
	out.Scope = pv.Definition.Plugin.Scope
	out.Revision = fmt.Sprintf("plugin:%s:%d", plugin, pv.Version)
	if v, yes := c.Get(PlanKind, plan); yes && v.Definition.Plan.Plugins[plugin] == Included {
		out.Sources = append(out.Sources, "PLAN:"+plan)
		out.Revision += fmt.Sprintf("/plan:%s:%d", plan, v.Version)
	}
	expired := false
	for _, a := range agreements {
		if a.Account != account || !contains(a.Terms.Effects.Plugins, plugin) || (a.Scope == sdk.PerBot && a.Bot != bot) {
			continue
		}
		if usable(a, account, bot, now) {
			if a.Scope == sdk.PerAccount {
				out.Scope = sdk.PerAccount
			}
			out.Sources = append(out.Sources, string(a.Source)+":"+a.ID)
			out.Revision += fmt.Sprintf("/agreement:%s:%d", a.ID, a.Revision)
		} else if CommercialState(a, now) == Expired {
			expired = true
		}
	}
	sort.Strings(out.Sources)
	out.Allowed = len(out.Sources) > 0
	if out.Allowed {
		out.Code = "ENTITLED"
	} else if expired {
		out.Code = "ENTITLEMENT_EXPIRED"
	} else if v, yes := c.Get(PlanKind, plan); !yes || v.Definition.Plan.Plugins[plugin] == Unavailable || v.Definition.Plan.Plugins[plugin] == "" {
		out.Code = "PLAN_UNAVAILABLE"
	}
	return out
}

type QuotaResult struct {
	Allowed   bool   `json:"allowed"`
	Code      string `json:"code"`
	Limit     Limit  `json:"limit"`
	Usage     int64  `json:"usage,string"`
	Requested int64  `json:"requested,string"`
	Bucket    string `json:"bucket"`
}

func Bucket(p Period, now time.Time) string {
	now = now.UTC()
	switch p {
	case Daily:
		return now.Format("2006-01-02")
	case Monthly:
		return now.Format("2006-01")
	default:
		return "lifetime"
	}
}
func EffectiveLimit(c Catalog, agreements []Agreement, plan, account, bot, metric string, now time.Time) (Limit, bool) {
	q, ok := c.Get(QuotaKind, metric)
	if !ok {
		return Limit{}, false
	}
	limit := q.Definition.Quota.Default
	if p, yes := c.Get(PlanKind, plan); yes {
		if l, exists := p.Definition.Plan.Limits[metric]; exists {
			limit = l
		}
	}
	for _, a := range agreements {
		if usable(a, account, bot, now) {
			n := a.Terms.Effects.Quotas[metric]
			if !limit.Unlimited {
				var valid bool
				limit.Value, valid = add(limit.Value, n)
				if !valid {
					return Limit{}, false
				}
			}
		}
	}
	return limit, true
}
func CheckQuota(limit Limit, used, requested int64, bucket string) QuotaResult {
	r := QuotaResult{Code: "QUOTA_EXCEEDED", Limit: limit, Usage: used, Requested: requested, Bucket: bucket}
	total, ok := add(used, requested)
	if !validLimit(limit) || !ok {
		r.Code = "INVALID_QUOTA"
		return r
	}
	r.Allowed = limit.Unlimited || total <= limit.Value
	if r.Allowed {
		r.Code = "QUOTA_AVAILABLE"
	}
	return r
}

// Calendar month, UTC, clamp end-of-month rather than Go's date normalization.
func monthEnd(now time.Time) time.Time {
	y, m, d := now.Date()
	next := time.Date(y, m+1, 1, now.Hour(), now.Minute(), now.Second(), now.Nanosecond(), time.UTC)
	last := next.AddDate(0, 1, -1).Day()
	if d > last {
		d = last
	}
	return next.AddDate(0, 0, d-1)
}
