# TELMIVO preserved full-source remediation

Repository: naseri-coder/TELMIVO. Audited baseline: `4599f431a97f212667be022b2ca1afa258a1a2ae`, R5B consumer-assistance branch. Preserved Wave 1: `644ed9ef8e44f594d61d3f4c0019c547565f864e`. No baseline reset, source recreation or replacement branch was used. PR 31 remains the authoritative stacked R5B PR.

The original 19 confirmed findings have source repairs; four additional source defects were confirmed during continuation. No original confirmed finding was dismissed as a false positive. This is SOURCE_REMEDIATION_COMPLETE with runtime acceptance pending, not release approval. The machine-readable ledger retains all eight verification questions and records the implemented repairs.

## Defect ledger

| Finding | Severity | Source repair | Validation boundary |
|---|---|---|---|
| TEL-R5-AUDIT-001 | HIGH | Typed quota/permission/state/dependency denial becomes durable SKIPPED; unexpected infrastructure errors propagate. Poll regression now makes a second exhausted and healthy occurrence due before worker start; runtime test COMPILED_NOT_EXECUTED. | Unit/static gates executed; PostgreSQL/browser/recovery runtime pending where relevant |
| TEL-R5-AUDIT-002 | HIGH | Explicit trustworthy reconciliation evidence matrix preserves UNKNOWN and active fence. | Unit/static gates executed; PostgreSQL/browser/recovery runtime pending where relevant |
| TEL-R5-AUDIT-003 | HIGH | Preserved native selector admits all 14 intended specs. | Unit/static gates executed; PostgreSQL/browser/recovery runtime pending where relevant |
| TEL-R5-AUDIT-004 | HIGH | Historical 23/25 checkpoint contracts retained; current source requires 26 after forward receipt migration. | Unit/static gates executed; PostgreSQL/browser/recovery runtime pending where relevant |
| TEL-R5-AUDIT-005 | MEDIUM | Canonical verified initData replay digest. | Unit/static gates executed; PostgreSQL/browser/recovery runtime pending where relevant |
| TEL-R5-AUDIT-006 | MEDIUM | Fresh intent flag and bounded provider lease prevent live replay interruption or duplicate effect. | Unit/static gates executed; PostgreSQL/browser/recovery runtime pending where relevant |
| TEL-R5-AUDIT-007 | MEDIUM | Order/payment cancellation serialization; late verified cancelled-order charge retains refund-required evidence without fulfillment event. | Unit/static gates executed; PostgreSQL/browser/recovery runtime pending where relevant |
| TEL-R5-AUDIT-008 | MEDIUM | Feature lock serializes channel count admission and insertion. | Unit/static gates executed; PostgreSQL/browser/recovery runtime pending where relevant |
| TEL-R5-AUDIT-009 | MEDIUM | Current uploader admission rechecked for replay, write and approval. | Unit/static gates executed; PostgreSQL/browser/recovery runtime pending where relevant |
| TEL-R5-AUDIT-010 | MEDIUM | Durable manual deletion intent and bounded sweeper recovery; physical capacity retained until deletion. | Unit/static gates executed; PostgreSQL/browser/recovery runtime pending where relevant |
| TEL-R5-AUDIT-011 | MEDIUM | Signed links cannot exceed retention; expired/sub-second links denied. | Unit/static gates executed; PostgreSQL/browser/recovery runtime pending where relevant |
| TEL-R5-AUDIT-012 | MEDIUM | Immutable per-request ingestion receipts, source/name conflict checks, cross-path key collision rejection and fail-closed legacy backfill. | Unit/static gates executed; PostgreSQL/browser/recovery runtime pending where relevant |
| TEL-R5-AUDIT-013 | MEDIUM | Safe classified AI HTTP errors retain durable UNKNOWN identity. | Unit/static gates executed; PostgreSQL/browser/recovery runtime pending where relevant |
| TEL-R5-AUDIT-014 | MEDIUM | Bounded binary-only proxy limit, ordinary JSON limit preserved. | Unit/static gates executed; PostgreSQL/browser/recovery runtime pending where relevant |
| TEL-R5-AUDIT-015 | MEDIUM | Trusted proxy peer configuration and spoof-resistant client rate-limit identity. | Unit/static gates executed; PostgreSQL/browser/recovery runtime pending where relevant |
| TEL-R5-AUDIT-016 | MEDIUM | Workspace lock precedes schedule row lock. | Unit/static gates executed; PostgreSQL/browser/recovery runtime pending where relevant |
| TEL-R5-AUDIT-017 | MEDIUM | Recovered service replay and exact consumer identity assertions added. | Unit/static gates executed; PostgreSQL/browser/recovery runtime pending where relevant |
| TEL-R5-AUDIT-018 | MEDIUM | Safe pre-effect expiry reachable; in-flight reconciliation graph retained. | Unit/static gates executed; PostgreSQL/browser/recovery runtime pending where relevant |
| TEL-R5-AUDIT-020 | MEDIUM | Downloader current permission on replay and direct read. | Unit/static gates executed; PostgreSQL/browser/recovery runtime pending where relevant |
| POST-AUDIT-NEW-001 | HIGH | Use ../../scripts/ci/check-native-selection.py; path regression and 14-spec real enumeration | PASS static/path/enumeration; browser NOT_EXECUTED |
| POST-AUDIT-NEW-002 | MEDIUM | Revalidate current capabilities at new request admission | TestConsumerUsesCurrentModelCapabilities PASS |
| POST-AUDIT-NEW-003 | MEDIUM | Return iterator error and reject partial batch | TestWorkerRejectsPartialOwnerBatch PASS (unit/race) |
| POST-AUDIT-NEW-004 | MEDIUM | Checked permission API used on automation admission dependency chain; preserve membership query error | TestPermissionPreservesInfrastructureFailure PASS (unit/race) |

