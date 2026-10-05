package payments

import (
	"context"
	"errors"
	"github.com/naseri-coder/TELMIVO/internal/platform/ledger"
	"time"
)

var ErrDisabled = errors.New("payment provider disabled during bootstrap")

type Intent struct {
	ID, WorkspaceID, Recipient, QuoteID string
	Money                               ledger.Money
	ExpiresAt                           time.Time
}
type Confirmation struct {
	Provider, Reference, Recipient, Status, QuoteID string
	Money                                           ledger.Money
	Confirmed                                       bool
	ObservedAt                                      time.Time
}
type Provider interface {
	CreateIntent(context.Context, Intent) (string, error)
	Verify(context.Context, Intent, []byte) (Confirmation, error)
	Refund(context.Context, string, ledger.Money) (string, error)
}

func Validate(i Intent, c Confirmation, now time.Time) error {
	if i.ID == "" || i.Recipient == "" || i.Money.AmountAtomic <= 0 || !now.Before(i.ExpiresAt) || c.ObservedAt.After(now.Add(30*time.Second)) || c.Reference == "" || !c.Confirmed || c.Status != "CONFIRMED" || c.Recipient != i.Recipient || c.Money != i.Money || c.QuoteID != i.QuoteID {
		return errors.New("unverified payment")
	}
	return nil
}
func RailAllowed(surface, product string, asset ledger.Asset) bool {
	if surface == "TELEGRAM" && product == "DIGITAL" {
		return asset == ledger.XTR
	}
	return true
}
