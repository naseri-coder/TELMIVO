# LIC-A9 Phase51-H — build-tagged genuine signed-V2 integration fixture and TestMain startup admission

Date: 2026-10-17. **DRAFT / NO MERGE / NO ACTIONS / NO PRODUCTION.**

## Immutable source lineage / scope

- Parent: TELMIVO Product Phase51-G Draft PR #111, HEAD `1f94dff833f472bbf8454f639ce0df42a06265f3`. New standalone review branch `development/lic-a9-phase51h-signed-v2-integration-bootstrap-20261017`; Product and Authority `main` untouched.
- No private release signer, real Authority, customers, servers, real provider, GitHub Actions, Termux or Production used.
- Actual model execution environment: `go version go1.23.2 linux/amd64`; `GOTOOLCHAIN=go1.27.2 GOPROXY=off GOSUMDB=off go version` **FAILED** because requested toolchain was unavailable. PostgreSQL, psql, nats-server and Docker/Podman binaries absent. Therefore **genuine Go1.27.2 Product, real V2 codec/Manager and PG16/NATS E2E NOT_RUN**.

## Real Product repository changes, ready for review only

1. `internal/platform/installationlicense/fixture_integration.go`, Git blob **`8e36387765e30c79a177f9a5a932bd12d40b0457`**.
   - Compiled **only with `//go:build integration`**, not ordinary Product builds. A separate explicit exported `NewTestOnlySignedV2RuntimeForNativeIntegration(stateDirectory)` constructs ephemeral Ed25519 root and signer; creates actual Product `Manager` via `Open`, reads native Installation identity, derives its hash, builds authentic `licensecrypto.Certificate` and **V2 claims** with `RuntimeSessionID`, `ConcurrencyGeneration=1`, and `Environment=TEST`; uses actual `Manager.Accept` and demands `Manager.Decision().State==VALID_ONLINE`. It does **not** weaken, monkeypatch or bypass the genuine Manager verification.
   - Strict guards: only a Go `*.test` executable, explicit ephemeral `TELMIVO_LIC_A9_NATIVE_FIXTURE=YES`, never `TELMIVO_ENV=production` or production license environment; both test DB URLs must use `postgres[ql]://`, an allowlisted host, exact `/telmivo_test` db name, expected TCP 5432/default port, and **exactly** `sslmode=disable` query. Host/service override query params and arbitrary ports are denied.
   - Still a TEST-only signing authority compiled when the `integration` tag is intentionally used; do not package `-tags integration` binaries for customers.
2. `tests/integration/native_test.go`, Git blob **`d8be9cd79a86f30de88b53034673b428280c55b6`**. After its preexisting localhost `/telmivo_test` URL validation, `TestMain` now creates a 0700 disposable temp directory, explicitly enables the test-only fixture flag only while creating/verifying its local V2 manager, removes that flag, and calls `InstallRuntime`. It proves `CheckRuntime` and a **synthetic no-external-I/O** `RuntimeEffect` before opening the test databases, then runs the existing Integration tests and removes the ephemeral local state directory on normal exit. There is **no** global production bypass, real authority request or real signer key. Existing Queue/Worker tests that previously ran without process-wide signed Runtime will now have a genuine fixture available in the native test process.
3. `internal/platform/installationlicense/phase51h_signed_v2_integration_test.go`, Git blob **`07fecb08563671281974def1df7e718f6a04899e`**: staged authentic Product V2 test and eight fail-closed negative cases: missing opt-in, Production environment, Production license environment, non-test DB, remote migration DB, unsupported scheme, query-host override, wrong TCP port.
4. `tests/integration/phase51h_signed_runtime_test.go`, Git blob **`a58748bbe78661bf5e43de7c69470b6970dec071`**: staged real Integration package regression verifying globally installed signed Manager admits one synthetic effect while `RequireContext(context.Background())` with no bound Manager still rejects admission. **Not executed in real Product**.
5. `tests/installationlicense/PHASE51H_SIGNED_V2_NATIVE_TEST_BOOTSTRAP_ACCEPTANCE_20261017.md` — this report.

