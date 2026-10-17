package config

import (
	"strings"
	"testing"
)

func roleTestDSN(user, mode string) string {
	return "postgresql://" + user + ":synthetic-secret@127.0.0.1:5432/telmivo_test?sslmode=" + mode
}

func TestA203RoleMapping(t *testing.T) {
	for _, tc := range []struct{ service, expected string }{
		{"api", "telmivo_api"}, {"main-bot", "telmivo_api"},
		{"group-bot", "telmivo_api"}, {"downloader-bot", "telmivo_api"},
		{"customer-bot-runtime", "telmivo_api"}, {"worker", "telmivo_worker"},
		{"media-worker", "telmivo_worker"},
	} {
		t.Run(tc.service, func(t *testing.T) {
			role, err := ExpectedDatabaseRole(tc.service)
			if err != nil || role != tc.expected {
				t.Fatalf("unexpected role = %q, err=%v", role, err)
			}
			if err := ValidateDatabaseRoleDSN(tc.service, roleTestDSN(tc.expected, "disable"), "test"); err != nil {
				t.Fatalf("valid isolated role rejected: %v", err)
			}
		})
	}
	for _, name := range []string{"", "migrate", "unknown", "administrator"} {
		if _, err := ExpectedDatabaseRole(name); err == nil {
			t.Fatalf("unrecognized service role %q admitted", name)
		}
	}
}
func TestA203DSNRejectsConfusedIdentityAndOverrides(t *testing.T) {
	valid := roleTestDSN("telmivo_worker", "require")
	cases := []string{
		roleTestDSN("telmivo_api", "require"),
		roleTestDSN("telmivo_runtime", "require"),
		"postgresql://telmivo_worker@127.0.0.1:5432/telmivo_test?sslmode=require",
		"postgresql://telmivo_worker:secret@127.0.0.1/telmivo_test?sslmode=require",
		"postgresql://telmivo_worker:secret@127.0.0.1:5432/?sslmode=require",
		"postgresql://telmivo_worker:secret@127.0.0.1:5432/telmivo_test",
		valid + "&host=production.example",
		valid + "&hostaddr=127.0.0.2",
		valid + "&service=production",
		valid + "&user=telmivo_api",
		valid + "&password=another",
		valid + "&options=-c%20role%3Dpostgres",
		valid + "&dbname=prod",
		valid + "&sslmode=disable",
		valid + "#fragment",
		"mysql://telmivo_worker:secret@127.0.0.1:5432/telmivo_test?sslmode=require",
	}
	for _, raw := range cases {
		t.Run(strings.ReplaceAll(raw, "/", "_"), func(t *testing.T) {
			if err := ValidateDatabaseRoleDSN("worker", raw, "production"); err == nil {
				t.Fatal("confused PostgreSQL credential admitted")
			}
		})
	}
	if err := ValidateDatabaseRoleDSN("worker", valid, "production"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDatabaseRoleDSN("worker", roleTestDSN("telmivo_worker", "disable"), "production"); err == nil {
		t.Fatal("production TLS disable allowed")
	}
}
func TestA203OptInFailClosedConfig(t *testing.T) {
	basic := map[string]string{
		"TELMIVO_ENV": "test",
		"DATABASE_URL": roleTestDSN("telmivo_worker", "disable"),
		"REDIS_URL": "redis://127.0.0.1:6379",
		"NATS_URL": "nats://127.0.0.1:4222",
		"TELMIVO_DB_ROLE_SPLIT": "required",
	}
	load := func(changes map[string]string) error {
		opts := map[string]string{}
		for k, v := range basic {
			opts[k] = v
		}
		for k, v := range changes {
			opts[k] = v
		}
		c, err := Load(func(key string) string { return opts[key] })
		if err == nil && !c.DatabaseRoleSplitRequired {
			t.Fatal("required mode silently disabled")
		}
		return err
	}
	if err := load(nil); err != nil {
		t.Fatal(err)
	}
	for _, changes := range []map[string]string{
		{"TELMIVO_DB_ROLE_SPLIT": "enabled"},
		{"PGSERVICE": "prod"},
		{"PGOPTIONS": "-c role=postgres"},
		{"PGHOST": "db.production"},
		{"PGPASSWORD": "inherited-secret"},
		{"TENANT_DATABASE_URL": roleTestDSN("telmivo_api", "disable")},
		{"WORKER_DATABASE_URL": roleTestDSN("telmivo_worker", "disable")},
		{"MIGRATION_DATABASE_URL": roleTestDSN("telmivo_migration", "disable")},
	} {
		if err := load(changes); err == nil {
			t.Fatalf("unsafe config admitted: keys=%v", changes)
		}
	}
	if err := load(map[string]string{
		"TELMIVO_DB_ROLE_SPLIT": "",
		"DATABASE_URL":          roleTestDSN("telmivo_api", "disable"),
	}); err == nil {
		t.Fatal("partial dedicated role rollout without admission flag accepted")
	}
}
