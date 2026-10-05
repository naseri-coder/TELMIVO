package ledger

import (
	"errors"
	"math"
)

type Asset string

const (
	IRR  Asset = "IRR"
	USD  Asset = "USD"
	USDT Asset = "USDT_TON"
	TON  Asset = "TON"
	XTR  Asset = "XTR"
)

var Scales = map[Asset]int{IRR: 0, USD: 2, USDT: 6, TON: 9, XTR: 0}

type Money struct {
	Asset        Asset `json:"asset"`
	AmountAtomic int64 `json:"amount_atomic,string"`
}
type Entry struct {
	AccountID string
	Money     Money
}

func Validate(entries []Entry) error {
	if len(entries) < 2 {
		return errors.New("at least two entries required")
	}
	sums := map[Asset]int64{}
	for _, e := range entries {
		if _, ok := Scales[e.Money.Asset]; !ok || e.AccountID == "" || e.Money.AmountAtomic == 0 {
			return errors.New("invalid entry")
		}
		n := e.Money.AmountAtomic
		s := sums[e.Money.Asset]
		if (n > 0 && s > math.MaxInt64-n) || (n < 0 && s < math.MinInt64-n) {
			return errors.New("money overflow")
		}
		sums[e.Money.Asset] = s + n
	}
	for _, s := range sums {
		if s != 0 {
			return errors.New("unbalanced ledger")
		}
	}
	return nil
}
