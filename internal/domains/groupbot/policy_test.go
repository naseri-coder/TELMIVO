package groupbot

import (
	"testing"
)

func TestDeterministicPolicy(t *testing.T) {
	p := DefaultPolicy()
	p.Enabled = true
	p.FloodCount = 4
	p.DuplicateCount = 3
	p.MaxMentions = 2
	p.DetectionAction = Warn
	p.LinkMode = "ALLOWLIST"
	p.Domains = []string{"example.com"}
	for _, c := range []struct {
		name, text, reason string
		n, d               int
	}{{"flood", "ok", "FLOOD", 4, 1}, {"threshold_change", "ok", "NO_MATCH", 3, 1}, {"duplicate", "ok", "DUPLICATE_SPAM", 3, 3}, {"mentions", "@a @b @c", "MENTION_SPAM", 1, 1}, {"allowed", "https://sub.example.com/p", "NO_MATCH", 1, 1}, {"suffix_attack", "https://example.com.attacker.test", "LINK_POLICY", 1, 1}, {"credentials", "https://example.com@attacker.test", "LINK_POLICY", 1, 1}, {"bad", "https://%", "NO_MATCH", 1, 1}} {
		t.Run(c.name, func(t *testing.T) {
			for i := 0; i < 10; i++ {
				got := Evaluate(p, Message{Text: c.text}, c.n, c.d)
				if got.Reason != c.reason {
					t.Fatal(got)
				}
			}
		})
	}
	p.Enabled = false
	if Evaluate(p, Message{}, 100, 100).Action != Log {
		t.Fatal("disabled")
	}
	p.Enabled = true
	p.TrustedBypass = true
	if Evaluate(p, Message{Trusted: true}, 100, 100).Action != Log {
		t.Fatal("bypass")
	}
	p.LinkMode = "DENYLIST"
	if DomainAllowed(p, "example.com") || !DomainAllowed(p, "other.test") {
		t.Fatal("denylist")
	}
}
func TestPolicyBounds(t *testing.T) {
	p := DefaultPolicy()
	if p.Validate() != nil {
		t.Fatal("default")
	}
	p.FloodCount = -1
	if p.Validate() == nil {
		t.Fatal("negative")
	}
	p = DefaultPolicy()
	p.Domains = []string{"https://evil.test"}
	if p.Validate() == nil {
		t.Fatal("domain")
	}
	if Fingerprint(" Same ") != Fingerprint("same") {
		t.Fatal("digest")
	}
}
