package queue

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/nats-io/nats.go"
	"time"
)

type Envelope struct {
	ID            string          `json:"id"`
	WorkspaceID   string          `json:"workspace_id"`
	Type          string          `json:"type"`
	CorrelationID string          `json:"correlation_id"`
	Payload       json.RawMessage `json:"payload"`
}
type Client struct {
	Conn *nats.Conn
	JS   nats.JetStreamContext
}

func Open(raw string) (*Client, error) {
	n, e := nats.Connect(raw, nats.Timeout(3*time.Second), nats.MaxReconnects(5), nats.ReconnectWait(time.Second))
	if e != nil {
		return nil, errors.New("NATS unavailable")
	}
	js, e := n.JetStream()
	if e != nil {
		n.Close()
		return nil, e
	}
	return &Client{Conn: n, JS: js}, nil
}
func (c *Client) Ensure() error {
	_, e := c.JS.AddStream(&nats.StreamConfig{Name: "TELMIVO", Subjects: []string{"telmivo.jobs.>", "telmivo.events.>"}, Storage: nats.FileStorage, Retention: nats.LimitsPolicy, MaxAge: 7 * 24 * time.Hour, MaxBytes: 512 << 20, Duplicates: 24 * time.Hour})
	return e
}
func (c *Client) Publish(ctx context.Context, subject string, event Envelope) error {
	b, e := json.Marshal(event)
	if e != nil {
		return e
	}
	m := nats.NewMsg(subject)
	m.Data = b
	m.Header.Set("Nats-Msg-Id", event.ID)
	_, e = c.JS.PublishMsg(m, nats.Context(ctx))
	return e
}
func (c *Client) Relay(ctx context.Context, db *database.Store) error {
	tx, e := db.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	rows, e := tx.Query(ctx, "SELECT id::text,COALESCE(workspace_id::text,''),subject,event_type,correlation_id,payload FROM outbox_events WHERE published_at IS NULL AND (lease_until IS NULL OR lease_until<now()) ORDER BY created_at LIMIT 32 FOR UPDATE SKIP LOCKED")
	if e != nil {
		return e
	}
	type item struct {
		subject string
		event   Envelope
	}
	var batch []item
	for rows.Next() {
		var x item
		if e = rows.Scan(&x.event.ID, &x.event.WorkspaceID, &x.subject, &x.event.Type, &x.event.CorrelationID, &x.event.Payload); e != nil {
			rows.Close()
			return e
		}
		batch = append(batch, x)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, x := range batch {
		if _, e = tx.Exec(ctx, "UPDATE outbox_events SET lease_until=now()+interval '30 seconds',attempts=attempts+1 WHERE id=$1", x.event.ID); e != nil {
			return e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	for _, x := range batch {
		if e = c.Publish(ctx, x.subject, x.event); e != nil {
			return e
		}
		if _, e = db.Pool.Exec(ctx, "UPDATE outbox_events SET published_at=now(),lease_until=NULL WHERE id=$1", x.event.ID); e != nil {
			return e
		}
	}
	return nil
}

type Handler func(context.Context, pgx.Tx, Envelope) error

func (c *Client) Consume(ctx context.Context, db *database.Store, subject, durable string, handler Handler) error {
	sub, e := c.JS.PullSubscribe(subject, durable, nats.BindStream("TELMIVO"), nats.AckExplicit(), nats.MaxDeliver(6), nats.AckWait(30*time.Second), nats.MaxAckPending(32), nats.BackOff([]time.Duration{time.Second, 5 * time.Second, 15 * time.Second, 30 * time.Second, time.Minute, 2 * time.Minute}))
	if e != nil {
		return e
	}
	defer sub.Unsubscribe()
	for ctx.Err() == nil {
		msgs, e := sub.Fetch(1, nats.MaxWait(time.Second))
		if errors.Is(e, nats.ErrTimeout) {
			continue
		}
		if e != nil {
			return e
		}
		for _, m := range msgs {
			var event Envelope
			e = json.Unmarshal(m.Data, &event)
			if e == nil && (event.ID == "" || event.WorkspaceID == "") {
				e = errors.New("invalid event envelope")
			}
			var tx pgx.Tx
			if e == nil {
				tx, e = db.Pool.Begin(ctx)
			}
			if e == nil {
				e = database.Scope(ctx, tx, event.WorkspaceID)
			}
			if e == nil {
				tag, err := tx.Exec(ctx, "INSERT INTO inbox_events(consumer,event_id) VALUES($1,$2) ON CONFLICT DO NOTHING", durable, event.ID)
				e = err
				if e == nil && tag.RowsAffected() > 0 {
					e = handler(ctx, tx, event)
				}
			}
			if tx != nil {
				if e == nil {
					e = tx.Commit(ctx)
				} else {
					tx.Rollback(ctx)
				}
			}
			if e == nil {
				if e = m.AckSync(nats.Context(ctx)); e != nil {
					return e
				}
				continue
			}
			meta, me := m.Metadata()
			if me == nil && meta.NumDelivered >= 6 {
				if event.ID == "" {
					event.ID = "00000000-0000-0000-0000-000000000000"
				}
				_, de := db.Pool.Exec(ctx, "INSERT INTO dead_letter_events(consumer,event_id,error_code,payload) VALUES($1,$2,'HANDLER_FAILED',$3) ON CONFLICT DO NOTHING", durable, event.ID, map[string]any{"type": event.Type, "workspace_id": event.WorkspaceID})
				if de != nil {
					return de
				}
				if e = m.Term(); e != nil {
					return e
				}
			} else {
				if e = m.NakWithDelay(5 * time.Second); e != nil {
					return e
				}
			}
		}
	}
	return ctx.Err()
}
