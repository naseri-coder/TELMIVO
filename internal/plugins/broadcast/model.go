// Package broadcast defines bounded CRM audiences and campaign lifecycle.
package broadcast

import (
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/plugins/crm"
	"strings"
	"time"
)

const ID = "broadcast"
const MaxRecipients = 10000
const MaxCampaigns = 100

// Audience is declarative. Empty tags means every eligible ACTIVE Telegram contact.
type Audience struct {
	Tags   []string   `json:"tags"`
	Locale string     `json:"locale"`
	Since  *time.Time `json:"since,omitempty"`
}

func (a Audience) Valid() bool {
	return len(a.Tags) <= 8 && crm.TagsValid(a.Tags) && len(a.Locale) <= 32
}

type Campaign struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Text       string     `json:"text"`
	Audience   Audience   `json:"audience"`
	Scheduled  *time.Time `json:"scheduled_at,omitempty"`
	State      string     `json:"state"`
	Revision   int64      `json:"revision"`
	Recipients int        `json:"recipients"`
	Sent       int        `json:"sent"`
	Failed     int        `json:"failed"`
	Unknown    int        `json:"unknown"`
}

func (c Campaign) Valid() bool {
	return strings.TrimSpace(c.Name) != "" && len(c.Name) <= 100 && strings.TrimSpace(c.Text) != "" && len(c.Text) <= 4000 && c.Audience.Valid()
}
func Transition(from, action string) (string, bool) {
	switch action {
	case "START":
		return "SCHEDULED", from == "DRAFT"
	case "PAUSE":
		return "PAUSED", from == "SCHEDULED" || from == "RUNNING"
	case "RESUME":
		return "SCHEDULED", from == "PAUSED"
	case "CANCEL":
		return "CANCELLED", from == "DRAFT" || from == "SCHEDULED" || from == "RUNNING" || from == "PAUSED"
	}
	return "", false
}
func Manifest() sdk.Manifest {
	return sdk.Manifest{SchemaVersion: 1, PluginID: ID, Name: "ارسال گروهی", Description: "کمپین و مخاطبان CRM با گزارش تحویل", Version: "1.0.0", CoreVersions: sdk.VersionRange{Minimum: "0.3.0", MaximumExclusive: "1.0.0"}, DataVersion: 1, Permissions: []string{"broadcast.manage", "telegram.send", "crm.read"}, Dependencies: []sdk.Dependency{{PluginID: crm.ID, Versions: sdk.VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"}}}, Settings: sdk.SettingsSchema{Properties: map[string]sdk.Setting{}}, Extensions: sdk.Extensions{PricingPolicy: ID, QuotaPolicy: "broadcast.messages.monthly"}}
}
