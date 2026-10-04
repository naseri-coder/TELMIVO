// Package crm defines reusable bot-scoped contact contracts without infrastructure.
package crm

import (
	"context"
	"encoding/json"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"regexp"
	"strings"
	"time"
)

const ID = "crm"

type Identity struct {
	Provider string `json:"provider"`
	Key      string `json:"key"`
}
type Contact struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Username   string            `json:"username"`
	Locale     string            `json:"locale"`
	Status     string            `json:"status"`
	Tags       []string          `json:"tags"`
	Notes      []string          `json:"notes"`
	Fields     map[string]string `json:"fields"`
	Identities []Identity        `json:"identities"`
	Revision   int64             `json:"revision"`
	FirstSeen  time.Time         `json:"first_seen"`
	LastSeen   time.Time         `json:"last_seen"`
}

var KeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,63}$`)

func TagsValid(tags []string) bool {
	if len(tags) > 20 {
		return false
	}
	seen := map[string]bool{}
	for _, s := range tags {
		if !KeyPattern.MatchString(s) || seen[s] {
			return false
		}
		seen[s] = true
	}
	return true
}
func (c Contact) Valid() bool {
	if strings.TrimSpace(c.Name) == "" || len(c.Name) > 200 || len(c.Username) > 64 || len(c.Locale) > 32 || (c.Status != "ACTIVE" && c.Status != "BLOCKED" && c.Status != "ARCHIVED") || !TagsValid(c.Tags) || len(c.Notes) > 20 || len(c.Fields) > 20 {
		return false
	}
	for _, n := range c.Notes {
		if len(n) > 1000 {
			return false
		}
	}
	for k, v := range c.Fields {
		if !KeyPattern.MatchString(k) || len(v) > 500 {
			return false
		}
	}
	b, _ := json.Marshal(c.Fields)
	return len(b) <= 8000
}

type ResolveRequest struct {
	Identity Identity `json:"identity"`
	Name     string   `json:"name"`
	Username string   `json:"username"`
	Locale   string   `json:"locale"`
}
type Plugin struct{ Runtime sdk.Invoker }

func (p Plugin) Resolve(ctx context.Context, r ResolveRequest) (Contact, error) {
	b, _ := json.Marshal(r)
	out, e := p.Runtime.Invoke(ctx, "crm.write", b)
	var c Contact
	if e == nil {
		e = json.Unmarshal(out, &c)
	}
	return c, e
}
func Manifest() sdk.Manifest {
	return sdk.Manifest{SchemaVersion: 1, PluginID: ID, Name: "مدیریت مخاطبان", Description: "هویت و اطلاعات مشتریان ربات", Version: "1.0.0", CoreVersions: sdk.VersionRange{Minimum: "0.3.0", MaximumExclusive: "1.0.0"}, DataVersion: 1, Permissions: []string{"crm.read", "crm.write"}, Settings: sdk.SettingsSchema{Properties: map[string]sdk.Setting{}}, Extensions: sdk.Extensions{PricingPolicy: ID, QuotaPolicy: "crm.contacts.count"}}
}
