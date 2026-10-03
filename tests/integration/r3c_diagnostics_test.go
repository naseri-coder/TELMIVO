//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"
)

type r3cDiagnosticCapture struct {
	mu      sync.Mutex
	records []map[string]any
	job     string
	ready   chan struct{}
}

func (c *r3cDiagnosticCapture) Write(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var r map[string]any
	if json.Unmarshal(b, &r) == nil && len(c.records) < 4096 {
		c.records = append(c.records, r)
		if r["event"] == "downloader_awaiting_format" && r["download_job_id"] == c.job {
			select {
			case c.ready <- struct{}{}:
			default:
			}
		}
	}
	return len(b), nil
}

// The signal is emitted only after the real analysis transaction commits.
// There is no provider/API interception or fixed sleep synchronization.
func TestR3CDiagnostics(t *testing.T) {
	u := fixture(t, true)
	capture := &r3cDiagnosticCapture{ready: make(chan struct{}, 1)}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(io.MultiWriter(os.Stdout, capture), nil)))
	defer slog.SetDefault(previous)
	id := r3cSubmit(t, u, "r3c-diagnostic-discovery")
	capture.mu.Lock()
	capture.job = id
	capture.mu.Unlock()
	w := r3cWorker(t, r3cProvider{}, &r3cDelivery{})
	w.DispatchFilter = func(_ context.Context, user, workspace string) bool {
		return user == u.UserID && workspace == u.WorkspaceID
	}
	pollCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.Poll(pollCtx, 2) }()
	select {
	case <-capture.ready:
	case err := <-done:
		t.Fatalf("poll exited before committed analysis: %v", err)
	case <-time.After(8 * time.Second):
		cancel()
		<-done
		t.Fatal("no committed discovery/analysis diagnostic")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("poll did not stop")
	}
	if v := r3cView(t, u, id); v.State != "AWAITING_FORMAT" || len(v.Formats) != 2 {
		t.Fatalf("analysis state/format snapshot: %+v", v)
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	required := []string{"downloader_job_created", "downloader_dispatch_candidate_observed", "downloader_worker_run_started", "downloader_lease_acquire_attempt", "downloader_lease_acquired", "downloader_state_transition", "downloader_analysis_started", "downloader_analysis_completed", "downloader_awaiting_format"}
	position := 0
	for _, r := range capture.records {
		if r["download_job_id"] != id {
			continue
		}
		if position < len(required) && r["event"] == required[position] {
			position++
		}
		if _, ok := r["source_url"]; ok {
			t.Fatal("source URL exposed")
		}
	}
	if position != len(required) {
		t.Fatalf("missing ordered diagnostic %s", required[position])
	}
	raw, _ := json.Marshal(capture.records)
	if bytes.Contains(raw, []byte("dQw4w9WgXcQ")) {
		t.Fatal("source reference logged")
	}
}