## Continued source review

Provider/model/consumer definitions, current capability admission, model configuration revisions, bounded provider requests, token/cost arithmetic, failure classification, secret encryption/redaction, retrieval ownership and tool write authorization were traced through API/domain/schema/tests. Retrieved or model-supplied content does not grant permissions. R5B remains actor-bound guided assistance; READY means USER_ATTESTED_ONLY and never verified Apple identity. Generic Foundation consumer creation/transition remains rejected.

Cross-feature checks covered canonical bot ingress/retired runtime; channel authority and durable delivery; uploader/File Manager/scoped storage ownership and cleanup; payment evidence/order/refund fences; scheduler/workflow lock ordering and typed denials; AI imported-source identity/retrieval; Apple lifecycle/RLS/history; downloader current permission; ledger/idempotent jobs; API decoding/errors/Origin; workflow/config selection; packaging and backup/restore definitions. These are source-inspection conclusions, not claims of executed native acceptance. No additional confirmed defect was identified in the remaining contract review beyond this ledger.

## Migration contract

No historical SQL migration 0001–0025 was rewritten. Migration 0026 introduces immutable AI ingestion receipts with a composite knowledge/workspace/bot FK and ENABLE/FORCE RLS. Legacy imported keys are explicitly ambiguous and fail closed; no original identity is fabricated. Historical R4G=23 and original R5B=25 are preserved. Current R5B_REMEDIATED requires exactly contiguous 0001–0026. Missing/duplicate/order/final-name failures are tested. Applied checksum/reapply and restored count assertions use the same checkpoint.

All 26 SQL files and their PL/pgSQL bodies parsed using pglast 7.0 / PostgreSQL 17.0 grammar. This establishes syntax only; FK/data compatibility, grants, trigger execution, fresh/upgrade/reapply and FORCE-RLS behavior require a disposable PostgreSQL 17 execution. The deliberate 0025 populated-consumer upgrade guard remains a policy question, not an invented automatic data conversion.

## Executed local validation

Go 1.27.1; Node 24.19; npm 11.9; Playwright 1.63. Go commands use GOTOOLCHAIN=local, GOPROXY=https://proxy.golang.org, isolated module/build caches.

