// Package aiassistant defines a tool-bounded assistant; infrastructure stays in Core.
package aiassistant

import (
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"strings"
)

const ID = "ai-assistant"
const MaxRounds = 2
const MaxCalls = 3
const MaxTools = 8
const MaxInput = 32768
const MaxOutput = 4000

var ReadTools = []string{"contact.summary", "ticket.context", "file.metadata", "access.evaluate"}
var WriteTools = []string{"contact.add_tag", "ticket.priority", "ticket.note", "ticket.assign", "ticket.status"}

func Contains(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}

type Config struct {
	Name               string   `json:"name"`
	Model              string   `json:"model"`
	Instructions       string   `json:"instructions"`
	Language           string   `json:"language"`
	Mode               string   `json:"mode"`
	Tools              []string `json:"tools"`
	WriteEnabled       bool     `json:"write_enabled"`
	Memory             bool     `json:"memory"`
	Recent             int      `json:"recent"`
	OutputTokens       int64    `json:"output_tokens"`
	RetrievalMode      string   `json:"retrieval_mode"`
	MaxKnowledgeChunks int      `json:"max_knowledge_chunks"`
	MaxCostMicrounits  int64    `json:"max_cost_microunits"`
	Knowledge          []string `json:"knowledge"`
	Sources            []string `json:"sources"`
	Files              []string `json:"files"`
	Revision           int64    `json:"revision"`
}

func (c Config) Retrieval() string {
	if c.RetrievalMode == "" {
		return "HYBRID"
	}
	return c.RetrievalMode
}
func (c Config) RetrievalLimit() int {
	if c.MaxKnowledgeChunks == 0 {
		return 4
	}
	return c.MaxKnowledgeChunks
}
func (c Config) Valid() bool {
	if strings.TrimSpace(c.Name) == "" || len(c.Name) > 100 || len(c.Instructions) > 4096 || len(c.Language) > 32 || c.Model == "" || len(c.Model) > 100 || !Contains([]string{"AI_ONLY", "AI_WITH_HANDOFF", "AGENT_ASSIST"}, c.Mode) || c.Recent < 0 || c.Recent > 12 || c.OutputTokens < 1 || c.OutputTokens > 2048 || !Contains([]string{"LEXICAL", "HYBRID"}, c.Retrieval()) || c.RetrievalLimit() < 1 || c.RetrievalLimit() > 8 || c.MaxCostMicrounits < 0 || c.MaxCostMicrounits > 1_000_000_000_000 || len(c.Tools) > MaxTools || len(c.Knowledge) > 20 || len(c.Sources) > 20 || len(c.Files) > 20 {
		return false
	}
	seen := map[string]bool{}
	for _, t := range c.Tools {
		if seen[t] || !Contains(ReadTools, t) && !Contains(WriteTools, t) || Contains(WriteTools, t) && !c.WriteEnabled {
			return false
		}
		seen[t] = true
	}
	for index, list := range [][]string{c.Knowledge, c.Sources, c.Files} {
		seen := map[string]bool{}
		for _, id := range list {
			max := 100
			if index == 1 {
				max = 220
			}
			if len(id) == 0 || len(id) > max || seen[id] {
				return false
			}
			seen[id] = true
		}
	}
	return true
}

type Structured struct {
	Category string `json:"category"`
	Priority string `json:"priority"`
}

func (s Structured) Valid() bool {
	return Contains([]string{"general", "technical", "billing"}, s.Category) && Contains([]string{"LOW", "NORMAL", "HIGH", "URGENT"}, s.Priority)
}
func Manifest() sdk.Manifest {
	return sdk.Manifest{SchemaVersion: 1, PluginID: ID, Name: "دستیار هوش مصنوعی", Description: "گفت‌وگو، ابزار مجاز و ارجاع به کارشناس", Version: "1.0.0", CoreVersions: sdk.VersionRange{Minimum: "0.3.0", MaximumExclusive: "1.0.0"}, DataVersion: 1, Permissions: []string{"ai.invoke", "telegram.send"}, Settings: sdk.SettingsSchema{Properties: map[string]sdk.Setting{}}, Extensions: sdk.Extensions{PricingPolicy: ID, QuotaPolicy: "ai.requests.monthly"}}
}
