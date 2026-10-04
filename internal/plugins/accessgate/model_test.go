package accessgate

import (
	"context"
	"testing"
)

type membership map[int64]string

func (m membership) Check(_ context.Context, c, u int64) (string, string) { return m[c], "" }
func TestMembershipPolicies(t *testing.T) {
	p := Policy{Name: "Premium", Enabled: true, Mode: "ALL", Requirements: []Requirement{{Channel: -1000000000001, JoinURL: "https://t.me/channel_one"}, {Channel: -1000000000002}}}
	for _, tc := range []struct {
		mode   string
		states membership
		allow  bool
		code   string
	}{{"ALL", membership{-1000000000001: "MEMBER", -1000000000002: "MEMBER"}, true, "GRANTED"}, {"ALL", membership{-1000000000001: "MEMBER", -1000000000002: "NOT_MEMBER"}, false, "DENIED_MISSING_MEMBERSHIP"}, {"ANY", membership{-1000000000001: "MEMBER", -1000000000002: "NOT_MEMBER"}, true, "GRANTED"}, {"ANY", membership{-1000000000001: "MEMBER", -1000000000002: "UNKNOWN"}, false, "MEMBERSHIP_CHECK_FAILED"}, {"ALL", membership{-1000000000001: "MEMBER", -1000000000002: "CHECK_FAILED"}, false, "MEMBERSHIP_CHECK_FAILED"}} {
		p.Mode = tc.mode
		d := Evaluate(context.Background(), p, 1, tc.states)
		if d.Allowed != tc.allow || d.Code != tc.code {
			t.Fatal(d)
		}
	}
	p.Enabled = false
	if Evaluate(context.Background(), p, 1, membership{}).Allowed {
		t.Fatal("disabled opened access")
	}
	p.Enabled = true
	p.Requirements[0].JoinURL = "https://evil.test/channel"
	if p.Valid() {
		t.Fatal("arbitrary URL")
	}
	p.Requirements[0].JoinURL = ""
	p.Requirements[0].Channel = 10
	if p.Valid() {
		t.Fatal("non-channel")
	}
}
