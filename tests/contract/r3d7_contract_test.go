package contract

import (
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/plugins/automation"
	"testing"
)

func TestR3D7IndependentPluginContracts(t *testing.T) {
	r, e := contentplugins.Registry()
	if e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{"broadcast", "forms", "commerce"} {
		m, e := r.Manifest(id)
		if e != nil || m.Validate("0.3.0") != nil || len(m.Dependencies) != 1 || m.Dependencies[0].PluginID != "crm" {
			t.Fatal(m, e)
		}
		for _, c := range m.Permissions {
			if !sdk.KnownCapability(c) {
				t.Fatal(c)
			}
		}
		for _, d := range m.Dependencies {
			if d.PluginID == "ai-assistant" || d.PluginID == "automation" {
				t.Fatal("unnecessary dependency")
			}
		}
	}
	for _, event := range []string{"broadcast.started", "broadcast.completed", "form.submitted", "commerce.order.created", "commerce.order.status_changed"} {
		if !automation.Contains(automation.Triggers, event) {
			t.Fatal("event unavailable", event)
		}
	}
}
