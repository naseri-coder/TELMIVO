# TELMIVO Foundation Validation R1

Verdict: **R1 core validation complete, with external administration/security-service blockers**. No R2 features, production operations or merge performed.

Repository `naseri-coder/TELMIVO` is PRIVATE. Branch remains `feature/platform-bootstrap-v0.1`; PR #1 remains open/Draft, targeting `develop`. Initial feature HEAD was exactly `493a016f71ad5ddf1d32f1110b759a5450713e9a`; `main` and `develop` were both `024b3a3465285f9758ce109551a6574ced087dc7`. The local recovery snapshot had a different commit history but exactly the same Git tree (`01d1acacd8eb94e802fdb0fd3f44af79f9b84fd1`); no unexpected source drift, reset, force push or history rewrite occurred.

Validated source/infrastructure commit: **`004bf65ff482ace9f8121f6f0b336d18beb8c1da`**. Documentation checkpoints add evidence and quality files; the final refresh changes evidence only. The delivery HEAD and full commit list are recorded in Draft PR #1 and the owner delivery response. [Machine-readable evidence](r1-evidence.json) and [complete changed-file inventory](r1-files.json) accompany this report.

## Implemented and validated

- Separate platform permission catalog/grants guard existing sensitive administration paths. A plan-only platform admin is denied wallet adjustment despite workspace OWNER rights. Global plan usage checks see other workspaces through a permission-checked SECURITY DEFINER boolean function; ordinary tenant queries retain FORCE RLS. Migration/runtime roles are distinct and runtime cannot inherit/set the privileged role.
- Stable operation keys survive uncertain HTTP responses and tab reload. Confirmed success permits a new key; different workspaces/payloads have separate operations. Asynchronous key generation captures the intended workspace. Mutation concurrency is guarded, workspace routes remount, and stale entitlement responses are ignored.
- Resource counts query actual persisted rows; monthly usage is a separate dimension. Capacity/item-size and unsupported reset policies fail closed. Downgrade preserves resources and denies over-limit creation while allowing reads. Plan change retries create only one event/audit record; temporary plan/grant expiry is enforced at access resolution.
- Session errors distinguish invalid sessions from infrastructure failures. Origin requires the configured scheme/host, secure headers are applied, request/correlation IDs enter structured logs, and raw transport/SQL/credential errors are not exposed/logged as user input. Credential encryption remains an AES-GCM context-bound interface; native tests persist ciphertext and deny another workspace access/decryption.
- Outbox claim ownership, expiry/retry availability, explicit durable binding, permanent inbox deduplication, malformed-event DLQ identifiers, bounded retries and graceful cancellation are implemented. Synthetic fixture state persists through worker-component restart, duplicate delivery and real broker restart.
- `.env.example`, Makefile, Dockerfiles/Compose, safe development guards, role provisioning and six workflows exist. Image builds include FFmpeg. Compose starts all eleven long-running services, API and synthetic bot health checks succeed, Mini App serves RTL HTML, and infrastructure ports remain private in the dev configuration.
- Sixteen concise ADRs, CODEOWNERS, three issue templates, PR template, SECURITY/CONTRIBUTING/CHANGELOG, development/recovery and quota documentation are present. Existing released migrations 0001–0003 remain unchanged; fixes use 0004–0006.

## Native infrastructure and tests

GitHub-hosted disposable Ubuntu runners supplied actual **PostgreSQL 17.11**, **Redis 7.4.11** and **NATS 2.11.17 with JetStream/file storage**. Migration uses `telmivo_migration` (NOSUPERUSER/BYPASSRLS); security/financial/service tests use `telmivo_runtime` (NOSUPERUSER/NOBYPASSRLS/NOINHERIT, not table owner). No superuser runtime or PGlite result is used as R1 evidence.

