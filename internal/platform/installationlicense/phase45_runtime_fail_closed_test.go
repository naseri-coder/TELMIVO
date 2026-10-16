package installationlicense

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func phase45FixtureManager(t *testing.T) *Manager {
	t.Helper()
	m, _ := phase19TestSignedConcurrentInstallation(t)
	return m
}

func TestLICA9Phase45MissingRuntimeNeverReturnsManager(t *testing.T) {
	ctx := context.Background()
	if m, err := runtimeForEffect(ctx, false, nil); m != nil || !errors.Is(err, ErrRestricted) {
		t.Fatalf("missing runtime fail open %v %v", m, err)
	}
	if m, err := runtimeForEffect(ctx, true, nil); m != nil || !errors.Is(err, ErrRestricted) {
		t.Fatalf("nil installed runtime fail open %v %v", m, err)
	}
}
func TestLICA9Phase45ContextRequiresExplicitManager(t *testing.T) {
	required := RequireContext(context.Background())
	m, _ := phase19TestSignedConcurrentInstallation(t)
	if got, err := runtimeForEffect(required, true, m); got != nil || !errors.Is(err, ErrRestricted) {
		t.Fatalf("required context used global fallback: %v %v", got, err)
	}
	if got, err := runtimeForEffect(WithManager(required, m), false, nil); got != m || err != nil {
		t.Fatalf("explicit valid manager failed: %v %v", got, err)
	}
}
func TestLICA9Phase45CancelledContextCannotRunEffect(t *testing.T) {
	ctx, cancel := context.WithCancel(WithManager(context.Background(), phase45FixtureManager(t)))
	cancel()
	if _, err := runtimeForEffect(ctx, false, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context not rejected: %v", err)
	}
}
func TestLICA9Phase45RestrictedExplicitManagerCannotExecuteEffect(t *testing.T) {
	ctx := WithManager(context.Background(), phase45FixtureManager(t))
	ran := false
	if err := RuntimeEffect(ctx, func() error { ran = true; return nil }); !errors.Is(err, ErrRestricted) || ran {
		t.Fatalf("restricted manager called side effect: %v %v", err, ran)
	}
}
func TestLICA9Phase45ValidExplicitManagerEffectAllowed(t *testing.T) {
	m, signed := phase19TestSignedConcurrentInstallation(t)
	if err := m.Accept(signed(m.runtimeSession, 1)); err != nil {
		t.Fatal(err)
	}
	ctx := WithManager(context.Background(), m)
	ran := 0
	if err := RuntimeEffect(ctx, func() error { ran++; return nil }); err != nil || ran != 1 {
		t.Fatalf("explicit valid context unexpected: %v %d", err, ran)
	}
	if err := CheckRuntime(ctx); err != nil {
		t.Fatal(err)
	}
}
func TestLICA9Phase45ChildProcessInitialState(t *testing.T) {
	if os.Getenv("LIC45_CHILD") != "YES" {
		return
	}
	// Started as a fresh process; test no other package test before assertion.
	ran := false
	if err := RuntimeEffect(context.Background(), func() error { ran = true; return nil }); !errors.Is(err, ErrRestricted) || ran {
		t.Fatalf("uninitialized effect fail-open: err=%v called=%v", err, ran)
	}
	if err := CheckRuntime(context.Background()); !errors.Is(err, ErrRestricted) {
		t.Fatalf("uninitialized CheckRuntime fail-open: %v", err)
	}
	called := false
	req, _ := http.NewRequest(http.MethodGet, "http://localhost/", nil)
	client := &http.Client{Transport: phase45RoundTripper(func(*http.Request) (*http.Response, error) { called = true; return nil, nil })}
	if resp, err := HTTP(context.Background(), client, req); !errors.Is(err, ErrRestricted) || resp != nil || called {
		t.Fatalf("unguarded HTTP: resp=%v err=%v called=%v", resp, err, called)
	}
}

type phase45RoundTripper func(*http.Request) (*http.Response, error)

func (f phase45RoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestLICA9Phase45ProcessStartsFailClosed(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestLICA9Phase45ChildProcessInitialState$", "-test.v")
	cmd.Env = append(os.Environ(), "LIC45_CHILD=YES")
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "--- PASS: TestLICA9Phase45ChildProcessInitialState") {
		t.Fatalf("child process initial state gate FAILED: %v %s", err, out)
	}
}
