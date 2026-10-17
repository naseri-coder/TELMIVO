-- LIC-A9 A2-02: additive, provenance-first journal migration.
-- INTEGRATION REVIEW ONLY: do not execute against a current installation.
--
-- These preexisting durable journals use (consumer,event_id) as the immutable
-- deduplication key. Never re-key, delete, truncate or auto-replay records.
-- There is no guaranteed historical join to derive the workspace of old
-- inbox/DLQ events. Legacy rows remain NULL for independent reconciliation.
-- Malformed external envelopes also have deliberately UNKNOWN provenance.
--
-- This does NOT claim that a tenant RLS boundary is active. It remains NO-GO
-- until independent API/worker SQL roles and full real PG16 tests are ready.
ALTER TABLE inbox_events
  ADD COLUMN workspace_id uuid REFERENCES workspaces(id);

ALTER TABLE dead_letter_events
  ADD COLUMN workspace_id uuid REFERENCES workspaces(id);

COMMENT ON COLUMN inbox_events.workspace_id IS
  'Verified workspace from new admitted event envelopes; NULL legacy rows require manual provenance reconciliation, never guessed backfill.';
COMMENT ON COLUMN dead_letter_events.workspace_id IS
  'Verified workspace only for valid new events; NULL is preserved for malformed/legacy records, never guessed from untrusted payload.';

-- No NOT NULL on historical rows, no irreversible mass backfill, no RLS
-- activation while global Relay and consumer still use telmivo_runtime.
-- Primary keys, attempts and UNKNOWN quarantine remain unchanged.
