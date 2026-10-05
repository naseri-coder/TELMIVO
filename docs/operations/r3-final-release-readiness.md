# R3 final release-readiness audit

Authoritative remote base: `c535bb149379a80f7eaeb02b9dcc6d5291293df2`.
Local start: clean `c68a39adccb5970e53479849470744a935fbb18f`.
Both point to tree `55fd637b2bf9bab77ad1176cf23adf207b7f40da`.
Delivery branch: `feature/r3-final-release-readiness-v0.1`.
This is a source release candidate, not deployment approval or final R3 PASS.

## Stack evidence

Connected GitHub compare results show every current PR head is ahead of its exact base,
with zero behind commits and a merge-base equal to that base. No PR was merged,
rebased or flattened. R3D-8 exact-head pull-request runs were independently checked:
CI Go, CI Web, Security Baseline and Build Images all concluded success.
Cancelled push duplicates do not contradict those successful exact-head PR runs.

| PR | Phase branch | Required base | Added commits |
|---|---|---|---|
| #1 | feature/platform-bootstrap-v0.1 | `024b3a3465285f9758ce109551a6574ced087dc7` | 20 |
| #2 | feature/r3a-main-miniapp-v0.1 | `ab640fbdbafc76354d0f56fc71e4033fff9db711` | 5 |
| #3 | feature/r3b-group-bot-v0.1 | `05fc043c53a7a4361b1b99b092c0d253fdf2e881` | 6 |
| #4 | feature/r3c-downloader-bot-v0.1 | `14b27792936c6d004e7e127f3d5de8f1be91ae79` | 9 |
| #5 | feature/r3d-1-plugin-platform-v0.1 | `23c38551649eba63d14804c6bee061e329306317` | 3 |
| #6 | feature/r3d-2-monetization-core-v0.1 | `183b35ab6e37c1b70c11abb04f20a86bcf873eb4` | 4 |
| #7 | feature/r3d-3-bot-factory-v0.1 | `498ea904d137eaf8e26715a37cd99e8c49949787` | 5 |
| #8 | feature/r3d-4-access-file-manager-v0.1 | `2dd3070be902ab10dd0942ba7b0e9b60854b9164` | 2 |
| #9 | feature/r3d-5-ticketing-crm-v0.1 | `702e5f4e129f4f5808aa40a229e98f4ef857b2ed` | 3 |
| #10 | feature/r3d-6-automation-ai-v0.1 | `5dc965ba791874747e5e805b909773787a69e22e` | 5 |
| #11 | feature/r3d-7-commerce-broadcast-forms-v0.1 | `bf3b3918432b7ad64b74e6b0fb126397d6f9d8f7` | 2 |
| #12 | feature/r3d-8-plugin-store-admin-hardening-v0.1 | `31f46207587f799e65b990a18ca4d998f79234fb` | 1 |

The bootstrap contains R1/R2 foundations. Earlier phase manifests, migrations and
domain modules remain present. Historical disabled bot-module descriptors under
`botmodules` are discovery placeholders, not an executable plugin loader or a
second authority for R3D plugin installation.

## Architectural and security review

PASS below means source audit plus available checks, with database/native acceptance
still a mandatory exact-checkpoint CI gate. It is not a claim that deferred tests ran locally.

