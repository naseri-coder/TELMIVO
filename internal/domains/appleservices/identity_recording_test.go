package appleservices

import (
	"errors"
	"testing"
)

func TestIdentityOutcomeRecordingFailureIsNotSwallowed(t *testing.T) {
	failure := errors.New("synthetic audit persistence failure")
	for _, outcome := range []error{ErrIdentityRejected, ErrIdentityUnknown} {
		calls := 0
		got := recordIdentityOutcome(func() error { calls++; return failure }, outcome)
		if calls != 1 || !errors.Is(got, failure) || !errors.Is(got, ErrIdentityUnknown) || errors.Is(got, ErrIdentityRejected) {
			t.Fatalf("record failure lost or false durable rejection: calls=%d err=%v", calls, got)
		}
		if got := recordIdentityOutcome(func() error { return nil }, outcome); got != outcome {
			t.Fatalf("durably recorded classification changed: %v", got)
		}
	}
}
