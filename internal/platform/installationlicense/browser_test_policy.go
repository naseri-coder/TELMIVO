//go:build lic_a9_browser_test

package installationlicense

import (
	"net/url"
	"path/filepath"
	"strings"
)

// BrowserTestAdmission is an exact synthetic-only, loopback-only build gate.
// The exported type contains no secret or trust-anchor data. The fixture is
// separately compiled out of ordinary Product binaries.
type BrowserTestAdmission struct {
	Executable, Role, Environment, LicenseEnvironment, OptIn string
	Synthetic, HTTPAddr, DatabaseURL, TestDatabaseURL string
	TestMigrationURL, RedisURL, NATSURL, MiniAppURL string
}

func (v BrowserTestAdmission) Allowed() bool {
	if filepath.Base(v.Executable) != "telmivo-lic-a9-browser-test-api" ||
		v.Role != "api" || v.OptIn != "YES" || v.Environment != "test" ||
		!strings.EqualFold(v.LicenseEnvironment, "TEST") ||
		v.Synthetic != "true" || v.HTTPAddr != "127.0.0.1:8080" ||
		v.MiniAppURL != "http://127.0.0.1:5173" ||
		v.RedisURL != "redis://127.0.0.1:6379/15" ||
		v.NATSURL != "nats://127.0.0.1:4222" ||
		v.DatabaseURL != v.TestDatabaseURL {
		return false
	}
	return browserTestPostgresURL(v.DatabaseURL, "telmivo_runtime") &&
		browserTestPostgresURL(v.TestMigrationURL, "telmivo_migration")
}

func browserTestPostgresURL(raw, role string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "postgres" || u.Host != "127.0.0.1:5432" ||
		u.Path != "/telmivo_test" || u.RawQuery != "sslmode=disable" ||
		u.Fragment != "" || u.User == nil || u.User.Username() != role {
		return false
	}
	_, hasPassword := u.User.Password()
	return hasPassword
}
