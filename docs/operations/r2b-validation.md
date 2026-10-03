# TELMIVO R2B validation

R2B closes the demonstrated foundation gaps recorded in `docs/architecture/r2b-foundation.md`. R1/R2A are preserved; this is neither a bootstrap nor product completion. Only the reconciled checkout was used. R0 and unrelated R1/R2A validation were not repeated.

## Source and delivery

- Repository: `naseri-coder/TELMIVO`, PRIVATE; branch `feature/platform-bootstrap-v0.1`.
- Starting HEAD: `7effd35a6287e095ed98440e2ec4fd20e63f75f8`; starting tree: `2abcef33cd3e279ed35fb5476829e10f115df9f0`; clean continuity check passed.
- Source checkpoint: `49c0d1d854a7a44b7bd67da28c46ddffbdd20d82`.
- Corrected source checkpoint: `fc608417f3fc33adbb599d9d4730263c348c877c`.
- Validated source HEAD: `90a0d83f5853d527fde654c98783ae0879ac063c`; tree: `a5c6e73c9b47f4ed7dfa3eb3abf9dadb5b83547f` (commit4). Previous blocked delivery: `de5295df0a8ce2bbd23ca472172318821f2196be`.
- Final-source CI checkout/tree identities are recorded in machine evidence; ephemeral PR test refs are not merged history.
- Final delivery is the documentation-only child carrying this report. Its exact SHA and all five commit identities are recorded in Draft PR #1 and the final owner response, avoiding a self-referential commit hash inside its own contents.
- `main` and `develop` remain `024b3a3465285f9758ce109551a6574ced087dc7`; PR #1 remains OPEN/DRAFT/NOT MERGED. No force push, rebase, visibility change or production operation.
- Only new migration: `0009_long_job_foundation.sql`. Git blob/content comparison proves 0001–0008 unchanged. Exact 32-path inventory: `r2b-files.json` (21 added, 11 modified).

## IMPLEMENTED

PostgreSQL job leases extend existing media jobs/outbox with owner, generation, acquired/expiry/deadline, bounded heartbeat renewal, cancellation, idempotent terminal transition and abandoned-generation reconciliation. Two-worker ownership and stale terminal writes are fenced. A companion allocation preserves the original quota reservation identity/snapshot while recording approved expiry extensions; failure settlement follows the original count-failure policy. Calendar-boundary renewal fails explicitly, without allocating another reservation.

The existing StorageProvider is preserved. The local adapter provides scoped deterministic keys, Put/Open/Delete/metadata, sizes/content types, atomic writes and explicit expiry cleanup. The S3-compatible contract requires a private bucket. An injected, permission-protected HTTP read boundary rejects foreign workspace objects.

Provider-neutral credential keyrings retain context-bound AES-GCM and stored key IDs. Active-key writes, decrypt-only prior keys, fail-closed unknown IDs and idempotent audited installation re-encryption are implemented. No external KMS or live credential is used.

Disposable backup procedure v1 restores all authoritative tables into a second fresh PostgreSQL service, compares canonical counts/content digests and independently checks ledger balance, validates migrations and restores restricted grants. Database-derived recovery wakeups, restored inbox dedup and fenced job/quota effects remain idempotent. Redis is disposable temporary state; JetStream is durable transport, with database outbox/inbox/job state governing reconstruction and replay.

Existing structured correlation logging is retained. HTTP outcome, policy denial, job/lease/reclaim, quota terminal, storage, retry and DLQ signals are present. Protected operations inspection returns bounded state counts without user/workspace IDs as metric labels. Existing Admin policy editing, optimistic identity, reason/actor/version/correlation audit and the shared Preview/runtime resolver remain preserved.

## VALIDATED

VALIDATED on GitHub-hosted disposable PostgreSQL **17.11**, Redis **7.4.11**, NATS JetStream **2.11 series** (`nats:2.11-alpine`; patch not explicitly measured), with a separate migration role and NOSUPERUSER/NOBYPASSRLS/NOINHERIT runtime. No live Telegram, media or financial operation.

