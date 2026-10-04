// Package filemanager owns file-reference and delivery contracts, not membership implementation.
package filemanager

import (
	"context"
	"encoding/json"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	gate "github.com/naseri-coder/TELMIVO/internal/plugins/accessgate"
	"regexp"
	"strings"
	"time"
)

const ID = "file-manager"

type File struct {
	Reference   string     `json:"reference"`
	FileID      string     `json:"telegram_file_id"`
	UniqueID    string     `json:"telegram_file_unique_id"`
	Kind        string     `json:"type"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Category    string     `json:"category"`
	Policy      string     `json:"access_policy_id"`
	Enabled     bool       `json:"enabled"`
	Revision    int64      `json:"revision"`
	Created     time.Time  `json:"created_at"`
	Updated     time.Time  `json:"updated_at"`
	Delivered   int64      `json:"delivered"`
	Denied      int64      `json:"denied"`
	Unique      int64      `json:"unique_requesters"`
	Last        *time.Time `json:"last_delivered_at"`
}

var identifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,512}$`)
var ReferencePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{32}$`)

func (f File) Valid() bool {
	return identifier.MatchString(f.FileID) && identifier.MatchString(f.UniqueID) && (f.Kind == "document" || f.Kind == "video" || f.Kind == "audio" || f.Kind == "photo") && strings.TrimSpace(f.Title) != "" && len(f.Title) <= 200 && len(f.Description) <= 2000 && len(f.Category) <= 100
}

type Access interface {
	Authorize(context.Context, string, int64) (gate.Decision, error)
}

func Authorize(ctx context.Context, f File, user int64, a Access) (gate.Decision, error) {
	if !f.Enabled {
		return gate.Decision{Code: "FILE_DISABLED", Checks: []gate.Check{}}, nil
	}
	if f.Policy == "" {
		return gate.Decision{Allowed: true, Code: "PUBLIC", Checks: []gate.Check{}}, nil
	}
	if a == nil {
		return gate.Decision{Code: "ACCESS_GATE_UNAVAILABLE", Checks: []gate.Check{}}, nil
	}
	return a.Authorize(ctx, f.Policy, user)
}

type Request struct {
	Reference string `json:"reference"`
	User      int64  `json:"user"`
	Key       string `json:"key"`
}
type Plugin struct{ Runtime sdk.Invoker }

func (p Plugin) Deliver(ctx context.Context, r Request) (json.RawMessage, error) {
	b, _ := json.Marshal(r)
	return p.Runtime.Invoke(ctx, "telegram.send", b)
}
func Manifest() sdk.Manifest {
	return sdk.Manifest{SchemaVersion: 1, PluginID: ID, Name: "مدیریت فایل", Description: "ثبت و ارسال فایل‌های تلگرام با دسترسی اختیاری", Version: "1.0.0", CoreVersions: sdk.VersionRange{Minimum: "0.3.0", MaximumExclusive: "1.0.0"}, DataVersion: 1, Permissions: []string{"files.read", "files.write", "telegram.send"}, OptionalDependencies: []sdk.Dependency{{PluginID: gate.ID, Versions: sdk.VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"}}}, Settings: sdk.SettingsSchema{Properties: map[string]sdk.Setting{}}, Extensions: sdk.Extensions{PricingPolicy: ID, QuotaPolicy: "downloads.monthly"}}
}
