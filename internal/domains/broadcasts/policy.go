package broadcasts

import "errors"

type Audience struct {
	Plan, Activity string
	Blocked        bool
}

func Matches(filter string, a Audience) bool {
	if a.Blocked {
		return false
	}
	switch filter {
	case "ALL":
		return true
	case "FREE", "PRO", "BUSINESS":
		return a.Plan == filter
	case "ACTIVE", "INACTIVE":
		return a.Activity == filter
	}
	return false
}
func ValidMode(mode string) bool { return mode == "SEND" || mode == "COPY" || mode == "FORWARD" }
func Transition(from, to string) error {
	m := map[string][]string{"DRAFT": {"SCHEDULED", "QUEUED", "CANCELLED"}, "SCHEDULED": {"QUEUED", "CANCELLED"}, "QUEUED": {"RUNNING", "CANCELLED"}, "RUNNING": {"PAUSED", "COMPLETED", "FAILED", "CANCELLED"}, "PAUSED": {"RUNNING", "CANCELLED"}, "FAILED": {"QUEUED"}}
	for _, v := range m[from] {
		if v == to {
			return nil
		}
	}
	return errors.New("invalid broadcast transition")
}

// UNKNOWN means Telegram may have accepted the call. Never retry it automatically.
func RetryableDelivery(status string) bool { return status == "RETRYABLE" }
