package shop

import (
	"github.com/naseri-coder/TELMIVO/internal/platform/ledger"
	"math"
	"testing"
)

func TestExactHistoricalSnapshotAndOverflow(t *testing.T) {
	p := Product{ID: "one", Name: "First", Enabled: true, Revision: 3, Price: ledger.Money{Asset: ledger.USD, AmountAtomic: 9007199254740993}}
	items := []Item{{"one", 2}}
	products := map[string]Product{"one": p}
	lines, total, ok := Snapshot(items, products)
	if !ok || total.AmountAtomic != 18014398509481986 {
		t.Fatal(total, ok)
	}
	p.Name = "Changed"
	p.Price.AmountAtomic = 1
	products["one"] = p
	if lines[0].Name != "First" || lines[0].Price.AmountAtomic != 9007199254740993 || lines[0].Revision != 3 {
		t.Fatal("snapshot drift")
	}
	p.Price.AmountAtomic = math.MaxInt64
	products["one"] = p
	if _, _, ok = Snapshot(items, products); ok {
		t.Fatal("multiplication overflow")
	}
	items = []Item{{"one", 1}, {"two", 1}}
	products["two"] = Product{ID: "two", Name: "Second", Enabled: true, Price: ledger.Money{Asset: ledger.USD, AmountAtomic: 1}}
	if _, _, ok = Snapshot(items, products); ok {
		t.Fatal("addition overflow")
	}
	products["two"] = Product{ID: "two", Name: "Second", Enabled: true, Price: ledger.Money{Asset: ledger.IRR, AmountAtomic: 1}}
	if _, _, ok = Snapshot(items, products); ok {
		t.Fatal("mixed currency")
	}
}
func TestOrderLifecycle(t *testing.T) {
	for _, v := range [][2]string{{"DRAFT", "PLACED"}, {"PLACED", "CONFIRMED"}, {"CONFIRMED", "FULFILLED"}, {"PLACED", "CANCELLED"}} {
		if !Transition(v[0], v[1]) {
			t.Fatal(v)
		}
	}
	for _, v := range [][2]string{{"FULFILLED", "CANCELLED"}, {"CANCELLED", "PLACED"}, {"DRAFT", "FULFILLED"}, {"PLACED", "PLACED"}} {
		if Transition(v[0], v[1]) {
			t.Fatal(v)
		}
	}
}
