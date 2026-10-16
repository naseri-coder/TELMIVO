package queue

// LIC-A9 Phase51-F: immutable SQL contract for the crash-safe, at-most-once
// broker-attempt ledger. The migration makes UNKNOWN a durable quarantine
// state, not a retry queue. SQL queries must stay guarded by the same state
// conditions and be tested against PostgreSQL before any release.
const (
	outboxSelectReadySQL      = "SELECT id::text,COALESCE(workspace_id::text,''),subject,event_type,correlation_id,payload FROM outbox_events WHERE delivery_state='READY' AND published_at IS NULL AND available_at<=now() AND (lease_until IS NULL OR lease_until<now()) ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED"
	outboxClaimUnknownSQL     = "UPDATE outbox_events SET delivery_state='UNKNOWN',delivery_attempted_at=clock_timestamp(),lease_until=clock_timestamp()+interval '60 seconds',lease_owner=$2,attempts=attempts+1 WHERE id=$1 AND delivery_state='READY' AND published_at IS NULL"
	outboxConfirmPublishedSQL = "UPDATE outbox_events SET delivery_state='PUBLISHED',published_at=clock_timestamp(),lease_until=NULL,lease_owner=NULL WHERE id=$1 AND delivery_state='UNKNOWN' AND published_at IS NULL AND lease_owner=$2"
)
