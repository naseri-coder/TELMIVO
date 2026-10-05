package intelligence

import (
	"context"
	"github.com/jackc/pgx/v5"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	assistant "github.com/naseri-coder/TELMIVO/internal/plugins/aiassistant"
	"time"
)

type Usage struct {
	ID      string    `json:"id"`
	Model   string    `json:"model"`
	State   string    `json:"state"`
	Code    string    `json:"code"`
	Input   int64     `json:"input_tokens"`
	Output  int64     `json:"output_tokens"`
	Tools   int       `json:"tool_calls"`
	Latency int64     `json:"latency_ms"`
	Created time.Time `json:"created_at"`
}

func (s *Service) Usage(ctx context.Context, actor string, scope sdk.Scope, after string) ([]Usage, error) {
	scope.PluginID = assistant.ID
	out := []Usage{}
	e := s.tenant(ctx, actor, scope, "ai.invoke", "ai.invoke", func(tx pgx.Tx) error {
		rs, e := tx.Query(ctx, "SELECT id::text,model_id,state,code,input_tokens,output_tokens,tool_calls,latency_ms,created_at FROM ai_requests WHERE workspace_id=$1 AND bot_id=$2 AND ($3='' OR id::text>$3) ORDER BY id LIMIT 50", scope.AccountID, scope.BotID, after)
		if e != nil {
			return e
		}
		defer rs.Close()
		for rs.Next() {
			var u Usage
			if e = rs.Scan(&u.ID, &u.Model, &u.State, &u.Code, &u.Input, &u.Output, &u.Tools, &u.Latency, &u.Created); e != nil {
				return e
			}
			out = append(out, u)
		}
		return rs.Err()
	})
	return out, e
}
