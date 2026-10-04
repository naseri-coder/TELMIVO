// Package forms contains declarative fields and bounded, typed validation.
package forms

import (
	"encoding/json"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/plugins/crm"
	"strconv"
	"strings"
)

const ID = "forms"

type Field struct {
	Key       string            `json:"key"`
	Label     string            `json:"label"`
	Type      string            `json:"type"`
	Required  bool              `json:"required"`
	MaxLength int               `json:"max_length"`
	Min       int64             `json:"min,string"`
	Max       int64             `json:"max,string"`
	Options   []string          `json:"options"`
	CRMField  string            `json:"crm_field"`
	Tags      map[string]string `json:"tags"`
}
type Form struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Enabled  bool    `json:"enabled"`
	Fields   []Field `json:"fields"`
	Access   string  `json:"access"`
	Policy   string  `json:"policy"`
	Revision int64   `json:"revision"`
}

func contains(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}
func (f Form) Valid() bool {
	if strings.TrimSpace(f.Name) == "" || len(f.Name) > 100 || len(f.Fields) < 1 || len(f.Fields) > 20 || !contains([]string{"PUBLIC_TO_BOT_USER", "ACCESS_POLICY_PROTECTED"}, f.Access) || (f.Access == "ACCESS_POLICY_PROTECTED" && len(f.Policy) != 36) || (f.Access == "PUBLIC_TO_BOT_USER" && f.Policy != "") {
		return false
	}
	seen := map[string]bool{}
	mapped := map[string]bool{}
	for _, v := range f.Fields {
		if !crm.KeyPattern.MatchString(v.Key) || seen[v.Key] || strings.TrimSpace(v.Label) == "" || len(v.Label) > 100 || !contains([]string{"TEXT", "LONG_TEXT", "NUMBER", "BOOLEAN", "SINGLE_CHOICE", "MULTI_CHOICE"}, v.Type) || v.MaxLength < 1 || v.MaxLength > 2000 || v.Min > v.Max || len(v.Options) > 20 || len(v.Tags) > 20 {
			return false
		}
		seen[v.Key] = true
		if v.CRMField != "" {
			if !crm.KeyPattern.MatchString(v.CRMField) || mapped[v.CRMField] {
				return false
			}
			mapped[v.CRMField] = true
		}
		choice := v.Type == "SINGLE_CHOICE" || v.Type == "MULTI_CHOICE"
		if choice && len(v.Options) < 1 || !choice && (len(v.Options) > 0 || len(v.Tags) > 0) {
			return false
		}
		opts := map[string]bool{}
		for _, o := range v.Options {
			if o == "" || len(o) > 100 || opts[o] {
				return false
			}
			opts[o] = true
		}
		for o, t := range v.Tags {
			if !opts[o] || !crm.KeyPattern.MatchString(t) {
				return false
			}
		}
	}
	b, _ := json.Marshal(f)
	return len(b) <= 32768
}

// Validate returns only allowlisted CRM custom fields/tags, never arbitrary operations.
func (f Form) Validate(values map[string]json.RawMessage) (map[string]string, []string, bool) {
	fields := map[string]string{}
	tags := []string{}
	if !f.Valid() || len(values) > len(f.Fields) {
		return nil, nil, false
	}
	known := map[string]bool{}
	for _, v := range f.Fields {
		known[v.Key] = true
		raw, ok := values[v.Key]
		if !ok {
			if v.Required {
				return nil, nil, false
			}
			continue
		}
		var text string
		selected := []string{}
		switch v.Type {
		case "TEXT", "LONG_TEXT", "SINGLE_CHOICE":
			if json.Unmarshal(raw, &text) != nil || len(text) > v.MaxLength || v.Required && strings.TrimSpace(text) == "" {
				return nil, nil, false
			}
			if v.Type == "SINGLE_CHOICE" {
				if !contains(v.Options, text) {
					return nil, nil, false
				}
				selected = []string{text}
			}
		case "NUMBER":
			if json.Unmarshal(raw, &text) != nil {
				return nil, nil, false
			}
			n, e := strconv.ParseInt(text, 10, 64)
			if e != nil || strconv.FormatInt(n, 10) != text || n < v.Min || n > v.Max {
				return nil, nil, false
			}
		case "BOOLEAN":
			var b bool
			if json.Unmarshal(raw, &b) != nil || string(raw) == "null" {
				return nil, nil, false
			}
			text = strconv.FormatBool(b)
		case "MULTI_CHOICE":
			if json.Unmarshal(raw, &selected) != nil || len(selected) > 20 || v.Required && len(selected) == 0 {
				return nil, nil, false
			}
			seen := map[string]bool{}
			for _, x := range selected {
				if !contains(v.Options, x) || seen[x] {
					return nil, nil, false
				}
				seen[x] = true
			}
			b, _ := json.Marshal(selected)
			text = string(b)
		}
		if string(raw) == "null" {
			return nil, nil, false
		}
		if v.CRMField != "" {
			if len(text) > 500 {
				return nil, nil, false
			}
			fields[v.CRMField] = text
		}
		for _, o := range selected {
			if tag := v.Tags[o]; tag != "" && !contains(tags, tag) {
				tags = append(tags, tag)
			}
		}
	}
	for k := range values {
		if !known[k] {
			return nil, nil, false
		}
	}
	b, _ := json.Marshal(values)
	return fields, tags, len(b) <= 8192 && crm.TagsValid(tags)
}
func Manifest() sdk.Manifest {
	return sdk.Manifest{SchemaVersion: 1, PluginID: ID, Name: "فرم‌ها", Description: "فرم محدود و ثبت پاسخ در CRM", Version: "1.0.0", CoreVersions: sdk.VersionRange{Minimum: "0.3.0", MaximumExclusive: "1.0.0"}, DataVersion: 1, Permissions: []string{"forms.manage", "forms.submit", "crm.write"}, Dependencies: []sdk.Dependency{{PluginID: crm.ID, Versions: sdk.VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"}}}, OptionalDependencies: []sdk.Dependency{{PluginID: "access-gate", Versions: sdk.VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"}}}, Settings: sdk.SettingsSchema{Properties: map[string]sdk.Setting{}}, Extensions: sdk.Extensions{PricingPolicy: ID, QuotaPolicy: "forms.submissions.monthly"}}
}
