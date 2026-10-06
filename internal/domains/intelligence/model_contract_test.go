package intelligence

import (
	provider "github.com/naseri-coder/TELMIVO/internal/platform/ai"
	assistant "github.com/naseri-coder/TELMIVO/internal/plugins/aiassistant"
	"testing"
)

func TestConsumerUsesCurrentModelCapabilities(t *testing.T) {
	m := provider.Model{Tools: true, Structured: true, MaxOutput: 64}
	c := assistant.Config{OutputTokens: 64, Tools: []string{"crm.read"}}
	if !modelCompatible(m, c, true) {
		t.Fatal("valid contract rejected")
	}
	m.Tools = false
	if modelCompatible(m, c, false) {
		t.Fatal("stale tools admitted")
	}
	m.Tools = true
	m.MaxOutput = 32
	if modelCompatible(m, c, false) {
		t.Fatal("stale token ceiling admitted")
	}
	m.MaxOutput = 64
	m.Structured = false
	if modelCompatible(m, c, true) {
		t.Fatal("unsupported classification admitted")
	}
	if !modelCompatible(m, c, false) {
		t.Fatal("plain chat requires structured capability")
	}
}
