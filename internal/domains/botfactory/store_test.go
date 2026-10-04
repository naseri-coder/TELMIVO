package botfactory

import (
	"encoding/json"
	"github.com/naseri-coder/TELMIVO/internal/domains/commerce"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"strings"
	"testing"
)

func TestStoreVisibilitySearchFiltersAndPrivacy(t *testing.T) {
	items := []Item{{ID: "a", Name: "فرم", Description: "CRM", State: sdk.Disabled, Configuration: json.RawMessage(`{"private":"saved"}`), Store: commerce.EffectivePolicy{Metadata: commerce.StoreMetadata{Visible: true, Category: "business"}}}, {ID: "b", Name: "Hidden", Store: commerce.EffectivePolicy{Metadata: commerce.StoreMetadata{Visible: false}}}}
	p, e := FilterStore(items, StoreFilter{Search: "crm", Category: "business"})
	if e != nil || len(p.Items) != 1 || len(p.Items[0].Configuration) != 0 || p.Items[0].State != sdk.Disabled {
		t.Fatal(p, e)
	}
	p, e = FilterStore(items, StoreFilter{Search: "hidden"})
	if e != nil || len(p.Items) != 0 {
		t.Fatal(p, e)
	}
	p, e = FilterStore(items, StoreFilter{After: "a"})
	if e != nil || len(p.Items) != 0 {
		t.Fatal(p, e)
	}
	if _, e = FilterStore(items, StoreFilter{Search: strings.Repeat("x", 101)}); e == nil {
		t.Fatal("unbounded search")
	}
}
