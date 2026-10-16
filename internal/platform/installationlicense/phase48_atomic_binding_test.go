package installationlicense

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// Local signing keys are provided by the real Product's Phase19 fixture when
// this test is compiled in the genuine installationlicense package.
func phase48SignedManager(t *testing.T) *Manager {
	t.Helper()
	m, sign := phase19TestSignedConcurrentInstallation(t)
	if err := m.Accept(sign(m.runtimeSession, 1)); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestLICA9Phase48ChildBinding(t *testing.T) {
	scenario := os.Getenv("LIC_A9_PHASE48_CHILD")
	if scenario == "" {
		return
	}
	ctx := context.Background()
	denied := func(c context.Context) {
		t.Helper()
		called := false
		if e := RuntimeEffect(c, func() error { called = true; return nil }); !errors.Is(e, ErrRestricted) || called {
			t.Fatalf("effect escaped: %v %v", e, called)
		}
		if e := CheckRuntime(c); !errors.Is(e, ErrRestricted) {
			t.Fatalf("check escaped: %v", e)
		}
	}
	switch scenario {
	case "fresh":
		denied(ctx)
		// Explicit context-bound owner is permitted before installation and
		// still must pass its own signed verification in the real Product.
		m := phase48SignedManager(t)
		if err := RuntimeEffect(WithManager(ctx, m), func() error { return nil }); err != nil {
			t.Fatal(err)
		}
	case "nil-installed":
		foreign := phase48SignedManager(t)
		InstallRuntime(nil)
		denied(WithManager(ctx, foreign))
		denied(ctx)
	case "valid-installed":
		m := phase48SignedManager(t)
		InstallRuntime(m)
		if err := CheckRuntime(ctx); err != nil {
			t.Fatal(err)
		}
		if err := RuntimeEffect(ctx, func() error { return nil }); err != nil {
			t.Fatal(err)
		}
		if err := RuntimeEffect(WithManager(ctx, m), func() error { return nil }); err != nil {
			t.Fatal(err)
		}
		denied(WithManager(ctx, phase48SignedManager(t)))
	case "reinstall":
		m := phase48SignedManager(t)
		InstallRuntime(m)
		first := installedRuntime.Load()
		if first == nil || first.manager != m {
			t.Fatal("initial invalid")
		}
		InstallRuntime(nil)
		second := installedRuntime.Load()
		if first == second || second == nil || second.manager != nil {
			t.Fatal("binding must be immutable and nonnil")
		}
		denied(WithManager(ctx, m))
		InstallRuntime(m)
		if err := RuntimeEffect(ctx, func() error { return nil }); err != nil {
			t.Fatal(err)
		}
	case "concurrent-snapshots":
		a, b := phase48SignedManager(t), phase48SignedManager(t)
		InstallRuntime(nil)
		var bad atomic.Int64
		var wg sync.WaitGroup
		for i := 0; i < 6; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				for j := 0; j < 12; j++ {
					binding := installedRuntime.Load()
					if binding == nil {
						bad.Add(1)
						return
					}
					if binding.manager != nil && binding.manager != a && binding.manager != b {
						bad.Add(1)
					}
					_ = CheckRuntime(ctx)
					_ = RuntimeEffect(WithManager(ctx, a), func() error { return nil })
				}
			}(i)
		}
		for j := 0; j < 100; j++ {
			InstallRuntime(a)
			InstallRuntime(nil)
			InstallRuntime(b)
		}
		wg.Wait()
		if bad.Load() != 0 {
			t.Fatalf("torn binding: %d", bad.Load())
		}
	case "cancelled":
		m := phase48SignedManager(t)
		InstallRuntime(m)
		c, cancel := context.WithCancel(WithManager(ctx, m))
		cancel()
		if err := RuntimeEffect(c, func() error { return nil }); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled: %v", err)
		}
	default:
		t.Fatal(fmt.Sprintf("unknown child scenario %q", scenario))
	}
}
func TestLICA9Phase48SeparateProcessBindingMatrix(t *testing.T) {
	for _, scenario := range []string{"fresh", "nil-installed", "valid-installed", "reinstall", "concurrent-snapshots", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestLICA9Phase48ChildBinding$", "-test.v")
			cmd.Env = append(os.Environ(), "LIC_A9_PHASE48_CHILD="+scenario)
			output, err := cmd.CombinedOutput()
			if err != nil || !strings.Contains(string(output), "--- PASS: TestLICA9Phase48ChildBinding") {
				t.Fatalf("subprocess %s: %v\n%s", scenario, err, output)
			}
		})
	}
}
