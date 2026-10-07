package runtime

import "testing"

func TestNotificationStatus(t *testing.T) {
	cases := map[string]string{
		"DELIVERY_ACKNOWLEDGED":    "DELIVERED",
		"DELIVERY_FAILED":          "FAILED",
		"RATE_LIMITED":             "FAILED",
		"DELIVERY_OUTCOME_UNKNOWN": "UNKNOWN",
		"unexpected":               "UNKNOWN",
	}
	for code, want := range cases {
		if got := notificationStatus(code); got != want {
			t.Fatalf("notificationStatus(%q)=%q want %q", code, got, want)
		}
	}
}
