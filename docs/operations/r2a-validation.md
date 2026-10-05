# TELMIVO Foundation Extension R2A

Scope: extend the validated R1 foundation with shared official discovery, dynamic commercial/trial access, and plugin resource governance. No new bootstrap, product implementation, production operation or merge.

Repository `naseri-coder/TELMIVO` remains PRIVATE; feature branch `feature/platform-bootstrap-v0.1` and existing Draft PR #1 → develop are retained. Initial remote delivery HEAD matched `1c2541b286467c670688eaf750a6f53e1e75aa4b`; main/develop matched `024b3a3465285f9758ce109551a6574ced087dc7`. The known local recovery history differs from remote, but its tree matched exactly (`e45265950baea6467d2563dee5a73cf80f879df2`). No unexpected source drift/reset/history rewrite occurred.

Validated source HEAD: `ac304274124505ff338a7d0ec9af004c5549e0c8`. Final documentation delivery HEAD is recorded in Draft PR #1 and the owner response. Source checkpoint commits:

- `8057f8eec7e5da16dd0e367a3d1d86926b1ac977` — shared versioned policy, onboarding, reservations, admin API/UI and targeted tests.
- `cf507f3766c9e16a4a9b8e944e57d4e7c0df02b3` — immutable reservation history, catalog revision audit, stronger direct-entry/group/API checks and precise browser selector.
- `7f367fc47f74edfb2806212ec69d2e83e982affa` — package IANA timezone data in backend images for dynamic policy calendars.
- `ac304274124505ff338a7d0ec9af004c5549e0c8` — deny heavy-operation admission without an allocatable durable quota policy; direct native and unit coverage.

Four source/correctness checkpoints plus one final documentation-only checkpoint preserve R2A. Changed inventory: **41 paths — 17 added, 24 modified**. No R1 source was regenerated.

[Machine evidence](r2a-evidence.json), [complete file inventory](r2a-files.json), [policy contracts](../architecture/dynamic-policies.md) and [ADR-017](../adr/ADR-017-dynamic-commercial-policy.md) accompany this report. R1 evidence remains in [its original report](r1-validation.md).

## IMPLEMENTED

- Freeze 1: `identity.Enter` reuses the unique Telegram/platform identity, default workspace and access state across MAIN_BOT/GROUP_BOT/DOWNLOADER_BOT/MINIAPP. Official configured bot signing credentials are accepted independently for Mini App login. `ClaimGroup` requires a verified authority adapter and workspace permission; a unique external group claim prevents silent transfer.
- Freeze 2: immutable typed policy versions, once-per-subject trial snapshot enrollments, configurable durations/eligibility/triggers/scopes/stacking/fallback, effective-dated prices/offers/paywalls, dynamic calendar/rolling quotas and durable operation reservations. Plan remains FREE/PRO/BUSINESS; trial is a grant source. PostgreSQL remains authoritative and no policy cache was introduced.
- Freeze 3: existing registry/status/dependencies/schema/permissions plus granular capability catalog, plan/resource/grant/add-on/override combinations, distinct storage/transfer metrics, concurrency/size/retention/processing dimensions and explicit ADD/MAX/MIN/REPLACE. Database-protected infrastructure caps apply after commercial resolution. Disabling a required safety metric fails closed. Heavy operations also fail closed with QUOTA_POLICY_UNCONFIGURED when a new/cloned plan lacks a durable allocation policy; item size checks alone cannot admit a job.
- Admin API has separate access-policy/commercial/offer/safety-read permissions. Version mutations audit actor/key/type/old-new revision/effective time/reason/correlation; existing plan/plugin/flag/override controls now capture catalog revisions and state summaries. Mini App `/plans` has Persian RTL management, labeled duration/quota controls, advanced typed JSON bundle editors, plan/plugin controls and backend Preview using the same runtime resolver.
- Synthetic downloads reserve before the job and settle with persisted fixture completion/cancellation. Synchronous ticket creation uses the shared quota lifecycle when configured; legacy counters read stored window metadata. No provider or ingestion implementation is fabricated.
- Add migrations **0007_dynamic_policy.sql** and **0008_catalog_policy_revisions.sql**. New tables: policy_versions, identity_entries, group_claims, trial_enrollments, plugin_capabilities, infrastructure_safety_caps, quota_usage and quota_reservations. Existing 0001–0006 remain byte-for-byte unchanged. R1 executables, ledger, adapters and infrastructure were preserved.

