package businessplugins

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"
	"github.com/naseri-coder/TELMIVO/internal/plugins/crm"
	"github.com/naseri-coder/TELMIVO/internal/plugins/shop"
)

func (s *Service) SaveProduct(ctx context.Context, actor string, scope sdk.Scope, id string, in Change[shop.Product]) (Definition, error) {
	scope.PluginID = shop.ID
	p := in.Definition
	if !p.Valid() {
		return Definition{}, database.ErrInvalid
	}
	p.ID = ""
	p.Revision = 0
	return s.save(ctx, actor, scope, id, p.Name, in.Key, in.Expected, p)
}
func order(ctx context.Context, tx pgx.Tx, scope sdk.Scope, id string) (shop.Order, error) {
	var o shop.Order
	var lines, total []byte
	e := tx.QueryRow(ctx, "SELECT id::text,contact_id::text,state,lines,total,revision FROM commerce_orders WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3", scope.AccountID, scope.BotID, id).Scan(&o.ID, &o.Contact, &o.State, &lines, &total, &o.Revision)
	if e != nil {
		return o, e
	}
	if e = json.Unmarshal(lines, &o.Lines); e != nil {
		return o, e
	}
	e = json.Unmarshal(total, &o.Total)
	return o, e
}

// PlaceTrusted accepts only product identifiers/quantities. Client prices are never accepted.
func (s *Service) PlaceTrusted(ctx context.Context, actor string, scope sdk.Scope, k string, c Customer, items []shop.Item) (shop.Order, error) {
	scope.PluginID = shop.ID
	var out shop.Order
	if c.User <= 0 || !key(k) || len(items) < 1 || len(items) > 20 {
		return out, database.ErrInvalid
	}
	fp := hash([]any{"order", actor, c, items})
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		b, exists, e := receipt(ctx, tx, scope, k, fp)
		if e != nil {
			return e
		}
		if exists {
			return json.Unmarshal(b, &out)
		}
		if e = s.admit(ctx, tx, actor, scope, "commerce.order", true); e != nil {
			return e
		}
		products := map[string]shop.Product{}
		for _, i := range items {
			d, e := get(ctx, tx, scope, i.Product)
			if e != nil {
				return e
			}
			var p shop.Product
			if e = json.Unmarshal(d.Data, &p); e != nil {
				return e
			}
			p.ID = d.ID
			p.Revision = d.Revision
			products[i.Product] = p
		}
		lines, total, valid := shop.Snapshot(items, products)
		if !valid {
			return database.ErrInvalid
		}
		if e = s.crm(ctx, tx, scope, true); e != nil {
			return e
		}
		if e = s.quota(ctx, tx, scope, "commerce.orders.monthly", 1, true); e != nil {
			return e
		}
		p, e := s.principal(ctx, tx, scope)
		if e != nil {
			return e
		}
		contact, e := s.Support.ResolveBusinessTx(ctx, tx, p, scope, crm.ResolveRequest{Identity: crm.Identity{Provider: "telegram", Key: fmtUser(c.User)}, Name: c.Name, Username: c.Username, Locale: c.Locale}, nil, nil)
		if e != nil {
			return e
		}
		out = shop.Order{Contact: contact.ID, State: "PLACED", Lines: lines, Total: total, Revision: 1}
		e = tx.QueryRow(ctx, "INSERT INTO commerce_orders(workspace_id,bot_id,contact_id,state,lines,total,created_at,updated_at) VALUES($1,$2,$3,'PLACED',$4,$5,$6,$6) RETURNING id::text", scope.AccountID, scope.BotID, contact.ID, body(lines), body(total), s.now()).Scan(&out.ID)
		if e != nil {
			return e
		}
		if e = s.history(ctx, tx, actor, scope, out); e != nil {
			return e
		}
		if e = emit(ctx, tx, actor, scope, "commerce.order.created", out.ID, contact.ID, map[string][]string{"order.id": {out.ID}, "order.status": {out.State}}); e != nil {
			return e
		}
		return record(ctx, tx, scope, k, fp, out)
	})
	return out, e
}
func (s *Service) history(ctx context.Context, tx pgx.Tx, actor string, scope sdk.Scope, o shop.Order) error {
	_, e := tx.Exec(ctx, "INSERT INTO commerce_order_history(workspace_id,bot_id,order_id,actor_id,state,revision,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)", scope.AccountID, scope.BotID, o.ID, actor, o.State, o.Revision, s.now())
	return e
}
func (s *Service) OrderAction(ctx context.Context, actor string, scope sdk.Scope, id string, in Action) (shop.Order, error) {
	scope.PluginID = shop.ID
	var out shop.Order
	if !key(in.Key) || in.Expected < 1 {
		return out, database.ErrInvalid
	}
	fp := hash([]any{actor, id, in})
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		b, exists, e := receipt(ctx, tx, scope, in.Key, fp)
		if e != nil {
			return e
		}
		if exists {
			return json.Unmarshal(b, &out)
		}
		out, e = order(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		// Lock the same order used by checkout admission before cancellation.
		if _, e = tx.Exec(ctx, "SELECT id FROM commerce_orders WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3 FOR UPDATE", scope.AccountID, scope.BotID, id); e != nil {
			return e
		}
		out, e = order(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		if in.Action == "CANCELLED" {
			var active bool
			if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM commerce_payment_intents WHERE workspace_id=$1 AND bot_id=$2 AND order_id=$3 AND state IN('CREATING','AWAITING_PAYMENT','PAID','UNKNOWN','REFUND_PENDING','REFUND_UNKNOWN'))", scope.AccountID, scope.BotID, id).Scan(&active); e != nil {
				return e
			}
			if active {
				return database.ErrConflict
			}
		}
		if out.Revision != in.Expected || !shop.Transition(out.State, in.Action) {
			return database.ErrConflict
		}
		if _, e = tx.Exec(ctx, "UPDATE commerce_orders SET state=$4,revision=revision+1,updated_at=$5 WHERE workspace_id=$1 AND bot_id=$2 AND id::text=$3", scope.AccountID, scope.BotID, id, in.Action, s.now()); e != nil {
			return e
		}
		out.State = in.Action
		out.Revision++
		if e = s.history(ctx, tx, actor, scope, out); e != nil {
			return e
		}
		if e = emit(ctx, tx, actor, scope, "commerce.order.status_changed", id, out.Contact, map[string][]string{"order.id": {id}, "order.status": {in.Action}}); e != nil {
			return e
		}
		return record(ctx, tx, scope, in.Key, fp, out)
	})
	return out, e
}

