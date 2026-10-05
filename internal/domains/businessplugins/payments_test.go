package businessplugins

import (
	"strings"
	"testing"

	"github.com/naseri-coder/TELMIVO/internal/platform/ledger"
)

func TestReconciledPaymentRequiresProviderEvidence(t *testing.T) {
	p := Payment{Amount: ledger.Money{Asset: ledger.USD, AmountAtomic: 500}}
	verified := ExternalResult{Outcome: OutcomeAcknowledged, Status: PaymentPaid, Reference: "synthetic-charge", Amount: p.Amount}
	if !validReconciledPayment(p, verified) {
		t.Fatal("complete verified provider evidence rejected")
	}
	cases := map[string]ExternalResult{}
	r := verified
	r.Amount = ledger.Money{}
	cases["missing amount"] = r
	r = verified
	r.Amount.AmountAtomic--
	cases["wrong amount"] = r
	r = verified
	r.Amount.Asset = ledger.XTR
	cases["wrong currency"] = r
	r = verified
	r.Reference = " "
	cases["missing reference"] = r
	r = verified
	r.Reference = strings.Repeat("x", 513)
	cases["oversized reference"] = r
	r = verified
	r.Outcome = OutcomeUnknown
	cases["unknown outcome"] = r
	r = verified
	r.Outcome = ""
	cases["missing outcome"] = r
	for name, result := range cases {
		t.Run(name, func(t *testing.T) {
			if validReconciledPayment(p, result) {
				t.Fatal("incomplete or mismatched provider evidence accepted")
			}
		})
	}
}
