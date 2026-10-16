//go:build integration

package installationlicense

import (
	"errors"
	"net/url"
)

// NativeIntegrationTargetAdmission performs no network I/O. It must run in
// TestMain before a signed TEST fixture is created or any DB/broker is opened.
// The external shell admission guard is NOT a substitute for this guard:
// callers may invoke `go test -tags=integration` without that shell script.
func NativeIntegrationTargetAdmission(lookup func(string) (string, bool)) error {
	if lookup == nil {
		return errors.New("NATIVE_TEST_TARGET_LOOKUP_MISSING")
	}
	get := func(k string) string { v, _ := lookup(k); return v }
	if err := nativeIntegrationPostgres(get("TEST_DATABASE_URL"), "telmivo_runtime", "5432"); err != nil {
		return err
	}
	if err := nativeIntegrationPostgres(get("TEST_MIGRATION_DATABASE_URL"), "telmivo_migration", "5432"); err != nil {
		return err
	}
	if get("TEST_NATS_URL") != "nats://127.0.0.1:4222" {
		return errors.New("NATIVE_TEST_NATS_MUST_BE_LOOPBACK")
	}
	_, restore := lookup("TEST_RESTORE_DATABASE_URL")
	_, migrateRestore := lookup("TEST_RESTORE_MIGRATION_DATABASE_URL")
	if restore != migrateRestore {
		return errors.New("NATIVE_TEST_RESTORE_TARGET_PAIR_REQUIRED")
	}
	if restore {
		if err := nativeIntegrationPostgres(get("TEST_RESTORE_DATABASE_URL"), "telmivo_runtime", "5433"); err != nil {
			return err
		}
		if err := nativeIntegrationPostgres(get("TEST_RESTORE_MIGRATION_DATABASE_URL"), "telmivo_migration", "5433"); err != nil {
			return err
		}
	}
	for _, names := range [][2]string{
		{"DATABASE_URL", "TEST_DATABASE_URL"},
		{"MIGRATION_DATABASE_URL", "TEST_MIGRATION_DATABASE_URL"},
		{"NATS_URL", "TEST_NATS_URL"},
	} {
		if inherited, exists := lookup(names[0]); exists && inherited != get(names[1]) {
			return errors.New("NATIVE_TEST_INHERITED_TARGET_CONFLICT")
		}
	}
	for _, pair := range [][2]string{
		{"TELMIVO_ENV", "test"},
		{"TELMIVO_LICENSE_ENVIRONMENT", "TEST"},
		{"SYNTHETIC_MODE", "true"},
		{"PGHOST", "127.0.0.1"},
		{"PGPORT", "5432"},
		{"PGDATABASE", "telmivo_test"},
	} {
		if v, exists := lookup(pair[0]); exists && v != pair[1] {
			return errors.New("NATIVE_TEST_INHERITED_ENVIRONMENT_CONFLICT")
		}
	}
	for _, key := range []string{
		"PGHOSTADDR", "PGSERVICE", "PGSERVICEFILE", "PGOPTIONS",
		"TELMIVO_LIC_A9_BROWSER_TEST", "TELMIVO_LIC_A9_NATIVE_FIXTURE",
	} {
		if _, exists := lookup(key); exists {
			return errors.New("NATIVE_TEST_INHERITED_OVERRIDE_FORBIDDEN")
		}
	}
	return nil
}

func nativeIntegrationPostgres(raw, role, port string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") ||
		u.Host != "127.0.0.1:"+port || u.Path != "/telmivo_test" ||
		u.RawPath != "" || u.RawQuery != "sslmode=disable" || u.Fragment != "" ||
		u.Opaque != "" || u.ForceQuery || u.User == nil || u.User.Username() != role {
		return errors.New("NATIVE_TEST_POSTGRES_TARGET_FORBIDDEN")
	}
	password, ok := u.User.Password()
	if !ok || password == "" {
		return errors.New("NATIVE_TEST_POSTGRES_PASSWORD_MISSING")
	}
	return nil
}
