package aiassistant

import (
	"strings"
	"testing"
)

func validConfig() Config {
	return Config{Name: "Assistant", Model: "approved", Mode: "AI_WITH_HANDOFF", Recent: 6, OutputTokens: 512}
}
func TestAssistantBoundaries(t *testing.T) {
	if !validConfig().Valid() {
		t.Fatal("valid")
	}
	for _, change := range []func(*Config){func(c *Config) { c.Instructions = strings.Repeat("x", 4097) }, func(c *Config) { c.Recent = 13 }, func(c *Config) { c.Tools = []string{"http"} }, func(c *Config) { c.Tools = []string{"ticket.note"} }, func(c *Config) { c.OutputTokens = 2049 }, func(c *Config) { c.Knowledge = []string{"same", "same"} }, func(c *Config) { c.Mode = "autonomous" }} {
		c := validConfig()
		change(&c)
		if c.Valid() {
			t.Fatal(c)
		}
	}
	c := validConfig()
	c.WriteEnabled = true
	c.Tools = []string{"ticket.note"}
	if !c.Valid() {
		t.Fatal("explicit write")
	}
}
func TestStructuredOutput(t *testing.T) {
	if !(Structured{Category: "technical", Priority: "HIGH"}).Valid() {
		t.Fatal("valid")
	}
	if (Structured{Category: "shell", Priority: "sudo"}).Valid() {
		t.Fatal("unsafe")
	}
}
