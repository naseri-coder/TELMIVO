// Package businessevents emits minimal scoped events through TELMIVO's existing outbox.
package businessevents

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"strings"
)

type Event struct {
	Bot       string              `json:"bot"`
	Actor     string              `json:"actor"`
	Ticket    string              `json:"ticket,omitempty"`
	Contact   string              `json:"contact,omitempty"`
	User      int64               `json:"user,omitempty"`
	Fields    map[string][]string `json:"fields"`
	Depth     int                 `json:"depth"`
	Workflows []string            `json:"workflows"`
	Causation string              `json:"causation"`
}
type key struct{}

func With(ctx context.Context, e Event) context.Context { return context.WithValue(ctx, key{}, e) }
func Emit(ctx context.Context, tx pgx.Tx, actor string, scope sdk.Scope, kind, entity string, fields map[string][]string) error {
	e, _ := ctx.Value(key{}).(Event)
	e.Bot = scope.BotID
	e.Actor = actor
	e.Fields = fields
	if strings.HasPrefix(kind, "crm.") {
		e.Contact = entity
	} else if strings.HasPrefix(kind, "ticket.") {
		e.Ticket = entity
	}
	return database.Event(ctx, tx, scope.AccountID, "telmivo.events.business", kind, e.Causation, e)
}

func WithUser(ctx context.Context, user int64) context.Context {
	e, _ := ctx.Value(key{}).(Event)
	e.User = user
	return With(ctx, e)
}
