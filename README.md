# TELMIVO — تلمیو

سامانه یکپارچه کسب‌وکار در تلگرام · Persian-first / RTL-first

**Private, proprietary repository. V0.1 preservation checkpoint — bootstrap incomplete; not production-ready.**

The owner selected the existing repository name `TELMIVO` instead of `telmivo-platform`. Implementation is on `feature/platform-bootstrap-v0.1`; `main` and `develop` remain at their initial source commit.

## Current foundation

- Go modular monolith with independently buildable API, main-bot, group-bot, downloader-bot, customer-bot-runtime, worker, media-worker and migration commands.
- Three PostgreSQL SQL migrations with identity/workspace, permission, entitlement, plugin, subscription, ledger, payment, ticket, broadcast and media foundations. Scoped tables have FORCE RLS and composite tenant foreign keys.
- Configurable entitlement resolver; administrator API for plans, feature flags, plugin access, quotas, workspace overrides, temporary grants and manual subscription changes.
- Integer IRR/USD internal-credit wallet adjustments, double-entry postings, audit and transactional outbox; real payments are disabled.
- Server-side Telegram initData verification, hashed opaque sessions, replay records, workspace authorization, Redis rate limiting and encrypted credential interface.
- JetStream outbox relay and durable consumers; synthetic notification capture and synthetic download orchestration.
- React / TypeScript / Vite Mini App with Persian localization, RTL layout, dashboard, ticket and wallet pages connected to API endpoints. Other navigable sections clearly report coming soon.

Buildability does not establish operational correctness. Group/downloader/customer bot processes, live notifications, media delivery and payment rails are incomplete. See the [exact checkpoint report](docs/operations/validation.md).

## Local source checks

Go toolchain used: `go1.27.1 linux/amd64`. Node used: `v24.19.0`.

```sh
go build ./cmd/...
go vet ./...
go test ./tests/contract -count=1
cd web/miniapp
npm ci
npm run build
npm run lint
```

Frontend automated tests have not been written. Integration, end-to-end, concurrency, broker recovery and financial database invariant tests are outstanding. There is no Docker Compose, Makefile or GitHub Actions setup at this checkpoint.

## Runtime prerequisites — not an installation guide

PostgreSQL is authoritative. Redis is temporary state/rate limiting; NATS JetStream handles asynchronous work. Migration and runtime database credentials must be distinct. Runtime processes reject superuser/BYPASSRLS roles. Necessary keys currently read by configuration include `TELMIVO_ENV`, `DATABASE_URL`, `REDIS_URL`, `NATS_URL`, `HTTP_ADDR`, `MINIAPP_URL`, `BOT_TOKEN`, `WEBHOOK_SECRET`, `ADMIN_TELEGRAM_IDS`, `SYNTHETIC_MODE`, `CREDENTIAL_KEY` and `MEDIA_CONCURRENCY`. No production credentials belong in Git.

`cmd/migrate` reads `MIGRATION_DATABASE_URL`. Migrations embed the SQL files and check checksums of previously applied migrations. A complete development environment and restricted-role provisioning are still pending; do not interpret this list as a validated deployment procedure.

## Safety and architecture

Read [domain, trust, tenant and financial boundaries](docs/architecture/boundaries.md). Internal credit and external rails are separate. IRR uses whole rials, USD uses cents, USDT on TON uses 6 decimal atomic units, TON uses nanoton, and Stars use integer XTR. Digital goods/services sold inside Telegram must use Stars; provider interfaces do not bypass platform constraints.

No merge, deployment, repository publication, real money operation or live broadcast was performed. No open-source license applies to TELMIVO source; dependency licenses remain their own. The npm application package is marked private and its generated ISC license declaration was removed.

© 2026 NASERI CODER
