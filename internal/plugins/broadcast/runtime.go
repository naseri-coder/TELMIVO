package broadcast

import (
	"context"
	"encoding/json"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
)

type Plugin struct{ Runtime sdk.Invoker }

func (p Plugin) Deliver(ctx context.Context, campaign string) error {
	b, _ := json.Marshal(map[string]string{"campaign": campaign})
	_, e := p.Runtime.Invoke(ctx, "telegram.send", b)
	return e
}
