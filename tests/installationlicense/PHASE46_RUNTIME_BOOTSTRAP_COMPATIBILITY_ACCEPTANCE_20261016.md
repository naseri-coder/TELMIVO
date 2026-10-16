# LIC-A9 Phase46 — Real Runtime Startup and HTTP Compatibility Matrix

Date: 2026-10-16.

**Status: EXACT PRODUCTION RUNTIME + HTTP SOURCE BYTES TESTED UNDER LINUX GO1.23 WITH A COMPATIBLE STUB MANAGER / PHASE46 18-OF-18 SUBPROCESS SCENARIOS PASS / FULL LOCAL PHASE45-46 PASS / AUTHENTIC PRODUCT GO1.27 NOT_RUN / PRODUCTION NO-GO.**

## Ancestry and scope

- Parent: Phase45 Draft PR #99 at `fab37675a2c7f5bf726e5e1daeb61395c08dbb59`.
- Phase46 branch: `development/lic-a9-phase46-bootstrap-compatibility-matrix-20261016`.
- Only **one new test file** `internal/platform/installationlicense/phase46_bootstrap_contract_test.go` and this acceptance report are introduced. **No new modifications to actual Product runtime code**, Production, licensing Authority, DB, service bootstrap, Go module or Actions.
- Test Go file expected exact Git blob `368501ffd8201842dc28ba8b9fee9d9e53b527ce`.

## Static Product call-chain audit (genuine GitHub source inspected, not executed)

- All real executable `cmd/api`, `cmd/main-bot`, `cmd/group-bot`, `cmd/downloader-bot`, `cmd/worker`, `cmd/media-worker`, `cmd/customer-bot-runtime` entrypoints use `internal/platform/runtime.Main(role)`.
- `internal/platform/runtime/runtime.go` (Git blob `05594cabc60423f6a29c1abb24aea9dd30e2dd8b`) calls `OpenEnvironment()`, wraps the context in `RequireContext`, calls `InstallRuntime(license)`, and only then opens DB/cache/queue and builds HTTP handlers. On failed OpenEnvironment, it explicitly installs a nil manager, which Phase45 must keep fail-closed.
- `cmd/licensectl/main.go` uses `OpenEnvironment` and `Client` directly. In `installationlicense/client.go`, `Client.request` sends through its dedicated authenticated `cl.http.Do`; it does **not** invoke `RuntimeEffect` or `installationlicense.HTTP`. Thus Phase45's default-deny change does not *directly* intercept this Authority bootstrap network path. This is a **source inspection**, not an executed activation E2E.
- `internal/platform/installationlicense/http.go` Middleware keeps restricted liveness and webhook ACK route forwarding but forces `RequireContext`; actual `httpapi.contentWebhook` and `telegram.GroupBotWebhook` check `CheckRuntime(r.Context())` before their protected work. The new local test reproduces that downstream check; it does not run either real handler with DB or Telegram.
- Protected work not explicitly routed through `RuntimeEffect`/`CheckRuntime` elsewhere remains outside this stage's guarantee.

## Exact production-source provenance

The model's isolated Linux environment contains **Go1.23.2 linux/amd64** and no network-resolvable Go1.27 download. The disposable compatibility module compiled byte-for-byte identical full copies of:

1. Product `internal/platform/installationlicense/runtime.go` Git blob `5b9941aa16560dc27314cc627b6efc0ce94e8515` (Phase45 candidate).
2. Product `internal/platform/installationlicense/http.go` Git blob `0ebde57dfdc10c753153bfff4403828e76cb3954` (unchanged).
3. Added Phase46 test `phase46_bootstrap_contract_test.go` Git blob `368501ffd8201842dc28ba8b9fee9d9e53b527ce`, independently matched after GitHub upload.
4. Previously tested Phase45 regression test file. The Manager, signed-lease Phase19 fixture and file state backend in this local module are **compatible STUB implementations**, not original Product crypto or actual Authority. PublicIdentity compile-only glue is stub and unexercised.

## Actually completed Linux/amd64 tests, Race Detector ON

Command:
```sh
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off go test -race -count=3 -timeout=180s -v -run '^TestLICA9Phase46' ./...
```

**PASS, exit code 0.** Six child-process scenarios per iteration × 3 repetitions = **18/18 completed scenario PASS**:
1. Fresh process prior to any `InstallRuntime`: `RuntimeEffect`, `CheckRuntime` and `HTTP` fail-closed and never call the synthetic effect or RoundTripper.
2. Explicit `InstallRuntime(nil)`: all three remain denied.
3. Installed valid **stub** Manager: unbound guarded effects and HTTP work; `RequireContext` without bound Manager is denied; explicitly bound valid Manager is permitted.
4. Nil→valid→nil transitions: restricted-before, eligible-during, restricted-after.
5. 24 goroutines × 30 attempted calls while installing valid Manager: no unexpected errors or data races, with a final authorized call followed by nil-manager denial. This is not an operational multi-UID/process fence.
6. Middleware: GET liveness and restricted webhook ACKs still return HTTP 200 while their local test handler's protected work is denied; restricted commercial POST returns HTTP 403; read-only GET remains responsive without allowing protected side effects.

Complete 3-repeat log SHA256 `ae859dbf1209203e54bcb71240fa65db9efab7833ce19b4821ea4b810999c697`. Reported package elapsed time **19.276s**.

Full local Go1.23.2 compatibility module with Phase45 + Phase46 test files: `go test -race -count=1 -timeout=180s -v ./...` **PASS**, exit 0, **8.087s**, log SHA256 `c80c7f134bc0f6d893a70af67d7c199bd8b36255ce90f49e7868cbd5bb224f05`. `go vet ./...` PASS, gofmt clean. A final shell-only `grep -c` without allowing leading indentation returned code 1 despite the Go tests exiting 0; a corrected match independently counted all **18** PASS scenario lines. That grep was **not a failed Go test**.

## Mandatory Product acceptance / high-value remaining tasks

- Run the Phase45 and Phase46 tests within the **authentic Product package** under Go1.27.2 (Android/Termux or separate disposable Linux), including real signed V2 fixtures. **NOT_RUN** for Phase46.
- Full Product entrypoint build and DB/cache/queue bootstrap regression: **NOT_RUN**. The static startup order is evidence but not a passing integrated test.
- Native Linux Go1.27+ `-race`, real UID-root supervisor bridge, five Product hardlink tests, real Authority-issued boot challenges, PostgreSQL16 E2E and provider settlement: **NOT_RUN / NOT_IMPLEMENTED**.
- Before merging the Phase45 **runtime behavioral fix**, obtain independent code review and genuine Product integration tests for startup, health, licensectl, webhook and worker roles.
- No live server, real customer data or production credentials used. No GitHub Actions, main merge or deployment.

**Disposition: PHASE46_COMPATIBILITY_TEST_SOURCE_VERIFIED / LINUX_GO123_EXACT_RUNTIME_AND_HTTP_WITH_STUB_PASS / REAL_PRODUCT_GO127_NOT_RUN / PRODUCTION NO-GO. Keep PR Draft; stop after Phase46.**
