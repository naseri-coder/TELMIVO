// Package mainbot implements customer commands over the shared identity entry contract.
package mainbot

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/naseri-coder/TELMIVO/internal/domains/accessgates"
	"github.com/naseri-coder/TELMIVO/internal/domains/identity"
	"github.com/naseri-coder/TELMIVO/internal/platform/auth"
	"github.com/naseri-coder/TELMIVO/internal/platform/localization"
)

var ErrUnsupported = errors.New("unsupported customer command")

type Service struct {
	Identity   *identity.Service
	MiniAppURL string
	Membership accessgates.MembershipService
}

func Route(text string) (string, bool) {
	fields := strings.Fields(text)
	if len(fields) > 0 && fields[0] == "/start" {
		return "/", true
	}
	for key, path := range map[string]string{"dashboard": "/", "channels": "/channels", "groups": "/groups", "bots": "/customer-bots", "tickets": "/support", "wallet": "/wallet", "plans": "/plans", "settings": "/settings", "admin": "/admin/policies"} {
		if text == localization.Text("menu."+key) {
			return path, true
		}
	}
	return "", false
}
func (s *Service) Handle(ctx context.Context, u identity.Update, correlation string) (identity.Result, error) {
	if u.Message == nil || u.Message.Chat.Type != "private" || u.Message.From.ID <= 0 || u.Message.From.ID != u.Message.Chat.ID || u.ID < 0 {
		return identity.Result{}, auth.ErrInvalid
	}
	path, ok := Route(u.Message.Text)
	if !ok {
		return identity.Result{}, ErrUnsupported
	}
	target, e := url.Parse(s.MiniAppURL)
	if e != nil || target.Scheme != "https" || target.Host == "" || target.User != nil {
		return identity.Result{}, auth.ErrInvalid
	}
	// Keep the configured base path; BrowserRouter routes are absolute within the Mini App origin.
	target.Path = strings.TrimRight(target.Path, "/") + path
	return s.Identity.Start(ctx, "main-bot", target.String(), correlation, u, s.Membership)
}
