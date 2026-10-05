// Package ai is a provider-neutral bounded API. Business plugins never see keys/endpoints.
package ai

import (
	"context"
	"encoding/json"
	"errors"
	cred "github.com/naseri-coder/TELMIVO/internal/platform/crypto"
)

type Model struct {
	ID         string               `json:"id"`
	Name       string               `json:"name"`
	Provider   string               `json:"provider"`
	Identifier string               `json:"identifier"`
	Enabled    bool                 `json:"enabled"`
	Tools      bool                 `json:"tools"`
	Structured bool                 `json:"structured"`
	MaxInput   int                  `json:"max_input"`
	MaxOutput  int64                `json:"max_output"`
	InputMicrounitsPerMillion  int64 `json:"input_microunits_per_million"`
	OutputMicrounitsPerMillion int64 `json:"output_microunits_per_million"`
	Endpoint   string               `json:"-"`
	Secret     cred.EncryptedSecret `json:"-"`
	Revision   int64                `json:"revision"`
}

func (m Model) Valid() bool {
	return m.ID != "" && len(m.ID) <= 100 && m.Name != "" && len(m.Name) <= 100 && m.Identifier != "" && len(m.Identifier) <= 100 && (m.Provider == "openai-compatible" || m.Provider == "synthetic") && m.MaxInput > 0 && m.MaxInput <= 32768 && m.MaxOutput > 0 && m.MaxOutput <= 2048 && m.InputMicrounitsPerMillion >= 0 && m.InputMicrounitsPerMillion <= 1_000_000_000_000 && m.OutputMicrounitsPerMillion >= 0 && m.OutputMicrounitsPerMillion <= 1_000_000_000_000
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type Tool struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}
type Request struct {
	Instructions string    `json:"instructions"`
	Context      []Message `json:"context"`
	Knowledge    []string  `json:"knowledge"`
	Tools        []string  `json:"tools"`
	Structured   bool      `json:"structured"`
	MaxOutput    int64     `json:"max_output"`
}
type Response struct {
	Text         string          `json:"text"`
	Structured   json.RawMessage `json:"structured"`
	Tools        []Tool          `json:"tools"`
	Handoff      bool            `json:"handoff"`
	InputTokens  int64           `json:"input_tokens"`
	OutputTokens int64           `json:"output_tokens"`
}
type Provider interface {
	Generate(context.Context, Model, Request) (Response, error)
}
type Failure struct {
	Code      string
	Retryable bool
	Unknown   bool
}

func (e *Failure) Error() string { return e.Code }
func Code(e error) string {
	var f *Failure
	if errors.As(e, &f) {
		return f.Code
	}
	return "AI_FAILURE"
}
