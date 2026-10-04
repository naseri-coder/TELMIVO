package ai

import "context"

type Request struct {
	WorkspaceID, Capability, Prompt string
	MaxTokens                       int
}
type Suggestion struct {
	Text                      string
	InputTokens, OutputTokens int
}

// Suggestions cannot execute domain mutations. A human separately authorizes any action.
type Provider interface {
	Suggest(context.Context, Request) (Suggestion, error)
}
