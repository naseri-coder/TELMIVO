package aiassistant

import (
	"context"
	"encoding/json"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
)

type Plugin struct{ Runtime sdk.Invoker }

func (p Plugin) Generate(ctx context.Context, request json.RawMessage) (json.RawMessage, error) {
	if p.Runtime == nil {
		return nil, sdk.ErrPermission
	}
	return p.Runtime.Invoke(ctx, "ai.invoke", request)
}
