package intelligence

import (
	"context"
	"github.com/jackc/pgx/v5"
	provider "github.com/naseri-coder/TELMIVO/internal/platform/ai"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	assistant "github.com/naseri-coder/TELMIVO/internal/plugins/aiassistant"
	"time"
)

type Usage struct {
	ID        string    `json:"id"`
	Model     string    `json:"model"`
	State     string    `json:"state"`
	Code      string    `json:"code"`
	Input     int64     `json:"input_tokens"`
	Output    int64     `json:"output_tokens"`
	Tools     int       `json:"tool_calls"`
	Cost      int64     `json:"cost_microunits"`
	Retrieved int       `json:"retrieved_chunks"`
	Latency   int64     `json:"latency_ms"`
	Created   time.Time `json:"created_at"`
}

func (s *Service) Usage(ctx context.Context, actor string, scope sdk.Scope, after string) ([]Usage, error) {
	scope.PluginID = assistant.ID
	out := []Usage{}
	e := s.tenant(ctx, actor, scope, "ai.invoke", "ai.invoke", func(tx pgx.Tx) error {
		rs, e := tx.Query(ctx, "SELECT id::text,model_id,state,code,input_tokens,output_tokens,tool_calls,cost_microunits,retrieved_chunks,latency_ms,created_at FROM ai_requests WHERE workspace_id=$1 AND bot_id=$2 AND ($3='' OR id::text>$3) ORDER BY id LIMIT 50", scope.AccountID, scope.BotID, after)
		if e != nil {
			return e
		}
		defer rs.Close()
		for rs.Next() {
			var u Usage
			if e = rs.Scan(&u.ID, &u.Model, &u.State, &u.Code, &u.Input, &u.Output, &u.Tools, &u.Cost, &u.Retrieved, &u.Latency, &u.Created); e != nil {
				return e
			}
			out = append(out, u)
		}
		return rs.Err()
	})
	return out, e
}

func tokenCostMicrounits(tokens, perMillion int64) int64 {
	if tokens <= 0 || perMillion <= 0 {
		return 0
	}
	return (tokens*perMillion + 999999) / 1000000
}

func requestCostMicrounits(m provider.Model, input, output int64) int64 {
	return tokenCostMicrounits(input, m.InputMicrounitsPerMillion) +
		tokenCostMicrounits(output, m.OutputMicrounitsPerMillion)
}
