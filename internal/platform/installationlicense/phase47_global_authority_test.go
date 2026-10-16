package installationlicense

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// All fixtures use ephemeral TEST-only signed V2 leases in the genuine Product
// package. No external provider, Authority, Production server or credentials.
func phase47SignedManager(t *testing.T) *Manager {
	t.Helper()
	m, sign := phase19TestSignedConcurrentInstallation(t)
	if err := m.Accept(sign(m.runtimeSession, 1)); err != nil {
		t.Fatal(err)
	}
	return m
}

func phase47Denied(t *testing.T, ctx context.Context) {
	t.Helper()
	called := false
	err := RuntimeEffect(ctx, func() error { called = true; return nil })
	if !errors.Is(err, ErrRestricted) || called {
		t.Fatalf("RuntimeEffect escaped global guard: err=%v called=%v", err, called)
	}
	if err := CheckRuntime(ctx); !errors.Is(err, ErrRestricted) {
		t.Fatalf("CheckRuntime escaped global guard: %v", err)
	}
	calls := 0
	client := &http.Client{Transport: phase47RoundTripper(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 204, Body: http.NoBody}, nil
	})}
	req, _ := http.NewRequest(http.MethodPost, "http://invalid.local/protected", nil)
	resp, err := HTTP(ctx, client, req)
	if !errors.Is(err, ErrRestricted) || calls != 0 || resp != nil {
		t.Fatalf("HTTP escaped global guard: err=%v calls=%d resp=%v", err, calls, resp)
	}
}

type phase47RoundTripper func(*http.Request) (*http.Response, error)

func (fn phase47RoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func TestLICA9Phase47ChildCase(t *testing.T) {
	scenario := os.Getenv("LIC_A9_PHASE47_CASE")
	if scenario == "" {
		return
	}
	ctx := context.Background()
	switch scenario {
	case "no-installed-global":
		InstallRuntime(nil)
		phase47Denied(t, WithManager(ctx, phase47SignedManager(t)))
	case "foreign-manager":
		official, foreign := phase47SignedManager(t), phase47SignedManager(t)
		InstallRuntime(official)
		phase47Denied(t, WithManager(ctx, foreign))
		if err := CheckRuntime(WithManager(ctx, official)); err != nil {
			t.Fatalf("same installed Manager denied: %v", err)
		}
		called := 0
		if err := RuntimeEffect(ctx, func() error { called++; return nil }); err != nil || called != 1 {
			t.Fatalf("global legitimate effect: %v %d", err, called)
		}
		if err := RuntimeEffect(WithManager(ctx, official), func() error { called++; return nil }); err != nil || called != 2 {
			t.Fatalf("identical context Manager denied: %v %d", err, called)
		}
	case "global-restriction-prevails":
		former := phase47SignedManager(t)
		InstallRuntime(former)
		InstallRuntime(nil)
		phase47Denied(t, WithManager(ctx, former))
	case "pre-install-explicit":
		manager := phase47SignedManager(t)
		ctx = WithManager(ctx, manager)
		phase47Denied(t, ctx)
		phase47Denied(t, RequireContext(ctx))
		InstallRuntime(manager)
		if err := RuntimeEffect(RequireContext(ctx), func() error { return nil }); err != nil {
			t.Fatalf("after approved install, signed Manager denied: %v", err)
		}
	case "cancellation":
		manager := phase47SignedManager(t)
		InstallRuntime(manager)
		ctx, cancel := context.WithCancel(WithManager(context.Background(), manager))
		cancel()
		called := false
		if err := RuntimeEffect(ctx, func() error { called = true; return nil }); !errors.Is(err, context.Canceled) || called {
			t.Fatalf("cancelled effect escaped: %v %v", err, called)
		}
	case "concurrent-foreign-denial":
		official, foreign := phase47SignedManager(t), phase47SignedManager(t)
		InstallRuntime(official)
		bound := WithManager(ctx, foreign)
		var called, unexpected atomic.Int64
		var group sync.WaitGroup
		for i := 0; i < 16; i++ {
			group.Add(1)
			go func() {
				defer group.Done()
				for j := 0; j < 60; j++ {
					err := RuntimeEffect(bound, func() error { called.Add(1); return nil })
					if !errors.Is(err, ErrRestricted) {
						unexpected.Add(1)
					}
				}
			}()
		}
		for i := 0; i < 100; i++ {
			InstallRuntime(nil)
			InstallRuntime(official)
		}
		group.Wait()
		if called.Load() != 0 || unexpected.Load() != 0 {
			t.Fatalf("foreign override under concurrent install/restrict: called=%d unexpected=%d", called.Load(), unexpected.Load())
		}
	default:
		t.Fatal(fmt.Sprintf("unknown case %q", scenario))
	}
}

func TestLICA9Phase47FreshProcessGlobalAuthorityMatrix(t *testing.T) {
	for _, scenario := range []string{"no-installed-global", "foreign-manager", "global-restriction-prevails", "pre-install-explicit", "cancellation", "concurrent-foreign-denial"} {
		t.Run(scenario, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestLICA9Phase47ChildCase$", "-test.v")
			cmd.Env = append(os.Environ(), "LIC_A9_PHASE47_CASE="+scenario)
			output, err := cmd.CombinedOutput()
			if err != nil || !strings.Contains(string(output), "--- PASS: TestLICA9Phase47ChildCase") {
				t.Fatalf("case %s failed: %v\n%s", scenario, err, output)
			}
		})
	}
}
