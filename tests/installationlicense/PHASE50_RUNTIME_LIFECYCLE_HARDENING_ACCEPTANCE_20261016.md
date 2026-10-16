# LIC-A9 Phase50 — Runtime authorization lifecycle hardening

Date: 2026-10-16
Status: **BRANCH-ONLY CANDIDATE / REAL PRODUCT GO1.27.2 NOT_RUN / AUTHORITY E2E NOT_RUN / PRODUCTION NO-GO**

## Immutable review source

- Parent: the **mainline-anchored, same-tree** Phase49 review commit `53a7a1f50aaa9f14ca2980506e013ea6715901b6` in `review/lic-a9-phase49-mainline-snapshot-20261016`; that commit is a single-parent snapshot from Product main `f8b7962b83760f780e1c96bc3b80872223dedaee`. The Phase49 development PR #103 remains DRAFT/UNMERGED.
- This isolated branch: `development/lic-a9-phase50-runtime-lifecycle-fence-20261016`. No write to either repository's main; no merging, deployment, Actions invocation, server, DB, signing key or customer data.
- Real Product code: `internal/platform/installationlicense/runtime.go`, **exact final Git blob** `edd96d3e8da065c990c03dea4d3fb35d69802a90`.
- New real-package test: `internal/platform/installationlicense/phase50_runtime_lifecycle_test.go`, **exact final Git blob** `02c2d40c9f99a75983533b2d2659c4735ca3539b`.
- Legacy Phase45–48 tests are updated in this same candidate to reject unsafe pre-install context authorization and nil→valid revival. They have NOT been executed in the full real Go1.27 Product module.
- No Authority source changes; Authority reference remains draft Phase21 PR #57 HEAD `d6960982e322d4458c7d963bb153f205a1f8fb59`.

## Security changes to actual Product Runtime

1. **No pre-install exemption**: `runtimeForEffect` returns `ErrRestricted` if global installation has not occurred; `WithManager(ctx, m)`, even with a signed valid Manager, cannot authorize protected operations on its own. A fresh process requires `InstallRuntime(m)` before protected effects.
2. **One-way lifecycle**: `InstallRuntime(nil)` creates terminal restriction for the process lifetime. The first successful installed Manager can be installed idempotently with the *same pointer*; an attempt to replace with a different or nil Manager transitions to terminal restriction. Any later `InstallRuntime(valid)` is ignored in that process. This is an in-process control and does not defend against arbitrary malicious code execution in the process.
3. **Effect-drain before revocation returns**: Runtime admission takes `runtimeGate sync.Mutex` and increments `runtimeActive` before invoking real `m.Effect(ctx, fn)`. After terminally marking the process restricted, `InstallRuntime(nil)` waits on a `sync.Cond` until admitted effects have returned. Newly arriving `RuntimeEffect` and `CheckRuntime` calls see terminal restriction. It does **not** cancel in-flight external operations or enforce provider-side exactly-once.
4. **Deadlock regression discovered and fixed in this phase**: Initial `sync.RWMutex` implementation could deadlock when an admitted callback performed `CheckRuntime` while a writer waited to revoke. An isolated regression reproduced this fail (exit 1), log SHA256 `5b4b2c89fa613bd4f62f583017507ead0d1380695c2c8637d6c5fecf2245fd34`. It was replaced with the active-effect counter and condition variable. **The same regression passed after the correction**.

### Important limitations

- **Do not invoke `InstallRuntime` synchronously from a `RuntimeEffect` callback**: a callback would be waiting on itself to drain. External administrative shutdown must occur from another goroutine/process. API redesign or bounded revocation deadlines are still pending.
- Revocation cannot preempt an already-admitted external provider operation. If that operation never finishes, synchronous revocation can wait indefinitely. This phase proves the local completion ordering under a *STUB Manager*, not an operational kill switch.
- A `CheckRuntime` result is not a durable permission token; callers must execute external effects through `RuntimeEffect` / Manager.Effect and idempotent durable owner/receipt mechanisms.
- Real `Manager.Effect` holds an operating-system file lock during callbacks. Nested `CheckRuntime` in a *genuine* Manager.Effect may have separate file-lock reentrancy implications; **full Product live-manager nested calls are NOT_RUN and must be explicitly audited**.
- Restart starts a new process lifecycle; no cross-process revocation witness or shared multi-UID fencing is proven here.

## Actually executed, with precise scope

Local disposable Linux/amd64, `go version go1.23.2 linux/amd64`; no Internet access needed. Test component compiled **byte-exact GitHub final runtime.go** and **byte-exact Phase50 test Go file**, confirmed with local `git hash-object` equal to GitHub blob SHAs. A deliberately minimal compatible **STUB Manager**, `Decision`, `Effect`, `WithManager` and signed fixture substitute was used (not genuine Product crypto or license lease). No full root Product module compiled because it declares Go 1.27.0.

Commands actually run:

```sh
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off \
 go test -race -count=3 -timeout=120s \
 -run '^TestLICA9Phase50FreshProcessLifecycleMatrix$' ./...
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off go vet ./...
gofmt -l runtime.go phase50_runtime_lifecycle_test.go
git hash-object runtime.go phase50_runtime_lifecycle_test.go
```

**Result: 8 fresh-process scenarios × 3 repetitions = 24/24 PASS, Race Detector exit 0, 25.898 seconds**, full log SHA256 `acf677471810a5f850f5ad7cdeeea3d5d88a88e9d483330ec0aa71160644b891`. `go vet` PASS, `gofmt -l` empty.

Eight scenarios:
- signed Context Manager denied before installation;
- nil installation cannot be revived;
- foreign rebind permanently restricts;
- idempotent same-Manager installation remains valid;
- revocation cannot return until admitted callback finishes;
- nested `CheckRuntime` during revocation terminates without deadlock;
- concurrent action attempts cannot revive terminal restriction;
- canceled context denies callback.

An additional `go test -race -count=1 -timeout=45s ./...` including the local independent nested-revocation regression passed, complete log SHA256 `d67ee91bd55ec9302bee93d291b029a9c97628cce7d2078acdb655080f9848e6`. This local-only extra test is not part of the committed Product test sources.

**These are meaningful executed component tests but NOT authenticated Product Go1.27.2, signed V2, Linux root multi-UID, Authority PostgreSQL16, real provider delivery, or installation acceptance.** Do not promote to release or merge.

## Mandatory Phase51 / release gates

- Obtain approved *disposable* test environment with actual patched Go1.27.2 and fully fetched dependencies; compile and execute **all** original and changed Phase45–50 Product test sources using real TEST-only signed V2 fixtures; capture independently verified logs and failures. Production servers and the deleted Authority host must remain untouched.
- Run true Product `go test -race ./...`, `go vet ./...`, exact 1.27.2 bootstrap/Webhook/worker behavioral compatibility, review all external-effect paths; explicitly audit nested file-lock use and no-callback-install policy.
- Verify real Authority with Go1.25+, PostgreSQL16 and the Product↔Authority signed concurrency lease, IPC owner/multi-UID, crash/failover/rollback/provenance and provider receipts in a disposable environment.
- Review bounded revocation timeout/cancellation design and durable coordination across multiple processes. Avoid claiming immediate kill of in-flight effects.
- Independent security review and explicit owner approval required before any merge, Actions, easy installation or Production decision.

**Verdict: PHASE50_ACTUAL_RUNTIME_CHANGED / SOURCE_IDENTICAL_LOCAL_STUB_RACE_24_PASS / REAL_PRODUCT_127_NOT_RUN / REAL_AUTHORITY_E2E_NOT_RUN / PRODUCTION_NO_GO.**
