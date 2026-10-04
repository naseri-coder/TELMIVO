package contract

import (
	"context"
	"encoding/json"
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	assistant "github.com/naseri-coder/TELMIVO/internal/plugins/aiassistant"
	workflow "github.com/naseri-coder/TELMIVO/internal/plugins/automation"
	"testing"
)

type r3d6Invoker struct {
	cap string
	raw json.RawMessage
}

func (i *r3d6Invoker) Invoke(_ context.Context, cap string, b json.RawMessage) (json.RawMessage, error) {
	i.cap = cap
	i.raw = b
	return json.RawMessage(`{}`), nil
}
func TestR3D6PluginContracts(t *testing.T) {
	r, e := contentplugins.Registry()
	if e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{workflow.ID, assistant.ID} {
		m, e := r.Manifest(id)
		if e != nil || m.Validate("0.3.0") != nil {
			t.Fatal(m, e)
		}
		if len(m.Dependencies) != 0 {
			t.Fatal("unnecessary hard dependency")
		}
		for _, cap := range m.Permissions {
			if !sdk.KnownCapability(cap) {
				t.Fatal(cap)
			}
		}
	}
	i := &r3d6Invoker{}
	if e := (workflow.Plugin{Runtime: i}).Run(context.Background(), "scoped-execution"); e != nil || i.cap != "workflow.execute" {
		t.Fatal(e)
	}
	if _, e = (assistant.Plugin{Runtime: i}).Generate(context.Background(), json.RawMessage(`{"text":"bounded"}`)); e != nil || i.cap != "ai.invoke" {
		t.Fatal(e)
	}
}
