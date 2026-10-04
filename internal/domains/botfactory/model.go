// Package botfactory composes existing bot identity, plugin lifecycle and commerce.
package botfactory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/naseri-coder/TELMIVO/internal/domains/commerce"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"regexp"
	"unicode/utf8"
)

type Bot struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}
type Item struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Description    string             `json:"description"`
	Version        string             `json:"version"`
	Scope          sdk.LicenseScope   `json:"scope"`
	Status         string             `json:"commercial_status"`
	Entitled       bool               `json:"entitled"`
	Reason         string             `json:"reason"`
	Included       bool               `json:"included"`
	Plan           string             `json:"plan"`
	Prices         *commerce.Prices   `json:"prices,omitempty"`
	TrialEligible  bool               `json:"trial_eligible"`
	TrialSeconds   int64              `json:"trial_seconds"`
	Dependencies   []string           `json:"dependencies"`
	Conflicts      []string           `json:"conflicts"`
	State          sdk.State          `json:"state"`
	Revision       int64              `json:"revision"`
	Settings       sdk.SettingsSchema `json:"settings"`
	Configuration  json.RawMessage    `json:"configuration,omitempty"`
	Installable    bool               `json:"installable"`
	PolicyRevision string             `json:"policy_revision"`
}
type Selection struct {
	Requested      []string                   `json:"requested"`
	Configurations map[string]json.RawMessage `json:"configurations"`
	Activate       bool                       `json:"activate"`
}
type Issue struct {
	Code   string `json:"code"`
	Plugin string `json:"plugin"`
	Other  string `json:"other,omitempty"`
}
type Review struct {
	Requested []string `json:"requested"`
	Required  []string `json:"required"`
	Order     []string `json:"order"`
	Plugins   []Item   `json:"plugins"`
	Issues    []Issue  `json:"issues"`
	Ready     bool     `json:"ready"`
	Digest    string   `json:"digest"`
}
type Create struct {
	Name         string    `json:"name"`
	Token        string    `json:"token"`
	Key          string    `json:"idempotency_key"`
	Selection    Selection `json:"selection"`
	Accepted     []string  `json:"accepted"`
	ReviewDigest string    `json:"review_digest"`
}

// Credentials are intentionally excluded from all log-facing formatting.
func (Create) String() string   { return "bot creation request [credential redacted]" }
func (Create) GoString() string { return "bot creation request [credential redacted]" }
func (Create) Format(f fmt.State, _ rune) {
	_, _ = f.Write([]byte("bot creation request [credential redacted]"))
}

type Rejected struct{ Review Review }

func (*Rejected) Error() string { return "BOT_SELECTION_REJECTED" }

var tokenPattern = regexp.MustCompile(`^[1-9][0-9]{4,19}:[A-Za-z0-9_-]{30,80}$`)
var operationPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)

func ValidInput(in Create) bool {
	return utf8.RuneCountInString(in.Name) > 0 && utf8.RuneCountInString(in.Name) <= 100 && tokenPattern.MatchString(in.Token) && operationPattern.MatchString(in.Key)
}
func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
