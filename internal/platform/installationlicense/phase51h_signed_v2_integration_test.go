//go:build integration

package installationlicense

import (
	"context"
	"errors"
	"testing"
)

const phase51HNativeURL = "postgres://telmivo_runtime:synthetic@127.0.0.1:5432/telmivo_test?sslmode=disable"
const phase51HMigrationURL = "postgres://telmivo_migration:synthetic@localhost:5432/telmivo_test?sslmode=disable"

func phase51HSetValidTestGate(t *testing.T) {
	t.Helper()
	t.Setenv("TELMIVO_ENV", "test")
	t.Setenv("TELMIVO_LICENSE_ENVIRONMENT", "TEST")
	t.Setenv("TELMIVO_LIC_A9_NATIVE_FIXTURE", "YES")
	t.Setenv("TEST_DATABASE_URL", phase51HNativeURL)
	t.Setenv("TEST_MIGRATION_DATABASE_URL", phase51HMigrationURL)
}

func TestLICA9Phase51HRealSignedV2FixtureAdmittedOnlyForNativeTest(t *testing.T) {
	phase51HSetValidTestGate(t)
	m, err := NewTestOnlySignedV2RuntimeForNativeIntegration(t.TempDir())
	if err != nil {
		t.Fatalf("real signed V2 TEST-only manager fixture: %v", err)
	}
	if d := m.Decision(); d.State != "VALID_ONLINE" {
		t.Fatalf("real test V2 invalid: %+v", d)
	}
	count := 0
	if err := m.Effect(context.Background(), func() error { count++; return nil }); err != nil || count != 1 {
		t.Fatalf("signed fixture effect failed: %v count=%d", err, count)
	}
}

func TestLICA9Phase51HFixtureFailsClosed(t *testing.T) {
	for _, scenario := range []struct{ name, change, value string }{
		{"explicit_opt_in_missing", "TELMIVO_LIC_A9_NATIVE_FIXTURE", ""},
		{"production_env", "TELMIVO_ENV", "production"},
		{"production_license_env", "TELMIVO_LICENSE_ENVIRONMENT", "PRODUCTION"},
		{"production_runtime_dsn", "TEST_DATABASE_URL", "postgres://runtime:synthetic@127.0.0.1:5432/telmivo_prod"},
		{"remote_migration_dsn", "TEST_MIGRATION_DATABASE_URL", "postgres://migration:synthetic@license.example:5432/telmivo_test"},
		{"nonpostgres_dsn", "TEST_DATABASE_URL", "file:///telmivo_test"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			phase51HSetValidTestGate(t)
			t.Setenv(scenario.change, scenario.value)
			_, err := NewTestOnlySignedV2RuntimeForNativeIntegration(t.TempDir())
			if !errors.Is(err, ErrState) {
				t.Fatalf("unsafe test fixture not rejected: %v", err)
			}
		})
	}
}
