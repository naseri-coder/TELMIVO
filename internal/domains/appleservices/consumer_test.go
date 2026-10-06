package appleservices

import (
	"testing"
	"time"
)

func TestR5BConsumerGuidanceAndActionContracts(t *testing.T) {
	now := time.Date(2026, time.October, 6, 0, 0, 0, 0, time.UTC)
	for _, in := range []ConsumerStart{
		{Goal: ConsumerCreateAccount, Channel: ConsumerWeb, Locale: "fa-IR", Region: "DE", Key: "create-1"},
		{Goal: ConsumerRecoverAccess, Channel: ConsumerMac, Locale: "en-US", Region: "US", Key: "recover-1"},
		{Goal: ConsumerCheckExisting, Channel: ConsumerAndroidWindows, Locale: "de-DE", Region: "", Key: "check-1"},
	} {
		if !in.Valid(now) {
			t.Fatal("valid consumer start rejected", in)
		}
	}
	for _, in := range []ConsumerStart{
		{Goal: "BAD", Channel: ConsumerWeb, Locale: "fa-IR", Region: "DE", Key: "bad"},
		{Goal: ConsumerCreateAccount, Channel: "BAD", Locale: "fa-IR", Region: "DE", Key: "bad"},
		{Goal: ConsumerCreateAccount, Channel: ConsumerWeb, Locale: "f", Region: "DE", Key: "bad"},
		{Goal: ConsumerCreateAccount, Channel: ConsumerWeb, Locale: "fa_IR", Region: "DE", Key: "bad"},
		{Goal: ConsumerCreateAccount, Channel: ConsumerWeb, Locale: "fa-IR", Region: "de", Key: "bad"},
		{Goal: ConsumerCreateAccount, Channel: ConsumerWeb, Locale: "fa-IR", Region: "DE", Key: ""},
	} {
		if in.Valid(now) {
			t.Fatal("invalid consumer start accepted", in)
		}
	}

	create := consumerGuidance(ConsumerCreateAccount, AppleActionRequired)
	if create.OfficialURL != AppleAccountURL || create.Code != "CREATE_ACCOUNT_AT_APPLE" || !create.AppleActionOnly || create.SensitiveInputAllowed || create.IdentityVerified {
		t.Fatal("create guidance contract drift", create)
	}
	recover := consumerGuidance(ConsumerRecoverAccess, AppleActionRequired)
	if recover.OfficialURL != AppleRecoveryURL || recover.Code != "RECOVER_ACCESS_AT_APPLE" {
		t.Fatal("recovery guidance contract drift", recover)
	}
	verification := consumerGuidance(ConsumerCreateAccount, AppleCompleted)
	if verification.Code != "COMPLETE_APPLE_REQUIRED_VERIFICATION" || !verification.AppleActionOnly || verification.IdentityVerified {
		t.Fatal("Apple-required verification guidance drift", verification)
	}
	pending := consumerGuidance(ConsumerCreateAccount, VerificationPending)
	if pending.AppleActionOnly || pending.IdentityVerified {
		t.Fatal("local completion confirmation guidance drift", pending)
	}

	ready := consumerGuidance(ConsumerCreateAccount, Ready)
	if ready.CompletionMeaning != "USER_ATTESTED_ONLY" || ready.IdentityVerified || ready.AppleActionOnly || ready.SensitiveInputAllowed {
		t.Fatal("R5B completion was confused with verified Apple identity", ready)
	}

	sequence := []struct {
		from   State
		action ConsumerAction
		to     State
	}{
		{Requested, ConsumerConfirmPrecheck, Precheck},
		{Precheck, ConsumerContinueAtApple, AppleActionRequired},
		{AppleActionRequired, ConsumerAppleFlowCompleted, AppleCompleted},
		{AppleCompleted, ConsumerVerificationStarted, VerificationPending},
		{VerificationPending, ConsumerConfirmCompletion, Ready},
	}
	for _, tc := range sequence {
		got, ok := consumerTarget(tc.from, tc.action)
		if !ok || got != tc.to {
			t.Fatalf("consumer action %s from %s = %s,%v; want %s", tc.action, tc.from, got, ok, tc.to)
		}
	}
	if _, ok := consumerTarget(Requested, ConsumerConfirmCompletion); ok {
		t.Fatal("consumer flow allowed direct completion")
	}
}
