package installationlicense

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// These subprocess cases use the actual Product's ephemeral Ed25519-signed V2
// fixture. No external provider, real Authority, or Production keys are used.
func TestLICA9Phase51DNestedManagerChild(t *testing.T) {
	scenario := os.Getenv("LIC_A9_PHASE51D_CHILD")
	if scenario == "" {
		return
	}
	m, sign := phase34LicensedManager(t)
	ctx := context.Background()
	before := m.Decision()
	if before.State != "VALID_ONLINE" {
		t.Fatalf("signed V2 fixture restricted: %+v", before)
	}
	nestedCalled := false
	err := m.Effect(ctx, func() error {
		start := time.Now()
		switch scenario {
		case "nested-effect":
			innerErr := m.Effect(ctx, func() error {
				nestedCalled = true
				return nil
			})
			if !errors.Is(innerErr, ErrRestricted) {
				t.Fatalf("nested Effect must fail closed after bounded wait: %v", innerErr)
			}
		case "nested-accept":
			// This envelope is deliberately not admitted while Effect owns
			// the critical section; no high-water or license state mutation.
			signed := sign(m.runtimeSession, before.ConcurrencyGeneration+1)
			innerErr := m.Accept(signed)
			if !errors.Is(innerErr, ErrPending) {
				t.Fatalf("nested Accept must remain pending: %v", innerErr)
			}
		case "nested-effect-cancel":
			cancelCtx, cancel := context.WithCancel(ctx)
			go func() {
				time.Sleep(50 * time.Millisecond)
				cancel()
			}()
			innerErr := m.Effect(cancelCtx, func() error {
				nestedCalled = true
				return nil
			})
			if !errors.Is(innerErr, context.Canceled) {
				t.Fatalf("nested canceled Effect: %v", innerErr)
			}
		default:
			t.Fatalf("unknown nested scenario: %s", scenario)
		}
		if elapsed := time.Since(start); elapsed > 3500*time.Millisecond {
			t.Fatalf("lock admission unbounded: %s", elapsed)
		}
		return nil
	})
	if err != nil || nestedCalled {
		t.Fatalf("outer signed V2 Effect failed or nested callback ran: %v called=%t", err, nestedCalled)
	}
	after := m.Decision()
	if after.State != "VALID_ONLINE" || after.ConcurrencyGeneration != before.ConcurrencyGeneration {
		t.Fatalf("nested call corrupted real signed V2 state: before=%+v after=%+v", before, after)
	}
}

func TestLICA9Phase51DNestedManagerEffectAcceptBounded(t *testing.T) {
	for _, scenario := range []string{"nested-effect", "nested-accept", "nested-effect-cancel"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLICA9Phase51DNestedManagerChild$", "-test.v")
			cmd.Env = append(os.Environ(), "LIC_A9_PHASE51D_CHILD="+scenario)
			output, err := cmd.CombinedOutput()
			if ctx.Err() == context.DeadlineExceeded {
				t.Fatalf("RELEASE_BLOCKER_NESTED_MANAGER_DEADLOCK: %s timed out; output=%s", scenario, output)
			}
			if err != nil || !strings.Contains(string(output), "--- PASS: TestLICA9Phase51DNestedManagerChild") {
				t.Fatalf("authentic signed V2 %s: %v\n%s", scenario, err, output)
			}
		})
	}
}
