# R5A — Apple Services Foundation

R5A starts TELMIVO R5 / v0.3.0 from the final R4 source checkpoint. It creates only the
workspace-level foundation required by later Apple Services stages. No live Apple, Google,
Microsoft or generic IdP connector is implemented in R5A.

## Product boundary

Apple Services is a workspace-level product plugin, not a customer-bot plugin. R5A registers
the feature and plugin catalog entry in BETA state, but no subscription plan or plugin plan
entitlement is enabled automatically. Commercial packaging remains R5G work.

The frozen future capabilities are:

- `apple.consumer.setup`
- `apple.verify`
- `apple.business.connect`
- `apple.provision`
- `apple.users`
- `apple.reconcile`

R5A defines those contracts only. R5B–R5F provide the corresponding product behavior.

## Data model

Migration 0024 adds:

- `apple_service_profiles`: one workspace foundation profile;
- `apple_service_operations`: durable idempotent workflow records;
- `apple_service_operation_events`: append-only transition history.

All three tables use FORCE RLS. Operation identity, subject reference, idempotency key,
fingerprint, creator and expiry are immutable after creation. Every transition increments a
revision fence and is also written to the normal TELMIVO audit log.

R5A deliberately contains no columns for Apple passwords, one-time codes, access tokens,
client secrets or provider credentials. Later connector stages must use the existing encrypted
credential boundary rather than extending these tables with secret material.

## Entitlement and quota

The `apple_services` feature is registered as BETA. OWNER and ADMIN receive the Apple
permissions; other workspace roles receive none in R5A. All existing plans have the feature
and plugin disabled by default. Explicit workspace grants can enable the foundation during
development and controlled rollout.

The initial quota is the existing feature usage counter for Apple Services operations. R5A
does not claim that any TELMIVO package size is an Apple limit.

## State machines

R5A defines separate bounded transition graphs for consumer assistance, Apple identity
verification, Apple Business connection, directory provisioning, managed-account lifecycle and
reconciliation. The database validates which states belong to each operation kind; trusted Core
code validates each transition edge.

The consumer foundation follows:
`REQUESTED → PRECHECK → APPLE_ACTION_REQUIRED → APPLE_COMPLETED → VERIFICATION_PENDING → READY`
with explicit failed/cancelled/expired exits.

Managed provisioning follows the frozen R5 lifecycle:
`REQUESTED → IDP_CREATING → IDP_CREATED → APPLE_SYNC_PENDING → ACTIVE`.
Explicit `SYNC_ERROR`, `RETRY_PENDING`, `RECONCILE_REQUIRED` and `SUSPENDED` paths cover
recoverable and administrative exceptions. No state change in R5A performs an external effect.

## API

R5A exposes server-side foundation endpoints for profile state, operation creation/listing,
transition actions and event history under `/api/v1/apple-services`. These endpoints are
authorization, entitlement, quota, RLS and audit boundaries for future R5 stages. No Mini App UX
is introduced here; that remains part of R5G.

## Validation

Native validation must prove permission defaults, explicit entitlement, quota accounting,
idempotent replay, invalid-transition rejection, FORCE-RLS isolation, audit/event history,
absence of secret-bearing schema columns and PostgreSQL backup/restore.

R5A does not authorize production deployment or any connection to a real Apple organization.
