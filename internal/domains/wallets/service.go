package wallets

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/auth"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/naseri-coder/TELMIVO/internal/platform/ledger"
	"math"
)

type Adjustment struct {
	Asset        ledger.Asset `json:"asset"`
	AmountAtomic int64        `json:"amount_atomic,string"`
	Reason       string       `json:"reason"`
	Key          string       `json:"idempotency_key"`
}

func Adjust(ctx context.Context, tx pgx.Tx, actor, workspace, correlation string, a Adjustment) (string, error) {
	if !database.PlatformPermission(ctx, tx, actor, "platform.wallets.adjust") {
		return "", database.ErrForbidden
	}
	if a.Asset != ledger.IRR && a.Asset != ledger.USD {
		return "", database.ErrInvalid
	}
	if a.AmountAtomic == 0 || a.AmountAtomic == math.MinInt64 || len(a.Reason) < 1 || len(a.Reason) > 1000 || len(a.Key) < 8 || len(a.Key) > 200 {
		return "", database.ErrInvalid
	}
	// Serialize idempotency and balance checks. Clearings use the same lock order as wallets.
	if _, e := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", workspace+":"+string(a.Asset)); e != nil {
		return "", e
	}
	payload, _ := json.Marshal(struct {
		Actor      string
		Adjustment Adjustment
	}{actor, a})
	fingerprint := auth.Digest(string(payload))
	var previous, hash string
	e := tx.QueryRow(ctx, "SELECT id::text,fingerprint FROM ledger_transactions WHERE workspace_id=$1 AND idempotency_key=$2", workspace, a.Key).Scan(&previous, &hash)
	if e == nil {
		if hash != fingerprint {
			return "", database.ErrConflict
		}
		return previous, nil
	}
	if e != pgx.ErrNoRows {
		return "", e
	}
	rows, e := tx.Query(ctx, "SELECT id::text,kind FROM wallet_accounts WHERE workspace_id=$1 AND asset=$2 ORDER BY id FOR UPDATE", workspace, a.Asset)
	if e != nil {
		return "", e
	}
	var wallet, clearing string
	for rows.Next() {
		var id, k string
		if e = rows.Scan(&id, &k); e != nil {
			rows.Close()
			return "", e
		}
		if k == "WALLET" {
			wallet = id
		} else {
			clearing = id
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return "", e
	}
	if wallet == "" || clearing == "" {
		return "", errors.New("wallet accounts missing")
	}
	entries := []ledger.Entry{{AccountID: wallet, Money: ledger.Money{Asset: a.Asset, AmountAtomic: a.AmountAtomic}}, {AccountID: clearing, Money: ledger.Money{Asset: a.Asset, AmountAtomic: -a.AmountAtomic}}}
	if e = ledger.Validate(entries); e != nil {
		return "", e
	}
	var balance int64
	if e = tx.QueryRow(ctx, "SELECT COALESCE(sum(amount_atomic),0)::bigint FROM ledger_entries WHERE account_id=$1", wallet).Scan(&balance); e != nil {
		return "", e
	}
	if a.AmountAtomic < 0 && balance < -a.AmountAtomic {
		return "", database.ErrInvalid
	}
	if a.AmountAtomic > 0 && balance > math.MaxInt64-a.AmountAtomic {
		return "", database.ErrInvalid
	}
	var id string
	e = tx.QueryRow(ctx, "INSERT INTO ledger_transactions(workspace_id,asset,transaction_type,reference,idempotency_key,fingerprint,actor_id,reason,correlation_id) VALUES($1,$2,'ADMIN_ADJUSTMENT',$3,$3,$4,$5,$6,$7) RETURNING id::text", workspace, a.Asset, a.Key, fingerprint, actor, a.Reason, correlation).Scan(&id)
	if e != nil {
		return "", e
	}
	for _, entry := range entries {
		if _, e = tx.Exec(ctx, "INSERT INTO ledger_entries(workspace_id,transaction_id,account_id,asset,amount_atomic) VALUES($1,$2,$3,$4,$5)", workspace, id, entry.AccountID, a.Asset, entry.Money.AmountAtomic); e != nil {
			return "", e
		}
	}
	if _, e = tx.Exec(ctx, "INSERT INTO financial_adjustments(workspace_id,ledger_transaction_id,asset,amount_atomic,actor_id,reason,correlation_id) VALUES($1,$2,$3,$4,$5,$6,$7)", workspace, id, a.Asset, a.AmountAtomic, actor, a.Reason, correlation); e != nil {
		return "", e
	}
	if e = database.Audit(ctx, tx, actor, workspace, "wallet.adjust", "ledger_transaction", id, correlation, map[string]any{}, a); e != nil {
		return "", e
	}
	e = database.Event(ctx, tx, workspace, "telmivo.events.wallet", "wallet.adjusted", correlation, map[string]any{"ledger_transaction_id": id, "asset": a.Asset})
	return id, e
}
