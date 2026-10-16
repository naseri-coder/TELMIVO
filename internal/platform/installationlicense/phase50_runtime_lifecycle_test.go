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
	"time"
)

type phase50Transport func(*http.Request) (*http.Response, error)

func (f phase50Transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func phase50Denied(t *testing.T, ctx context.Context) {
	t.Helper()
	called := false
	if err := RuntimeEffect(ctx, func() error { called = true; return nil }); !errors.Is(err, ErrRestricted) || called {
		t.Fatalf("unauthorized callback: err=%v called=%v", err, called)
	}
	if err := CheckRuntime(ctx); !errors.Is(err, ErrRestricted) {
		t.Fatalf("unauthorized CheckRuntime: %v", err)
	}
	request, _ := http.NewRequest(http.MethodPost, "http://invalid.local/phase50", nil)
	hits := 0
	client := &http.Client{Transport: phase50Transport(func(*http.Request) (*http.Response, error) {
		hits++
		return &http.Response{StatusCode: 204, Body: http.NoBody}, nil
	})}
	response, err := HTTP(ctx, client, request)
	if !errors.Is(err, ErrRestricted) || response != nil || hits != 0 {
		t.Fatalf("unauthorized HTTP: err=%v response=%v hits=%d", err, response, hits)
	}
}

func TestLICA9Phase50ChildLifecycle(t *testing.T) {
	scenario := os.Getenv("LIC_A9_PHASE50_CHILD")
	if scenario == "" {
		return
	}
	ctx := context.Background()
	switch scenario {
	case "preinstall-signed-context-denied":
		m := phase48SignedManager(t)
		phase50Denied(t, WithManager(ctx, m))
		phase50Denied(t, ctx)
		InstallRuntime(m)
		if err := RuntimeEffect(WithManager(ctx, m), func() error { return nil }); err != nil {
			t.Fatalf("first installed signed Manager denied: %v", err)
		}
	case "nil-install-terminal":
		m := phase48SignedManager(t)
		InstallRuntime(nil)
		InstallRuntime(m)
		phase50Denied(t, WithManager(ctx, m))
		if b := installedRuntime.Load(); b == nil || b.manager != nil {
			t.Fatal("terminal nil installation was revived")
		}
	case "foreign-rebind-terminal":
		official, foreign := phase48SignedManager(t), phase48SignedManager(t)
		InstallRuntime(official)
		InstallRuntime(foreign)
		phase50Denied(t, WithManager(ctx, official))
		InstallRuntime(official)
		phase50Denied(t, ctx)
	case "same-manager-idempotent":
		official := phase48SignedManager(t)
		InstallRuntime(official)
		before := installedRuntime.Load()
		InstallRuntime(official)
		if before != installedRuntime.Load() {
			t.Fatal("idempotent install unexpectedly rotated the binding")
		}
		if err := RuntimeEffect(ctx, func() error { return nil }); err != nil {
			t.Fatalf("same Manager was refused: %v", err)
		}
		InstallRuntime(nil)
		phase50Denied(t, ctx)
	case "revoke-waits-for-admitted-effect":
		official := phase48SignedManager(t)
		InstallRuntime(official)
		entered := make(chan struct{})
		release := make(chan struct{})
		effectDone := make(chan error, 1)
		go func() {
			effectDone <- RuntimeEffect(ctx, func() error {
				close(entered)
				<-release
				return nil
			})
		}()
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("effect did not enter")
		}
		revokeDone := make(chan struct{})
		go func() { InstallRuntime(nil); close(revokeDone) }()
		select {
		case <-revokeDone:
			t.Fatal("revocation returned while an effect remained admitted")
		case <-time.After(50 * time.Millisecond):
		}
		close(release)
		select {
		case err := <-effectDone:
			if err != nil {
				t.Fatalf("in-flight effect failed: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("in-flight effect failed to finish")
		}
		select {
		case <-revokeDone:
		case <-time.After(5 * time.Second):
			t.Fatal("revocation could not finish after effect")
		}
		phase50Denied(t, ctx)
	case "concurrent-revocation-stable":
		official := phase48SignedManager(t)
		InstallRuntime(official)
		var unexpected atomic.Int64
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 30; j++ {
					err := RuntimeEffect(ctx, func() error { return nil })
					if err != nil && !errors.Is(err, ErrRestricted) {
						unexpected.Add(1)
					}
				}
			}()
		}
		InstallRuntime(nil)
		InstallRuntime(official)
		wg.Wait()
		if unexpected.Load() != 0 {
			t.Fatalf("unexpected authorization errors: %d", unexpected.Load())
		}
		phase50Denied(t, ctx)
	case "cancelled-context":
		official := phase48SignedManager(t)
		InstallRuntime(official)
		cancelled, cancel := context.WithCancel(WithManager(ctx, official))
		cancel()
		called := false
		if err := RuntimeEffect(cancelled, func() error { called = true; return nil }); !errors.Is(err, context.Canceled) || called {
			t.Fatalf("cancellation escaped: err=%v called=%v", err, called)
		}
	default:
		t.Fatal(fmt.Sprintf("unknown lifecycle scenario %q", scenario))
	}
}

func TestLICA9Phase50FreshProcessLifecycleMatrix(t *testing.T) {
	for _, scenario := range []string{
		"preinstall-signed-context-denied", "nil-install-terminal", "foreign-rebind-terminal",
		"same-manager-idempotent", "revoke-waits-for-admitted-effect",
		"concurrent-revocation-stable", "cancelled-context",
	} {
		t.Run(scenario, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestLICA9Phase50ChildLifecycle$", "-test.v")
			cmd.Env = append(os.Environ(), "LIC_A9_PHASE50_CHILD="+scenario)
			output, err := cmd.CombinedOutput()
			if err != nil || !strings.Contains(string(output), "--- PASS: TestLICA9Phase50ChildLifecycle") {
				t.Fatalf("child %s: %v\n%s", scenario, err, output)
			}
		})
	}
}
