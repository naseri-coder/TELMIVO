# R4A — Customer Bot Runtime Convergence

Base release: TELMIVO v0.1.1.

R4A removes the split-brain runtime model without introducing a second bot engine or a new
database schema. The existing `customer_bots.status` values remain authoritative.

## Canonical ingress

Customer Telegram updates enter only through:

`POST /api/v1/customer-bot-updates/{account}/{bot}`

The endpoint requires the per-bot derived Telegram secret header and also fails closed unless
the persisted bot state is `RUNNING`. Draft, verified-but-not-running and disabled bots
cannot execute customer plugin behavior.

## Lifecycle

The authenticated management surface exposes read-only runtime status plus explicit connect
and disconnect actions. Connect verifies the encrypted Telegram credential through
`getMe`, provisions the canonical webhook through `setWebhook`, records the Telegram
username and moves the bot to `RUNNING`. Disconnect calls `deleteWebhook` with
`drop_pending_updates=false` and only then persists `DISABLED`. Repeating disconnect on an
already disabled bot is idempotent and does not repeat the Telegram effect. Reconnect from
`DISABLED` is explicit and supported.

Disabling runtime does not uninstall plugins or delete plugin settings/history.

## Legacy process

`cmd/customer-bot-runtime` remains buildable only as a compatibility marker. Starting it
fails immediately with `CUSTOMER_BOT_RUNTIME_RETIRED_USE_API_INGRESS`, before configuration
or infrastructure connections are opened. Production Compose already excludes it.

## Security boundaries

Bot tokens remain encrypted and never enter API responses. Runtime status exposes only bot
ID, Telegram username, lifecycle state and the public ingress path; it never exposes the
derived webhook secret. Existing plugin execution checks still require a RUNNING bot before
external effects. The webhook route now enforces RUNNING independently before parsing and
dispatching the update.

No arbitrary plugin code, production deployment, real Telegram credential, payment provider,
production database or migration is part of R4A. Migration chain 0001–0018 remains unchanged.

## Validation

R4A adds integration coverage for DRAFT rejection, connect, canonical ingress admission,
disconnect, idempotent disconnect and explicit reconnect. The contract gate verifies the
legacy process fails closed. Existing native customer journeys, browser acceptance,
backup/restore and security gates remain mandatory.
