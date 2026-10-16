package installationlicense

import "testing"

// These cases execute against the real Product helper during Go1.27.2 package
// admission. The helper must never misclassify advisory flock contention as
// an Authority revocation or a new lease-signing request.
func TestLICA9Phase51CRefreshDecisionClassification(t *testing.T) {
	cases := []struct {
		name     string
		decision Decision
		want     bool
	}{
		{"busy_is_not_a_revocation", Decision{State: "RESTRICTED", Reason: "STATE_LOCK_UNAVAILABLE"}, false},
		{"busy_does_not_refresh_even_if_due", Decision{State: "RESTRICTED", Reason: "STATE_LOCK_UNAVAILABLE", RefreshDue: true}, false},
		{"signed_online_not_due", Decision{State: "VALID_ONLINE"}, false},
		{"signed_online_due", Decision{State: "VALID_ONLINE", RefreshDue: true}, true},
		{"authority_revoked", Decision{State: "RESTRICTED", Reason: "AUTHORITY_RESTRICTED"}, true},
		{"signed_state_expired", Decision{State: "RESTRICTED", Reason: "VERIFICATION_FAILED"}, true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldRefreshForDecision(test.decision); got != test.want {
				t.Fatalf("refresh classification %s = %t, want %t", test.name, got, test.want)
			}
		})
	}
}
