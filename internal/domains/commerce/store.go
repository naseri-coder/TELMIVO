package commerce

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
)

const agreementColumns = `id::text,workspace_id::text,COALESCE(bot_id::text,''),license_scope,source,state,starts_at,ends_at,grace_until,terms,revision`

func scanAgreement(row pgx.Row) (Agreement, error) {
	var a Agreement
	var b []byte
	e := row.Scan(&a.ID, &a.Account, &a.Bot, &a.Scope, &a.Source, &a.State, &a.StartsAt, &a.EndsAt, &a.GraceUntil, &b, &a.Revision)
	if e == nil {
		e = decodeStrict(b, &a.Terms)
	}
	return a, e
}
func readAgreements(ctx context.Context, tx pgx.Tx, account string) ([]Agreement, error) {
	rows, e := tx.Query(ctx, "SELECT "+agreementColumns+" FROM commercial_agreements WHERE workspace_id=$1 ORDER BY id", account)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Agreement{}
	for rows.Next() {
		a, e := scanAgreement(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func effectivePlan(ctx context.Context, tx pgx.Tx, account string, now time.Time) (string, error) {
	var plan, previous string
	var scheduled *string
	var expires, scheduledAt *time.Time
	e := tx.QueryRow(ctx, `SELECT plan_key,COALESCE(previous_plan_key,'FREE'),expires_at,scheduled_plan_key,scheduled_at FROM subscriptions WHERE workspace_id=$1`, account).Scan(&plan, &previous, &expires, &scheduled, &scheduledAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return "FREE", nil
	}
	if e != nil {
		return "", e
	}
	if expires != nil && !now.Before(*expires) {
		plan = previous
	}
	if scheduled != nil && scheduledAt != nil && !now.Before(*scheduledAt) {
		plan = *scheduled
	}
	var enabled bool
	e = tx.QueryRow(ctx, "SELECT active FROM plans WHERE key=$1", plan).Scan(&enabled)
	if e != nil {
		return "", e
	}
	if !enabled {
		return "", nil
	}
	return plan, nil
}
func requireBot(ctx context.Context, tx pgx.Tx, account, bot string) error {
	if bot == "" {
		return nil
	}
	var yes bool
	e := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM customer_bots WHERE id=$1 AND workspace_id=$2)", bot, account).Scan(&yes)
	if e != nil {
		return e
	}
	if !yes {
		return database.ErrForbidden
	}
	return nil
}
func (s *Service) Issue(ctx context.Context, actor, account, correlation string, in Issue) (Agreement, error) {
	var a Agreement
	e := s.admin(ctx, actor, account, func(tx pgx.Tx) error {
		var e error
		a, e = s.issue(ctx, tx, actor, account, correlation, in, false)
		return e
	})
	return a, e
}

// StartTrial is the only unprivileged issuance path; durations and scopes come from catalog.
func (s *Service) StartTrial(ctx context.Context, actor, account, correlation, key, plugin, bot string) (Agreement, error) {
	var a Agreement
	e := s.DB.Tenant(ctx, actor, account, false, func(tx pgx.Tx) error {
		if !database.Permission(ctx, tx, actor, account, "bots.manage") {
			return database.ErrForbidden
		}
		if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,7100213))", account); e != nil {
			return e
		}
		var e error
		a, e = s.issue(ctx, tx, actor, account, correlation, Issue{Key: key, Kind: PluginKind, CatalogKey: plugin, Bot: bot, Reason: "customer trial"}, true)
		return e
	})
	return a, e
}
func (s *Service) issue(ctx context.Context, tx pgx.Tx, actor, account, correlation string, in Issue, trial bool) (Agreement, error) {
	if !identifier.MatchString(in.Key) || in.Reason == "" || (!trial && in.Kind != PluginKind && in.Kind != BundleKind && in.Kind != AddonKind) || in.RepeatTrial && (!in.AdminGrant || in.Kind != PluginKind) || in.ExpiresAt != nil && !in.AdminGrant {
		return Agreement{}, database.ErrInvalid
	}
	now := s.now()
	b, _ := json.Marshal(struct {
		Issue Issue
		Trial bool
	}{in, trial})
	h := sha256.Sum256(b)
	fingerprint := hex.EncodeToString(h[:])
	var saved string
	a, e := scanAgreement(tx.QueryRow(ctx, "SELECT "+agreementColumns+" FROM commercial_agreements WHERE workspace_id=$1 AND operation_key=$2", account, in.Key))
	if e == nil {
		if e = tx.QueryRow(ctx, "SELECT fingerprint FROM commercial_agreements WHERE id=$1", a.ID).Scan(&saved); e != nil {
			return Agreement{}, e
		}
		if saved != fingerprint {
			return Agreement{}, database.ErrConflict
		}
		return a, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return Agreement{}, e
	}
	c, e := readCatalog(ctx, tx, now)
	if e != nil {
		return Agreement{}, e
	}
	v, ok := c.Get(in.Kind, in.CatalogKey)
	if !ok {
		return Agreement{}, database.ErrInvalid
	}
	a = Agreement{Account: account, State: Active, StartsAt: now, Revision: 1}
	a.Terms = Terms{Catalog: v, Period: in.Period, Effects: Effects{Quotas: map[string]int64{}}}
	var prices Prices
	var sale bool
	if in.Kind == PluginKind {
		p := v.Definition.Plugin
		a.Scope = p.Scope
		prices = p.Prices
		sale = p.SaleEnabled
		a.Source = Standalone
		a.Terms.Effects.Plugins = []string{v.Key}
		a.Terms.GraceSeconds = p.GraceSeconds
		if trial || in.RepeatTrial {
			if !p.Trial.Enabled {
				return Agreement{}, database.ErrInvalid
			}
			a.Source = TrialSource
			end := now.Add(time.Duration(p.Trial.DurationSeconds) * time.Second)
			a.EndsAt = &end
			a.Terms.GraceSeconds = 0
		}
	} else {
		p := v.Definition.Product
		a.Scope = p.Scope
		prices = p.Prices
		sale = p.SaleEnabled
		a.Terms.Effects.Plugins = p.Plugins
		a.Terms.Effects.Quotas = p.Quotas
		a.Terms.GraceSeconds = p.GraceSeconds
		if in.Kind == BundleKind {
			a.Source = BundleSource
		} else {
			a.Source = AddonSource
		}
	}
	// Revalidate membership after catalog scope changes; old agreements keep their snapshot.
	for _, id := range a.Terms.Effects.Plugins {
		member, ok := c.Get(PluginKind, id)
		if !ok || member.Definition.Plugin.Scope != a.Scope {
			return Agreement{}, database.ErrInvalid
		}
	}
	for id := range a.Terms.Effects.Quotas {
		metric, ok := c.Get(QuotaKind, id)
		if !ok || metric.Definition.Quota.Scope != a.Scope {
			return Agreement{}, database.ErrInvalid
		}
	}
	if a.Scope == sdk.PerBot {
		if in.Bot == "" {
			return Agreement{}, database.ErrInvalid
		}
		a.Bot = in.Bot
	} else if in.Bot != "" {
		return Agreement{}, database.ErrInvalid
	}
	if e = requireBot(ctx, tx, account, a.Bot); e != nil {
		return Agreement{}, e
	}
	if in.AdminGrant && !in.RepeatTrial {
		a.Source = AdminSource
		if in.ExpiresAt != nil {
			end := in.ExpiresAt.UTC().Truncate(time.Microsecond)
			a.EndsAt = &end
		}
		a.Terms.GraceSeconds = 0
	}
	if !in.AdminGrant {
		plan, e := effectivePlan(ctx, tx, account, now)
		if e != nil {
			return Agreement{}, e
		}
		p, exists := c.Get(PlanKind, plan)
		if !exists {
			return Agreement{}, &Denied{Decision: Decision{Code: "PLAN_UNAVAILABLE"}}
		}
		for _, plugin := range a.Terms.Effects.Plugins {
			if _, yes := c.Get(PluginKind, plugin); !yes {
				return Agreement{}, database.ErrInvalid
			}
			if policy := p.Definition.Plan.Plugins[plugin]; policy != PaidAddon && policy != Included {
				return Agreement{}, &Denied{Decision: Decision{Code: "PLAN_UNAVAILABLE"}}
			}
		}
	}
	if a.Source != TrialSource && a.Source != AdminSource {
		if !sale {
			return Agreement{}, &Denied{Decision: Decision{Code: "SALE_DISABLED"}}
		}
		var price Money
		switch in.Period {
		case Daily:
			price = prices.Daily
			end := now.Add(24 * time.Hour)
			a.EndsAt = &end
		case Monthly:
			price = prices.Monthly
			end := monthEnd(now)
			a.EndsAt = &end
		default:
			return Agreement{}, database.ErrInvalid
		}
		a.Terms.Price = &price
	} else if in.Period != "" {
		return Agreement{}, database.ErrInvalid
	}
	if a.EndsAt != nil {
		if !a.EndsAt.After(now) {
			return Agreement{}, database.ErrInvalid
		}
		if a.Terms.GraceSeconds > 0 {
			end := a.EndsAt.Add(time.Duration(a.Terms.GraceSeconds) * time.Second)
			a.GraceUntil = &end
		}
	}
	terms, e := json.Marshal(a.Terms)
	if e != nil {
		return Agreement{}, e
	}
	if e = tx.QueryRow(ctx, `INSERT INTO commercial_agreements(workspace_id,operation_key,fingerprint,bot_id,license_scope,source,starts_at,ends_at,grace_until,catalog_kind,catalog_key,catalog_version,terms,actor_id,reason) VALUES($1,$2,$3,NULLIF($4,'')::uuid,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING id::text`, account, in.Key, fingerprint, a.Bot, a.Scope, a.Source, now, a.EndsAt, a.GraceUntil, v.Kind, v.Key, v.Version, terms, actor, in.Reason).Scan(&a.ID); e != nil {
		return Agreement{}, e
	}
	if a.Source == TrialSource {
		subject := account
		if a.Scope == sdk.PerBot {
			subject = a.Bot
		}
		var consumed bool
		if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM commercial_trial_consumptions WHERE workspace_id=$1 AND plugin_id=$2 AND license_scope=$3 AND subject_id=$4)", account, v.Key, a.Scope, subject).Scan(&consumed); e != nil {
			return Agreement{}, e
		}
		if consumed && !in.RepeatTrial {
			return Agreement{}, &Denied{Decision: Decision{Code: "TRIAL_CONSUMED"}}
		}
		if !consumed {
			if _, e = tx.Exec(ctx, "INSERT INTO commercial_trial_consumptions(workspace_id,plugin_id,license_scope,subject_id,agreement_id,consumed_at) VALUES($1,$2,$3,$4,$5,$6)", account, v.Key, a.Scope, subject, a.ID, now); e != nil {
				return Agreement{}, e
			}
		}
	}
	return a, database.Audit(ctx, tx, actor, account, "commercial.agreement.issue", "commercial_agreement", a.ID, correlation, map[string]any{}, a)
}

func (s *Service) SetState(ctx context.Context, actor, account, correlation, id string, expected int64, state State, reason string) (Agreement, error) {
	var a Agreement
	if (state != Active && state != Suspended && state != Cancelled && state != Expired) || reason == "" {
		return a, database.ErrInvalid
	}
	e := s.admin(ctx, actor, account, func(tx pgx.Tx) error {
		old, e := scanAgreement(tx.QueryRow(ctx, "SELECT "+agreementColumns+" FROM commercial_agreements WHERE id=$1 AND workspace_id=$2 FOR UPDATE", id, account))
		if e != nil {
			return e
		}
		if old.Revision != expected {
			return database.ErrConflict
		}
		a, e = scanAgreement(tx.QueryRow(ctx, "UPDATE commercial_agreements SET state=$1,revision=revision+1 WHERE id=$2 RETURNING "+agreementColumns, state, id))
		if e != nil {
			return e
		}
		return database.Audit(ctx, tx, actor, account, "commercial.agreement.state", "commercial_agreement", id, correlation, map[string]any{"agreement": old}, map[string]any{"agreement": a, "reason": reason})
	})
	return a, e
}
