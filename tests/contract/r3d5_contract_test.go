package contract

import (
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	"github.com/naseri-coder/TELMIVO/internal/plugins/crm"
	tickets "github.com/naseri-coder/TELMIVO/internal/plugins/ticketing"
	"testing"
)

func TestR3D5ManifestDependency(t *testing.T) {
	r, e := contentplugins.Registry()
	if e != nil {
		t.Fatal(e)
	}
	ids, e := r.Resolve([]string{tickets.ID})
	if e != nil || len(ids) != 2 || ids[0] != crm.ID || ids[1] != tickets.ID {
		t.Fatal(ids, e)
	}
}