- 19 Go top-level unit/contract/security tests under race detection; gofmt/vet/build pass.
- 8 R2B native top-level tests (including fresh restore replay) + 5 affected retained tests: **13 top-level / 18 subtests pass**. No unrelated full R1/R2A matrix rerun.
- 4 web unit tests, 8 retained intercepted browser tests and **1 native browser test pass**; lint/typecheck/build pass.
- Security: CI source pattern scan 176 files and final staged evidence scan 179 files with zero matches, zero reachable/imported-package Go vulnerabilities, npm audit zero vulnerabilities. Inherited **GO-2026-5932** remains in an unimported/uncalled required module; the graph is not advisory-free.
- Fresh restore canonical manifests match for all authoritative public tables; ledger independently balances. Critical restored counts/digests and artifact identities are in machine evidence.

| Workflow | Run | Result |
| --- | --- | --- |
| CodeQL | [37114808210](https://github.com/naseri-coder/TELMIVO/actions/runs/37114808210) | SKIPPED_EXTERNAL |
| Security Baseline | [37114808232](https://github.com/naseri-coder/TELMIVO/actions/runs/37114808232) | SUCCESS |
| CI Web | [37114808213](https://github.com/naseri-coder/TELMIVO/actions/runs/37114808213) | SUCCESS |
| Build Images | [37114808258](https://github.com/naseri-coder/TELMIVO/actions/runs/37114808258) | SUCCESS |
| CI Go | [37114808374](https://github.com/naseri-coder/TELMIVO/actions/runs/37114808374) | SUCCESS |

Initial failures were ENVIRONMENT/HARNESS: external SDK overwrote synthetic init data; hosted pg_dump 16 could not dump server 17. Both corrections were targeted. The logged denied SET ROLE/schema_migrations operations are deliberate restricted-runtime negative probes in TestMigration, not a reason to weaken security. Run **37113096968** is explicitly invalid closure evidence: TestR2BRecovered failed at fixture discovery and tee hid its exit. Commit4 corrects privileged coordinate discovery only, keeps every behavioral assertion on the restricted runtime, audits both piped tests for pipefail, and adds explicit restored-role/snapshot/ledger/reservation/handler-replay checks. Replacement run **37114808374**, with TestR2BRecovered PASS and inspected failure-free test logs, is authoritative. Four source/correctness commits plus one evidence commit use the authorized one-time five-commit exception; no sixth commit.

## Gates

PASS below means the cited execution demonstrated the behavior. External controls remain separately blocked.

| Gate | Result | Evidence |
| --- | --- | --- |
| SOURCE_CONTINUITY_GATE | PASS | Clean reconciled checkout, Git/API identity and preserved ancestry plus one controlled recovery correction; frozen bases and old migration blobs verified. |
| FOUNDATION_GAP_LEDGER_GATE | PASS | R1/R2A reports and ADRs read; architecture ledger A implemented, B deferred, C preserved. |
| NATIVE_BROWSER_E2E_GATE | PASS | One Playwright native test: signed synthetic login, UI/API workspace and permission resolution, policy read/mutation, Preview, runtime and denied non-admin mutation; no API interception or external browser request. |
| WORKSPACE_TRIAL_GATE | PASS | TestR2BTrialScopes/PER_WORKSPACE: eight concurrent entries, deterministic subject, snapshot v1, expiry/fallback, cross-entry once behavior and RLS isolation. |
| INSTALLATION_TRIAL_GATE | PASS | TestR2BTrialScopes/PER_INSTALLATION: synthetic owned installation, deterministic subject, concurrency, version/expiry/once behavior and foreign ownership denial. |
| PLUGIN_CONTRACT_GATE | PASS | TestR2BPluginContracts: nine representative subtests plus four nested merge strategies; independent plan/trial/add-on/temporary entitlement, permission/capability/dependency denial, override/conflict and safety cap. |
| JOB_LEASE_GATE | PASS | TestR2BJobLease: two competing workers, one holder, renewal, expiry/reclaim and idempotent terminal transition; PostgreSQL bounded lease/deadline contract. |
| LEASE_FENCING_GATE | PASS | TestR2BJobLease and TestR2BRecovered reject stale renew/finish after reclaim; substituted reservation handle rejected. |
| QUOTA_LEASE_GATE | PASS | TestR2BJobLease preserves retry identity, extends beyond original TTL and charges once; failure/cancel release; TestR2BRecoveryPreparation records count-failure; TestR2BCalendarBoundary denies cross-window double allocation. |
| STORAGE_FOUNDATION_GATE | PASS | TestWorkspaceStorageLifecycle executes deterministic scoped Put/Open/metadata/Delete through cleanup, size/content type/expiry; TestR2BStorageAPI allows owner and denies foreign workspace. S3 is a private provider contract. |
| CREDENTIAL_ROTATION_GATE | PASS | TestRotation and TestR2BCredentialRotation demonstrate retained decryption, active writes, idempotent audited re-encryption and unknown/context rejection. |
| BACKUP_RESTORE_GATE | PASS | PostgreSQL 17 client/container dump restored into a fresh independent 17 service; identical canonical manifests, balanced ledger, migration validation and restricted runtime grants; TestR2BRecovered passes restored behavioral assertions. |
| RECOVERY_REPLAY_GATE | PASS | TestR2BRecovered: restored inbox dedup, committed outbox relay, one recovery wakeup/generation, reclaim/fencing, one terminal event and quota posting. TestJetStreamRecovery proves bounded malformed/permanent retry/DLQ. |
| WORKSPACE_ISOLATION_GATE | PASS | Native trial/installation/job/storage tests and restored DB tenant probe; FORCE RLS and restricted runtime checks in TestMigration. |
| ACCESS_PREVIEW_EQUIVALENCE_GATE | PASS | Native browser asserts exact runtime/Preview response equality and the rendered Admin Preview result. |
| OBSERVABILITY_GATE | PASS | Native/API logs show request outcomes and correlation IDs, job/lease/reclaim, quota, denial, storage and DLQ signals; protected operations counts are bounded. Existing headers/readiness redaction preserved. |
| SECURITY_REGRESSION_GATE | PASS | gofmt, vet, race tests, source pattern scan, govulncheck, npm audit and restricted role/origin/trust/credential tests pass; inherited unreachable advisory remains disclosed. |
| NATIVE_PERSISTENCE_GATE | PASS | Fresh migrations through 0009, native scoped transactions, concurrency, restore and replay executed on isolated PostgreSQL; Redis/JetStream are actual disposable services. |
| CI_GATE | PASS | All required final-source Go/native, Web, Security and Images workflow families success; actual test logs have no hidden FAIL/panic; both tee test pipelines guarded by pipefail. CodeQL externally skipped. |
| DOCUMENTATION_GATE | PASS | ADR-018, architecture gap/recovery contracts, validation report, machine evidence and Git-generated file inventory delivered. |

## DEFERRED

Real YouTube/Instagram/Facebook extraction, private/DRM/paywall bypass, full Downloader UX, Group moderation/anti-spam/anti-flood/captcha/raid/live Telegram authority, full Uploader/Factory/customer runtime, payment rails/Stars/crypto/refunds, broadcasts, AI, production deployment/secrets/database operations. No R3 work was begun.

## BLOCKED_EXTERNAL

CodeQL remains BLOCKED_EXTERNAL under the inherited repository/account scanning restriction; its prepared workflow is skipped. Branch Protection remains EXTERNAL_BLOCKER under the inherited integration restriction. Neither administration blocker was retried or changed.

## UNTESTED

No real Telegram/WebView, media provider, payment, production restore/RPO/RTO, sustained load, comprehensive penetration/accessibility test, external KMS or network S3/MinIO verification. Representative plugin classes are covered; every theoretical permutation is not claimed.

## KNOWN_LIMITATIONS

The S3 delivery is an adapter contract; only local storage is executed. The read endpoint requires dependency injection; default runtime has no configured storage provider. Cleanup requires an explicit scheduled caller. Lease renewal is bounded to five minutes per heartbeat and a 24-hour immutable job horizon. Reclaim requires an unexpired allocation; allocation/horizon exhaustion needs an explicit operator decision rather than silent reallocation. Crossing a calendar quota boundary is rejected; automatic migration into another quota window is deferred.

The first source checkpoint passed native contracts but failed browser login because the external Telegram SDK replaced the synthetic fixture. The correction strips that SDK only in native-test Vite mode; normal builds remain unchanged. No native API interception is used, and browser requests are asserted to stay on localhost. Three targeted correction checkpoints were required, including the authorized R2B-R1 exception. Existing concurrency handled duplicate push/PR event workflows; no workflow was manually triggered or cancelled during final resume. The final evidence commit changes documentation only and does not retrigger source workflows.

Stop at R2B; keep Draft PR #1. No merge, readiness transition, deployment or product implementation.
