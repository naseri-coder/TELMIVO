package testfixtures

import (
	"context"
	"encoding/json"
	provider "github.com/naseri-coder/TELMIVO/internal/platform/ai"
)

// Injected only behind existing test + synthetic + ContentTestFixture guard.
type AIProvider struct{}

func (AIProvider) Generate(_ context.Context, m provider.Model, r provider.Request) (provider.Response, error) {
	if m.Provider != "synthetic" {
		return provider.Response{}, &provider.Failure{Code: "MODEL_UNAVAILABLE"}
	}
	out := provider.Response{Text: "پاسخ آزمایشی دستیار", InputTokens: 10, OutputTokens: 5}
	if r.Structured {
		out.Text = ""
		out.Structured = json.RawMessage(`{"category":"technical","priority":"HIGH"}`)
	}
	return out, nil
}
