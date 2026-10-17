# LIC-A9 A2-02 — journal provenance and database role cutover contract

**CURRENT STATE: REVIEW ONLY / RLS NOT ENFORCED / NO MERGE OR DEPLOY.**

This milestone stages an **additive** migration and modifies only future Inbox and
Dead Letter writes. This is not a claim of a completed API/Worker role split.

## Durable journal invariants

- `0020_journal_workspace_provenance.sql` adds **nullable** `workspace_id uuid
  REFERENCES workspaces(id)` to `inbox_events` and `dead_letter_events`.
- Existing `(consumer,event_id)` unique/primary keys remain intact. The broker,
  domain idempotency and historical outbox `UNKNOWN` rows must not be replayed.
- New valid events record the envelope's validated workspace; new malformed
  DLQ envelopes record **NULL**. Verified cross-workspace collisions are
  **rejected**, not interpreted as harmless duplicates.
- Old inbox/DLQ rows retain **NULL**, even when untrusted JSON includes a
  `workspace_id` property. No bulk backfill is allowed without independent
  provenance from a separately authenticated source.
- `outbox_events.workspace_id` remains nullable because system-level events
  can be global. No schema-wide NOT NULL or automatic tenant RLS is allowed
  until the global Relay is redirected to a separate database role.
- The existing (consumer,event_id) deduplication key must not change in this
  phase. Unknown workspace implies unknown provenance, **not** proof of a
  particular tenant.

## Required next role-split sequence: NOT YET PERFORMED

1. **Offline role inventory**: run `a2_02_readonly_role_inventory.sql` only
   against a *verified disposable* PG16 installation. Confirm the old runtime
   still uses broad DML grants, and locate every direct query from API, bots,
   Workers and Relay. This inventory is read-only and is NOT an attestation.
2. **Distinct database identities**: provision a tenant/API credential and a
   physically separate worker/Relay credential. Both need `NOSUPERUSER`,
   `NOBYPASSRLS`, `NOINHERIT`, distinct secrets and a tested deny of `SET
   ROLE` and cross-tenant writes. Never trust `SET app.worker=1` set by the
   same login: it is caller-controlled and cannot be an authorization bypass.
3. **Explicit startup routing**: independently map API, Main/Group bots,
   media Worker, Queue Relay and migrations to roles and DSNs. The current
   `runtime.Run` opens the same `c.DatabaseURL` for all roles. Changing only
   PostgreSQL grants before code/DSNs is an outage risk.
4. **RLS/admission**: enforce `ENABLE` + `FORCE ROW LEVEL SECURITY` and
   privileged worker policies only after the worker credential is live and
   independently constrained to queue objects. Prove multi-tenant API
   isolation and global Relay/Consumer recovery under real PG16.
5. **Migration/backout**: stop old Relay, verify no active UNKNOWN reconciliation
   owner, preserve all journal rows and high-water marks, run new migration in
   the normal migration transaction, test rollback from a disposable snapshot.
   Never delete/recreate UNKNOWN rows, replace durable consumer identities or
   try to undo a broker send.
6. **Acceptance gates**: isolated PG16/Go1.27.2 true integration covering old
   NULL rows, new valid provenance, malformed DLQ, tenant collision, live
   worker role grants, denied cross-tenant attempts, failed transaction, crash,
   lost ACK and no accidental retries; real Manager V2, NATS and race tests.

**PRODUCTION NO-GO** until *all* the above are proven. This document does not
create accounts or grants, does not enable RLS, and does not touch any server.