| Validation | Exact execution and result |
| --- | --- |
| Go | gofmt verification, `go vet ./...`, `go test -race ./internal/... ./tests/contract -count=1`, `go build ./cmd/...`: PASS. 11 contract tests and 2 HTTP security tests at top level |
| Native core | Fresh `go run ./cmd/migrate`; `go test -race -v -tags=integration ./tests/integration -count=1 -timeout=3m`: PASS. 13 top-level tests + 18 subtests; the separate broker case intentionally skips in this first invocation |
| Broker restart | `docker restart telmivo-test-nats`; `TEST_BROKER_RESTART=true go test -race -v -tags=integration ./tests/integration -run TestBrokerRestart -count=1 -timeout=30s`: PASS. Total 14 distinct native top-level tests |
| Financial concurrency | 16 concurrent identical +USD adjustments yield one balanced transaction/credit. Two competing debits allow exactly one and retain nonnegative balance. Native deferred unbalanced/overdraft rejection, sealed entry sets, append-only mutation rejection, cross-tenant denial, audit/outbox and idempotency all PASS |
| Five verticals | Validated synthetic identity→user/workspace/FREE/wallet/session/access; ticket create→persist→authorized reply/outbox; FREE→PRO→FREE plus expiry/downgrade; synthetic adjustment→ledger/balance/audit; fixture job→SQL outbox→JetStream→consumer/inbox→persisted completion: PASS |
| Redis | Ping, fixed-window rate primitive, temporary value read/expiry, rate expiry and closed-client failure denial: PASS |
| JetStream | Stream/durable creation, explicit ack, redelivery, reconnect/resubscription, duplicate Msg-ID and inbox suppression, transient retry, six-attempt DLQ, malformed event, cancellation and broker/storage restart: PASS |
| Web | `npm ci`, ESLint, TypeScript, Vitest and production Vite build: PASS; 4 unit tests including lost-response retry and intended-workspace capture |
| Browser | `npm run test-browser` with actual Chromium 153.0.8010.12: 6 PASS (3 cases each at 390×844 mobile and 1100×760 desktop). RTL/fa-IR, workspace switching, overflow, label associations, modal Tab/Shift-Tab/Escape/focus restoration, wallet atomic display/form are checked |
| Images/dev | Backend/frontend `docker build`, FFmpeg version, Compose config/start, readiness and 11 running services: PASS; images are built locally on CI, never published |
| Security | Source secret-pattern scan, govulncheck and npm audit: PASS under baseline policy. Zero callable Go vulnerabilities and zero npm advisories; one non-imported module advisory remains described below. Final staged source-pattern scan includes all 141 files, zero matches |

Browser tests use intercepted synthetic API fixtures, not a browser-to-native-API end-to-end environment or a live Telegram WebView. Native service tests cover persistence independently. Two screenshots (mobile ticket and desktop dashboard) were also visually inspected; six screenshots are retained in CI artifacts. No complete visual/accessibility audit is claimed.

## Inspected CI executions

All below use validated commit `004bf65ff482ace9f8121f6f0b336d18beb8c1da`; completed job steps/logs were inspected.