## VALIDATED

GitHub-hosted isolated PostgreSQL **17.11**, Redis **7.4.11** and NATS JetStream **2.11.17**. Migration uses the dedicated migration role; acceptance queries use NOSUPERUSER/NOBYPASSRLS/NOINHERIT runtime credentials. No PGlite or superuser-runtime result is used as evidence. Local Docker/native servers were absent; no privileged installation was attempted.

| Tests/checks | Evidence |
| --- | --- |
| Policy unit tests | 4 new top-level tests: merge strategies/overflow, timezone/calendar boundaries, invalid policy/money inputs and required allocation configuration |
| Native R2A | 7 new top-level tests, 13 subtests: shared identity, once trial, old/new snapshots, expiry fallback, group authority/ownership/settings and 7→30-day/disabled policy, 5/day→5/week→3/day, configured offers/paywall, 12 competing reservations limited to 3, retry/release, rolling/failure/expiry semantics, plugin resource/capability/70GB add-on/5GB safety, runtime=Preview, granular denial, immutable audit/price references, direct official signing and successful admin API/catalog revision writes and denial of an unmetered cloned-plan job |
| Affected native R1 regressions | 8 top-level tests, 11 subtests: migrations/checksums/role separation, identity/workspace, tenant isolation, ticketing, platform permissions/global plans, plan changes, durable download completion and cancellation |
| Combined native execution | 15 top-level + 24 subtests PASS; exact targeted command below |
| Go regression/build | gofmt verification, `go vet ./...`, `go test -race ./internal/... ./tests/contract -count=1`, `go build ./cmd/...`; 11 existing contracts + 2 security tests + 4 new policy tests (17 top-level tests total) |
| Web | npm ci/lint/typecheck, 4 retained operation unit tests and production build; 8 browser acceptance tests PASS (10.0s) |
| Browser | 6 retained RTL/accessibility core cases + 2 new policy edit/backend Preview cases across desktop/mobile; synthetic API interception, no browser-to-native or live Telegram E2E claim |
| Images/security | Backend/frontend image build, packaged readable IANA calendar archive, FFmpeg presence and synthetic eleven-service Compose readiness; secret-pattern scan, reachable Go vulnerability scan and npm audit. No images published |

```sh
go test -race -v -tags=integration ./tests/integration -run 'TestR2A|TestMigration|TestIdentityWorkspace|TestWorkspaceIsolation|TestTicketing|TestPlatformPermissionsAndGlobalPlans|TestPlanChange|TestDownloadQueue|TestDownloadCancel' -count=1 -timeout=3m
```

R1's unrelated financial concurrency, full broker-restart and Redis-specific matrices were not manually rerun. Their accepted evidence remains authoritative. Native CI now runs the focused R2A/affected selector through the existing reusable workflow, and branch concurrency cancels redundant push/PR executions. No manual rerun was requested.

## CI inspected