- `go test -mod=readonly ./internal/... ./tests/contract ./db/migrations -count=1`: PASS (31 packages contain executed tests; other packages report no test files).
- `go test -race ./internal/... ./tests/contract -count=1`: PASS (30 packages contain tests). Final affected config/database/intelligence/businessplugins race rerun also PASS.
- `go vet ./...`: PASS.
- `go build ./cmd/...`: PASS.
- `go test -mod=readonly -c -tags=integration ./tests/integration -o /tmp/telmivo-post-audit-integration.test`: compiled; NOT database execution.
- `gofmt -l cmd internal db tests`, `git diff --check`: PASS.
- `python3 -m unittest discover -s scripts/ci -p 'test_*.py'`: six definition regressions PASS.
- `python3 scripts/ci/r4g-release-check.py --checkpoint current`: PASS, 26 current migrations. Explicit historical R4G rejects this 26-file tree as intended.
- `python3 scripts/ci/check-native-selection.py --enumerate`: all 14 intended specs select actual tests, including R4E/R4F. NOT browser execution.
- `npm ci`, `npm run typecheck`, `npm run lint`, `npm test`, `npm run build`: PASS. Mini App unit results: five test files, 13 tests.
- `python3 scripts/security/scan.py`: zero source secret-pattern matches; supplementary scan, not exhaustive secret proof.
- `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`: zero called/imported-package vulnerabilities; GO-2026-5932 concerns unmaintained openpgp in required golang.org/x/crypto; TELMIVO neither imports nor calls that package. No fix is published for that advisory.
- `npm audit --audit-level=high --json`: zero vulnerabilities.

Transient errors introduced during editing (one integration callback return, one unused import, and a malformed new Python test artifact) were corrected and their relevant gates rerun. They are not recorded as inherited defects.

## Unexecuted runtime gates

COMPILED_NOT_EXECUTED: PostgreSQL-backed integration, fresh/upgrade/reapply/checksum execution, tenant/runtime-role isolation and backup/restore/recovered service replay. This root-only environment has no usable PostgreSQL/non-root service or Docker/Podman. Known unsupported root/user-namespace approaches were not repeated.

BROWSER_VALIDATION_BLOCKED_ENVIRONMENT: no installed Chromium/Chrome, and Playwright's expected executable is absent. A dependency-setup attempt obtained a zero/invalid archive and failed ZIP extraction. Browser assertions, 390/1280 payment widths, RTL/reload and native acceptance remain NOT_EXECUTED; enumeration is not PASS. No repeated download was attempted after confirming the missing runtime.

Container image/non-root/Compose acceptance and CodeQL remain NOT_EXECUTED locally. GitHub pre-runner jobs, if still runner_id=0/steps=[], are infrastructure evidence only. No workflow was manually rerun/dispatched/cancelled. Final exact-head Actions must be recorded separately at publication; neither old successful checks nor pre-step failures validate this candidate.

## Residual questions

NV-02 populated-R5A consumer upgrade policy; NV-03 Apple feature/plugin grant precedence; NV-04 authoritative Stars ambiguous-refund resolution; NV-05 conservative admitted-budget vs actual AI quota metering; NV-08 concurrent Foundation suspension policy remain unmodified contract questions. No blind retry, assumed provider success, fabricated identity verification or unsafe legacy conversion was introduced.

## Changed-file scope