| Workflow | Run | Result |
| --- | --- | --- |
| CI Go | [37096551199](https://github.com/naseri-coder/TELMIVO/actions/runs/37096551199) | SUCCESS; includes reusable native/financial job |
| CI Web | [37096550886](https://github.com/naseri-coder/TELMIVO/actions/runs/37096550886) | SUCCESS |
| Native Infrastructure / migration-check | [37096550917](https://github.com/naseri-coder/TELMIVO/actions/runs/37096550917) | SUCCESS |
| Security Baseline | [37096550965](https://github.com/naseri-coder/TELMIVO/actions/runs/37096550965) | SUCCESS with non-reachable module advisory |
| Build Images + synthetic Compose | [37096550919](https://github.com/naseri-coder/TELMIVO/actions/runs/37096550919) | SUCCESS |
| CodeQL prepared job | [37096550875](https://github.com/naseri-coder/TELMIVO/actions/runs/37096550875) | SKIPPED; externally disabled capability, not PASS |

CI used Go 1.27.0 and Node 24.21.0; local and container Go is 1.27.1. Full relevant CI ran at the validated source checkpoint. GitHub also automatically started checks when repository templates were added; unchanged source suites were not manually restarted. Native integration/financial validation is additionally required directly by ci-go through the reusable migration-check workflow.

## Failures found and resolved

| Earlier failing command/test | Component/files | Established cause and resolution |
| --- | --- | --- |
| Native `TestWallet`, `TestFinancialConcurrency`, run 37094524657 | Historical `0001_core.sql` ledger function; corrective `0006_native_ledger_trigger.sql` | PostgreSQL SQLSTATE 42703: CASE expression references `NEW.transaction_id` on a transaction row that has no such field. Separate PL/pgSQL IF branches fix it; native balancing/concurrency subsequently pass |
| Native `TestDownloadQueue`, runs 37094524657/37094984034 | `tests/integration/native_test.go`, `internal/platform/queue/nats.go` | Timeout. Early test relayed one batch despite earlier outbox records; test now drains until effect and surfaces consumer errors. Restart path previously unconditionally added an existing consumer; it now validates and binds it. Exact earlier broker startup error was not captured; final restart/idempotency/recovery tests pass |
| Browser smoke, run 37094983987 | `components/ui.tsx`, `e2e/smoke.spec.ts` | Select labels included embedded option content; explicit htmlFor/control IDs fix association. Mobile test attempted to click the obscured menu toggle; it now closes through the visible scrim. Six final cases pass |
| Compose startup, run 37095415946 | `build-images.yml` | API ready, immediate frontend curl received reset during Nginx startup. Readiness retries resolve the timing race; all eleven services pass final smoke |

Valid source was preserved through each checkpoint. No failed migration was edited retrospectively or financial record deleted to make a test pass.

## Blocked, deferred and untested

**BLOCKED_EXTERNAL:** Branch protection remains the previously established HTTP 403 `Resource not accessible by integration`; no retry was made. CodeQL run [37094524662](https://github.com/naseri-coder/TELMIVO/actions/runs/37094524662) reported `Code scanning is not enabled for this repository` and `Resource not accessible by integration`. The prepared workflow requires owner-enabled capability plus `CODEQL_ENABLED`; no paid plan/security setting was changed and no uploaded CodeQL result is claimed.

**Known dependency advisory:** [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932) concerns unmaintained `golang.org/x/crypto/openpgp` in required module `golang.org/x/crypto@v0.57.0`. That package is neither imported nor called by TELMIVO; govulncheck reports zero reachable findings and no fixed release for this advisory. Keep OpenPGP out of future integrations; reassess dependency/feature changes. This is explicitly retained as a limitation rather than claiming an advisory-free dependency graph.

**DEFERRED/INCOMPLETE:** Live Main/Group/Downloader/customer bot behavior, real Telegram delivery, real YouTube/Instagram/Facebook extraction, actual FFmpeg/storage ingestion, S3, payment rails/refunds, full Bot Factory/Uploader/Broadcast/AI/admin UI, platform permission-management UI and unimplemented capacity/item-size/commercial credit policies. Existing process shells/interfaces/Coming Soon pages remain truthful.

**UNTESTED:** Live Telegram WebView/cookie behavior, live credentials/key rotation/KMS, production financial/provider operations, full browser-to-native API E2E, sustained load/large media/long-running handler heartbeats, abrupt OS-process kill during a heavy job, backup/restore/disaster recovery drills and comprehensive penetration/accessibility testing. Native fixture recovery proves R1 transaction/queue behavior; it does not prove these broader paths.

No known unresolved defect remains in the tested R1 vertical slices. External blockers do not alter their evidence. Future capability activation requires its own validation; R2 was not started.

## All R1 gates

PASS applies to the bounded R1 acceptance scope and evidence above, not full commercial product completeness.

| Gate | Verdict | Evidence |
| --- | --- | --- |
| SOURCE_IDENTITY_GATE | PASS | Private repo, expected initial refs/tree/clean source; ordinary fast-forward checkpoint pushes |
| KNOWN_CORRECTNESS_ISSUES_GATE | PASS | Global usage guard, granular admin permissions, stable keys, quota dimensions and hardening tests |
| DEV_INFRA_GATE | PASS | Compose/roles/env/Makefiles; eleven services started, synthetic health/readiness |
| GO_BUILD_GATE | PASS | CI Go, all cmd binaries and backend image |
| WEB_BUILD_GATE | PASS | Typecheck/Vite build and frontend image |
| POSTGRES_NATIVE_GATE | PASS | Actual PostgreSQL 17.11 with restricted runtime role |
| MIGRATION_GATE | PASS | Fresh apply, reapply, checksum drift rejection, role separation, 6 migrations |
| REDIS_NATIVE_GATE | PASS | Actual Redis ping/rate/cache/expiry/failure tests |
| NATS_JETSTREAM_GATE | PASS | Actual stream/durable/ack/redelivery/dedup/DLQ/restart |
| WORKSPACE_ISOLATION_GATE | PASS | Native read/update/FK/RLS/credential/financial matrix, narrow global boolean |
| PLATFORM_ADMIN_PERMISSION_GATE | PASS | Plan-only admin denied unrelated wallet mutation |
| IDENTITY_WORKSPACE_VERTICAL_GATE | PASS | Validated signed identity, registration replay, session/FREE/wallet/access |
| TICKETING_VERTICAL_GATE | PASS | Native create/support read/reply/idempotency/outbox, role and transition denial |
| PLAN_CHANGE_VERTICAL_GATE | PASS | Native upgrade/downgrade/expiry/event/audit, no deletion, count limit |
| WALLET_LEDGER_VERTICAL_GATE | PASS | Native synthetic posting/derived balance/immutable audit/outbox |
| FINANCIAL_CONCURRENCY_GATE | PASS | 16 duplicate adjustments and competing debit transactions, deferred invariants |
| DOWNLOAD_QUEUE_VERTICAL_GATE | PASS | Controlled fixture outbox→JetStream→inbox→persisted completion and cancellation |
| OUTBOX_INBOX_GATE | PASS | Lease protection/reclaim, retries, duplicate suppression, malformed/permanent DLQ |
| FRONTEND_IDEMPOTENCY_GATE | PASS | 4 unit tests including uncertain response retry and workspace capture |
| RTL_SMOKE_GATE | PASS | 6 actual browser cases, two viewports and visual screenshot inspection |
| CI_DEFINITION_GATE | PASS | Six valid workflows; no publish/deploy or production-secret requirement |
| CI_RUN_GATE | PASS | Five final relevant executions completed and inspected; CodeQL blocker separately recorded |
| SECURITY_BASELINE_GATE | PASS | Origin/error tests, native role/tenant tests, selected scanners; scope limits/advisory explicit |
| ADR_GATE | PASS | All 16 required ADRs document actual foundation decisions |
| DOCUMENTATION_GATE | PASS | README, security/contribution/quality files, operations/quotas/ADRs/evidence/limitations |
| BRANCH_PROTECTION_GATE | EXTERNAL_BLOCKER | Preserved HTTP 403 evidence; not retried |
