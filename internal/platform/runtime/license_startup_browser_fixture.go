//go:build lic_a9_browser_test

package runtime

import (
	"errors"
	"os"

	"github.com/naseri-coder/TELMIVO/internal/platform/config"
	"github.com/naseri-coder/TELMIVO/internal/platform/installationlicense"
)

// This is exclusively compiled with -tags=lic_a9_browser_test. It NEVER
// falls back to a restricted-but-listening HTTP server on bad test settings.
func openStartupLicense(role string, c config.Config) (*installationlicense.Manager, *installationlicense.Client, func(), bool, error) {
	noop := func() {}
	admission := installationlicense.BrowserTestAdmission{
		Executable:         os.Args[0],
		Role:               role,
		Environment:        c.Environment,
		LicenseEnvironment: os.Getenv("TELMIVO_LICENSE_ENVIRONMENT"),
		OptIn:              os.Getenv("TELMIVO_LIC_A9_BROWSER_TEST"),
		Synthetic:          os.Getenv("SYNTHETIC_MODE"),
		HTTPAddr:           c.HTTPAddr,
		DatabaseURL:        c.DatabaseURL,
		TestDatabaseURL:    os.Getenv("TEST_DATABASE_URL"),
		TestMigrationURL:   os.Getenv("TEST_MIGRATION_DATABASE_URL"),
		RedisURL:           c.RedisURL,
		NATSURL:            c.NATSURL,
		MiniAppURL:         c.MiniAppURL,
	}
	if !admission.Allowed() {
		return nil, nil, noop, true, errors.New("TEST_BROWSER_API_LICENSE_ADMISSION_DENIED")
	}
	dir, err := os.MkdirTemp("", "telmivo-lic-a9-browser-v2-")
	if err != nil {
		return nil, nil, noop, true, errors.New("TEST_BROWSER_API_LICENSE_DIRECTORY_UNAVAILABLE")
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	manager, err := installationlicense.NewTestOnlySignedV2RuntimeForNativeBrowser(dir)
	if err != nil {
		cleanup()
		return nil, nil, noop, true, errors.New("TEST_BROWSER_API_SIGNED_V2_REJECTED")
	}
	return manager, nil, cleanup, true, nil
}
