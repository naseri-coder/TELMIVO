package testfixtures

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	tickets "github.com/naseri-coder/TELMIVO/internal/plugins/ticketing"
	"strings"
	"time"
)

// ContentProvider is injected exclusively in test+synthetic mode. No network or real files.
type ContentProvider struct{}
type contentTransport struct{ bot string }

func (ContentProvider) ForBot(_ context.Context, _ pgx.Tx, s sdk.Scope) (contentplugins.Transport, error) {
	return contentTransport{s.BotID}, nil
}
func (contentTransport) Check(context.Context, int64, int64) (string, string) {
	return "NOT_MEMBER", ""
}
func (contentTransport) SendFile(context.Context, int64, string, string, string) (int64, string) {
	return 101, "DELIVERY_ACKNOWLEDGED"
}
func (contentTransport) Prompt(context.Context, int64, string, any) error { return nil }
func (t contentTransport) Connect(context.Context, string, string) (string, error) {
	return "synthetic_" + strings.ReplaceAll(t.bot, "-", ""), nil
}

func (contentTransport) SupportReply(context.Context, int64, string, []tickets.Attachment) (int64, string) {
	return 101, "DELIVERY_ACKNOWLEDGED"
}

func (contentTransport) BroadcastReply(context.Context, int64, string) (int64, string, time.Duration) {
	return 101, "DELIVERY_ACKNOWLEDGED", 0
}
