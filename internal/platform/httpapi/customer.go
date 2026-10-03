package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/domains/entitlements"
	"github.com/naseri-coder/TELMIVO/internal/domains/policies"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
)

type customerSettings struct {
	Name    string `json:"name"`
	CanEdit bool   `json:"can_edit"`
}
type customerOverview struct {
	Workspace string            `json:"workspace_id"`
	Name      string            `json:"name"`
	Plan      string            `json:"plan"`
	PlanLabel string            `json:"plan_label"`
	Channels  int64             `json:"channels"`
	Groups    int64             `json:"groups"`
	Bots      int64             `json:"bots"`
	Access    policies.Decision `json:"access"`
}

type customerPlan struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Version int64  `json:"version"`
}

func (a *API) customerCatalog(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		rs, e := tx.Query(r.Context(), "SELECT key,name,version FROM plans WHERE active ORDER BY key LIMIT 100")
		if e != nil {
			return nil, e
		}
		defer rs.Close()
		out := []customerPlan{}
		for rs.Next() {
			var p customerPlan
			if e = rs.Scan(&p.Key, &p.Name, &p.Version); e != nil {
				return nil, e
			}
			out = append(out, p)
		}
		return struct {
			Plans []customerPlan `json:"plans"`
		}{out}, rs.Err()
	})
}

func customerDecision(r *http.Request, tx pgx.Tx, feature string) (policies.Decision, error) {
	var role, permission string
	if e := tx.QueryRow(r.Context(), "SELECT role_key FROM workspace_members WHERE workspace_id=$1 AND user_id=$2", workspace(r), user(r)).Scan(&role); e != nil {
		return policies.Decision{}, e
	}
	if e := tx.QueryRow(r.Context(), "SELECT permission_key FROM features WHERE key=$1", feature).Scan(&permission); e != nil {
		return policies.Decision{}, e
	}
	return policies.Resolve(r.Context(), tx, policies.Request{User: user(r), Workspace: workspace(r), Role: role, Feature: feature, Permission: permission})
}
func (a *API) customerOverview(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		out := customerOverview{Workspace: workspace(r)}
		var e error
		out.Plan, e = entitlements.EffectivePlan(r.Context(), tx, workspace(r))
		if e != nil {
			return nil, e
		}
		if e = tx.QueryRow(r.Context(), "SELECT w.name,p.name FROM workspaces w CROSS JOIN plans p WHERE w.id=$1 AND p.key=$2", workspace(r), out.Plan).Scan(&out.Name, &out.PlanLabel); e != nil {
			return nil, e
		}
		if e = tx.QueryRow(r.Context(), "SELECT (SELECT count(*) FROM channels WHERE workspace_id=$1),(SELECT count(*) FROM groups WHERE workspace_id=$1),(SELECT count(*) FROM customer_bots WHERE workspace_id=$1)", workspace(r)).Scan(&out.Channels, &out.Groups, &out.Bots); e != nil {
			return nil, e
		}
		out.Access, e = customerDecision(r, tx, "downloads")
		return out, e
	})
}
func (a *API) customerAccess(w http.ResponseWriter, r *http.Request) {
	feature := r.URL.Query().Get("feature")
	if feature == "" {
		feature = "downloads"
	}
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) { return customerDecision(r, tx, feature) })
}
func readCustomerSettings(r *http.Request, tx pgx.Tx) (customerSettings, error) {
	var out customerSettings
	e := tx.QueryRow(r.Context(), "SELECT name,owner_id=$2::uuid FROM workspaces WHERE id=$1", workspace(r), user(r)).Scan(&out.Name, &out.CanEdit)
	return out, e
}
func (a *API) customerSettings(w http.ResponseWriter, r *http.Request) {
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) { return readCustomerSettings(r, tx) })
}
func (a *API) customerSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name         string `json:"name"`
		ExpectedName string `json:"expected_name"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if utf8.RuneCountInString(in.Name) < 1 || utf8.RuneCountInString(in.Name) > 200 || strings.ContainsFunc(in.Name, unicode.IsControl) {
		fail(w, 400, "invalid")
		return
	}
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		old, e := readCustomerSettings(r, tx)
		if e != nil {
			return nil, e
		}
		if !old.CanEdit {
			return nil, database.ErrForbidden
		}
		tag, e := tx.Exec(r.Context(), "UPDATE workspaces SET name=$1 WHERE id=$2 AND owner_id=$3 AND name=$4", in.Name, workspace(r), user(r), in.ExpectedName)
		if e != nil {
			return nil, e
		}
		if tag.RowsAffected() != 1 {
			return nil, database.ErrConflict
		}
		out := customerSettings{Name: in.Name, CanEdit: true}
		return out, database.Audit(r.Context(), tx, user(r), workspace(r), "workspace.name.update", "workspace", workspace(r), correlation(r), old, out)
	})
}

type walletCursor struct {
	Time time.Time `json:"time"`
	ID   string    `json:"id"`
}
type customerLedgerEntry struct {
	ID      string    `json:"id"`
	Asset   string    `json:"asset"`
	Amount  string    `json:"amount_atomic"`
	Reason  string    `json:"reason"`
	Created time.Time `json:"created_at"`
}
type customerLedgerPage struct {
	Items []customerLedgerEntry `json:"items"`
	Next  string                `json:"next_cursor"`
}

func (a *API) customerHistory(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 1 || n > 50 {
			fail(w, 400, "invalid")
			return
		}
		limit = n
	}
	cursor := walletCursor{Time: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), ID: "ffffffff-ffff-ffff-ffff-ffffffffffff"}
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		b, e := base64.RawURLEncoding.DecodeString(raw)
		if e != nil || len(b) > 256 || json.Unmarshal(b, &cursor) != nil || cursor.Time.IsZero() || len(cursor.ID) != 36 {
			fail(w, 400, "invalid")
			return
		}
	}
	a.tenant(w, r, false, func(tx pgx.Tx) (any, error) {
		if !database.Permission(r.Context(), tx, user(r), workspace(r), "wallet.read") {
			return nil, database.ErrForbidden
		}
		rs, e := tx.Query(r.Context(), `SELECT e.id::text,t.asset,e.amount_atomic::text,t.reason,t.created_at FROM ledger_transactions t JOIN ledger_entries e ON e.transaction_id=t.id JOIN wallet_accounts a ON a.id=e.account_id WHERE t.workspace_id=$1 AND a.kind='WALLET' AND (t.created_at,e.id)<($2,$3::uuid) ORDER BY t.created_at DESC,e.id DESC LIMIT $4`, workspace(r), cursor.Time, cursor.ID, limit+1)
		if e != nil {
			return nil, e
		}
		defer rs.Close()
		out := customerLedgerPage{Items: []customerLedgerEntry{}}
		for rs.Next() {
			var item customerLedgerEntry
			if e = rs.Scan(&item.ID, &item.Asset, &item.Amount, &item.Reason, &item.Created); e != nil {
				return nil, e
			}
			out.Items = append(out.Items, item)
		}
		if e = rs.Err(); e != nil {
			return nil, e
		}
		if len(out.Items) > limit {
			out.Items = out.Items[:limit]
			last := out.Items[limit-1]
			b, _ := json.Marshal(walletCursor{Time: last.Created, ID: last.ID})
			out.Next = base64.RawURLEncoding.EncodeToString(b)
		}
		return out, nil
	})
}
