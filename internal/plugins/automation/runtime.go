package automation

import (
	"context"
	"encoding/json"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
)

type Plugin struct{ Runtime sdk.Invoker }

func (p Plugin) Run(ctx context.Context, id string) error {
	if p.Runtime == nil {
		return sdk.ErrPermission
	}
	b, _ := json.Marshal(map[string]string{"execution": id})
	_, e := p.Runtime.Invoke(ctx, "workflow.execute", b)
	return e
}
