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
	"github.com/naseri-coder/TELMIVO/internal/platform/installationlicense"
	"github.com/nats-io/nats.go"
	"log/slog"
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

// No transport error can prove that JetStream rejected the request. This
// sentinel signals durable quarantine: never retry without reconciliation.
var ErrOutboxDeliveryUnknown = errors.New("OUTBOX_DELIVERY_UNKNOWN_RECONCILIATION_REQUIRED")

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
// Ensure may create a JetStream stream and therefore MUST be admitted as a
// signed external effect. Never bootstrap broker state without the installed
// Manager's current authorization, including during restricted startup.
func (c *Client) Ensure(ctx context.Context) error {
	return installationlicense.RuntimeEffect(ctx, func() error {
		_, e := c.JS.AddStream(&nats.StreamConfig{Name: "TELMIVO", Subjects: []string{"telmivo.jobs.>", "telmivo.events.>"}, Storage: nats.FileStorage, Retention: nats.LimitsPolicy, MaxAge: 7 * 24 * time.Hour, MaxBytes: 512 << 20, Duplicates: 24 * time.Hour})
		return e
	})
}
// Publish is the final process-wide signed-license fence for NATS effects.
// Relay preflight is not an authorization lease: revocation can occur after
// its database transaction and before the transport call. RuntimeEffect
// rechecks the live signed Manager while holding its guarded effect admission.
// Nats-Msg-Id and the existing outbox lease/receipt remain the replay fences.
func (c *Client) Publish(ctx context.Context, subject string, event Envelope) error {
	return installationlicense.RuntimeEffect(ctx, func() error {
		b, e := json.Marshal(event)
		if e != nil {
			return e
		}
		m := nats.NewMsg(subject)
		m.Data = b
		m.Header.Set("Nats-Msg-Id", event.ID)
		_, e = c.JS.PublishMsg(m, nats.Context(ctx))
		return e
	})
}
func (c *Client) Relay(ctx context.Context, db *database.Store) error {
	if e := installationlicense.CheckRuntime(ctx); e != nil {
		return e
	}
	owner, e := auth.Token()
	if e != nil {
		return e
	}
	tx, e := db.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	// Claim only one row per transaction. A crash after claiming cannot
	// quarantine unrelated unattempted rows from the same batch.
	rows, e := tx.Query(ctx, outboxSelectReadySQL)
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
		// This immutable pre-transport UNKNOWN fence is COMMITTED before
		// calling JetStream. Even a crash during PublishMsg is not replayed.
		tag, updateErr := tx.Exec(ctx, outboxClaimUnknownSQL, x.event.ID, owner)
		if updateErr != nil {
			return updateErr
		}
		if tag.RowsAffected() != 1 {
			return ErrOutboxDeliveryUnknown
		}
	}
	if e = tx.Commit(ctx); e != nil {
		// The claim commit outcome is ambiguous. No network I/O occurred.
		// Do NOT infer eligibility for retry or send a message on this path.
		return e
	}
	for _, x := range batch {
		publishCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		e = c.Publish(publishCtx, x.subject, x.event)
		cancel()
		if e != nil {
			// Unknown broker outcome, even on timeout/connection failure.
			// Leave the durable UNKNOWN row quarantined, retaining the lease
			// owner and attempt evidence for operator reconciliation.
			return ErrOutboxDeliveryUnknown
		}
		// JetStream acknowledged receipt. Mark PUBLISHED only while the same
		// owner still holds the matching unknown-attempt fence.
		tag, updateErr := db.Pool.Exec(ctx, outboxConfirmPublishedSQL, x.event.ID, owner)
		if updateErr != nil || tag.RowsAffected() != 1 {
			// A lost DB acknowledgment cannot revoke or replay a published
			// broker message. The UNKNOWN row remains for reconciliation.
			return ErrOutboxDeliveryUnknown
		}
	}
	return nil
}

type Handler func(context.Context, pgx.Tx, Envelope) error

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func (c *Client) Consume(ctx context.Context, db *database.Store, subject, durable string, handler Handler) error {
	return c.consume(ctx, db, subject, durable, handler, false)
}

// ConsumeExternal fences non-idempotent transport before invoking it. Unlike pure
// transactional handlers, interrupted external attempts must never be replayed.
func (c *Client) ConsumeExternal(ctx context.Context, db *database.Store, subject, durable string, handler Handler) error {
	return c.consume(ctx, db, subject, durable, handler, true)
}

