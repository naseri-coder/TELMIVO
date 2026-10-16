# LIC-A9 Phase51-B — fail-closed nonblocking genuine-Manager decision candidate

Date: 2026-10-16.
Disposition: **BRANCH-ONLY RUNTIME SECURITY REMEDIATION / OFFLINE STUB+REAL LINUX FLOCK TESTED / AUTHENTIC GO1.27.2 + SIGNED V2 NOT_RUN / PRODUCTION NO-GO**.

## Scope and exact provenance

- Started from TELMIVO Phase51 Draft PR #105 immutable HEAD `904c68a921fbdaaa7593157b7ef424c0bada7d61`.
- New independent branch: `development/lic-a9-phase51b-real-manager-nonblocking-decision-20261016`.
- No changes to TELMIVO or Authority `main`, no merge, Actions, Termux, Production, real license keys, server/database, or provider side effects.
- Exact patched Git blobs:
  - `internal/platform/installationlicense/state.go`: `cf70164fe2085c66c8ee6894373781fc86be0896`.
  - `internal/platform/installationlicense/runtime.go`: `f9b580b63cb484dd1428df5580fbcc045341d3f8`.
  - `internal/platform/installationlicense/http.go`: `e57634c5069627dc4d379ec1a878a585759f0d2a`.
  - `internal/platform/installationlicense/phase51_real_manager_admission_test.go`: `46a1d183efe73e1a72ba0b58877c95d45a225475`.

## Exact source flaw and scoped remediation

The actual `Manager.Effect` uses `m.lock` with `syscall.Flock(LOCK_EX)` around `fn()`. A protected callback that invokes `CheckRuntime(ctx)` calls `Manager.Decision()`, which tries to open a **new** descriptor of the same lock file and acquire a second blocking exclusive flock. This can self-deadlock on Linux. Phase51 HTTP Middleware also used blocking `m.Decision()` in a callback.

Phase51-B preserves the original exclusive `Manager.Effect` lock, credentials/lease verification, state persistence and hardlink/path checks. It:
1. Factors `m.lock` into existing blocking `m.lockMode(syscall.LOCK_EX,...)`, so existing effect/acceptance behavior stays unchanged.
2. Adds private `m.decisionNonblocking()` using the **same** real `decisionLocked()` authentic signed-V2 validation, with `LOCK_EX|LOCK_NB`. When the file lock is busy/unavailable, it returns `Decision{State:"RESTRICTED", Reason:"STATE_LOCK_UNAVAILABLE"}` without waiting. If the callback obtained the lock but signed validation rejected a lease, it preserves the real restricted decision and reason.
3. Makes `CheckRuntime` and new `RuntimeDecision(ctx)` use the nonblocking decision while respecting Phase50's global binding, context cancellation, Manager identity and terminal process restriction.
4. Changes HTTP Middleware to obtain **one** process-bound nonblocking `RuntimeDecision` instead of blocking on `m.Decision()`. A live protected callback encountering a nested commercial HTTP call is restricted instead of deadlocking.
5. Updates the pre-existing genuine signed V2 Phase51 subprocess test: nested `CheckRuntime` must return `ErrRestricted` quickly; nested commercial POST must be 403 with no handler call; after the original effect returns, `CheckRuntime` must return success for the still-valid manager. Timeout remains a release-blocking failure.

**Deliberate trade-off:** under an independently held file lock, `CheckRuntime` and Middleware now fail closed rather than waiting to check until the other effect finishes. This can temporarily reject otherwise valid requests and change availability. It must be reviewed together with queue retry and at-most-once external attempts.

**Still unresolved:** `m.Decision()` direct callers remain blocking, `m.Effect()`/other blocking state APIs called recursively under the same file lock can still deadlock, synchronous `InstallRuntime` from inside an admitted callback remains unsafe, and a forever-running external effect may prevent revocation completion. These pathways require their own separate audit rather than claiming universal deadlock elimination.

## Actual isolated Linux evidence

Toolchain: `go version go1.23.2 linux/amd64`; GitHub connector read/write worked. The shell could not resolve `github.com` or `go.dev`; Go 1.27.2 was not present, and no external deployment was used.

An offline disposable Go module used byte-exact new Product `runtime.go` and `http.go`: local `git hash-object` values matched the GitHub blob SHAs `f9b580b63cb484dd1428df5580fbcc045341d3f8` and `e57634c5069627dc4d379ec1a878a585759f0d2a`. The Manager was an **explicit STUB**, incorporating Linux `syscall.Flock` semantics and a matching `lockMode`/nonblocking decision shape, **not** the full true `state.go`, genuine V2 crypto, or real Product codebase.

- **Initial baseline reproduced FAILURE**: a deliberately reverted blocking-decision variant failed the nested-check scenario after a 4-second DEADLOCK timeout. Exit code 1 was expected; log SHA256 `1f5ce70d5625c42f6ca1520ea63d42c424595934166134d5a1dad57bd636cf9e`.
- **Patched variant**: `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off go test -race -count=3 -timeout=180s -v ./...` exited **0**, six fresh-process scenarios repeated three times = **18/18 PASS**, including nested HTTP denial, concurrency, new runtime install, and terminal revocation; Race Detector reported no data race. Log SHA256 `d7007c6c2fe3249bd46a351def6e7ed11e47c255da58bcba76e08b492ba3900e`.
- `go vet ./...`: exit 0, output empty. `gofmt -l`: empty. The stub is identified in the evidence bundle and no test is represented as authentic Product signed-V2 execution.
- Those tests do not exercise full `Manager.Decision`, `Manager.Effect` in the complete Product, or the declared `go 1.27.0` module. **Authentic signed V2 test: NOT_RUN**.

## Mandatory release gates (do not waive)

1. Execute the Phase51 signed-V2 subprocess tests in the real full Product with genuine Go1.27.2, and run full Phase45–51 `go test -race` and `go vet`. Verify the diagnostic reason for a busy flock and a separately restricted signed lease; ensure non-blocking failure does not alter security release gates.
2. Audit every direct `m.Decision`, `m.Effect`, `m.lock`, `m.Accept` call from external-effect callbacks, HTTP Middleware, queue workers and the license polling routine for nested lock acquisition.
3. End-to-end Startup, webhooks, Worker dispatch, queue idempotency and high-water signed concurrency fencing under Linux root/multi-UID supervisor. Treat at-most-once preflight denials as potentially availability-impacting.
4. Execute actual Authority Go1.25+, PostgreSQL16 Product↔Authority signed V2 test, recovery, offline install, revocation/rollback, provider outcome reconciliation. No borrowed production hosts.
5. Independent security review + explicit owner approval before any merge or deployment.

**Verdict:** `PHASE51B_PATCHED_SOURCE_AND_KERNEL_STUB_RACE_PASS / AUTHENTIC_GO127_V2_NOT_RUN / NESTED_DIRECT_MANAGER_PATHS_NOT_CLOSED / PRODUCTION_NO_GO`.
