package botmodules

import (
	"errors"
	"sort"
)

type Module struct {
	Key          string         `json:"key"`
	NameKey      string         `json:"name_key"`
	Version      string         `json:"version"`
	Status       string         `json:"status"`
	Dependencies []string       `json:"dependencies"`
	Permissions  []string       `json:"permissions"`
	Schema       map[string]any `json:"configuration_schema"`
}
type Registry struct{ modules map[string]Module }

func New(ms []Module) (*Registry, error) {
	r := &Registry{modules: map[string]Module{}}
	for _, m := range ms {
		if m.Key == "" || m.Version == "" || r.modules[m.Key].Key != "" {
			return nil, errors.New("invalid or duplicate module")
		}
		r.modules[m.Key] = m
	}
	enabled := map[string]bool{}
	for key := range r.modules {
		enabled[key] = true
	}
	if e := r.Validate(enabled); e != nil {
		return nil, e
	}
	return r, nil
}
func (r *Registry) Validate(enabled map[string]bool) error {
	visiting := map[string]bool{}
	done := map[string]bool{}
	var walk func(string) error
	walk = func(key string) error {
		m, ok := r.modules[key]
		if !ok || !enabled[key] {
			return errors.New("missing enabled dependency")
		}
		if visiting[key] {
			return errors.New("cyclic dependency")
		}
		if done[key] {
			return nil
		}
		visiting[key] = true
		for _, d := range m.Dependencies {
			if e := walk(d); e != nil {
				return e
			}
		}
		visiting[key] = false
		done[key] = true
		return nil
	}
	for k, on := range enabled {
		if on {
			if e := walk(k); e != nil {
				return e
			}
		}
	}
	return nil
}
func (r *Registry) List() []Module {
	out := make([]Module, 0, len(r.modules))
	for _, m := range r.modules {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
func Defaults() *Registry {
	var ms []Module
	for _, k := range []string{"support", "faq", "sales", "payments", "subscription", "affiliate", "booking", "uploader"} {
		deps := []string{}
		if k == "subscription" {
			deps = append(deps, "payments")
		}
		ms = append(ms, Module{Key: k, NameKey: "plugins." + k, Version: "0.1.0", Status: "DISABLED", Dependencies: deps, Permissions: []string{"bots.manage"}, Schema: map[string]any{"type": "object", "additionalProperties": false}})
	}
	r, _ := New(ms)
	return r
}
