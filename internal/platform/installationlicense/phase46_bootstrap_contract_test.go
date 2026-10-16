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

func phase46TestSignedManager(t *testing.T) *Manager {
	t.Helper()
	m, sign := phase19TestSignedConcurrentInstallation(t)
	if err := m.Accept(sign(m.runtimeSession, 1)); err != nil {
		t.Fatal(err)
	}
	return m
}

func phase46AssertUnavailable(t *testing.T) {
	t.Helper()
	ran := false
	if err := RuntimeEffect(context.Background(), func() error { ran = true; return nil }); !errors.Is(err, ErrRestricted) || ran {
		t.Fatalf("RuntimeEffect fail-open: err=%v called=%v", err, ran)
	}
	if err := CheckRuntime(context.Background()); !errors.Is(err, ErrRestricted) {
		t.Fatalf("CheckRuntime fail-open: %v", err)
	}
	count := 0
	request, err := http.NewRequest(http.MethodPost, "http://invalid.local/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: phase45RoundTripper(func(*http.Request) (*http.Response, error) {
		count++
		return &http.Response{StatusCode: 204, Body: http.NoBody}, nil
	})}
	response, err := HTTP(context.Background(), client, request)
	if !errors.Is(err, ErrRestricted) || count != 0 || response != nil {
		t.Fatalf("HTTP fail-open: err=%v roundTrips=%d response=%v", err, count, response)
	}
}

func TestLICA9Phase46ChildBootstrapCases(t *testing.T) {
	scenario := os.Getenv("LIC_A9_PHASE46_SCENARIO")
	if scenario == "" {
		return
	}
	switch scenario {
	case "fresh":
		phase46AssertUnavailable(t)
	case "nil-runtime":
		InstallRuntime(nil)
		phase46AssertUnavailable(t)
	case "valid-runtime":
		m := phase46TestSignedManager(t)
		InstallRuntime(m)
		if err := CheckRuntime(context.Background()); err != nil {
			t.Fatalf("valid installed Manager denied: %v", err)
		}
		called := 0
		if err := RuntimeEffect(context.Background(), func() error { called++; return nil }); err != nil || called != 1 {
			t.Fatalf("valid effect denied: %v / %d", err, called)
		}
		client := &http.Client{Transport: phase45RoundTripper(func(*http.Request) (*http.Response, error) {
			called++
			return &http.Response{StatusCode: 204, Body: http.NoBody}, nil
		})}
		req, _ := http.NewRequest(http.MethodPost, "http://invalid.local/synthetic", nil)
		resp, err := HTTP(context.Background(), client, req)
		if err != nil || resp == nil || resp.StatusCode != 204 || called != 2 {
			t.Fatalf("legitimate guarded HTTP failed: err=%v resp=%v count=%d", err, resp, called)
		}
		if err := CheckRuntime(RequireContext(context.Background())); !errors.Is(err, ErrRestricted) {
			t.Fatalf("RequireContext unexpectedly used global Manager: %v", err)
		}
		ctx := WithManager(RequireContext(context.Background()), m)
		if err := RuntimeEffect(ctx, func() error { called++; return nil }); err != nil || called != 3 {
			t.Fatalf("explicit Manager with required context failed: %v / %d", err, called)
		}
	case "late-install":
		InstallRuntime(nil)
		phase46AssertUnavailable(t)
		InstallRuntime(phase46TestSignedManager(t))
		if err := CheckRuntime(context.Background()); !errors.Is(err, ErrRestricted) {
			t.Fatalf("nil-to-valid reinstallation must remain restricted: %v", err)
		}
		InstallRuntime(nil)
		phase46AssertUnavailable(t)
	case "concurrent-install":
		m := phase46TestSignedManager(t)
		var authorized atomic.Int64
		var bad atomic.Int64
		const workers = 24
		var group sync.WaitGroup
		ready := make(chan struct{})
		for i := 0; i < workers; i++ {
			group.Add(1)
			go func() {
				defer group.Done()
				<-ready
				for j := 0; j < 30; j++ {
					err := RuntimeEffect(context.Background(), func() error { authorized.Add(1); return nil })
					if err != nil && !errors.Is(err, ErrRestricted) {
						bad.Add(1)
					}
				}
			}()
		}
		close(ready)
		InstallRuntime(m)
		group.Wait()
		if bad.Load() != 0 {
			t.Fatalf("unexpected concurrent authorization error: %d", bad.Load())
		}
		if err := RuntimeEffect(context.Background(), func() error { authorized.Add(1); return nil }); err != nil {
			t.Fatalf("valid installed runtime not admitted: %v", err)
		}
		if authorized.Load() < 1 {
			t.Fatal("no authorized calls")
		}
		InstallRuntime(nil)
		phase46AssertUnavailable(t)
	case "webhook":
		InstallRuntime(nil)
		handler := Middleware(nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/health/live":
				w.WriteHeader(http.StatusOK)
			case "/webhook", "/api/v1/customer-bot-updates/account/bot":
				if CheckRuntime(r.Context()) == nil {
					t.Error("restricted webhook permitted protected action")
					w.WriteHeader(500)
					return
				}
				w.Header().Set("X-Telmivo-License", "restricted")
				w.WriteHeader(http.StatusOK)
			default:
				if err := RuntimeEffect(r.Context(), func() error { t.Error("protected callback executed"); return nil }); !errors.Is(err, ErrRestricted) {
					t.Errorf("bad effect status: %v", err)
				}
				w.WriteHeader(http.StatusOK)
			}
		}))
		probe := func(method, path string, want int) {
			t.Helper()
			r, _ := http.NewRequest(method, "http://localhost"+path, nil)
			w := newPhase46Recorder()
			handler.ServeHTTP(w, r)
			if w.status != want {
				t.Errorf("%s %s returned %d want %d", method, path, w.status, want)
			}
		}
		probe("GET", "/health/live", 200)
		probe("POST", "/webhook", 200)
		probe("POST", "/api/v1/customer-bot-updates/account/bot", 200)
		probe("POST", "/api/v1/commercial/effective", 403)
		probe("GET", "/shop", 200) // read-only response; no protected work
	default:
		t.Fatal(fmt.Sprintf("unknown scenario %q", scenario))
	}
}

type phase46Recorder struct {
	header http.Header
	status int
}

func newPhase46Recorder() *phase46Recorder     { return &phase46Recorder{header: make(http.Header)} }
func (r *phase46Recorder) Header() http.Header { return r.header }
func (r *phase46Recorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = 200
	}
	return len(p), nil
}
func (r *phase46Recorder) WriteHeader(status int) { r.status = status }

func TestLICA9Phase46FreshSubprocessBootstrapMatrix(t *testing.T) {
	scenarios := []string{"fresh", "nil-runtime", "valid-runtime", "late-install", "concurrent-install", "webhook"}
	for _, scenario := range scenarios {
		t.Run(scenario, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestLICA9Phase46ChildBootstrapCases$", "-test.v")
			cmd.Env = append(os.Environ(), "LIC_A9_PHASE46_SCENARIO="+scenario)
			output, err := cmd.CombinedOutput()
			if err != nil || !strings.Contains(string(output), "--- PASS: TestLICA9Phase46ChildBootstrapCases") {
				t.Fatalf("scenario %s failed: %v\n%s", scenario, err, output)
			}
		})
	}
}
