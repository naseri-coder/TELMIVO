package businessplugins

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/plugins/forms"
	"github.com/naseri-coder/TELMIVO/internal/plugins/shop"
	"io"
	"strings"
)

func strict(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return database.ErrInvalid
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return database.ErrInvalid
	}
	return nil
}
func (s *Service) HandleSupport(ctx context.Context, scope sdk.Scope, corr string, u contentplugins.Update) error {
	m := u.Message
	if m == nil || m.Chat.Type != "private" || m.From.ID <= 0 || m.From.ID != m.Chat.ID {
		return nil
	}
	parts := strings.SplitN(m.Text, " ", 3)
	command := strings.Split(parts[0], "@")[0]
	if command != "/form" && command != "/order" {
		if s.Fallback != nil {
			return s.Fallback.HandleSupport(ctx, scope, corr, u)
		}
		return nil
	}
	if len(m.Text) > 16000 || u.ID < 0 {
		return database.ErrInvalid
	}
	var actor string
	tx, e := s.DB.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if e = database.Scope(ctx, tx, scope.AccountID); e != nil {
		return e
	}
	actor, e = s.principal(ctx, tx, scope)
	if e != nil {
		return e
	}
	if e = tx.Rollback(ctx); e != nil {
		return e
	}
	name := m.From.Name
	if name == "" {
		name = "مشتری تلگرام"
	}
	c := Customer{User: m.From.ID, Name: name, Username: m.From.Username, Locale: m.From.Locale}
	k := "telegram:" + fmtUser(u.ID)
	switch command {
	case "/form":
		if len(parts) != 3 {
			return database.ErrInvalid
		}
		var answers map[string]json.RawMessage
		if strict([]byte(parts[2]), &answers) != nil {
			return database.ErrInvalid
		}
		scope.PluginID = forms.ID
		_, e = (forms.Plugin{Runtime: s.BindCustomer(actor, scope, c)}).Submit(ctx, forms.Submit{Form: parts[1], Key: k, Values: answers})
	case "/order":
		raw := strings.TrimSpace(strings.TrimPrefix(m.Text, parts[0]))
		var items []shop.Item
		if strict([]byte(raw), &items) != nil {
			return database.ErrInvalid
		}
		scope.PluginID = shop.ID
		_, e = (shop.Plugin{Runtime: s.BindCustomer(actor, scope, c)}).Place(ctx, shop.Place{Key: k, Items: items})
	}
	return e
}
