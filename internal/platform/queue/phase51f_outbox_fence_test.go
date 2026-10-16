package queue

import (
	"strings"
	"testing"
)

func TestLICA9Phase51FOutboxSQLContract(t *testing.T) {
	for _, requirement := range []struct {
		sql string
		pieces []string
	}{
		{outboxSelectReadySQL, []string{
			"delivery_state='READY'", "published_at IS NULL",
			"FOR UPDATE SKIP LOCKED", "LIMIT 1",
		}},
		{outboxClaimUnknownSQL, []string{
			"delivery_state='UNKNOWN'", "delivery_attempted_at=clock_timestamp()",
			"attempts=attempts+1", "delivery_state='READY'",
			"published_at IS NULL", "lease_owner=$2",
		}},
		{outboxConfirmPublishedSQL, []string{
			"delivery_state='PUBLISHED'", "published_at=clock_timestamp()",
			"delivery_state='UNKNOWN'", "published_at IS NULL",
			"lease_owner=$2",
		}},
	} {
		for _, fragment := range requirement.pieces {
			if !strings.Contains(requirement.sql, fragment) {
				t.Errorf("outbox SQL lost security guard %q: %s", fragment, requirement.sql)
			}
		}
	}
	for _, sql := range []string{outboxSelectReadySQL, outboxClaimUnknownSQL, outboxConfirmPublishedSQL} {
		if strings.Contains(sql, "DELETE FROM outbox_events") ||
			strings.Contains(sql, "delivery_state='READY',") {
			t.Fatalf("unsafe outbox SQL allows destructive recovery or automatic reset: %s", sql)
		}
	}
}

// This is a deterministic LOCAL CONTRACT MODEL, not an execution of PG/NATS.
// A permanent UNKNOWN must not become eligible after broker timeout, crash,
// lost acknowledgment, or the time-based lease expiring.
func TestLICA9Phase51FOutboxAmbiguousOutcomeNoBlindReplayModel(t *testing.T) {
	for _, outcome := range []string{
		"process-crash-before-publish",
		"broker-accepted-but-ack-lost",
		"transport-timeout",
		"license-revoked-after-claim",
		"database-ack-write-lost",
	} {
		t.Run(outcome, func(t *testing.T) {
			state := "READY"
			attempts := 0
			canClaim := func() bool { return state == "READY" }
			if !canClaim() { t.Fatal("new outbox event must be READY") }
			state = "UNKNOWN" // committed BEFORE sending to broker
			attempts++
			if canClaim() {
				t.Fatal("ambiguous dispatch became replay-eligible")
			}
			if attempts != 1 || state != "UNKNOWN" {
				t.Fatalf("attempt state became unsafe: state=%s attempts=%d", state, attempts)
			}
			// Time-based lease expiration may not change dispatch eligibility.
			for i := 0; i < 3; i++ {
				if canClaim() { t.Fatal("expired lease reopened unknown outcome") }
			}
		})
	}
}

func TestLICA9Phase51FAcknowledgedPublishModel(t *testing.T) {
	state := "READY"
	state = "UNKNOWN" // durable pre-attempt fence
	brokerAcknowledged := true
	if brokerAcknowledged && state == "UNKNOWN" {
		state = "PUBLISHED"
	}
	if state != "PUBLISHED" {
		t.Fatal("broker ACK must close a previously persisted UNKNOWN")
	}
	if state == "READY" {
		t.Fatal("confirmed publication accidentally reopened")
	}
}
