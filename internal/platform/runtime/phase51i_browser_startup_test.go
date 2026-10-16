//go:build lic_a9_browser_test

package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/naseri-coder/TELMIVO/internal/platform/config"
)

func phase51iSyntheticBrowserConfig() config.Config {
	return config.Config{
		Environment: "test",
		Synthetic: true,
		HTTPAddr: "127.0.0.1:8080",
		DatabaseURL: "postgres://telmivo_runtime:synthetic@127.0.0.1:5432/telmivo_test?sslmode=disable",
		RedisURL: "redis://127.0.0.1:6379/15",
		NATSURL: "nats://127.0.0.1:4222",
		MiniAppURL: "http://127.0.0.1:5173",
	}
}

func phase51iSyntheticBrowserEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("TELMIVO_ENV", "test")
	t.Setenv("TELMIVO_LICENSE_ENVIRONMENT", "TEST")
	t.Setenv("SYNTHETIC_MODE", "true")
	t.Setenv("TELMIVO_LIC_A9_BROWSER_TEST", "YES")
	t.Setenv("TEST_DATABASE_URL", "postgres://telmivo_runtime:synthetic@127.0.0.1:5432/telmivo_test?sslmode=disable")
	t.Setenv("TEST_MIGRATION_DATABASE_URL", "postgres://telmivo_migration:synthetic@127.0.0.1:5432/telmivo_test?sslmode=disable")
}

func TestLICA9Phase51IBrowserNativeSignedV2Startup(t *testing.T) {
	if filepath.Base(os.Args[0]) != "telmivo-lic-a9-browser-test-api" {
		t.Skip("requires Go test binary renamed with go test -c -o telmivo-lic-a9-browser-test-api")
	}
	phase51iSyntheticBrowserEnvironment(t)
	m, client, cleanup, strict, err := openStartupLicense("api", phase51iSyntheticBrowserConfig())
	if err != nil || !strict || m == nil || client != nil {
		t.Fatalf("real TEST V2 API startup: strict=%t manager=%t client=%t err=%v", strict, m != nil, client != nil, err)
	}
	defer cleanup()
	if d := m.Decision(); d.State != "VALID_ONLINE" {
		t.Fatalf("ephemeral signed V2 not valid: %+v", d)
	}
	called := false
	if err := m.Effect(context.Background(), func() error { called = true; return nil }); err != nil || !called {
		t.Fatalf("signed V2 callback rejected: %v called=%t", err, called)
	}
}

func TestLICA9Phase51IBrowserNativeFailClosed(t *testing.T) {
	if filepath.Base(os.Args[0]) != "telmivo-lic-a9-browser-test-api" {
		t.Skip("requires renamed disposable Go test executable")
	}
	for _, testcase := range []struct {
		name string
		change func(*config.Config)
		key string
		value string
	}{
		{"worker_role_will_be_tested_separately", nil, "", ""},
		{"http_public", func(c *config.Config) { c.HTTPAddr="0.0.0.0:8080" }, "", ""},
		{"production", func(c *config.Config) { c.Environment="production" }, "TELMIVO_ENV", "production"},
		{"remote_db", func(c *config.Config) { c.DatabaseURL="postgres://telmivo_runtime:synthetic@db.example:5432/telmivo_test?sslmode=disable" }, "", ""},
		{"no_optin", nil, "TELMIVO_LIC_A9_BROWSER_TEST", ""},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			phase51iSyntheticBrowserEnvironment(t)
			c := phase51iSyntheticBrowserConfig()
			if testcase.change != nil { testcase.change(&c) }
			if testcase.key != "" { t.Setenv(testcase.key, testcase.value) }
			role := "api"
			if testcase.name == "worker_role_will_be_tested_separately" { role = "worker" }
			manager, _, cleanup, strict, err := openStartupLicense(role, c)
			cleanup()
			if !strict || err == nil || manager != nil {
				t.Fatalf("unsafe native API started: strict=%t error=%v", strict, err)
			}
		})
	}
	// This test never calls InstallRuntime: process-global state is not
	// mutated, and a failed browser startup cannot authorize effects.
}
