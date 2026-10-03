package queue

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/naseri-coder/TELMIVO/internal/platform/auth"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	"github.com/nats-io/nats.go"
	"regexp"
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
	owner, e := auth.Token()
	if e != nil {
		return e
	}
	tx, e := db.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	rows, e := tx.Query(ctx, "SELECT id::text,COALESCE(workspace_id::text,''),subject,event_type,correlation_id,payload FROM outbox_events WHERE published_at IS NULL AND available_at<=now() AND (lease_until IS NULL OR lease_until<now()) ORDER BY created_at LIMIT 8 FOR UPDATE SKIP LOCKED")
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
		if _, e = tx.Exec(ctx, "UPDATE outbox_events SET lease_until=now()+interval '60 seconds',lease_owner=$2,attempts=attempts+1 WHERE id=$1", x.event.ID, owner); e != nil {
			return e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	for _, x := range batch {
		publishCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		e = c.Publish(publishCtx, x.subject, x.event)
		cancel()
		if e != nil {
			_, releaseErr := db.Pool.Exec(ctx, "UPDATE outbox_events SET lease_until=NULL,lease_owner=NULL,available_at=now()+LEAST(attempts,60)*interval '1 second' WHERE id=$1 AND lease_owner=$2", x.event.ID, owner)
			if releaseErr != nil {
				return releaseErr
			}
			return e
		}
		if _, e = db.Pool.Exec(ctx, "UPDATE outbox_events SET published_at=now(),lease_until=NULL,lease_owner=NULL WHERE id=$1 AND lease_owner=$2", x.event.ID, owner); e != nil {
			return e
		}
	}
	return nil
}

type Handler func(context.Context, pgx.Tx, Envelope) error

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func (c *Client) Consume(ctx context.Context, db *database.Store, subject, durable string, handler Handler) error {
	// Explicitly create the durable, then bind: unsubscribe must not delete it.
	_, err := c.JS.AddConsumer("TELMIVO", &nats.ConsumerConfig{Durable: durable, FilterSubject: subject, AckPolicy: nats.AckExplicitPolicy, AckWait: 30 * time.Second, MaxDeliver: 6, MaxAckPending: 32, BackOff: []time.Duration{time.Second, 2 * time.Second, 3 * time.Second, 4 * time.Second, 5 * time.Second, 6 * time.Second}})
	if err != nil {
		return err
	}
	sub, err := c.JS.PullSubscribe(subject, durable, nats.Bind("TELMIVO", durable))
	if err != nil {
		return err
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
			malformed := e != nil || !uuidPattern.MatchString(event.ID) || !uuidPattern.MatchString(event.WorkspaceID) || event.Type == ""
			meta, me := m.Metadata()
			if me != nil {
				return me
			}
			if malformed {
				e = errors.New("invalid envelope")
			} else {
				tx, beginErr := db.Pool.Begin(ctx)
				e = beginErr
				if e == nil {
					e = database.Scope(ctx, tx, event.WorkspaceID)
					if e == nil {
						tag, insertErr := tx.Exec(ctx, "INSERT INTO inbox_events(consumer,event_id) VALUES($1,$2) ON CONFLICT DO NOTHING", durable, event.ID)
						e = insertErr
						if e == nil && tag.RowsAffected() > 0 {
							e = handler(ctx, tx, event)
						}
					}
					if e == nil {
						e = tx.Commit(ctx)
					} else {
						_ = tx.Rollback(context.Background())
					}
				}
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if e == nil {
				if e = m.AckSync(nats.Context(ctx)); e != nil {
					return e
				}
				continue
			}
			if malformed || meta.NumDelivered >= 6 {
				code := "HANDLER_FAILED"
				id := event.ID
				if malformed {
					code = "MALFORMED_EVENT"
					h := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", meta.Stream, meta.Sequence.Stream)))
					id = fmt.Sprintf("%x-%x-%x-%x-%x", h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
				}
				_, e = db.Pool.Exec(ctx, "INSERT INTO dead_letter_events(consumer,event_id,error_code,payload) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING", durable, id, code, map[string]any{"stream_sequence": meta.Sequence.Stream})
				if e != nil {
					return e
				}
				if e = m.Term(); e != nil {
					return e
				}
			} else {
				if e = m.NakWithDelay(time.Duration(meta.NumDelivered) * time.Second); e != nil {
					return e
				}
			}
		}
	}
	return ctx.Err()
}
