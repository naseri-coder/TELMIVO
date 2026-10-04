package businessplugins

import (
	"context"
	"encoding/json"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/plugins/broadcast"
	"github.com/naseri-coder/TELMIVO/internal/plugins/forms"
	"github.com/naseri-coder/TELMIVO/internal/plugins/shop"
)

// BindCustomer fixes the verified webhook principal outside untrusted plugin payloads.
func (s *Service) BindCustomer(actor string, scope sdk.Scope, customer Customer) sdk.Invoker {
	return bound{s, actor, scope, customer}
}

type bound struct {
	s        *Service
	actor    string
	scope    sdk.Scope
	customer Customer
}

func (b bound) Invoke(ctx context.Context, cap string, raw json.RawMessage) (json.RawMessage, error) {
	switch {
	case b.scope.PluginID == forms.ID && cap == "forms.submit":
		var in forms.Submit
		if strict(raw, &in) != nil {
			return nil, sdk.ErrConfiguration
		}
		out, e := b.s.SubmitTrusted(ctx, b.actor, b.scope, in.Form, in.Key, b.customer, in.Values)
		return body(out), e
	case b.scope.PluginID == shop.ID && cap == "commerce.order":
		var in shop.Place
		if strict(raw, &in) != nil {
			return nil, sdk.ErrConfiguration
		}
		out, e := b.s.PlaceTrusted(ctx, b.actor, b.scope, in.Key, b.customer, in.Items)
		return body(out), e
	case b.scope.PluginID == broadcast.ID && cap == "telegram.send":
		var in struct {
			Campaign string `json:"campaign"`
		}
		if strict(raw, &in) != nil {
			return nil, sdk.ErrConfiguration
		}
		e := b.s.DeliverOne(ctx, b.actor, b.scope, in.Campaign)
		return body(map[string]bool{"completed": e == nil}), e
	}
	return nil, sdk.ErrPermission
}
