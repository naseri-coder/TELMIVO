// Package testfixtures contains deterministic adapters gated by explicit test+synthetic configuration.
package testfixtures

import (
	"context"
	"github.com/naseri-coder/TELMIVO/internal/domains/groupbot"
)

type GroupAuthority struct{}

func (GroupAuthority) Inspect(_ context.Context, chat, user int64) (groupbot.Member, error) {
	if chat != -97000001 {
		return groupbot.Member{}, groupbot.ErrAuthority
	}
	if user == 97000001 || user == 97000999 {
		return groupbot.Member{Status: "administrator", Delete: true, Restrict: true, Manage: true}, nil
	}
	return groupbot.Member{Status: "member"}, nil
}
func (g GroupAuthority) Bot(ctx context.Context, chat int64) (int64, groupbot.Member, error) {
	m, e := g.Inspect(ctx, chat, 97000999)
	return 97000999, m, e
}
