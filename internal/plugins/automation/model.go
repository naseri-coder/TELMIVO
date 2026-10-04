// Package automation contains only bounded declarative workflow contracts.
package automation

import (
	"encoding/json"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"strconv"
	"strings"
	"time"
)

const ID = "automation"
const MaxActions = 8
const MaxConditions = 16
const MaxWorkflows = 100
const MaxDepth = 4
const MaxAttempts = 2
const MaxDuration = 60 * time.Second

var Triggers = []string{"manual", "crm.contact.created", "crm.contact.updated", "crm.tag.added", "ticket.created", "ticket.customer_message", "ticket.agent_message", "ticket.status_changed", "ticket.priority_changed", "ticket.assigned", "file.delivery_allowed", "file.delivery_denied"}
var Actions = []string{"crm.add_tag", "crm.remove_tag", "crm.set_field", "ticket.assign", "ticket.priority", "ticket.status", "ticket.note", "ticket.reply", "telegram.send", "ai.invoke", "ai.store_result"}
var Fields = []string{"ticket.priority", "ticket.status", "ticket.department", "contact.tags", "file.reference", "ai.priority", "ai.category"}

func Contains(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}

type Condition struct {
	Field  string   `json:"field"`
	Op     string   `json:"op"`
	Values []string `json:"values"`
}
type Group struct {
	Mode  string      `json:"mode"`
	Items []Condition `json:"items"`
}
type Action struct {
	Type  string `json:"type"`
	Value string `json:"value"`
	Field string `json:"field,omitempty"`
}
type Workflow struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Enabled      bool      `json:"enabled"`
	Trigger      string    `json:"trigger"`
	Conditions   Group     `json:"conditions"`
	Actions      []Action  `json:"actions"`
	DelaySeconds int       `json:"delay_seconds"`
	MaxAttempts  int       `json:"max_attempts"`
	Revision     int64     `json:"revision"`
	Created      time.Time `json:"created_at"`
	Updated      time.Time `json:"updated_at"`
}

func (w Workflow) Valid() bool {
	if strings.TrimSpace(w.Name) == "" || len(w.Name) > 100 || len(w.Description) > 500 || !Contains(Triggers, w.Trigger) || len(w.Actions) < 1 || len(w.Actions) > MaxActions || len(w.Conditions.Items) > MaxConditions || w.DelaySeconds < 0 || w.DelaySeconds > 86400 || w.MaxAttempts < 1 || w.MaxAttempts > MaxAttempts || !Contains([]string{"ALL", "ANY"}, w.Conditions.Mode) {
		return false
	}
	for _, c := range w.Conditions.Items {
		if !Contains(Fields, c.Field) || !Contains([]string{"EQUALS", "NOT_EQUALS", "IN", "NOT_IN", "EXISTS", "NOT_EXISTS", "CONTAINS", "GREATER_THAN", "LESS_THAN"}, c.Op) || len(c.Values) > 16 {
			return false
		}
		if Contains([]string{"EQUALS", "NOT_EQUALS", "GREATER_THAN", "LESS_THAN", "CONTAINS"}, c.Op) && len(c.Values) != 1 {
			return false
		}
		if Contains([]string{"EXISTS", "NOT_EXISTS"}, c.Op) && len(c.Values) != 0 {
			return false
		}
		for _, v := range c.Values {
			if len(v) > 200 {
				return false
			}
		}
		if c.Op != "EXISTS" && c.Op != "NOT_EXISTS" && len(c.Values) == 0 {
			return false
		}
	}
	aiCalls := 0
	for _, a := range w.Actions {
		if a.Type == "ai.invoke" {
			aiCalls++
			if aiCalls > 1 {
				return false
			}
		}
		if !Contains(Actions, a.Type) || len(a.Value) > 4000 || len(a.Field) > 64 {
			return false
		}
	}
	b, _ := json.Marshal(w)
	return len(b) <= 32768
}
func (g Group) Match(data map[string][]string) bool {
	if len(g.Items) == 0 {
		return true
	}
	result := g.Mode == "ALL"
	for _, c := range g.Items {
		v, exists := data[c.Field]
		match := false
		switch c.Op {
		case "EXISTS":
			match = exists
		case "NOT_EXISTS":
			match = !exists
		case "EQUALS", "IN":
			for _, x := range v {
				match = match || Contains(c.Values, x)
			}
		case "NOT_EQUALS", "NOT_IN":
			match = exists
			for _, x := range v {
				if Contains(c.Values, x) {
					match = false
				}
			}
		case "CONTAINS":
			for _, x := range v {
				match = match || Contains(c.Values, x)
			}
		case "GREATER_THAN", "LESS_THAN":
			if len(v) == 1 && len(c.Values) == 1 {
				a, ea := strconv.ParseInt(v[0], 10, 64)
				b, eb := strconv.ParseInt(c.Values[0], 10, 64)
				if ea == nil && eb == nil {
					match = c.Op == "GREATER_THAN" && a > b || c.Op == "LESS_THAN" && a < b
				}
			}
		}
		if g.Mode == "ALL" {
			result = result && match
		} else {
			result = result || match
		}
	}
	return result
}
func Manifest() sdk.Manifest {
	return sdk.Manifest{SchemaVersion: 1, PluginID: ID, Name: "اتوماسیون", Description: "گردش کار محدود و قابل ممیزی", Version: "1.0.0", CoreVersions: sdk.VersionRange{Minimum: "0.3.0", MaximumExclusive: "1.0.0"}, DataVersion: 1, Permissions: []string{"workflow.execute", "telegram.send"}, Settings: sdk.SettingsSchema{Properties: map[string]sdk.Setting{}}, Extensions: sdk.Extensions{PricingPolicy: ID, QuotaPolicy: "automation.executions.monthly"}}
}
