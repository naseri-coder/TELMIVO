//go:build !lic_a9_browser_test

package runtime

import (
	"github.com/naseri-coder/TELMIVO/internal/platform/config"
	"github.com/naseri-coder/TELMIVO/internal/platform/installationlicense"
)

// Normal Product builds ALWAYS use explicit configured public trust and the
// real Authority endpoint. No test signer or synthetic lease is available.
func openStartupLicense(_ string, _ config.Config) (*installationlicense.Manager, *installationlicense.Client, func(), bool, error) {
	m, cl, err := installationlicense.OpenEnvironment()
	return m, cl, func() {}, false, err
}
