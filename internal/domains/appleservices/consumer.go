package appleservices

import (
	"strings"
	"time"
)

const (
	AppleAccountURL  = "https://account.apple.com/"
	AppleRecoveryURL = "https://iforgot.apple.com/"
	AppleSupportURL  = "https://support.apple.com/apple-account"
)

type ConsumerGoal string

const (
	ConsumerCreateAccount ConsumerGoal = "CREATE_ACCOUNT"
	ConsumerRecoverAccess ConsumerGoal = "RECOVER_ACCESS"
	ConsumerCheckExisting ConsumerGoal = "CHECK_EXISTING"
)

func (g ConsumerGoal) Valid() bool {
	return g == ConsumerCreateAccount || g == ConsumerRecoverAccess || g == ConsumerCheckExisting
}

type ConsumerChannel string

const (
	ConsumerWeb            ConsumerChannel = "WEB"
	ConsumerIPhoneIPad     ConsumerChannel = "IPHONE_IPAD"
	ConsumerMac            ConsumerChannel = "MAC"
	ConsumerAndroidWindows ConsumerChannel = "ANDROID_WINDOWS"
	ConsumerOther          ConsumerChannel = "OTHER"
)

func (c ConsumerChannel) Valid() bool {
	switch c {
	case ConsumerWeb, ConsumerIPhoneIPad, ConsumerMac, ConsumerAndroidWindows, ConsumerOther:
		return true
	default:
		return false
	}
}

type ConsumerAction string

const (
	ConsumerConfirmPrecheck     ConsumerAction = "CONFIRM_PRECHECK"
	ConsumerContinueAtApple     ConsumerAction = "CONTINUE_AT_APPLE"
	ConsumerAppleFlowCompleted  ConsumerAction = "APPLE_FLOW_COMPLETED"
	ConsumerVerificationStarted ConsumerAction = "VERIFICATION_STARTED"
	ConsumerConfirmCompletion   ConsumerAction = "CONFIRM_COMPLETION"
	ConsumerCancel              ConsumerAction = "CANCEL"
	ConsumerReportFailure       ConsumerAction = "REPORT_FAILURE"
)

func (a ConsumerAction) Valid() bool {
	switch a {
	case ConsumerConfirmPrecheck, ConsumerContinueAtApple, ConsumerAppleFlowCompleted,
		ConsumerVerificationStarted, ConsumerConfirmCompletion, ConsumerCancel, ConsumerReportFailure:
		return true
	default:
		return false
	}
}

type ConsumerStart struct {
	Goal    ConsumerGoal    `json:"goal"`
	Channel ConsumerChannel `json:"channel"`
	Locale  string          `json:"locale"`
	Region  string          `json:"region"`
	Key     string          `json:"idempotency_key"`
}

func (in ConsumerStart) Valid(_ time.Time) bool {
	if !in.Goal.Valid() || !in.Channel.Valid() || !validKey(strings.TrimSpace(in.Key)) ||
		!validLocale(strings.TrimSpace(in.Locale)) || !validRegion(strings.TrimSpace(in.Region)) {
		return false
	}
	return true
}

type ConsumerActionRequest struct {
	Action   ConsumerAction `json:"action"`
	Expected int64          `json:"expected_revision"`
}

type ConsumerGuidance struct {
	Code                  string `json:"code"`
	OfficialURL           string `json:"official_url,omitempty"`
	SupportURL            string `json:"support_url"`
	AppleActionOnly       bool   `json:"apple_action_only"`
	SensitiveInputAllowed bool   `json:"sensitive_input_allowed"`
	IdentityVerified      bool   `json:"identity_verified"`
	CompletionMeaning     string `json:"completion_meaning"`
}

type ConsumerAssistance struct {
	Operation        Operation        `json:"operation"`
	Goal             ConsumerGoal     `json:"goal"`
	Channel          ConsumerChannel  `json:"channel"`
	Locale           string           `json:"locale"`
	Region           string           `json:"region"`
	GuidanceVersion  string           `json:"guidance_version"`
	CompletionSource string           `json:"completion_source"`
	Guidance         ConsumerGuidance `json:"guidance"`
	Created          time.Time        `json:"created_at"`
	Updated          time.Time        `json:"updated_at"`
}

func validLocale(v string) bool {
	if len(v) < 2 || len(v) > 35 {
		return false
	}
	for i, r := range v {
		ok := r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-'
		if !ok || i == 0 && (r == '-' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func validRegion(v string) bool {
	if v == "" {
		return true
	}
	return len(v) == 2 && v[0] >= 'A' && v[0] <= 'Z' && v[1] >= 'A' && v[1] <= 'Z'
}
