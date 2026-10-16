//go:build integration

package installationlicense

import "testing"

func phase51mGoodTargets() map[string]string {
	return map[string]string{
		"TEST_DATABASE_URL":           "postgres://telmivo_runtime:synthetic@127.0.0.1:5432/telmivo_test?sslmode=disable",
		"TEST_MIGRATION_DATABASE_URL": "postgres://telmivo_migration:synthetic@127.0.0.1:5432/telmivo_test?sslmode=disable",
		"TEST_NATS_URL":               "nats://127.0.0.1:4222",
		"TELMIVO_ENV":                 "test",
		"TELMIVO_LICENSE_ENVIRONMENT": "TEST",
		"SYNTHETIC_MODE":              "true",
	}
}
func phase51mCheck(m map[string]string) error {
	return NativeIntegrationTargetAdmission(func(key string) (string, bool) { v, ok := m[key]; return v, ok })
}
func TestLICA9Phase51MNativeGoDirectAdmission(t *testing.T) {
	if err := phase51mCheck(phase51mGoodTargets()); err != nil {
		t.Fatal(err)
	}
}
func TestLICA9Phase51MNativeGoRestoreTargets(t *testing.T) {
	m := phase51mGoodTargets()
	m["TEST_RESTORE_DATABASE_URL"] = "postgres://telmivo_runtime:synthetic@127.0.0.1:5433/telmivo_test?sslmode=disable"
	m["TEST_RESTORE_MIGRATION_DATABASE_URL"] = "postgres://telmivo_migration:synthetic@127.0.0.1:5433/telmivo_test?sslmode=disable"
	if err := phase51mCheck(m); err != nil {
		t.Fatal(err)
	}
}
func TestLICA9Phase51MNativeGoRejectsOverrides(t *testing.T) {
	cases := []struct{ name, key, value string }{
		{"driver_host_override", "TEST_DATABASE_URL", "postgres://telmivo_runtime:synthetic@127.0.0.1:5432/telmivo_test?sslmode=disable&host=production.example"},
		{"driver_service_override", "TEST_DATABASE_URL", "postgres://telmivo_runtime:synthetic@127.0.0.1:5432/telmivo_test?sslmode=disable&service=prod"},
		{"untrusted_host", "TEST_DATABASE_URL", "postgres://telmivo_runtime:synthetic@database.example:5432/telmivo_test?sslmode=disable"},
		{"untrusted_db", "TEST_DATABASE_URL", "postgres://telmivo_runtime:synthetic@127.0.0.1:5432/telmivo_prod?sslmode=disable"},
		{"wrong_role", "TEST_DATABASE_URL", "postgres://postgres:synthetic@127.0.0.1:5432/telmivo_test?sslmode=disable"},
		{"wrong_port", "TEST_DATABASE_URL", "postgres://telmivo_runtime:synthetic@127.0.0.1:6432/telmivo_test?sslmode=disable"},
		{"missing_password", "TEST_DATABASE_URL", "postgres://telmivo_runtime@127.0.0.1:5432/telmivo_test?sslmode=disable"},
		{"remote_nats", "TEST_NATS_URL", "nats://broker.example:4222"},
		{"inherited_remote_db", "DATABASE_URL", "postgres://runtime:secret@db.example:5432/telmivo_prod?sslmode=disable"},
		{"inherited_remote_nats", "NATS_URL", "nats://broker.example:4222"},
		{"production", "TELMIVO_ENV", "production"},
		{"production_license", "TELMIVO_LICENSE_ENVIRONMENT", "PRODUCTION"},
		{"external_PGHOST", "PGHOST", "production.example"},
		{"fake_PGDATABASE", "PGDATABASE", "telmivo_prod"},
		{"absent_runtime", "TEST_DATABASE_URL", ""},
		{"explicit_browser_fixture", "TELMIVO_LIC_A9_BROWSER_TEST", "YES"},
		{"explicit_native_fixture", "TELMIVO_LIC_A9_NATIVE_FIXTURE", "YES"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := phase51mGoodTargets()
			m[tc.key] = tc.value
			if err := phase51mCheck(m); err == nil {
				t.Fatal("unsafe direct Go native test target was admitted")
			}
		})
	}
}
func TestLICA9Phase51MNativeGoRejectsIncompleteRestore(t *testing.T) {
	m := phase51mGoodTargets()
	m["TEST_RESTORE_DATABASE_URL"] = "postgres://telmivo_runtime:synthetic@127.0.0.1:5433/telmivo_test?sslmode=disable"
	if err := phase51mCheck(m); err == nil {
		t.Fatal("single restore DSN was accepted")
	}
}
func TestLICA9Phase51MNativeGoRejectsNilLookup(t *testing.T) {
	if err := NativeIntegrationTargetAdmission(nil); err == nil {
		t.Fatal("nil environment accepted")
	}
}
