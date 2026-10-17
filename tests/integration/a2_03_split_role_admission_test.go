//go:build integration

package integration

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/naseri-coder/TELMIVO/internal/platform/database"
)

// This runs only after native TestMain admitted the genuine signed TEST-V2
// Manager and an isolated PG16 fixture. The legacy test login cannot pretend
// to be either of the new service logins just by setting a configuration flag.
func TestLICA9A203LegacyRuntimeCannotImpersonateSplitService(t *testing.T) {
	callCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	for _, expected := range []string{"telmivo_api", "telmivo_worker"} {
		t.Run(expected, func(t *testing.T) {
			store, err := database.OpenForExpectedRole(callCtx, os.Getenv("TEST_DATABASE_URL"), expected)
			if store != nil {
				store.Pool.Close()
				t.Fatal("legacy runtime accepted as dedicated split service")
			}
			if err == nil {
				t.Fatal("service identity check did not fail")
			}
		})
	}
	if store, err := database.OpenForExpectedRole(context.Background(), os.Getenv("TEST_DATABASE_URL"), "postgres"); err == nil {
		store.Pool.Close()
		t.Fatal("administrative database role was accepted as a service role")
	} else if !errors.Is(err, context.Canceled) && err == nil {
		t.Fatal("unsupported role not rejected")
	}
}
