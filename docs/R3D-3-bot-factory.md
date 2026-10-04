# R3D-3 Bot Factory

Create Bot → read-only selection preview → commercial/dependency review → atomic
bot creation + plugin installation → optional activation → per-bot dashboard.

Selection is not entitlement; entitlement is not activation; disable is not
uninstall. This customer layer composes R3D-1 and R3D-2 rather than introducing a
second bot model, commercial evaluator or lifecycle. It reuses `customer_bots`,
`plugin_installations`, existing workspace memberships and `bots.manage` for all
factory/lifecycle reads and writes. Customers cannot mutate commercial policy.

## API

All routes are authenticated `/api/v1/customer-bots`, scoped by X-Workspace-ID:

| Operation | Route |
| --- | --- |
| List bots / create | GET / POST root |
| Customer-safe catalog | GET `/catalog` or `/{bot}/plugins` |
| Preview selection | POST `/preview` or `/{bot}/preview` |
| Preview lifecycle consequences | POST `/{bot}/plugins/{plugin}/impact` |
| Lifecycle mutation | POST `/{bot}/plugins/{plugin}/actions` |
| Explicit controlled trial | POST `/{bot}/plugins/{plugin}/trial` |

Catalog projection returns name/description/version, settings, dependencies,
conflicts, lifecycle/revision, price unit/scale/atomic amounts, inclusion, trial
eligibility and current access. It excludes raw manifests, capabilities, migration
metadata, other bot agreements, credentials and retained plugin data. Prices use
R3D-2's exact denomination/scale; browser formatting uses BigInt, never floating
point or inferred currency conversion.

Preview `selection` contains `requested`, `configurations`, `activate`. It resolves
required closure deterministically; optional dependencies are not implied.
Results include requested/required IDs, dependency-first order, commercial cards,
structured issues and readiness. A digest binds the projected current policy and
selection. Preview never starts trials, grants entitlement or writes bot/plugin
state. Required additions must be visibly accepted; paid or unavailable dependency
blocks final creation. Unknown/conflicting selections return specific plugin IDs.

Creation requires name, token, idempotency_key, selection, exact accepted closure
and review_digest. Selection changes invalidate review. Server rechecks current
policy/permission/configuration/conflicts and consent before inserting anything.
Paid plugins without entitlement remain blocked: no purchase or grant is faked.
PER_BOT licenses on an existing bot cannot authorize a new bot. Customers may create
an empty bot, explicitly start an eligible trial, then install/activate; preview
never consumes a trial on their behalf.

## Atomicity and concurrency

R3D-1 `Change` retains behavior and delegates to a trusted Core `ChangeTx`; R3D-2
`Authorize` similarly delegates to `AuthorizeTx`. Neither exposes pgx through the
Plugin SDK. Factory composition uses one tenant transaction and a transaction-bound
AdmissionPolicy, so authorization can see the new uncommitted bot. The same bot
lock, revision checks, dependency/conflict validation, hooks and audit remain.
Any failed install/activation rolls back the bot and all installations.

Factory creation/mutation uses R3D-2's existing commercial admin advisory lock and
the account trial/usage lock, in that order. Preview uses shared variants. This
small critical section prevents policy/revocation races; no network calls occur
inside it. Exact operation replay returns the existing bot without resetting
plugin state; conflicting replay rejects. Migration 0014 adds only two nullable
private hash columns and a scoped unique credential-digest index to the existing
bot table. This missing replay/duplicate-token evidence justified the migration;
prior migrations are untouched. Existing legacy rows remain valid.

## Token handling and connection

No prior customer-bot online verification path exists. R3D-3 performs bounded local
token-format validation and stores the bot **DRAFT**, explicitly awaiting Telegram
verification/connection; it never invents VERIFIED/RUNNING or calls Telegram.
Users supply a bot token obtained from BotFather. Bot creation does not run a bot.
The existing R3D-1 runtime rejects execution for DRAFT bots.

The existing AES-GCM CredentialStore encrypts with workspace:bot associated data.
`CREDENTIAL_KEY` is a base64-encoded 32-byte key, key ID `primary-v1`; missing/invalid
key fails creation closed. No default production key exists. Tokens never enter
list/detail/audit responses, request formatting or logs. Browser tokens remain in
password field memory until success/navigation; operation storage retains only a
hash and operation key. Private SHA-256 token/request hashes provide replay and
duplicate detection without storing raw tokens; rotation preserves ciphertext
scope. There is no credential-read customer endpoint.

## Lifecycle and UX

Desktop/mobile RTL customer navigation now exposes bot list, creation review and
plugin dashboard. Actions use R3D-1 states and expected revisions. INSTALL previews
missing installed dependencies with their prices/access and asks users to install
them explicitly first. ACTIVATE requires active dependencies and current entitlement.
DISABLE is visibly separate from request/confirm uninstall. Impact preview shows
active dependents/conflicts before mutation; Core repeats validation at commit.
Uninstall is two-step, with an explicit retention warning. Current R3D-1 retention
keeps configuration/data even after uninstall; automatic reinstall is unsupported.

Access loss is a separate badge/problem, not a lifecycle rewrite. Installed ACTIVE
records may remain retained but fail runtime admission. Safe disable/uninstall
remain available after expiry. State changes on one bot never toggle another bot,
including with PER_ACCOUNT entitlement. No client-only lifecycle or hidden purchase.

## Registry and later stage

Trusted runtime registry is empty until real SDK plugins are deployed; commercial
prices alone do not register executable code. Production customers can create
empty bots and see a clear unpublished-plugin state. Metadata-only acceptance
fixtures are injected solely with TEST_BOT_FACTORY_FIXTURE in test + synthetic mode;
production/development reject that flag. Test entitlements are seeded only by
isolated integration fixtures, never a production HTTP path.

R3D-4 can supply real trusted manifests/handlers and Telegram verification/provisioning
through existing bot identity and runtime contracts. Future checkout consumes the
review/access-required seam and uses R3D-2 fulfillment; no payment provider, business
plugin, automatic Telegram provisioning or production deployment is implemented.