- `.env.production.example` — REQUIRED_REMEDIATION
- `.github/workflows/migration-check.yml` — REQUIRED_REMEDIATION
- `.github/workflows/production-readiness.yml` — REQUIRED_REMEDIATION
- `README.md` — REQUIRED_DOCUMENTATION
- `SECURITY.md` — REQUIRED_DOCUMENTATION
- `compose.production.yaml` — REQUIRED_REMEDIATION
- `db/migrations/0026_ai_ingestion_receipts.sql` — REQUIRED_REMEDIATION
- `db/migrations/checkpoint.go` — REQUIRED_REMEDIATION
- `db/migrations/checkpoint_test.go` — REQUIRED_REGRESSION_TEST
- `deployments/reverse-proxy/nginx.production.conf.template` — REQUIRED_REMEDIATION
- `docs/R4C-uploader-storage-platform.md` — REQUIRED_DOCUMENTATION
- `docs/R4D-commerce-checkout-payments.md` — REQUIRED_DOCUMENTATION
- `docs/R4F-ai-assistant-2.md` — REQUIRED_DOCUMENTATION
- `docs/R5A-apple-services-foundation.md` — REQUIRED_DOCUMENTATION
- `docs/operations/post-audit-ledger.json` — REQUIRED_DOCUMENTATION
- `docs/operations/source-audit-wave1.md` — REQUIRED_DOCUMENTATION
- `internal/domains/appleservices/expiry_test.go` — REQUIRED_REGRESSION_TEST
- `internal/domains/appleservices/service.go` — REQUIRED_REMEDIATION
- `internal/domains/appleservices/state.go` — REQUIRED_REMEDIATION
- `internal/domains/businessplugins/commerce.go` — REQUIRED_REMEDIATION
- `internal/domains/businessplugins/payments.go` — REQUIRED_REMEDIATION
- `internal/domains/channels/service.go` — REQUIRED_REMEDIATION
- `internal/domains/commerce/admission.go` — REQUIRED_REMEDIATION
- `internal/domains/downloads/product.go` — REQUIRED_REMEDIATION
- `internal/domains/identity/service.go` — REQUIRED_REMEDIATION
- `internal/domains/intelligence/admission.go` — REQUIRED_REMEDIATION
- `internal/domains/intelligence/admission_test.go` — REQUIRED_REGRESSION_TEST
- `internal/domains/intelligence/assistant.go` — REQUIRED_REMEDIATION
- `internal/domains/intelligence/configuration.go` — REQUIRED_REMEDIATION
- `internal/domains/intelligence/knowledge.go` — REQUIRED_REMEDIATION
- `internal/domains/intelligence/model_contract_test.go` — REQUIRED_REGRESSION_TEST
- `internal/domains/intelligence/schedules.go` — REQUIRED_REMEDIATION
- `internal/domains/intelligence/service.go` — REQUIRED_REMEDIATION
- `internal/domains/intelligence/worker.go` — REQUIRED_REMEDIATION
- `internal/domains/intelligence/worker_test.go` — REQUIRED_REGRESSION_TEST
- `internal/domains/pluginplatform/store.go` — REQUIRED_REMEDIATION
- `internal/domains/uploads/service.go` — REQUIRED_REMEDIATION
- `internal/platform/auth/telegram.go` — REQUIRED_REMEDIATION
- `internal/platform/auth/telegram_replay_test.go` — REQUIRED_REGRESSION_TEST
- `internal/platform/config/config.go` — REQUIRED_REMEDIATION
- `internal/platform/config/proxy_test.go` — REQUIRED_REGRESSION_TEST
- `internal/platform/database/database.go` — REQUIRED_REMEDIATION
- `internal/platform/database/permission_test.go` — REQUIRED_REGRESSION_TEST
- `internal/platform/httpapi/ai_failure_test.go` — REQUIRED_REGRESSION_TEST
- `internal/platform/httpapi/api.go` — REQUIRED_REMEDIATION
- `internal/platform/httpapi/client_ip.go` — REQUIRED_REMEDIATION
- `internal/platform/httpapi/client_ip_test.go` — REQUIRED_REGRESSION_TEST
- `internal/platform/httpapi/downloader.go` — REQUIRED_REMEDIATION
- `internal/platform/httpapi/intelligence.go` — REQUIRED_REMEDIATION
- `internal/platform/storage/s3.go` — REQUIRED_REMEDIATION
- `internal/platform/storage/s3_test.go` — REQUIRED_REGRESSION_TEST
- `scripts/ci/migration_checkpoint.py` — REQUIRED_REMEDIATION
- `scripts/ci/r4g-release-check.py` — REQUIRED_REMEDIATION
- `scripts/ci/test_migration_checkpoint.py` — REQUIRED_REGRESSION_TEST
- `scripts/ci/test_upload_proxy_contract.py` — REQUIRED_REGRESSION_TEST
- `tests/integration/native_test.go` — REQUIRED_REGRESSION_TEST
- `tests/integration/post_audit_payments_test.go` — REQUIRED_REGRESSION_TEST
- `tests/integration/post_audit_recovery_test.go` — REQUIRED_REGRESSION_TEST
- `tests/integration/post_audit_test.go` — REQUIRED_REGRESSION_TEST
- `tests/integration/r5b_test.go` — REQUIRED_REGRESSION_TEST
- `tests/integration/wave1_automation_test.go` — REQUIRED_REGRESSION_TEST

- `docs/operations/post-audit-remediation.md` — REQUIRED_DOCUMENTATION

Only required remediation, regression and synchronized documentation files are included. Generated caches/binaries/dependency installs are excluded. No source changes implement R5C. No production database, credentials, customer data, Telegram/Apple/payment/refund traffic, deployment, develop/main merge, tag or release was used.
