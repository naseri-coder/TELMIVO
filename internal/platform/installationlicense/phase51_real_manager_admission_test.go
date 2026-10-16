package installationlicense

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The phase34LicensedManager fixture uses authentic ephemeral Ed25519-signed V2
// claims in TEST, not a stub Manager. Each scenario runs in a fresh process to
// prevent global Runtime state and lock contamination between tests.
func TestLICA9Phase51RealProductRuntimeChild(t *testing.T) {
	scenario := os.Getenv("LIC_A9_PHASE51_REAL_CHILD")
	if scenario == "" {
		return
	}
	m, _ := phase34LicensedManager(t)
	ctx := WithManager(context.Background(), m)
	var effects int
	deny := func() {
		t.Helper()
		if err := Effect(ctx, func() error { effects++; return nil }); !errors.Is(err, ErrRestricted) {
			t.Fatalf("unapproved direct context Effect: %v", err)
		}
		if err := RuntimeEffect(ctx, func() error { effects++; return nil }); !errors.Is(err, ErrRestricted) {
			t.Fatalf("unapproved global RuntimeEffect: %v", err)
		}
		if effects != 0 {
			t.Fatalf("denied callbacks executed: %d", effects)
		}
	}
	callMutation := func(wantStatus int, wantCalls int) {
		t.Helper()
		var called int
		handler := Middleware(m, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called++
			w.WriteHeader(http.StatusNoContent)
		}))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/commercial/effective", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != wantStatus || called != wantCalls {
			t.Fatalf("middleware POST: status %d calls %d; expected %d/%d", w.Code, called, wantStatus, wantCalls)
		}
	}
	switch scenario {
	case "preinstall-signed-V2":
		if got := m.Decision().State; got != "VALID_ONLINE" {
			t.Fatalf("test-only genuine signed lease invalid: %s", got)
		}
		deny()
		callMutation(http.StatusForbidden, 0)
	case "installed-then-restricted":
		InstallRuntime(m)
		if err := Effect(ctx, func() error { effects++; return nil }); err != nil || effects != 1 {
			t.Fatalf("valid real Manager denied: %v %d", err, effects)
		}
		callMutation(http.StatusNoContent, 1)
		InstallRuntime(nil)
		effects = 0
		deny()
		callMutation(http.StatusForbidden, 0)
		InstallRuntime(m)
		deny()
	case "nested-real-flock-check":
		InstallRuntime(m)
		// Explicit, release-blocking regression. Manager.Effect holds a file
		// flock while calling this callback; CheckRuntime -> Decision opens a
		// new descriptor and may self-deadlock on Linux. The parent will
		// terminate this subprocess if the lock cannot complete promptly.
		if err := RuntimeEffect(ctx, func() error {
			// The Manager holds a genuine exclusive file lock here. A nested
			// decision must return RESTRICTED immediately rather than block.
			if e := CheckRuntime(ctx); !errors.Is(e, ErrRestricted) {
				t.Fatalf("nested CheckRuntime did not fail closed: %v", e)
			}
			var calls int
			handler := Middleware(m, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(http.StatusNoContent)
			}))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/commercial/effective", nil))
			if w.Code != http.StatusForbidden || calls != 0 {
				t.Fatalf("nested middleware escaped locked Manager: status=%d calls=%d", w.Code, calls)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := CheckRuntime(ctx); err != nil {
			t.Fatalf("post-effect signed Manager was spuriously restricted: %v", err)
		}
	default:
		t.Fatalf("unknown scenario %q", scenario)
	}
}

// Two directly relevant source integration tests, executed only when the
// entire authentic Product package can compile using its declared toolchain.
func TestLICA9Phase51RealV2RuntimeAndMiddleware(t *testing.T) {
	for _, scenario := range []string{"preinstall-signed-V2", "installed-then-restricted"} {
		t.Run(scenario, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestLICA9Phase51RealProductRuntimeChild$", "-test.v")
			cmd.Env = append(os.Environ(), "LIC_A9_PHASE51_REAL_CHILD="+scenario)
			output, err := cmd.CombinedOutput()
			if err != nil || !strings.Contains(string(output), "--- PASS: TestLICA9Phase51RealProductRuntimeChild") {
				t.Fatalf("authentic V2 case %s: %v\n%s", scenario, err, output)
			}
		})
	}
}

func TestLICA9Phase51RealManagerNestedCheckMustNotDeadlock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLICA9Phase51RealProductRuntimeChild$", "-test.v")
	cmd.Env = append(os.Environ(), "LIC_A9_PHASE51_REAL_CHILD=nested-real-flock-check")
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("REAL_MANAGER_NESTED_CHECK_DEADLOCK_RELEASE_BLOCKER: child timed out after 3s")
	}
	if err != nil || !strings.Contains(string(output), "--- PASS: TestLICA9Phase51RealProductRuntimeChild") {
		t.Fatalf("real Manager nested CheckRuntime: %v\n%s", err, output)
	}
}
