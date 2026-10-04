package commerce

import (
	"context"
	"github.com/jackc/pgx/v5"
	"time"
)

// Snapshot is a trusted Core read model. Customer HTTP responses must project it;
// raw catalog capabilities, agreement terms and other bot identifiers stay private.
type Snapshot struct {
	Catalog    Catalog
	Agreements []Agreement
	Plan       string
	Consumed   map[string]bool
	Now        time.Time
}

func (s *Service) SnapshotTx(ctx context.Context, tx pgx.Tx, account, bot string) (Snapshot, error) {
	out := Snapshot{Now: s.now(), Consumed: map[string]bool{}}
	var e error
	if e = requireBot(ctx, tx, account, bot); e != nil {
		return out, e
	}
	out.Catalog, e = readCatalog(ctx, tx, out.Now)
	if e != nil {
		return out, e
	}
	out.Agreements, e = readAgreements(ctx, tx, account)
	if e != nil {
		return out, e
	}
	out.Plan, e = effectivePlan(ctx, tx, account, out.Now)
	if e != nil {
		return out, e
	}
	rs, e := tx.Query(ctx, `SELECT plugin_id FROM commercial_trial_consumptions WHERE workspace_id=$1 AND (license_scope='PER_ACCOUNT' OR subject_id=$2)`, account, bot)
	if e != nil {
		return out, e
	}
	defer rs.Close()
	for rs.Next() {
		var id string
		if e = rs.Scan(&id); e != nil {
			return out, e
		}
		out.Consumed[id] = true
	}
	return out, rs.Err()
}
