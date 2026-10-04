# ADR-018 — Leased work, quota allocation and recovery

Status: R2B foundation candidate; native validation must establish acceptance.

Extend existing `media_jobs`, transactional outbox and inbox. A companion PostgreSQL lease carries worker identity, monotonic generation, acquisition/expiry and an immutable deadline. Row locking serializes claim/reclaim. Completion locks and verifies ownership, settles quota and records the terminal job/event atomically. Database triggers guard terminal jobs and allocation identity. Redis never owns a lease.

Leases are bounded to five minutes per heartbeat and a 24-hour total job lifetime. Worker identity is trusted server-side; lease generations are internal fencing values, never authorization to another tenant. Clients must acquire/renew/finish using separate short transactions; heavy computation and external I/O happen outside transactions. Stale ownership fails closed. Reconciliation emits at most one existing-outbox wakeup per abandoned generation from database job state, including after broker loss.

Quota reservation identity/policy/amount/bucket/terminal history remains immutable. A companion allocation records approved effective expiry, coupled to the active lease. Resolver and settlement consult that expiry; extension rechecks current permissions/resource limits and caps. Calendar crossing is rejected, with no allocation in a second bucket; long jobs should use a suitable rolling/non-resetting policy or a future explicitly audited cross-window contract. Reclaim requires the quota allocation still to be valid. A retry cannot create another reservation.

A provider-neutral credential keyring supports one active key and previous decrypt-only keys. Rotation locks an existing installation, validates workspace/context, changes ciphertext/key ID and audits only identifiers. Retire an old key only after all its records have rotated and restored historical backups have an explicit decrypt policy.

Preserve StorageProvider. Add a private S3 transport contract, rooted atomic local adapter and membership-authorized scoped read endpoint; public signed URLs are disabled. Temporary objects carry expiry and explicit cleanup. No product ingestion pipeline or S3 vendor is installed.

Recovery uses a versioned synthetic-only PostgreSQL custom dump and a fresh second service. Canonical table counts/content identities, migrations, balanced ledger, isolation, snapshots and replay are checked. PostgreSQL is authoritative; Redis temporary state can be dropped. Restore/recreate JetStream streams and durables, relay pending outbox and reconcile nonterminal work from PostgreSQL. Inbox dedup and fencing survive restore. Backups and key material require separate secure operational storage; production operations are not delivered by this stage.
