# R3D-8 Plugin Store, Super Admin and readiness foundation

Authoritative remote parent: `31f46207587f799e65b990a18ca4d998f79234fb`.
Local equivalent base: `61189fa7d4ce2f099ed79e6319973078fe4ffda3`.
Base tree: `2e887a4f6cb99a99c16ad5e3e021b29cf0645898`.

## Ownership and separation

The trusted Registry supplies identity, versions, dependencies, conflicts and settings.
R3D-2 commercial catalog supplies integer atomic prices, inclusion, trial/grace,
licensing and quotas. R3D-1 installation rows supply lifecycle state. The Store
projects these existing authorities; it does not grant access, charge customers,
register executable code or maintain a separate commercial engine.

Per-bot customer Store list/detail APIs require authenticated membership, bots.manage,
and a scoped bot. Search/category/commercial-status filtering and stable ID pagination
are bounded to 25 items. Hidden plugins are omitted. Saved configuration is not copied
into the Store response. Detail includes effective entitlement sources, scoped
trial/agreement states and read-only quota checks. Installation/activation/disable
reuse the existing transactional Factory impact/action endpoints and server RBAC,
entitlement, dependency/conflict, revision and audit checks. Disable retains records.
No payment or purchase action exists; zero price never implies entitlement.

## Catalog operations

Optional `store.visible`, `store.category`, `store.summary`, `store.features`,
`store.icon` and `store.operational` fields reside in the existing versioned
Definition.Metadata map. Category/icon are bounded identifiers, not external URLs.
Operational state is OPERATIONAL, MAINTENANCE or UNAVAILABLE. Metadata validation
rejects unknown reserved fields, invalid states and oversized values. Defaults preserve
previous trusted plugins' visibility and availability.

Operational admission is independent of entitlement: maintenance blocks target install,
activation and capability execution and checks direct required dependencies. The existing
whole-active-set commercial validation still checks entitlement; maintenance on an unrelated
active plugin does not itself block another target's operational admission. Disabling
remains available through the established lifecycle, without removing historical data.

## Super Admin

The Persian commercial center reuses commercial catalog/history/issue/revoke APIs.
Only explicit platform.commercial.read/manage permissions authorize the corresponding
server operations; plan updates also require platform.plans.manage. Customer role hiding
is not authorization. Plugin prices/trial/grace/visibility/category/operational state have
form controls; plan inclusion has per-plugin selectors; quotas expose scope/reset/limit.
Bundle/add-on composition, denomination and advanced definitions use a validated JSON
editor. Nothing is hardcoded as a launch price.

All writes retain existing expected-version/revision checks and immutable policy history,
with a required reason and confirmation. Server time is used for immediate policy changes.
The effective-policy endpoint uses the same R3D-2 evaluator and read-only meter, exposing
sources, trial/grace contributions and scope without customer credentials. The commercial
account audit endpoint returns bounded actor/action/target/correlation/time evidence.
Catalog history returns the latest 100 versions in chronological order; older immutable
versions remain retained in storage. This is not a purge operation.

## Concrete hardening and existing controls

| Area | Evidence / outcome |
|---|---|
| Startup secrets | Invalid supplied AES credential keys rejected; production requires a 32-byte key, bounded-strength webhook secret and HTTPS Mini App URL without embedded credentials/query/fragment |
| Database | Pool DSN parsed before connection; default four connections, maximum 32, validated minimum; connect timeout at most 10 seconds, connection lifetime/idle bounds; runtime NOBYPASSRLS check retained |
| HTTP | Existing 5/15/15/60 second header/read/write/idle timeouts, 16 KiB headers, 64 KiB JSON limit retained |
| Readiness | Missing database/cache/queue objects now fail 503 before access; existing dependency connectivity checks retained |
| Requests | Existing random request/correlation IDs, sanitized errors, no-store, nosniff, CSP, HSTS and origin checks retained |
| Worker safety | Existing bounded workers, shutdown cancellation/wait, durable outbox reservations, uncertain-send no-replay and quotas retained |
| Security boundaries | Existing RLS/FORCE RLS, tenant and bot scope, trusted registry, credential store, media SSRF/path restrictions, support privacy and AI/Automation authorization retained |

No migrations changed or added: metadata fits the existing commercial schema, and the
validated 0001–0018 chain remains byte-identical. No manifests, executable plugin loaders,
production deployment files or payment integrations changed.

## Validation and release gates

Focused local Go tests/contracts/race, scoped vet, build, frontend lint/typecheck/tests/build
and YAML/branch-filter/migration/secret checks are the local gate. New integration tests
cover Store isolation, visibility, maintenance admission, disable retention, admin RBAC,
revisions, invalid money, policy history, plan/trial/quota and bundle/add-on updates.
Native browser journeys cover customer and Super Admin at 390 and 1280 pixels.

Database-backed integration/race and browser journeys against the real isolated API are
DEFERRED_TO_GITHUB_CI because this Work runtime has no isolated PostgreSQL/Docker.
Guards remain intact. migration-check adds TestR3D8 and store.native.spec.ts to its
existing disposable PostgreSQL/native acceptance flow. All four primary workflows have
R3D-8 push and R3D-7 PR-base filters. Exact-checkpoint CI success is required before
R3D-8 final PASS; this checkpoint does not claim production readiness or authorize deploy.

Release review must separately verify exact-head CI, migration checksums, image startup,
backup/restore, runtime least privilege, secret provisioning and operational capacity in
an authorized isolated environment before any later production approval.

## Limits and safety

Metadata remains first-party trusted data. No arbitrary code upload, SQL, shell, webhook,
remote plugin loader, provider checkout or customer self-grant is offered. Advanced
commercial definitions are intentionally an admin JSON editor; denomination/scope changes
and grants require review. Native browser and database outcomes are not claimed locally.
No production infrastructure/database, real customers, real Telegram sends, real payment
provider, deployment or merge is used. R3D-7 and PR #11 remain unchanged.
