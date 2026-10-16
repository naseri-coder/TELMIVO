package installationlicense

import (
	"context"
	"errors"
	"net/http"
	"syscall"
	"testing"
	"time"
)

type phase51eRoundTripFunc func(*http.Request) (*http.Response, error)

func (f phase51eRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestLICA9Phase51EAdministrativeLockReleasedAfterCallback(t *testing.T) {
	m := &Manager{cfg: Config{Directory: t.TempDir()}}
	n := 0
	if err := m.administrativeLock(context.Background(), func() error {
		n++
		return nil
	}); err != nil || n != 1 {
		t.Fatalf("administrative lock failed: err=%v calls=%d", err, n)
	}
	if err := m.administrativeLock(context.Background(), func() error {
		n++
		return nil
	}); err != nil || n != 2 {
		t.Fatalf("administrative lock was not released: err=%v calls=%d", err, n)
	}
}

func TestLICA9Phase51EAdministrativeContextCancellationNeverMutates(t *testing.T) {
	m := &Manager{cfg: Config{Directory: t.TempDir()}}
	ran := false
	err := m.lockMode(syscall.LOCK_EX, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 70*time.Millisecond)
		defer cancel()
		inner := m.administrativeLock(ctx, func() error {
			ran = true
			return nil
		})
		if !errors.Is(inner, context.DeadlineExceeded) {
			t.Errorf("canceled admin call was not rejected: %v", inner)
		}
		return nil
	})
	if err != nil || ran {
		t.Fatalf("nested admin mutated state: err=%v ran=%t", err, ran)
	}
}

func TestLICA9Phase51EAdministrativeBusyRemainsPending(t *testing.T) {
	m := &Manager{cfg: Config{Directory: t.TempDir()}}
	ran := false
	err := m.lockMode(syscall.LOCK_EX, func() error {
		start := time.Now()
		inner := m.administrativeLock(context.Background(), func() error {
			ran = true
			return nil
		})
		if !errors.Is(inner, ErrPending) {
			t.Errorf("nested administrative operation did not return pending: %v", inner)
		}
		if time.Since(start) > 4*time.Second {
			t.Error("nested administrative operation exceeded bounded wait")
		}
		return nil
	})
	if err != nil || ran {
		t.Fatalf("nested admin unexpectedly ran: err=%v ran=%t", err, ran)
	}
}

func TestLICA9Phase51ESignedV2BusyAdministrationNeverCallsAuthority(t *testing.T) {
	// Genuine ephemeral, TEST-only Ed25519-signed Product fixture.
	m, _ := phase34LicensedManager(t)
	hits := 0
	cl := &Client{
		manager: m,
		base:    "https://authority.invalid",
		http: &http.Client{Transport: phase51eRoundTripFunc(func(*http.Request) (*http.Response, error) {
			hits++
			return nil, errors.New("unexpected authority request")
		})},
	}
	for _, operation := range []struct {
		name string
		call func(context.Context) error
	}{
		{"refresh-mutation", func(ctx context.Context) error { return cl.mutate(ctx, "refresh", []byte("{}")) }},
		{"status", func(ctx context.Context) error { return cl.Status(ctx) }},
		{"activation", func(ctx context.Context) error { return cl.Activate(ctx, "synthetic-test-code", "invalid-id", 1) }},
		{"concurrency", func(ctx context.Context) error { return cl.Concurrency(ctx) }},
		{"refresh", func(ctx context.Context) error { return cl.Refresh(ctx) }},
		{"deactivate", func(ctx context.Context) error { return cl.Deactivate(ctx) }},
	} {
		t.Run(operation.name, func(t *testing.T) {
			err := m.lock(func() error {
				ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
				defer cancel()
				got := operation.call(ctx)
				if !errors.Is(got, context.DeadlineExceeded) {
					t.Errorf("%s expected cancellation before lock admission: %v", operation.name, got)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
	if hits != 0 {
		t.Fatalf("network request escaped busy lock: count=%d", hits)
	}
	if d := m.Decision(); d.State != "VALID_ONLINE" {
		t.Fatalf("signed V2 state changed after denied admin attempts: %+v", d)
	}
	var st State
	if err := m.lock(func() error { return m.read("state.json", &st) }); err != nil {
		t.Fatal(err)
	}
	if st.Pending != nil {
		t.Fatalf("busy admin attempt created a persistent operation: kind=%s", st.Pending.Kind)
	}
}

