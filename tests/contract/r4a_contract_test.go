package contract

import (
	"strings"
	"testing"

	"github.com/naseri-coder/TELMIVO/internal/platform/runtime"
)

func TestR4ALegacyCustomerRuntimeFailsClosed(t *testing.T) {
	e := runtime.Run("customer-bot-runtime")
	if e == nil || !strings.Contains(e.Error(), "CUSTOMER_BOT_RUNTIME_RETIRED_USE_API_INGRESS") {
		t.Fatal("legacy customer runtime did not fail closed", e)
	}
}
