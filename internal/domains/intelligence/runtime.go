package intelligence

import (
	"context"
	"encoding/json"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	assistant "github.com/naseri-coder/TELMIVO/internal/plugins/aiassistant"
	workflow "github.com/naseri-coder/TELMIVO/internal/plugins/automation"
)

// Bind exposes only a scoped capability invoker to first-party plugin handlers.
// Admission and durable reservation occur in the authoritative service; the
// invoker intentionally holds no transaction across external provider calls.
func (s *Service) Bind(actor string, scope sdk.Scope) sdk.Invoker { return bound{s, actor, scope} }

type bound struct {
	s     *Service
	actor string
	scope sdk.Scope
}

func (b bound) Invoke(ctx context.Context, cap string, raw json.RawMessage) (json.RawMessage, error) {
	switch {
	case b.scope.PluginID == workflow.ID && cap == "workflow.execute":
		var in struct {
			Execution string `json:"execution"`
		}
		if strict(raw, &in) != nil || in.Execution == "" {
			return nil, sdk.ErrConfiguration
		}
		e := b.s.Run(ctx, b.actor, b.scope, in.Execution)
		return body(map[string]bool{"completed": e == nil}), e
	case b.scope.PluginID == assistant.ID && cap == "ai.invoke":
		var in Chat
		if strict(raw, &in) != nil {
			return nil, sdk.ErrConfiguration
		}
		in.Purpose = "PLAYGROUND"
		if in.Ticket != "" {
			in.Purpose = "AGENT_ASSIST"
		}
		out, e := b.s.Chat(ctx, b.actor, b.scope, in)
		return body(out), e
	}
	return nil, sdk.ErrPermission
}
