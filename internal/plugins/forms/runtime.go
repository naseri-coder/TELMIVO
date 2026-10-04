package forms

import (
	"context"
	"encoding/json"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
)

type Submit struct {
	Form   string                     `json:"form"`
	Key    string                     `json:"idempotency_key"`
	Values map[string]json.RawMessage `json:"values"`
}
type Plugin struct{ Runtime sdk.Invoker }

func (p Plugin) Submit(ctx context.Context, in Submit) (json.RawMessage, error) {
	b, e := json.Marshal(in)
	if e != nil {
		return nil, e
	}
	return p.Runtime.Invoke(ctx, "forms.submit", b)
}
