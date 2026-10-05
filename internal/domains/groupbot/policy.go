// Package groupbot separates deterministic policy decisions from Telegram delivery.
package groupbot

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"net/url"
	"strings"
	"unicode/utf8"
)

type Action string

const (
	Log      Action = "LOG_ONLY"
	Delete   Action = "DELETE_MESSAGE"
	Warn     Action = "WARN"
	Restrict Action = "TEMP_RESTRICT"
	Ban      Action = "BAN"
	Unmute   Action = "UNMUTE"
	Unban    Action = "UNBAN"
	Clear    Action = "CLEAR_WARNINGS"
)

type Policy struct {
	Enabled          bool     `json:"enabled"`
	Rules            []string `json:"rules"`
	RulesEnabled     bool     `json:"rules_enabled"`
	FloodCount       int      `json:"flood_count"`
	WindowSeconds    int      `json:"window_seconds"`
	DuplicateCount   int      `json:"duplicate_count"`
	MaxMentions      int      `json:"max_mentions"`
	LinkMode         string   `json:"link_mode"`
	Domains          []string `json:"domains"`
	TrustedBypass    bool     `json:"trusted_bypass"`
	DetectionAction  Action   `json:"detection_action"`
	WarningThreshold int      `json:"warning_threshold"`
	WarningAction    Action   `json:"warning_action"`
	RestrictSeconds  int      `json:"restrict_seconds"`
}

func DefaultPolicy() Policy {
	return Policy{Rules: []string{}, Domains: []string{}, WindowSeconds: 30, LinkMode: "ALLOW_ALL", DetectionAction: Log, WarningAction: Log, RestrictSeconds: 300}
}
func actionValid(a Action) bool {
	return a == Log || a == Delete || a == Warn || a == Restrict || a == Ban
}
func (p Policy) Validate() error {
	if len(p.Rules) > 20 || p.WindowSeconds < 1 || p.WindowSeconds > 3600 || p.FloodCount < 0 || p.FloodCount > 1000 || p.DuplicateCount < 0 || p.DuplicateCount > 1000 || p.MaxMentions < 0 || p.MaxMentions > 100 || p.WarningThreshold < 0 || p.WarningThreshold > 100 || p.RestrictSeconds < 30 || p.RestrictSeconds > 86400 || len(p.Domains) > 50 || !actionValid(p.DetectionAction) || !actionValid(p.WarningAction) {
		return database.ErrInvalid
	}
	if p.LinkMode != "ALLOW_ALL" && p.LinkMode != "DENY_EXTERNAL" && p.LinkMode != "ALLOWLIST" && p.LinkMode != "DENYLIST" {
		return database.ErrInvalid
	}
	for _, r := range p.Rules {
		if utf8.RuneCountInString(r) < 1 || utf8.RuneCountInString(r) > 1000 {
			return database.ErrInvalid
		}
	}
	for _, d := range p.Domains {
		if len(d) > 253 || d == "" || strings.ContainsAny(d, " /:@?#\\\n\r") || d != strings.ToLower(d) {
			return database.ErrInvalid
		}
	}
	return nil
}

type Message struct {
	ID      int64    `json:"message_id"`
	User    int64    `json:"user_id"`
	Text    string   `json:"text"`
	Time    int64    `json:"time"`
	Trusted bool     `json:"trusted"`
	Links   []string `json:"links"`
}
type Decision struct {
	Action Action `json:"action"`
	Reason string `json:"reason"`
}

func Fingerprint(s string) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(strings.ToLower(s))))
	return hex.EncodeToString(h[:])
}
func DomainAllowed(p Policy, host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	match := false
	for _, d := range p.Domains {
		if host == d || strings.HasSuffix(host, "."+d) {
			match = true
			break
		}
	}
	switch p.LinkMode {
	case "ALLOW_ALL":
		return true
	case "ALLOWLIST":
		return match
	case "DENYLIST":
		return !match
	default:
		return false
	}
}
func Evaluate(p Policy, m Message, count, duplicates int) Decision {
	d := Decision{Log, "NO_MATCH"}
	if !p.Enabled || p.TrustedBypass && m.Trusted {
		return d
	}
	if p.FloodCount > 0 && count >= p.FloodCount {
		return Decision{p.DetectionAction, "FLOOD"}
	}
	if p.DuplicateCount > 0 && duplicates >= p.DuplicateCount {
		return Decision{p.DetectionAction, "DUPLICATE_SPAM"}
	}
	if p.MaxMentions > 0 && strings.Count(m.Text, "@") > p.MaxMentions {
		return Decision{p.DetectionAction, "MENTION_SPAM"}
	}
	// Parse bounded text only; classification never follows URLs or fetches content.
	for _, word := range append(strings.Fields(m.Text), m.Links...) {
		word = strings.Trim(word, "()<>[],.!؟")
		raw := word
		if strings.HasPrefix(strings.ToLower(raw), "www.") || !strings.Contains(raw, ":") && strings.Contains(raw, ".") {
			raw = "https://" + raw
		}
		u, e := url.Parse(raw)
		if e == nil && u.Hostname() != "" && (u.Scheme == "http" || u.Scheme == "https") && !DomainAllowed(p, u.Hostname()) {
			return Decision{p.DetectionAction, "LINK_POLICY"}
		}
	}
	return d
}