| Workflow | Run | Result |
| --- | --- | --- |
| CI Go (includes native R2A job) | [37107426384](https://github.com/naseri-coder/TELMIVO/actions/runs/37107426384) | SUCCESS |
| CI Web | [37107426293](https://github.com/naseri-coder/TELMIVO/actions/runs/37107426293) | SUCCESS |
| Security Baseline | [37107426300](https://github.com/naseri-coder/TELMIVO/actions/runs/37107426300) | SUCCESS |
| Build Images / synthetic Compose | [37107426392](https://github.com/naseri-coder/TELMIVO/actions/runs/37107426392) | SUCCESS |
| CodeQL prepared job | [37107426285](https://github.com/naseri-coder/TELMIVO/actions/runs/37107426285) | SKIPPED; inherited external constraint |

The earlier R2A browser run [37105703448](https://github.com/naseri-coder/TELMIVO/actions/runs/37105703448) had 6 passing/2 failing tests. The new test selected every role=status node while successful save and loading indicator coexisted; this was a strict locator ambiguity. The selector now targets the success message. Earlier native R2A execution already passed. No native/product reset or fake success was used to resolve this failure.

## DEFERRED / BLOCKED / UNTESTED

**DEFERRED:** real Downloader providers/extraction, Group moderation/authority adapter, full Uploader/Factory, object storage/FFmpeg ingestion, real payment/refund/Stars/crypto operations, broadcast delivery, AI, deployment, purchasing/activating paid add-ons, full admin workspace directory/permission editor, bulk policy migration and complete commercial storefront. Existing shells/Coming Soon routes remain explicit.

**BLOCKED:** Branch Protection remains EXTERNAL_BLOCKER (historical HTTP 403, Resource not accessible by integration). CodeQL remains BLOCKED_EXTERNAL/prepared/skipped (historical code scanning disabled/integration access restriction). Neither capability was retried or upgraded.

**Known limitations:** lightweight admin UI operates in the selected workspace; API clients may explicitly select another authorized request workspace. Advanced bundles use a JSON editor rather than a complete visual campaign/product builder. Policy optimistic version conflicts require refreshing the stored version after an unknown response; they cannot silently duplicate a version. Existing active enrollment snapshots are not rewritten. PER_INSTALLATION enrollment is guarded by existing customer-bot identity but no provisioning product is delivered. Native acceptance covers user/group trial flows; dedicated workspace/installation trial end-to-end flows and every capability/dependency permutation remain untested. Calendar reservations expire at the window boundary; successful late settlement is rejected. Future long-running adapters need timeout/lease/resource measurement design; retention/capacity reclamation and arbitrary rate-counter volume are not validated products.

The inherited non-imported/non-called OpenPGP advisory GO-2026-5932 remains recorded in R1. It is not an advisory-free dependency graph. **UNTESTED:** live Telegram/credentials/providers, real commercial purchases, native-connected browser E2E, sustained load/heavy-media/long-job lease renewal, backup/restore/DR and comprehensive accessibility/penetration testing. No complete product or production-readiness claim is made. No demonstrated R2A defect remains unresolved in the exercised acceptance scope; the listed limitations remain.

## R2A gates

PASS is limited to this foundation contract and the evidence above.

| Gate | Verdict | Evidence |
| --- | --- | --- |
| SOURCE_CONTINUITY_GATE | PASS | Expected private remote HEAD/base refs and exact local recovery tree; old migrations immutable |
| MULTI_ENTRY_IDENTITY_GATE | PASS | Downloader→Main→Mini discovery shares identity/default workspace; direct official Mini App signature |
| GLOBAL_TRIAL_GATE | PASS | Once enrollment across entry sources; expiry returns configured FREE allowance |
| POLICY_VERSIONING_GATE | PASS | Existing enrollment retains v1 snapshot; new user gets v2; price v1 retained |
| DYNAMIC_QUOTA_GATE | PASS | Stored 5/day→5/week→3/day and rolling policy changes without source changes |
| QUOTA_RESERVATION_GATE | PASS | 12 competing requests admit 3, stable retry/release, expiry, terminal guard and fail-closed missing allocation policy |
| PLUGIN_GOVERNANCE_GATE | PASS | Registry reused, capability permission checks and shared resource/grant resolution |
| PLUGIN_RESOURCE_LIMIT_GATE | PASS | Uploader file/storage/transfer/concurrency/processing/retention metrics, 70GB add-on |
| SAFETY_CAP_GATE | PASS | Commercial 10GB capped at 5GB; 6GB request denied; runtime cap mutation denied |
| COMMERCIAL_POLICY_GATE | PASS | Effective-dated typed price/offer/grant configuration and fixed historical price references |
| DYNAMIC_PAYWALL_GATE | PASS | Quota denial returns configured paywall and offer reference |
| GROUP_TRIAL_POLICY_GATE | PASS | Verified claim, active/expired group trial, 7→30 days and disabled policy, settings retained |
| ADMIN_POLICY_API_GATE | PASS | Native successful policy/plan API mutations and unrelated permission denial |
| ADMIN_POLICY_UI_GATE | PASS | Two new browser policy-edit/backend Preview cases plus six retained desktop/mobile checks |
| ACCESS_PREVIEW_GATE | PASS | Native runtime response equals backend admin Preview response |
| AUDIT_GATE | PASS | Immutable native audit, old/new catalog revision and policy version evidence |
| NATIVE_PERSISTENCE_GATE | PASS | Eight fresh migrations, actual restricted PostgreSQL role and persisted reservations/trials/policies |
| REGRESSION_GATE | PASS | Affected Go/contracts/native suites, web build/unit tests and all 8 browser checks |
| CI_GATE | PASS | Final source Go/native, Web, security and images/Compose runs completed; logs inspected |

PR #1 remains Draft/open; main/develop unchanged. Stop after R2A. No merge, deployment, visibility change, real Telegram/financial/media operation or R2B implementation is authorized/performed.
