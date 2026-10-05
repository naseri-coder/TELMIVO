# R4C — Uploader & Storage Platform

R4C completes the bounded Uploader lifecycle on the existing TELMIVO storage foundation.

## Scope

- durable upload workflows with type, size, approval and retention policy;
- explicit lifecycle: RECEIVED, VALIDATING, PENDING_APPROVAL, PROCESSING, STORED,
  REJECTED, FAILED and EXPIRED;
- quota reservation/release and retention cleanup;
- immutable upload identity and revision-fenced state changes;
- File Manager handoff compatibility;
- local private storage for development and a provider-neutral S3-compatible boundary for
  authorized environments.

Uploader data stays workspace scoped. Stored object identity is never accepted from an
arbitrary browser path. Production object-storage credentials remain server-side.

R4C validation covers lifecycle/replay, approval/rejection, quota limits, cross-workspace
denial, retention cleanup, File Manager handoff, native browser acceptance and PostgreSQL
backup/restore of relational upload state. Object bytes are intentionally outside the
PostgreSQL restore drill and require the separately authorized object-store recovery plan.

R4C does not authorize production storage provisioning or deployment.
