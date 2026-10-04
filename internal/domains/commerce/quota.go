package commerce

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
)

// Quota checks are read-only. Consume is a trusted Core service, not an HTTP
// self-reporting meter; transactions serialize the limit check and increment.
func (s *Service) quota(ctx context.Context, actor, account, bot, metric string, amount int64, consume bool) (QuotaResult, error) {
	var out QuotaResult
	if amount < 0 {
		return out, database.ErrInvalid
	}
	now := s.now()
	e := s.DB.Tenant(ctx, actor, account, false, func(tx pgx.Tx) error {
		if consume && !database.Permission(ctx, tx, actor, account, "bots.manage") {
			return database.ErrForbidden
		}
		if e := requireBot(ctx, tx, account, bot); e != nil {
			return e
		}
		if consume {
			if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,7100213))", account); e != nil {
				return e
			}
		}
		c, e := readCatalog(ctx, tx, now)
		if e != nil {
			return e
		}
		v, ok := c.Get(QuotaKind, metric)
		if !ok {
			out.Code = "QUOTA_UNDEFINED"
			return nil
		}
		subject := account
		if v.Definition.Quota.Scope == sdk.PerBot {
			if bot == "" {
				return database.ErrInvalid
			}
			subject = bot
		}
		as, e := readAgreements(ctx, tx, account)
		if e != nil {
			return e
		}
		p, e := effectivePlan(ctx, tx, account, now)
		if e != nil {
			return e
		}
		limit, ok := EffectiveLimit(c, as, p, account, bot, metric, now)
		if !ok {
			return database.ErrInvalid
		}
		bucket := Bucket(v.Definition.Quota.Reset, now)
		var used int64
		if e = tx.QueryRow(ctx, "SELECT COALESCE((SELECT used FROM commercial_usage WHERE workspace_id=$1 AND metric=$2 AND subject_id=$3 AND bucket=$4),0)", account, metric, subject, bucket).Scan(&used); e != nil {
			return e
		}
		out = CheckQuota(limit, used, amount, bucket)
		if consume && out.Allowed {
			next, _ := add(used, amount)
			_, e = tx.Exec(ctx, `INSERT INTO commercial_usage(workspace_id,metric,subject_id,bucket,used) VALUES($1,$2,$3,$4,$5) ON CONFLICT(workspace_id,metric,subject_id,bucket) DO UPDATE SET used=EXCLUDED.used`, account, metric, subject, bucket, next)
			return e
		}
		return nil
	})
	return out, e
}
func (s *Service) Check(ctx context.Context, actor, account, bot, metric string, amount int64) (QuotaResult, error) {
	return s.quota(ctx, actor, account, bot, metric, amount, false)
}
func (s *Service) Consume(ctx context.Context, actor, account, bot, metric string, amount int64) (QuotaResult, error) {
	return s.quota(ctx, actor, account, bot, metric, amount, true)
}
func (s *Service) Agreements(ctx context.Context, actor, account string) ([]Agreement, error) {
	var out []Agreement
	e := s.DB.Tenant(ctx, actor, account, true, func(tx pgx.Tx) error {
		if !database.PlatformPermission(ctx, tx, actor, "platform.commercial.read") {
			return database.ErrForbidden
		}
		var e error
		out, e = readAgreements(ctx, tx, account)
		if e == nil {
			now := s.now()
			for i := range out {
				out[i].State = CommercialState(out[i], now)
			}
		}
		return e
	})
	return out, e
}