func (c *Client) consume(ctx context.Context, db *database.Store, subject, durable string, handler Handler, external bool) error {
	// All ConsumerInfo/AddConsumer/PullSubscribe requests happen under the
	// process-wide signed effect fence. No broker call is allowed on a
	// restricted/uninstalled/revoked license. The subscription is bound to
	// an existing durable; its eventual Unsubscribe only releases the handle.
	var sub *nats.Subscription
	err := installationlicense.RuntimeEffect(ctx, func() error {
		info, e := c.JS.ConsumerInfo("TELMIVO", durable)
		if errors.Is(e, nats.ErrConsumerNotFound) {
			info, e = c.JS.AddConsumer("TELMIVO", &nats.ConsumerConfig{Durable: durable, FilterSubject: subject, AckPolicy: nats.AckExplicitPolicy, AckWait: 30 * time.Second, MaxDeliver: 6, MaxAckPending: 32, BackOff: []time.Duration{time.Second, 2 * time.Second, 3 * time.Second, 4 * time.Second, 5 * time.Second, 6 * time.Second}})
		}
		if e != nil {
			return e
		}
		if info.Config.FilterSubject != subject || info.Config.AckPolicy != nats.AckExplicitPolicy || info.Config.MaxDeliver != 6 {
			return errors.New("durable consumer configuration mismatch")
		}
		sub, e = c.JS.PullSubscribe(subject, durable, nats.Bind("TELMIVO", durable))
		return e
	})
	if err != nil {
		return err
	}
	defer sub.Unsubscribe()
	for ctx.Err() == nil {
		if installationlicense.CheckRuntime(ctx) != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
				continue
			}
		}
		// The earlier CheckRuntime is advisory only. A signed license can
		// be revoked between that check and the next broker request.
		var msgs []*nats.Msg
		e := installationlicense.RuntimeEffect(ctx, func() error {
			var fetchErr error
			msgs, fetchErr = sub.Fetch(1, nats.MaxWait(time.Second))
			return fetchErr
		})
		if errors.Is(e, installationlicense.ErrRestricted) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
				continue
			}
		}
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
				e = Process(ctx, db, durable, event, external, handler)
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if e == nil {
				if e = installationlicense.RuntimeEffect(ctx, func() error { return m.AckSync(nats.Context(ctx)) }); e != nil {
					return e
				}
				continue
			}
			if malformed || meta.NumDelivered >= 6 {
				slog.Warn("event dead letter", "state", "DLQ", "malformed", malformed)
				code := "HANDLER_FAILED"
				id := event.ID
				if malformed {
					code = "MALFORMED_EVENT"
					h := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", meta.Stream, meta.Sequence.Stream)))
					id = fmt.Sprintf("%x-%x-%x-%x-%x", h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
				}
				// A malformed envelope has no trustworthy tenant identity:
				// leave workspace_id NULL rather than deriving it from
				// untrusted bytes. Syntactically valid events carry only
				// their CLAIMED workspace, not producer authentication.
				var workspace any
				var workspaceText string
				if !malformed {
					workspace = event.WorkspaceID
					workspaceText = event.WorkspaceID
				}
				e = installationlicense.RuntimeEffect(ctx, func() error {
					tag, writeErr := db.Pool.Exec(ctx, "INSERT INTO dead_letter_events(consumer,event_id,workspace_id,error_code,payload) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING", durable, id, workspace, code, map[string]any{"stream_sequence": meta.Sequence.Stream})
					if writeErr != nil {
						return writeErr
					}
					if tag.RowsAffected() == 0 {
						// Duplicate records may represent a prior failed
						// Term. Only replay the broker disposition if the
						// existing durable journal has the SAME provenance;
						// never reassign unknown/other tenant history.
						var recorded string
						if qErr := db.Pool.QueryRow(ctx, "SELECT COALESCE(workspace_id::text,'') FROM dead_letter_events WHERE consumer=$1 AND event_id=$2", durable, id).Scan(&recorded); qErr != nil {
							return qErr
						}
						if recorded != workspaceText {
							return database.ErrForbidden
						}
					}
					return m.Term()
				})
				if e != nil {
					return e
				}
			} else {
				if e = installationlicense.RuntimeEffect(ctx, func() error {
					return m.NakWithDelay(time.Duration(meta.NumDelivered) * time.Second)
				}); e != nil {
					return e
				}
			}
		}
	}
	return ctx.Err()
}
