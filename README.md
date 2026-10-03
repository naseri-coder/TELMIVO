# TELMIVO — تلمیو

سامانه یکپارچه کسب‌وکار در تلگرام · Persian-first / RTL-first

Private, proprietary foundation. Continue on `feature/platform-bootstrap-v0.1`; Draft PR #1 targets `develop`. No open-source license, automatic merge or production deployment.

TELMIVO is a Go modular monolith with independently runnable API, Main Bot, Group Bot, Downloader Bot, Customer Bot Runtime, Worker and Media Worker. PostgreSQL owns business state, Redis supplies temporary cache/rate limiting, NATS JetStream processes durable jobs, and the React/TypeScript/Vite Mini App defaults to Persian and RTL. ClickHouse is deferred.

The four foundations are identity/workspaces, configurable access/entitlements/plugins, integer double-entry service-credit ledger/billing interfaces, and transactional outbox/inbox jobs. Eight checksummed migrations include tenant FORCE RLS/composite foreign keys, separate platform permissions, append-only/sealed financial postings and queue leases. Global plan use checks have a narrow privileged boolean path; ordinary runtime connections cannot bypass RLS.

The current source supports synthetic identity/session registration, persisted ticket create/reply, FREE→PRO→FREE/expiry without resource deletion, synthetic USD/IRR adjustments, and controlled fixture downloads through JetStream. The Mini App has API-connected dashboard/ticket/wallet pages and stable operation retry keys. The policy management area provides versioned access/commercial editors and shared backend Preview. Other product routes explicitly show coming soon. Real integrations are not implied by interface presence.

## Development

Requirements: Docker Compose v2, Go from go.mod, Node 24/npm.

```
make bootstrap
make up
```

Open `http://localhost:5173`; API is at `http://localhost:8080`. Infrastructure has no host ports. Synthetic bot shells start with disabled webhooks; real tokens are unnecessary. Empty official-bot credentials intentionally leave Telegram login unavailable; configured Main/Group/Downloader signing credentials are accepted independently. Native CI signs controlled identities with a synthetic token and does not contact Telegram.

```
make build
make test
make lint
make security-check
```

Native tests additionally require explicit isolated `telmivo_test` URLs and Redis/JetStream services. If Docker is unavailable, GitHub Actions provisions disposable native infrastructure. See [development and recovery](docs/operations/development.md), [R1 results and all gates](docs/operations/r1-validation.md), and [historical source checkpoint](docs/operations/validation.md). CI definitions alone do not establish passing results.

## Boundaries and limitations

Read [domain/trust/tenant/financial boundaries](docs/architecture/boundaries.md), [limit semantics](docs/architecture/quotas.md), [ADRs](docs/adr) and [security policy](SECURITY.md). Workspace RBAC is distinct from platform administration. Grants and overrides resolve centrally; resource count limits preserve existing resources and restrict creation after downgrade. Capacity/item-size enforcement is explicitly unavailable until implemented.

Wallet balances derive from immutable integer ledger entries. IRR uses whole rials, USD cents, USDT on TON six-decimal atomic units, TON nanoton and Stars integer XTR. Internal service credit is separate from external rails. Payment/refund interfaces do not implement providers, verification posting or real financial operations. R1 uses only synthetic internal credits.

Main/Group/Downloader/customer bot live behavior, full Bot Factory/Uploader/Broadcast, real provider extraction, FFmpeg storage ingestion, S3, AI, live notification delivery and complete admin UI remain deferred. Storage/Telegram/AI/payment abstractions isolate vendors; actual adapters are not fabricated. Backup/restore drills and broader load/production security testing are untested.

© 2026 NASERI CODER. All TELMIVO source rights reserved; dependencies retain their respective licenses.

## Foundation Extension R2A

Official discovery sources share one identity through `identity.Enter`. Immutable policy versions and trial snapshots extend the existing plan/plugin catalogs. Durable quota reservations support calendar/rolling windows, capacities and concurrency. Resource grant merge strategies and separate read-only safety caps protect the Uploader reference configuration. Offers reference historical integer price versions; no payment or purchased add-on flow is enabled.

See [dynamic policy contracts](docs/architecture/dynamic-policies.md) and [R2A validation](docs/operations/r2a-validation.md). Admin controls live under the Persian **پلن‌ها و دسترسی‌ها** route. Configured owners receive the new granular permissions at their next verified login; other administrators require explicit grants.
