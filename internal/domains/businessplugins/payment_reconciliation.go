package businessplugins

import "strings"

// Reconciliation evidence must identify the provider record and exact money.
// Status alone never resolves an ambiguous external effect.
func reconciledTransition(p Payment, r ExternalResult) (next, refund string, ok bool) {
	if !validMoney(r.Amount) || r.Amount != p.Amount || strings.TrimSpace(r.Reference) == "" || len(r.Reference) > 512 {
		return "", "", false
	}
	// A deterministic rejection can release only an UNKNOWN checkout fence.
	// Rejected/unknown outcomes cannot establish payment or refund success.
	if r.Outcome == OutcomeRejected {
		if p.State == PaymentUnknown && r.Status == PaymentFailed {
			return PaymentFailed, "", true
		}
		return "", "", false
	}
	if r.Outcome != OutcomeAcknowledged {
		return "", "", false
	}
	switch p.State {
	case PaymentUnknown:
		switch r.Status {
		case PaymentAwaiting, PaymentFailed, PaymentPaid, PaymentRefunded:
			return r.Status, "", true
		}
	case PaymentRefundUnknown:
		switch r.Status {
		case PaymentRefunded:
			return PaymentRefunded, "REFUNDED", true
		case PaymentPaid:
			return PaymentPaid, "FAILED", true
		}
	}
	return "", "", false
}
