//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/naseri-coder/TELMIVO/internal/platform/installationlicense"
)

// The signed V2 Runtime is installed in native TestMain before DB/queue tests.
// This is a pure synthetic effect, not external provider or broker I/O.
func TestLICA9Phase51HNativeIntegrationSignedV2Runtime(t *testing.T) {
	if err := installationlicense.CheckRuntime(context.Background()); err != nil {
		t.Fatalf("no installed signed V2 runtime: %v", err)
	}
	called := 0
	if err := installationlicense.RuntimeEffect(context.Background(), func() error { called++; return nil }); err != nil || called != 1 {
		t.Fatalf("signed V2 effect: %v count=%d", err, called)
	}
	required := installationlicense.RequireContext(context.Background())
	if err := installationlicense.CheckRuntime(required); !errors.Is(err, installationlicense.ErrRestricted) {
		t.Fatalf("context requiring explicit Manager escaped: %v", err)
	}
	called = 0
	if err := installationlicense.RuntimeEffect(required, func() error { called++; return nil }); !errors.Is(err, installationlicense.ErrRestricted) || called != 0 {
		t.Fatalf("restricted context escaped: %v count=%d", err, called)
	}
}