## Actually executed offline evidence — precisely bounded

Local Go1.23.2 isolated module compiled the **byte-exact real fixture source** and **byte-exact real test source** from GitHub:
- `fixture_integration.go` `8e36387765e30c79a177f9a5a932bd12d40b0457`;
- `phase51h_signed_v2_integration_test.go` `07fecb08563671281974def1df7e718f6a04899e`.
- Test-only package dependencies are **explicit STUB** `Manager` + `licensecrypto` JSON codec with genuine Go standard-library `crypto/ed25519` key generation and verification. **NOT the real Product manager/codec**, and no real signed-V2 Product admission is claimed.
- `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off go test -tags=integration -race -count=10 -timeout=120s -v ./...` **PASS**: ten positive synthetic signing tests plus **80 negative fail-closed case executions**; Race Detector reported no failures. Log SHA256 **`3e5962f75e5d7147a206cbfebcf6c01eaef0ea2e115e0b7d8d851fcdf89a`**.
- `go vet -tags=integration ./...` **PASS**, empty output SHA256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.
- `go list -f '{{.GoFiles}}|{{.TestGoFiles}}'` on the stub harness default build showed fixture **absent**; same command with `-tags=integration` included the fixture and its tests. Local `gofmt -l` returned empty. GitHub source-only audit **21/21 structural assertions PASS**, including signing, environment guards, TestMain installation order, restricted context and unchanged product fail-closed policy.
- Verified local evidence ZIP **`/mnt/data/LIC_A9_PHASE51H_TEST_EVIDENCE.zip`** SHA256 `a8d9f3005a204a7d80dbd628269c669cc31125a2cf313b2e873f62e50a24cce8`: exact source, explicitly labelled stub implementation, raw test logs, build-tag list checks, SHA manifests and reproducible offline script. **No private keys or actual customer data**.

## Full Product and integration gates still NOT_RUN

1. On owner-authorized disposable Linux with Go1.27.2, execute the **real** `installationlicense` and `tests/integration` packages with `-tags=integration`, `go test -race`, full Phase45–51 regressions, and `go vet`. Demand actual signed-V2 TEST fixture construction PASS and verify the binary remains unable to activate on production DB/environment.
2. Start entirely **synthetic** PostgreSQL16 + JetStream sandbox with `TEST_DATABASE_URL`/`TEST_MIGRATION_DATABASE_URL` pointing ONLY at `/telmivo_test`; apply migration 0019 to seeded historical data, run new Phase51-F/G native tests, and original `TestOutboxLease`, `TestJetStreamRecovery`, `TestDownloadQueue`, Retry/DLQ and consumer external-result reconciliation.
3. Test ACK loss, crash-after-UNKNOWN claim, replay prohibition with expired lease, real SQL triggers and consumer inbox dedup using genuine broker; verify 1-attempt policy and operator-run UNKNOWN reconciliation. Mixed old Relay workers must not be running during migration.
4. Integrating new global Runtime into legacy tests may reveal additional failures (previously unguarded operations, contention, browser API in a **different process**); **none of those suites were executed**. The native test fixture installs only in the package's TestMain process, **not** in the separately launched `cmd/api` browser-integration binary. Do not invent test bypasses for it.
5. Review old test-created contexts requiring Manager binding and any concurrent tests relying on process-global default denial. No Production or released signatures should be used. Migration/rollback, Authority PG16, multi-UID/license recovery, installer remain pending.

**Decision: `PHASE51H_NATIVE_TEST_V2_BOOTSTRAP_STAGED / EXACT_FIXTURE_SOURCE_STUB_RACE_PASS / FULL_GO1.27_PRODUCT_REAL_SIGNEDV2_PG_NATS_NOT_RUN / PRODUCTION_NO_GO`.**
