package entitlement

import "time"

type Rule struct {
	Enabled   bool       `json:"enabled"`
	Limit     *int64     `json:"limit"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}
type Input struct {
	Global                           bool
	Plan                             Rule
	Grant, Override                  *Rule
	Permission                       bool
	Usage, Requested                 int64
	Now                              time.Time
	PluginAllowed, DependenciesValid bool
}
type Decision struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
	Limit   *int64 `json:"limit,omitempty"`
	Usage   int64  `json:"usage"`
}

func active(r *Rule, now time.Time) bool {
	return r != nil && (r.ExpiresAt == nil || now.Before(*r.ExpiresAt))
}
func Resolve(i Input) Decision {
	d := Decision{Usage: i.Usage}
	if !i.Global {
		d.Reason = "GLOBAL_DISABLED"
		return d
	}
	r := i.Plan
	if active(i.Grant, i.Now) {
		r = *i.Grant
	}
	if active(i.Override, i.Now) {
		r = *i.Override
	}
	d.Limit = r.Limit
	if !r.Enabled {
		d.Reason = "FEATURE_DISABLED"
		return d
	}
	if !i.PluginAllowed {
		d.Reason = "PLUGIN_DISABLED"
		return d
	}
	if !i.DependenciesValid {
		d.Reason = "DEPENDENCY_INVALID"
		return d
	}
	if !i.Permission {
		d.Reason = "PERMISSION_DENIED"
		return d
	}
	if i.Usage < 0 || i.Requested < 0 {
		d.Reason = "INVALID_USAGE"
		return d
	}
	if i.Requested > 0 && r.Limit != nil && (*r.Limit < 0 || i.Usage > *r.Limit || i.Requested > *r.Limit-i.Usage) {
		d.Reason = "QUOTA_EXCEEDED"
		return d
	}
	d.Allowed = true
	d.Reason = "ALLOWED"
	return d
}
