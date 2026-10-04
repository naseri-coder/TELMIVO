package shop

import (
	"context"
	"encoding/json"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
)

type Place struct {
	Key   string `json:"idempotency_key"`
	Items []Item `json:"items"`
}
type Plugin struct{ Runtime sdk.Invoker }

func (p Plugin) Place(ctx context.Context, in Place) (json.RawMessage, error) {
	b, e := json.Marshal(in)
	if e != nil {
		return nil, e
	}
	return p.Runtime.Invoke(ctx, "commerce.order", b)
}
