package httpapi

import (
	provider "github.com/naseri-coder/TELMIVO/internal/platform/ai"
	"testing"
)

func TestAIFailureStatusIsSafe(t *testing.T) {
	for code, status := range map[string]int{"AI_COST_LIMIT": 403, "CONTEXT_TOO_LARGE": 400, "PROVIDER_RATE_LIMIT": 429, "MODEL_UNAVAILABLE": 503, "PROVIDER_OUTCOME_UNKNOWN": 503, "secret-provider-detail": 500} {
		got, safe := aiFailureStatus(&provider.Failure{Code: code})
		if got != status {
			t.Fatal(code, got)
		}
		if status == 500 && safe != "internal" {
			t.Fatal("raw provider code exposed")
		}
	}
}
