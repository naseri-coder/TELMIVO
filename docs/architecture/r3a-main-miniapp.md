# R3A Main Bot and Mini App customer control center

R3A is a product adapter/read-model layer on frozen Foundation delivery `ab640fbdbafc76354d0f56fc71e4033fff9db711`. No database migration or generic infrastructure redesign is needed. Existing workspaces.name safely represents the single durable setting; locale/timezone are explicitly presentation-only.

## Entry and transport

Telegram private update -> secret-checked bounded decoder -> Main Bot application command -> existing identity.Service.Start / identity.Enter -> shared Platform User, default Workspace and immutable trial enrollment -> existing transactional outbox. Duplicate Telegram update keys return without duplicate customer state or outbox item. Supported customer menu commands choose the appropriate Mini App route; `/start` opens Dashboard. The menu always includes the eight customer entries and professional-panel CTA. The administration entry requires existing platform.access_policies.read authority, not workspace ownership.

The existing worker/JetStream inbox sends only the supported bot.menu event through the existing Telegram Messenger client when live transport is configured. Validation uses synthetic fixtures and never contacts Telegram. Delivery is at-least-once in the external ambiguous-send/crash window; no exactly-once Telegram claim. Other live notification/product types remain disabled.

Mini App entry independently uses the unchanged signed initData, freshness, replay, session and shared identity foundation. Production configuration still rejects synthetic mode. Native tests inject a synthetic signed payload into the frontend's Telegram SDK boundary and run the actual authentication API; no production bypass was added.

## Customer API and tenancy

Existing auth/me, auth/permissions, workspaces, wallets and runtime policy endpoints remain authoritative. Workspace lists are membership-scoped with stable ordering and capped at 100. A locally remembered workspace is accepted only if still in the authorized list; each subsequent request independently goes through database.Store.Tenant. Frontend selection never grants authority.

| Product read/mutation | Contract |
| --- | --- |
| GET /customer/overview | Typed scoped workspace name, effective plan/catalog name, real supported counts, one existing policy resolver call |
| GET /customer/access | Typed existing policy Decision; shared runtime resolver, no React policy implementation |
| GET /customer/catalog | Typed active catalog, stable key order, at most 100 plans; no purchase execution |
| GET /wallets | Existing authoritative security-invoker ledger balance view, atomic strings, wallet.read |
| GET /customer/wallet/history | wallet.read; keyset `(transaction.created_at, entry.id)` descending; default 20, maximum 50, workspace predicate and RLS |
| GET /customer/settings | Current workspace name and owner edit eligibility |
| PATCH /customer/settings | Current owner/member only, bounded non-control-character name, expected prior name, audit actor/old/new/correlation |

All paths are under /api/v1 with the existing session/origin/error/security/correlation controls. No SQL, secrets, ledger mutations or customer-computed authorization are exposed. No additional authoritative balances, identity store, plan-name branches, queues or resolvers were introduced.

## UI and scope

Persian fa-IR / RTL, bundled Vazirmatn, dark surfaces and restrained accents. The shell links Dashboard, Plans & Access, Wallet and Settings. Channels, Groups, My Bots and Support use truthful deferred states. Loading/error/retry/empty/unauthorized states are real. Focus styles and existing modal focus behavior remain; CSS respects reduced motion. Tables scroll inside their bounded container on mobile.

Dashboard uses only factual data; policy/trial/quota/offer/paywall output comes from the existing resolver. The initial selected quota presentation is downloads policy inspection, not a Downloader operation. Wallet formats integer strings with BigInt at supported asset scales and paginates history. Settings persists only workspace name; language and browser timezone are labeled as non-persistent server preferences.

Existing Foundation admin policies and test-only authorized adjustment/ticket regression components remain compatible. The customer Wallet has no write controls; Support does not advertise full ticketing. The admin-policy route is visible only under existing granular authority; backend endpoints independently deny ordinary users.

## Operational evidence

The restricted PostgreSQL role remains NOSUPERUSER / NOBYPASSRLS / NOINHERIT. Main Bot emits bounded outcome categories and correlation IDs without raw updates/identity/secrets. HTTP request outcomes retain Foundation structured logging. PostgreSQL business truth, trial snapshots, immutable ledger, quota reservations, leases/fencing and recovery are unchanged.

See [R3A validation](../operations/r3a-validation.md) and [machine evidence](../operations/r3a-evidence.json) for shared-entry fixtures, real browser -> frontend -> API -> PostgreSQL journey, security and affected Foundation regressions. No live Telegram, payment, S3, KMS or production operation was part of validation.