| Area | Evidence and result |
|---|---|
| Registry/runtime | `pluginplatform.Registry` validates identity, core/dependency versions, cycles and active conflicts; SDK has no database/credential handles. `contentplugins.Registry` composes the nine first-party R3D manifests. PASS |
| Commercial separation | `commerce.Evaluate`, scoped agreements and versioned catalog own plugin licensing; `subscriptions` owns account plan assignment; installation rows own lifecycle; `QuotaTx` owns plugin usage. Store projects these authorities. PASS |
| Existing feature policies | `entitlements.ResolveForUser` delegates to `policies.Resolve` for foundation/Main/Group/Downloader feature policies. These feature/resource policies are distinct from R3D plugin commercial agreements, not an alternative plugin entitlement engine. PASS |
| Lifecycle | Disable changes installation state/revision, preserves configuration/data, and checks active dependents/conflicts. Uninstall is a separate retained lifecycle; no destructive cleanup was introduced. PASS |
| Licensing/trials/bundles | PER_ACCOUNT/PER_BOT scope is checked by the canonical evaluator; exclusive expiry/grace boundaries, immutable agreement terms, consumed trial snapshots and quota effects remain. R3D-2 integration/contracts cover these. PASS |
| Money | Ledger/shop/commercial domain values use checked int64 atomic amounts and string JSON serialization; frontend uses BigInt or decimal strings. Orders snapshot historical product revision/price and reject overflow/mixed denominations. No installation or price grants paid access; payment adapters are interfaces only. PASS |
| Store/admin | Customer Store requires membership, bots.manage and scoped bot; saved configuration is omitted. Commercial read/manage and plan.manage are explicit server permissions; expected version/revision, reason and durable audit govern mutations. PASS |
| Access Gate/File Manager | Protected delivery requires active/entitled gate and current policy; optional gate is not forced for public files. Scoped metadata, opaque references and reserved delivery protect boundaries. PASS |
| CRM/Ticketing/business | CRM identities, tickets, recipients, submissions and orders use workspace/bot filters and composite foreign keys. Ticketing/Broadcast/Forms/Commerce declare CRM dependency and revalidate execution. Forms snapshots/rechecks revision before atomic CRM/submission changes; orders accept product IDs/quantities only. PASS |
| Automation/AI | Allowlisted events/actions and bounded lineage; target authorization rechecked. AI tool arguments cannot select arbitrary tenant/bot; write tools need operator permission/confirmation and are denied for customer execution. Ticket context excludes INTERNAL messages; memories are scoped to workspace/bot/thread. PASS |
| Isolation | DB.Tenant checks membership before transaction scope; runtime rejects superuser/BYPASSRLS. Tenant tables retain FORCE RLS; composite references and explicit bot predicates add isolation. Global catalogs/queue dedup are trusted system metadata, not customer data endpoints. PASS |
| Authentication/HTTP | Telegram HMAC/age/duplicate-field checks, hashed sessions, HttpOnly cookies, origin enforcement, 64 KiB strict JSON and bounded rate limiting; sanitized error codes and generated request IDs. Existing security headers retained. PASS |
| Credentials/files/network | AES-GCM associated scope and external key; no browser secret responses. Root-scoped local storage, bounded upload/media/FFmpeg paths. Media and AI transports enforce approved HTTPS/public addresses, pinned dialing/no redirects; arbitrary shell/code/SQL/plugin packages are not exposed. PASS |
| Events/recovery | Atomic outbox and business writes, stable event ID, inbox dedup, fenced relay ownership and bounded retry/DLQ. Business sends reserve before effects and stop replay on uncertain outcome. Legacy live notifications now seal a pre-transport attempt fence. PASS |
| Operations | Startup validates environment/key/required production HTTPS+webhook configuration; pools/connect timeout bounded; API dependency readiness and HTTP timeouts retained; workers cancel/wait before resources close. No production configuration changed. PASS |
| Bootstrap | docker-compose.yml provides PostgreSQL roles, migrations then grants, Redis, JetStream, API/bot/worker/media processes and Mini App; only loopback UI/API ports exposed. Make guards restrict disposable dev configuration. Real login requires separately provisioned signing credentials; empty defaults intentionally disable it. PASS |

## First-party registry

| Plugin identity | Required dependency | Optional dependency | Canonical primary quota |
|---|---|---|---|
| access-gate | — | — | membership_checks.monthly |
| file-manager | — | access-gate | downloads.monthly |
| crm | — | — | crm.contacts.count |
| ticketing | crm | — | ticketing.messages.monthly |
| automation | — | — | automation.executions.monthly |
| ai-assistant | — | — | ai.requests.monthly |
| broadcast | crm | — | broadcast.messages.monthly |
| forms | crm | access-gate | forms.submissions.monthly |
| commerce | crm | — | commerce.orders.monthly |

Each manifest includes stable version/core compatibility, settings, known capabilities,
data version, pricing identity and quota extension. No first-party conflicts are invented;
registry tests cover conflicting alternatives. Commercial migrations register capabilities
and quotas with runtime/admin configurable policy; default zero prices are not entitlement.
New native continuity coverage cross-checks every registered manifest against commercial
capabilities and primary quotas and asserts Store responses omit private configuration.

## Migration audit

Exactly 18 SQL files have contiguous unique prefixes 0001–0018. Their current bytes match
the authoritative R3D-8 tree. Each previously validated phase's SQL was compared byte-for-byte
with the next phase: all earlier migration checksums are preserved. Migrate sorts names,
uses an advisory lock, checks SHA256 and applies SQL plus checksum transactionally.

