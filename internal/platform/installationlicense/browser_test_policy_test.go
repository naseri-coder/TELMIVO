//go:build lic_a9_browser_test

package installationlicense

import "testing"

func phase51iAllowedBrowser() BrowserTestAdmission {
	return BrowserTestAdmission{
		Executable: "/tmp/telmivo-lic-a9-browser-test-api", Role: "api",
		Environment: "test", LicenseEnvironment: "TEST", OptIn: "YES",
		Synthetic: "true", HTTPAddr: "127.0.0.1:8080",
		DatabaseURL:      "postgres://telmivo_runtime:synthetic@127.0.0.1:5432/telmivo_test?sslmode=disable",
		TestDatabaseURL:  "postgres://telmivo_runtime:synthetic@127.0.0.1:5432/telmivo_test?sslmode=disable",
		TestMigrationURL: "postgres://telmivo_migration:synthetic@127.0.0.1:5432/telmivo_test?sslmode=disable",
		RedisURL:         "redis://127.0.0.1:6379/15", NATSURL: "nats://127.0.0.1:4222",
		MiniAppURL: "http://127.0.0.1:5173",
	}
}

func TestLICA9Phase51IBrowserAdmission(t *testing.T) {
	if !phase51iAllowedBrowser().Allowed() {
		t.Fatal("synthetic native browser input refused")
	}
	cases := []struct {
		name   string
		change func(*BrowserTestAdmission)
	}{
		{"wrong_executable", func(v *BrowserTestAdmission) { v.Executable = "/tmp/telmivo-r2b-api" }},
		{"worker_role", func(v *BrowserTestAdmission) { v.Role = "worker" }},
		{"missing_optin", func(v *BrowserTestAdmission) { v.OptIn = "" }},
		{"production_env", func(v *BrowserTestAdmission) { v.Environment = "production" }},
		{"production_license", func(v *BrowserTestAdmission) { v.LicenseEnvironment = "PRODUCTION" }},
		{"non_synthetic", func(v *BrowserTestAdmission) { v.Synthetic = "false" }},
		{"public_http", func(v *BrowserTestAdmission) { v.HTTPAddr = "0.0.0.0:8080" }},
		{"bad_miniapp", func(v *BrowserTestAdmission) { v.MiniAppURL = "https://public.example" }},
		{"remote_redis", func(v *BrowserTestAdmission) { v.RedisURL = "redis://redis.example:6379/15" }},
		{"remote_nats", func(v *BrowserTestAdmission) { v.NATSURL = "nats://nats.example:4222" }},
		{"different_db", func(v *BrowserTestAdmission) {
			v.DatabaseURL = "postgres://telmivo_runtime:synthetic@127.0.0.1:5432/customer?sslmode=disable"
		}},
		{"db_host_override", func(v *BrowserTestAdmission) {
			v.DatabaseURL += "&host=public.example"
			v.TestDatabaseURL = v.DatabaseURL
		}},
		{"db_port_override", func(v *BrowserTestAdmission) {
			v.DatabaseURL = "postgres://telmivo_runtime:synthetic@127.0.0.1:443/telmivo_test?sslmode=disable"
			v.TestDatabaseURL = v.DatabaseURL
		}},
		{"db_user_override", func(v *BrowserTestAdmission) {
			v.DatabaseURL = "postgres://postgres:synthetic@127.0.0.1:5432/telmivo_test?sslmode=disable"
			v.TestDatabaseURL = v.DatabaseURL
		}},
		{"non_test_migration", func(v *BrowserTestAdmission) {
			v.TestMigrationURL = "postgres://telmivo_migration:synthetic@127.0.0.1:5432/customers?sslmode=disable"
		}},
		{"remote_migration", func(v *BrowserTestAdmission) {
			v.TestMigrationURL = "postgres://telmivo_migration:synthetic@database.example:5432/telmivo_test?sslmode=disable"
		}},
		{"query_extra", func(v *BrowserTestAdmission) { v.TestMigrationURL += "&host=/var/run/postgresql" }},
		{"url_fragment", func(v *BrowserTestAdmission) { v.TestMigrationURL += "#other" }},
		{"wrong_db_role", func(v *BrowserTestAdmission) {
			v.TestMigrationURL = "postgres://postgres:synthetic@127.0.0.1:5432/telmivo_test?sslmode=disable"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			x := phase51iAllowedBrowser()
			tc.change(&x)
			if x.Allowed() {
				t.Fatal("unsafe browser fixture admitted")
			}
		})
	}
}
