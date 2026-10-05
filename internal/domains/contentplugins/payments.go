package contentplugins

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
)

type starsTransport interface {
	CreateStarsInvoice(context.Context, string, string, string, int64) (string, string)
	AnswerStarsPreCheckout(context.Context, string, bool) string
	RefundStars(context.Context, int64, string) string
}

func (s *Service) CreateStarsInvoice(ctx context.Context, actor string, scope sdk.Scope, title, description, payload string, amount int64) (string, string) {
	if amount <= 0 || len(payload) < 1 || len(payload) > 128 || len(title) < 1 || len(title) > 32 || len(description) < 1 || len(description) > 255 {
		return "", "CHECKOUT_REJECTED"
	}
	var url, code string
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		tr, e := s.transport(ctx, tx, scope)
		if e != nil {
			return e
		}
		p, ok := tr.(starsTransport)
		if !ok {
			return sdk.ErrDependency
		}
		url, code = p.CreateStarsInvoice(ctx, title, description, payload, amount)
		return nil
	})
	if e != nil {
		if e == sdk.ErrDependency || e == sdk.ErrPermission || e == database.ErrForbidden {
			return "", "CHECKOUT_REJECTED"
		}
		return "", "CHECKOUT_UNKNOWN"
	}
	return url, code
}

func (s *Service) AnswerStarsPreCheckout(ctx context.Context, actor string, scope sdk.Scope, query string, ok bool) string {
	if query == "" {
		return "PRECHECKOUT_REJECTED"
	}
	code := "PRECHECKOUT_REJECTED"
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		tr, e := s.transport(ctx, tx, scope)
		if e != nil {
			return e
		}
		p, yes := tr.(starsTransport)
		if !yes {
			return sdk.ErrDependency
		}
		code = p.AnswerStarsPreCheckout(ctx, query, ok)
		return nil
	})
	if e != nil {
		return "PRECHECKOUT_UNKNOWN"
	}
	return code
}

func (s *Service) RefundStars(ctx context.Context, actor string, scope sdk.Scope, user int64, charge string) string {
	if user <= 0 || charge == "" {
		return "REFUND_REJECTED"
	}
	code := "REFUND_REJECTED"
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		tr, e := s.transport(ctx, tx, scope)
		if e != nil {
			return e
		}
		p, yes := tr.(starsTransport)
		if !yes {
			return sdk.ErrDependency
		}
		code = p.RefundStars(ctx, user, charge)
		return nil
	})
	if e != nil {
		return "REFUND_UNKNOWN"
	}
	return code
}