Reviewed schema relationships: scoped customer bots/installations; immutable catalog and
agreement terms/trials/usage; content reference/policy FKs; CRM identity uniqueness and
ticket/conversation privacy; workflow event dedup and AI thread/request/delivery identities;
campaign recipient uniqueness/rate index; form snapshots; historical order revisions.
Tenant FORCE RLS and table-specific retention/revision triggers remain intact.
No concrete schema defect was identified. MIGRATION = NONE; no 0019 created.
Fresh SQL execution, reapply/checksum rejection and PostgreSQL constraint/RLS behavior
remain mandatory in disposable CI; they were not executed against any local/production DB.

## Concrete corrections

1. Legacy live notification replay: send happened inside an uncommitted inbox transaction.
   A successful or ambiguous external effect followed by commit failure could repeat.
   `ConsumeExternal` now seals inbox plus scrubbed `EXTERNAL_OUTCOME_UNCONFIRMED` evidence
   before transport, releases the connection, then opens the result transaction.
   Confirmation removes only provisional uncertainty evidence atomically with domain results.
   Failure/interruption leaves the fence and evidence; redelivery never invokes transport again.
   Pure database and synthetic handlers keep atomic rollback/retry semantics.
   Native regression covers result rollback after a simulated effect, handler failure,
   concurrent duplicate claims, confirmed evidence cleanup and one-connection pools.
2. Telegram false acknowledgements: Send/Copy/Forward accepted missing/zero/negative message
   IDs; malformed results and HTTP 5xx were not uniformly classified as unknown.
   Positive message IDs are now required and ambiguous responses return a scrubbed unknown
   error. Fake-transport unit/race tests cover all three methods; no Telegram traffic occurs.
3. Store quota omission: plugin-ID prefixes omitted metrics named differently from their
   plugins (Access Gate, File Manager and AI Assistant). Detail now also projects the declared
   manifest quota, using the same read-only canonical meter. Native continuity regression
   covers all first-party manifests.
4. Stale documentation: README still directed development to bootstrap, counted eight
   migrations and described implemented R3 features as deferred. README/boundaries/development
   now reflect current scope, live adapter boundaries and external-attempt recovery semantics.

Four primary workflow branch filters add final push and R3D-8 PR base without changing jobs.
The native reusable workflow adds `TestR3Final` to its existing guarded race selection.
CodeQL policy and historical migration files are unchanged.

## Final local gate and mandatory CI

- PASS: full Go vet; all internal unit/race and contract tests; Go build.
- PASS: frontend lint/typecheck, all five Vitest files (13 tests), frontend build.
- PASS: affected source formatting, YAML/filter semantics, contiguous migration/checksum
  preservation, exact diff/whitespace and supplementary source secret-pattern scan.
- PASS: integration tests compile with integration and race flags.
- DEFERRED_TO_GITHUB_CI: disposable PostgreSQL-backed integration/race/migration execution,
  Redis/JetStream recovery and backup/restore, Compose image/bootstrap smoke and browser acceptance.
- Database guard refused execution without explicit isolated telmivo_test URLs.
  No Docker/PostgreSQL capability was available locally.
- Browser smoke attempted but Chromium executable was absent; no browser assertion ran.
  Native browser acceptance therefore also remains CI-only. No guards/assertions were weakened.

All release-blocking source corrections require exact new checkpoint CI success.
No final R3 PASS is claimed while those gates are pending.

## Install path and remaining boundaries

For an authorized disposable development environment, clone this candidate branch,
enter TELMIVO, run `make bootstrap`, inspect the synthetic .env and run `make up`.
Compose applies migrations/grants before starting dependents; check API /health/live and
/health/ready, then open the loopback Mini App. Backend/frontend local builds use
`make build`; native validation uses the explicit isolated URLs shown in CI.
Do not point dev guards at production, substitute a production database, erase initialized
volumes to repair grants, or assume an empty signing token enables Telegram login.

External fencing deliberately prefers at-most-once attempts over unsafe replay; a crash
before transport can leave a delivery unattempted. Reconciliation needs separate review;
do not clear inbox fences automatically. Synthetic-mode acceptance does not prove real
provider availability, production networking, KMS/rotation, backup restore capacity or load.
Payments, public executable marketplace and production deployment remain out of scope.

No production infrastructure/database/customer data, real Telegram/payment side effects,
DNS/Cloudflare changes, deployment, merge, release tag or GitHub Release were performed.