type OrderPage struct {
	Items []shop.Order `json:"items"`
	Next  string       `json:"next_cursor"`
}

func (s *Service) Orders(ctx context.Context, actor string, scope sdk.Scope, after, status, contact string) (OrderPage, error) {
	scope.PluginID = shop.ID
	out := OrderPage{Items: []shop.Order{}}
	if len(after) > 36 || len(status) > 20 || len(contact) > 36 {
		return out, database.ErrInvalid
	}
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		rs, e := tx.Query(ctx, "SELECT id::text FROM commerce_orders WHERE workspace_id=$1 AND bot_id=$2 AND ($3='' OR id::text>$3) AND ($4='' OR state=$4) AND ($5='' OR contact_id::text=$5) ORDER BY id LIMIT 51", scope.AccountID, scope.BotID, after, status, contact)
		if e != nil {
			return e
		}
		ids := []string{}
		for rs.Next() {
			var id string
			if e = rs.Scan(&id); e != nil {
				rs.Close()
				return e
			}
			ids = append(ids, id)
		}
		rs.Close()
		if e = rs.Err(); e != nil {
			return e
		}
		if len(ids) > 50 {
			ids = ids[:50]
			out.Next = ids[49]
		}
		for _, id := range ids {
			o, e := order(ctx, tx, scope, id)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, o)
		}
		return nil
	})
	return out, e
}
func (s *Service) OrderHistory(ctx context.Context, actor string, scope sdk.Scope, id, after string) ([]map[string]any, error) {
	scope.PluginID = shop.ID
	out := []map[string]any{}
	if !validHistoryCursor(after) {
		return out, database.ErrInvalid
	}
	e := s.tenant(ctx, actor, scope, func(tx pgx.Tx) error {
		rs, e := tx.Query(ctx, "SELECT id::text,state,actor_id::text,created_at,revision FROM commerce_order_history WHERE workspace_id=$1 AND bot_id=$2 AND order_id::text=$3 AND revision>$4 ORDER BY revision LIMIT 50", scope.AccountID, scope.BotID, id, historyAfter(after))
		if e != nil {
			return e
		}
		defer rs.Close()
		for rs.Next() {
			var rid, state, a string
			var at any
			var revision int64
			if e = rs.Scan(&rid, &state, &a, &at, &revision); e != nil {
				return e
			}
			out = append(out, map[string]any{"id": rid, "state": state, "actor": a, "created_at": at, "cursor": fmtUser(revision)})
		}
		return rs.Err()
	})
	return out, e
}
