package plugincontract

import (
	"encoding/json"
	"regexp"
	"unicode/utf8"
)

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,99}$`)

func ValidID(s string) bool { return identifier.MatchString(s) }

// SettingsSchema deliberately supports a bounded flat object, not general JSON Schema.
// Unknown keys/types fail closed. Future schema dialects need a new manifest version.
type SettingsSchema struct {
	Properties map[string]Setting `json:"properties"`
	Required   []string           `json:"required,omitempty"`
}

type Setting struct {
	Type      string   `json:"type"` // string, boolean or integer
	MaxLength int      `json:"max_length,omitempty"`
	Enum      []string `json:"enum,omitempty"`
	Minimum   *int64   `json:"minimum,omitempty"`
	Maximum   *int64   `json:"maximum,omitempty"`
}

func (s SettingsSchema) Validate() error {
	if len(s.Properties) > 64 {
		return ErrConfiguration
	}
	for k, f := range s.Properties {
		if !ValidID(k) {
			return ErrConfiguration
		}
		switch f.Type {
		case "string":
			if f.MaxLength < 1 || f.MaxLength > 4096 || f.Minimum != nil || f.Maximum != nil {
				return ErrConfiguration
			}
			seen := map[string]bool{}
			for _, v := range f.Enum {
				if utf8.RuneCountInString(v) > f.MaxLength || seen[v] {
					return ErrConfiguration
				}
				seen[v] = true
			}
		case "integer":
			if f.MaxLength != 0 || len(f.Enum) != 0 || (f.Minimum != nil && f.Maximum != nil && *f.Minimum > *f.Maximum) {
				return ErrConfiguration
			}
		case "boolean":
			if f.MaxLength != 0 || len(f.Enum) != 0 || f.Minimum != nil || f.Maximum != nil {
				return ErrConfiguration
			}
		default:
			return ErrConfiguration
		}
	}
	seen := map[string]bool{}
	for _, k := range s.Required {
		if _, ok := s.Properties[k]; !ok || seen[k] {
			return ErrConfiguration
		}
		seen[k] = true
	}
	return nil
}

func (s SettingsSchema) ValidateConfiguration(raw json.RawMessage) error {
	if s.Validate() != nil || len(raw) > 65536 || !uniqueJSON(raw) {
		return ErrConfiguration
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(raw, &values) != nil || values == nil {
		return ErrConfiguration
	}
	for _, k := range s.Required {
		if _, ok := values[k]; !ok {
			return ErrConfiguration
		}
	}
	for k, raw := range values {
		f, ok := s.Properties[k]
		if !ok || string(raw) == "null" {
			return ErrConfiguration
		}
		switch f.Type {
		case "string":
			var v string
			if json.Unmarshal(raw, &v) != nil || utf8.RuneCountInString(v) > f.MaxLength {
				return ErrConfiguration
			}
			if len(f.Enum) != 0 && !Contains(f.Enum, v) {
				return ErrConfiguration
			}
		case "boolean":
			var v bool
			if json.Unmarshal(raw, &v) != nil {
				return ErrConfiguration
			}
		case "integer":
			var v int64
			if json.Unmarshal(raw, &v) != nil || (f.Minimum != nil && v < *f.Minimum) || (f.Maximum != nil && v > *f.Maximum) {
				return ErrConfiguration
			}
		}
	}
	return nil
}

func Contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
