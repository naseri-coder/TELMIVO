# R2B foundation extension

See ADR-018 for durable contracts, expiry boundaries and provider-neutral adapters.

## Gap ledger

A — foundation gaps: native-connected browser acceptance; workspace/installation trial scope coverage; representative plugin permission/dependency/merge cases; durable job lease/fencing; correlated long-job quota allocation; local/private storage and cleanup; credential key rotation; disposable backup/restore and recovery reconciliation; operational outcome signals.

B — product deferred: real media extraction/ingestion, Downloader UX, moderation/anti-spam/anti-flood/captcha/raid/Telegram authority adapter, complete Uploader/Factory/customer runtime, payments/Stars/crypto/refunds, broadcasts, AI and production deployment/secrets/database operations.

C — already satisfied/preserved: migration 0001–0008 byte identity; identity/workspace/access/ledger catalogs; immutable policy and trial snapshots; granular/versioned/reasoned/audited policy mutation; shared runtime/Admin Preview; FORCE RLS/restricted runtime role; origin/headers/error redaction; context-bound AES-GCM and stored key IDs; StorageProvider; transactional outbox/inbox and bounded retry/DLQ; structured correlation logging.

## Operations

`jobs.Create`, `Acquire`, `Renew`, `Finish`, `Reconcile` operate in authorized scoped PostgreSQL transactions. Cancellation is a fenced terminal transition; a different result on replay is a conflict. No full media processor is provided. A job past its immutable deadline, or abandoned after its allocation expires, requires an explicit operator recovery decision, not a silent new reservation. Production workers must heartbeat before expiry and run the bounded reconciliation sweep for each authorized workspace.

`identity.RotateCredential` accepts an injected `crypto.RotatingStore`. Key material is loaded outside the domain through a provider; no KMS vendor is selected. Existing installation ciphertext is context-bound to workspace/installation identity. Unknown/malformed IDs and incorrect context fail closed.

`storage.Scoped` is the domain boundary. Its workspace argument must be the authorized tenant context. `Local` uses an OS-rooted filesystem, deterministic workspace/hash keys, bounded size, metadata, atomic replacement and expiring-object cleanup. `S3Compatible` requires a private bucket; a production network adapter and private-bucket native verification remain outside this contract delivery. The optional HTTP read endpoint receives a scoped provider through dependency injection and enforces membership and uploads.read; default runtime has no configured object provider.

HTTP logs retain generated correlation/request IDs and add outcome status. Lease/reclaim/job state, quota terminal state, policy denial, storage outcome, retry and DLQ signals are structured. Authorized `/api/v1/operations` returns bounded state counts for the selected workspace and an admin-only global DLQ count; it does not expose credentials or use tenant/user IDs as metric labels.

## Disposable recovery procedure v1

CI provisions two fresh PostgreSQL services with separate migration/runtime roles and synthetic data. `scripts/ci/backup-restore.sh` enforces fixed CI/localhost/database/role/source-port targets, creates a custom dump, refuses a nonempty restore target, restores without ownership/ACLs, reapplies runtime grants, validates migration checksums and compares canonical table manifests. Independent ledger checks reject unbalanced transactions. The dump is removed and never uploaded; manifests contain only table counts/digests.

Redis is discarded temporary cache/session-rate state (authoritative sessions remain in PostgreSQL). JetStream durable transport is recreated/reconnected; it is not a business datastore. Existing database outbox/inbox and job recovery generation records govern replay. The fresh-restore test verifies inbox dedup, abandoned lease reclaim, stale-worker denial, one terminal event and exactly-once quota settlement.

No production RPO/RTO, unattended production backup scheduling, sustained load, external storage/KMS availability or live Telegram behavior is claimed. Native evidence is recorded separately; code inspection alone never establishes a native gate PASS.
