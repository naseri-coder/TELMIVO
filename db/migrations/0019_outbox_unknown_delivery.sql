-- LIC-A9 Phase51-F: durable, fail-closed JetStream outbox dispatch.
-- A committed UNKNOWN means a publish was authorized to start, NOT that the
-- broker definitely received it. Only an acknowledged PublishMsg can be
-- marked PUBLISHED by the active relay. UNKNOWN never auto-reenters READY.
-- Deployment requires all previous Relay workers to be stopped before upgrade.
ALTER TABLE outbox_events
 ADD COLUMN delivery_state text NOT NULL DEFAULT 'READY',
 ADD COLUMN delivery_attempted_at timestamptz;

-- PRE-UPGRADE SAFETY: an old Relay may have sent to JetStream but lost
-- the acknowledgment before saving published_at. Its attempts counter is
-- durable even when the broker outcome is not. Quarantine ALL preexisting
-- unacknowledged attempts, including attempts with an expired lease. The
-- timestamp is the quarantine migration observation time, NOT proof of
-- a historical broker send timestamp. Manual broker/consumer reconciliation
-- is mandatory before any independent disposition; never auto-reopen READY.
-- Old Relay processes MUST be stopped and fenced before this migration.
UPDATE outbox_events SET delivery_state='UNKNOWN',
 delivery_attempted_at=clock_timestamp()
 WHERE published_at IS NULL AND attempts>0;

-- Only historically acknowledged rows are known to have published; these
-- rows already have their actual successful publication timestamp.
UPDATE outbox_events SET delivery_state='PUBLISHED',
 delivery_attempted_at=published_at
 WHERE published_at IS NOT NULL;

ALTER TABLE outbox_events
 ADD CONSTRAINT outbox_delivery_state_valid
 CHECK (delivery_state IN ('READY','UNKNOWN','PUBLISHED')),
 ADD CONSTRAINT outbox_delivery_published_consistency
 CHECK ((delivery_state='PUBLISHED')=(published_at IS NOT NULL)),
 ADD CONSTRAINT outbox_delivery_attempt_consistency
 CHECK (delivery_state='READY' OR delivery_attempted_at IS NOT NULL);

-- A partially acknowledged request is quarantined before transport. Explicit
-- proof of delivery/absence is required for any external reconciliation.
CREATE INDEX outbox_delivery_ready ON outbox_events(available_at,created_at,id)
 WHERE delivery_state='READY' AND published_at IS NULL;
CREATE INDEX outbox_delivery_unknown ON outbox_events(delivery_attempted_at,id)
 WHERE delivery_state='UNKNOWN';

CREATE FUNCTION guard_outbox_delivery_transition() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN
  IF OLD.delivery_state='UNKNOWN' THEN
   RAISE EXCEPTION 'unknown outbox attempt is durable and requires reconciliation';
  END IF;
  RETURN OLD;
 END IF;
 IF OLD.delivery_state='PUBLISHED' AND NEW.delivery_state<>'PUBLISHED' THEN
  RAISE EXCEPTION 'published outbox event cannot be reopened';
 END IF;
 IF OLD.delivery_state='UNKNOWN' AND NEW.delivery_state='READY' THEN
  RAISE EXCEPTION 'unknown outbox outcome cannot be automatically replayed';
 END IF;
 IF OLD.delivery_state='READY' AND NEW.delivery_state='PUBLISHED' THEN
  RAISE EXCEPTION 'outbox publish requires a durable UNKNOWN pre-transport fence';
 END IF;
 IF OLD.delivery_state='READY' AND NEW.delivery_state='UNKNOWN' THEN
  IF NEW.lease_owner IS NULL OR NEW.lease_until IS NULL
     OR NEW.attempts<=OLD.attempts OR NEW.delivery_attempted_at IS NULL THEN
   RAISE EXCEPTION 'invalid outbox dispatch fence';
  END IF;
 END IF;
 IF OLD.delivery_state='UNKNOWN' AND NEW.delivery_state='PUBLISHED' THEN
  IF NEW.published_at IS NULL THEN
   RAISE EXCEPTION 'published outbox requires acknowledged timestamp';
  END IF;
 END IF;
 RETURN NEW;
END
$$;
CREATE TRIGGER outbox_delivery_transition
 BEFORE UPDATE OR DELETE ON outbox_events
 FOR EACH ROW EXECUTE FUNCTION guard_outbox_delivery_transition();
