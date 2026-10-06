package businessplugins

import (
	"strings"
	"testing"

	"github.com/naseri-coder/TELMIVO/internal/platform/ledger"
)

func TestReconciliationOutcomeStatusMatrix(t *testing.T) {
	p := Payment{State: PaymentUnknown, Amount: ledger.Money{Asset: ledger.USD, AmountAtomic: 500}}
	for _, state := range []string{PaymentUnknown, PaymentRefundUnknown} {
		p.State = state
		for _, outcome := range []string{OutcomeUnknown, "", "UNRECOGNIZED"} {
			for _, status := range []string{PaymentFailed, PaymentAwaiting, PaymentPaid, PaymentRefunded} {
				r := ExternalResult{Outcome: outcome, Status: status, Reference: "synthetic-record", Amount: p.Amount}
				if next, _, ok := reconciledTransition(p, r); ok || next != "" {
					t.Fatalf("ambiguity resolved: state=%s outcome=%q status=%s", state, outcome, status)
				}
			}
		}
	}
	for _, tc := range []struct{ state, outcome, status, next, refund string }{
		{PaymentUnknown, OutcomeAcknowledged, PaymentPaid, PaymentPaid, ""},
		{PaymentUnknown, OutcomeAcknowledged, PaymentRefunded, PaymentRefunded, ""},
		{PaymentUnknown, OutcomeAcknowledged, PaymentAwaiting, PaymentAwaiting, ""},
		{PaymentUnknown, OutcomeAcknowledged, PaymentFailed, PaymentFailed, ""},
		{PaymentUnknown, OutcomeRejected, PaymentFailed, PaymentFailed, ""},
		{PaymentRefundUnknown, OutcomeAcknowledged, PaymentRefunded, PaymentRefunded, "REFUNDED"},
		{PaymentRefundUnknown, OutcomeAcknowledged, PaymentPaid, PaymentPaid, "FAILED"},
	} {
		p.State = tc.state
		r := ExternalResult{Outcome: tc.outcome, Status: tc.status, Reference: "synthetic-record", Amount: p.Amount}
		if next, refund, ok := reconciledTransition(p, r); !ok || next != tc.next || refund != tc.refund {
			t.Fatalf("valid evidence rejected: %+v: %s/%s/%v", tc, next, refund, ok)
		}
		for _, invalid := range []ExternalResult{
			{Outcome: r.Outcome, Status: r.Status, Amount: r.Amount},
			{Outcome: r.Outcome, Status: r.Status, Reference: " ", Amount: r.Amount},
			{Outcome: r.Outcome, Status: r.Status, Reference: strings.Repeat("x", 513), Amount: r.Amount},
			{Outcome: r.Outcome, Status: r.Status, Reference: r.Reference},
			{Outcome: r.Outcome, Status: r.Status, Reference: r.Reference, Amount: ledger.Money{Asset: ledger.USD, AmountAtomic: 499}},
			{Outcome: r.Outcome, Status: r.Status, Reference: r.Reference, Amount: ledger.Money{Asset: ledger.XTR, AmountAtomic: 500}},
		} {
			if _, _, ok := reconciledTransition(p, invalid); ok {
				t.Fatalf("incomplete evidence accepted: %+v", invalid)
			}
		}
	}
	for _, state := range []string{PaymentUnknown, PaymentRefundUnknown, PaymentPaid, PaymentFailed, PaymentRefunded} {
		p.State = state
		for _, status := range []string{PaymentAwaiting, PaymentPaid, PaymentRefunded, "UNRECOGNIZED"} {
			if _, _, ok := reconciledTransition(p, ExternalResult{Outcome: OutcomeRejected, Status: status, Reference: "synthetic-record", Amount: p.Amount}); ok {
				t.Fatal("rejected evidence fabricated payment/refund success or unsupported state")
			}
		}
	}
}
