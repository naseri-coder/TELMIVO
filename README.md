# TELMIVO — تلمیو

سامانه یکپارچه کسب‌وکار در تلگرام · Persian-first / RTL-first

Private, proprietary platform. R3 release-readiness work is stacked on the validated R3D-8 checkpoint; earlier phase PRs remain separate. See [final R3 audit and validation boundaries](docs/operations/r3-final-release-readiness.md). No open-source license, automatic merge or production deployment.

TELMIVO is a Go modular monolith with independently runnable API, Main Bot, Group Bot, Downloader Bot, Worker and Media Worker. Customer-bot Telegram updates use the canonical authenticated API ingress; the historical standalone customer-bot runtime is a fail-closed compatibility stub. PostgreSQL owns business state, Redis supplies temporary cache/rate limiting, NATS JetStream processes durable jobs, and the React/TypeScript/Vite Mini App defaults to Persian and RTL. ClickHouse is deferred.

The four foundations are identity/workspaces, configurable access/entitlements/plugins, integer double-entry service-credit ledger/billing interfaces, and transactional outbox/inbox jobs. Twenty-four checksummed migrations (0001–0024) include tenant FORCE RLS/composite foreign keys, separate platform permissions, append-only/sealed financial postings, queue leases, Channel Manager, Uploader storage lifecycle and Commerce payment/refund records. Global plan use checks have a narrow privileged boolean path; ordinary runtime connections cannot bypass RLS.

The source includes Main/Group/Downloader flows, a channel manager with authority-checked draft/scheduled publishing, canonical customer-bot webhook lifecycle through the API, a trusted Plugin Registry/Runtime, commercial plans/prices/entitlements/quotas, Bot Factory, Access Gate, File Manager, CRM, Ticketing, Automation, AI Assistant, Broadcast, Forms, Commerce, customer Plugin Store and Super Admin controls. The Persian Mini App uses API-backed state and authorized lifecycle previews. Disable preserves records; pricing, entitlement, subscription, installation and activation remain separate. Configured live adapters exist, but isolated tests use synthetic transports; their presence does not establish production readiness.

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

Read [domain/trust/tenant/financial boundaries](docs/architecture/boundaries.md), [limit semantics](docs/architecture/quotas.md), [ADRs](docs/adr) and [security policy](SECURITY.md). Workspace RBAC is distinct from platform administration. Grants and overrides resolve centrally; resource count limits preserve existing resources and restrict creation after downgrade. Resource limits are domain-specific: media processing and upload admission have bounded size/concurrency checks; no universal capacity guarantee is implied.

Wallet balances derive from immutable integer ledger entries. IRR uses whole rials, USD cents, USDT on TON six-decimal atomic units, TON nanoton and Stars integer XTR. Internal service credit is separate from external rails. Customer Commerce now includes verified Telegram Stars/XTR checkout for digital goods plus a provider-neutral external payment boundary, immutable payment events, full-refund state and explicit reconciliation for ambiguous external outcomes. No external payment vendor is hard-coded or enabled by this source checkpoint; validation uses synthetic transports and creates no real charge.

External payment-provider settlement/merchant onboarding, arbitrary third-party executable plugins, public developer marketplace and production S3 provisioning remain deferred. R4 channel work is documented in [R4B Channel Manager](docs/R4B-channel-manager.md), Uploader/storage in [R4C Uploader & Storage Platform](docs/R4C-uploader-storage-platform.md), and checkout/payment boundaries in [R4D Commerce Checkout & Payments](docs/R4D-commerce-checkout-payments.md). Automation 2.0 scheduling boundaries are documented in [R4E Automation 2.0](docs/R4E-automation-2.md). AI Assistant 2.0 retrieval, ingestion and cost-control boundaries are documented in [R4F AI Assistant 2.0](docs/R4F-ai-assistant-2.md). **R4G v0.2.0 Final Hardening** and the exact release evidence boundary are documented in [R4G Final Hardening](docs/R4G-v0.2.0-final-hardening.md) and [v0.2.0 Release Readiness](docs/operations/v0.2.0-release-readiness.md). R5 / v0.3.0 begins with the workspace-level [R5A Apple Services Foundation](docs/R5A-apple-services-foundation.md): BETA governance, permissions, entitlement/quota, FORCE-RLS state, audited operations and bounded state machines only; no live Apple/IdP connector or credential is enabled. Production deployment packaging and the controlled deployment checklist are documented in [the v0.1.1 production-readiness runbook](docs/operations/production-v0.1.1.md); this does not authorize production access or deployment. Telegram, bounded media/FFmpeg and operator-configured AI adapters require separate real-environment authorization. Disposable backup/restore and cross-phase native acceptance are CI gates; production restore/capacity/security approval is still required before deployment. No production feature guarantee follows from a passing source checkpoint.

© 2026 NASERI CODER. All TELMIVO source rights reserved; dependencies retain their respective licenses.

## Foundation Extension R2A

Official discovery sources share one identity through `identity.Enter`. Immutable policy versions and trial snapshots extend the existing plan/plugin catalogs. Durable quota reservations support calendar/rolling windows, capacities and concurrency. Resource grant merge strategies and separate read-only safety caps protect the Uploader reference configuration. Offers reference historical integer price versions; no payment or purchased add-on flow is enabled.

See [dynamic policy contracts](docs/architecture/dynamic-policies.md) and [R2A validation](docs/operations/r2a-validation.md). Admin controls live under the Persian **پلن‌ها و دسترسی‌ها** route. Configured owners receive the new granular permissions at their next verified login; other administrators require explicit grants.
