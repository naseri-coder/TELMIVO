# LIC-A9 Phase51-C — direct-Decision deadlock mitigation and worker admission audit
Date: 2026-10-16

**Verdict: CANDIDATE SOURCE FIXED / OFFLINE LINUX STUB RACE PASS / FULL PRODUCT GO1.27.2 + AUTHENTIC SIGNED V2 NOT_RUN / AUTHORITY PG16 NOT_RUN / PRODUCTION NO-GO**

## Immutable lineage and repository changes

Base: Phase51-B TELMIVO Draft PR #106 at commit `d49df3d1ee13d60008d37117783560dbf9f96947`. All changes go to `development/lic-a9-phase51c-direct-decision-worker-audit-20261016` only; no merge, Actions, Termux, servers, Production, real signing identity, or other repo writes.

- `internal/platform/installationlicense/state.go` Git blob `691fc203684c71c280ce0bde2cb81b379f833ae4`: public `Manager.Decision()` now delegates to existing `decisionNonblocking()` (existing real `decisionLocked()` signed-state verifier and the same checked `O_NOFOLLOW`, `LOCK_EX|LOCK_NB` path). Previously `Manager.Decision()` synchronously acquired a blocking exclusive flock and would self-deadlock if called inside its own `Manager.Effect` callback, even after Phase51-B had made `CheckRuntime()` nonblocking. Busy locks yield `RESTRICTED / STATE_LOCK_UNAVAILABLE` rather than cached permission.
- `internal/platform/installationlicense/client.go` Git blob `4ecb7ede55bd60281ca5ad8e059b6d5606488083`: `shouldRefreshForDecision(d)` excludes `STATE_LOCK_UNAVAILABLE`. The license polling loop calls this helper instead of treating every transient busy lock as an authority revocation deserving a refresh attempt. Real expiry/restricted states and signed `RefreshDue` retain their refresh behavior.
- `internal/platform/installationlicense/phase51_real_manager_admission_test.go` Git blob `f2d7335b55046bc0e33e7efdc4a54a390ce17b88`: extends existing *authentic signed V2 TEST fixture test source* to verify direct public `m.Decision()` inside a signed `m.Effect()` callback immediately returns `STATE_LOCK_UNAVAILABLE`, and returns `VALID_ONLINE` after effect completes. **This real-fixture test source is NOT_RUN here**.
- New real-package `internal/platform/installationlicense/phase51c_refresh_busy_test.go` Git blob `5d56ff85fa1df04fda3c9e3694a88f2dbf0b5f29`: six refresh-classification cases, including busy with `RefreshDue`, valid online without refresh, valid refresh-due, authority restricted, and failed signature verification. **Full Product package execution NOT_RUN**.

## Executed offline Linux evidence — claims bounded to actual scope

Environment: `go version go1.23.2 linux/amd64`. `go1.27.2` toolchain and Product dependency downloads unavailable because shell DNS network resolution failed. No Production or existing server accessed.

Disposable harness in `LIC_A9_PHASE51C_TEST_EVIDENCE.zip` contains **byte-exact Product source** `runtime.go` Git blob `f9b580b63cb484dd1428df5580fbcc045341d3f8` and `http.go` blob `e57634c5069627dc4d379ec1a878a585759f0d2a` (verified locally via `git hash-object`), an **explicit source-derived STUB Manager with real Linux syscall.Flock**, and source-derived polling decision helper. The entire `state.go` or `client.go` was **not** compiled. This is **not** signed V2 cryptography.

- `go test -race -count=3 -timeout=120s -v ./...`: exit 0, **six independent subprocess runtime scenarios × three repeats = 18/18 PASS**; plus **six pure poll-classification scenarios × three repeats = 18/18 PASS**. Total 36 case-repetitions, zero Race Detector issues reported. Log SHA256 `153d52130c3b452f6121c1295ca5f7e8d8effe4357abf945342bdf75699b5967`.
- `go vet ./...`: exit 0; empty output SHA256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.
- Explicit **negative baseline** used same harness with only public `Decision()` changed back to blocking flock: `go test -race -count=1 -run '^TestPhase51BIsolatedMatrix/nested-check-middleware$'` exit 1; child timed out after 4 seconds with **DEADLOCK**, log SHA256 `563ff777e99128b17841e512baec15f59148a9a35d1f7b350b811ab0ba3a4146`.
- **Patched** nested scenario completed: exit 0; log SHA256 `e8c2a493b0b20c567c077e5e89919324b81e0b0a5a81303031f1aeddddf7f8fd`.
- Zip SHA256 `504826f1a4edcaa97587018c3fdb7364bf6fb5b5af90ef39fb9cc83958ef6124`. Contains runnable `REPRODUCE.sh`, logs, hashes, patched and intentionally failing baseline; NO private keys.

## Source audit — actual GitHub HEAD reads, 15/15 structural observations PASS

Inspected eight Product files: `state.go`, `client.go`, `runtime.go`, `http.go`, both relevant test files, `queue/process.go` and `queue/nats.go`.

Confirmed direct `Decision()` now nonblocking; underlying signature/lease decision retained; protected callback still holds the blocking exclusive file lock; `Client.Poll()` skips refresh on busy; installed global Runtime guard retained; HTTP Middleware uses RuntimeDecision; signed V2 test source asserts nonblocking direct decision and post-callback valid return.

**Worker audit finding (UNRESOLVED):** `queue.Process` checks `CheckRuntime` before DB transaction and, for external handlers, before pre-transport durable uncertainty commit. The actual external `handler(ctx,tx,event)` is invoked later: there is no atomic gate covering the interval between preflight and handler-side external transport. `queue/nats.go` has `MaxDeliver=6`; blind retries can be suppressed by durable inbox fences, but transient license lock contention near external attempts may leave an uncertainty marker requiring operator reconciliation. In this phase we deliberately made **no changes to queue processing** because without a real PostgreSQL16/NATS E2E and provider outcome fixtures, wrapping full transactional handlers with `RuntimeEffect` risks new lock inversion and unknown outcomes.

## Remaining risk and mandatory admission tests

1. The actual end-to-end Product `go test -race ./internal/platform/installationlicense/...`, `go vet ./...`, other phases 45–51, V2 Ed25519 TEST-only fixture, and full startup/Worker suites on **Go1.27.2**: NOT_RUN. DO NOT label SOURCE checks or Go1.23 STUB tests as authentic Go1.27/real Manager PASS.
2. Direct recursive `Manager.Effect` and `Manager.Accept` from an already active Effect callback still can block. `Manager.Effect` intentionally keeps an exclusive durable state lock through callback; do not silently relax concurrency/rollback protection. Synchronous `InstallRuntime` within an admitted callback remains unsafe; permanently stalled effects delay revocation completion.
3. Product-to-Authority signed concurrent leasing, PostgreSQL16, NATS at-most-once producer/consumer, multi-UID/IPC, provider receipts, recovery, installer and rollback: NOT_RUN. Independent disposable environment + review required. Never reuse Production.
4. Before authorizing release, audit every actual external handler/provider path for final `RuntimeEffect` guard and durable outcome/idempotency. Explicit owner approval and separate merge decision are required.

**Final:** `PHASE51C_DIRECT_DECISION_SOURCE_PATCHED / POLL_BUSY_CLASSIFIED / INDEPENDENT_STUB_RACE_36_PASS / NEGATIVE_DEADLOCK_REPRODUCED / GENUINE_PRODUCT_GO127_V2_NOT_RUN / WORKER_E2E_NOT_RUN / PRODUCTION_NO_GO`.
