# LIC-A9 Phase51-D — bounded Effect/Accept file-lock admission; independent executed evidence

Date: 2026-10-16.
Verdict: **DIRECT-MANAGER LOCK WAIT REMEDIATION CANDIDATE / GO1.23.2 ISOLATED RACE PASS / REAL PRODUCT GO1.27.2 AND SIGNED V2 NOT_RUN / PRODUCTION NO-GO**.

## Immutable provenance and restrictions

- Parent: TELMIVO Phase51-C Draft PR #107 HEAD `cb6157dc3b9039ea10229dd557768829cb03c30a`; branch `development/lic-a9-phase51d-bounded-manager-lock-admission-20261016` is the **only** development branch updated in this phase.
- No GitHub Actions, Merge, `main`, Production/real servers, Termux, confidential private release signer or real provider interaction.
- Product's `go.mod` still declares `go 1.27.0`; the release-target toolchain specified in roadmap is **Go 1.27.2**. Local runtime `go version go1.23.2 linux/amd64`; attempted Go1.27.2 auto-toolchain via `GOTOOLCHAIN=go1.27.2 GOPROXY=off GOSUMDB=off go version` fails: **toolchain not available**. Shell git/github DNS unavailable, GitHub connector reads/writes succeeded.
- Exact actually tested helper `internal/platform/installationlicense/flock_admission.go` Git blob **`48eb4d88a6587946a92ea47ec45f086b0b969e61`** equals local `git hash-object`. Both real package test files and the real signed-V2 subprocess gate are staged but their full Product execution has **NOT** been performed.

## Actual repository security changes

1. **Single byte-identical Go helper `flock_admission.go`** (standard-library only): `acquireExclusiveFlock(ctx, fd, maxWait, busy)` polls Linux `LOCK_EX|LOCK_NB`, honors `ctx.Done()`, returns the supplied denial on timeout and rejects unexpected syscall failures. No signed V2 verification is delegated to this helper, and no Linux syscall lock is released early.
2. **`internal/platform/installationlicense/state.go`** (real Product): factor the original open + `O_NOFOLLOW` + `Fstat` + regular-file + `Nlink==1` + `0600` + same-UID security checks into `m.lockModeWithWait(...)`; existing `m.lockMode` and `m.lock` keep their original blocking behavior. `Manager.Effect(ctx, fn)` now uses a cancellable **2-second admission window** with `ErrRestricted` on contention timeout. The actual `m.decisionLocked()` (signed lease, session, high-water and identity) and the entire `fn()` are executed **only after** acquiring the same physical exclusive file lock, and the lock is held through callback return; no automatic retry.
3. **`Manager.Accept(envelope)`** now uses a **2-second admission window**, returning `ErrPending` on contention timeout *without* executing `acceptLocked` or mutating high-water/state. The actual signed V2 acceptance logic and ordering remain unchanged. Retries must be explicitly governed by owner lifecycle and idempotency, never blind retries.
4. **Genuine real-package Go test sources**:
   - `phase51d_flock_admission_test.go`: native Linux FD two-descriptor wait/timeout, cancellation, unlock, invalid arguments.
   - `phase51d_manager_lock_test.go`: actual `lockModeWithWait` owner/regular-file symlink/hardlink-denial, nested busy-effect/Accept and cancellation tests without signing fixtures.
   - `phase51d_bounded_manager_test.go`: **authentic existing ephemeral TEST-only Ed25519-signed V2 fixture** from `phase34LicensedManager`, in a fresh subprocess per scenario; nested direct `Manager.Effect` must fail closed without calling inner callback; nested direct `Manager.Accept` must return `ErrPending` without changing signed concurrency generation; canceled nested Effect returns `context.Canceled`. Child hard timeout 9s. **THESE GENUINE V2 TESTS ARE NOT_RUN**.

## Executed Linux unit and syscall evidence (scoped)

A disposable `/mnt/data/lica9_phase51d_lock` module includes:
- the **exact GitHub helper blob** above;
- a **source-derived typed STUB Manager** reproducing the precise Product `lockModeWithWait` open/`Fstat`/`flock`/unlock body;
- tests of two separate file descriptors, timeout, canceled context, lock release, invalid argument, nested denial, pending Accept, hardlink and symlink rejection; no cryptography or real Product binaries.

**Actually executed** with `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off`, Go 1.23.2:
- `go test -race -count=4 -v` on five initial standalone-helper cases: **20/20 PASS**, no races. Full log `testlog.txt`, SHA256 `accfd0d95892b968f3d9906df1b546b6b922d54ba8aee79eefb3fe14fc70fe6e`.
- `go test -race -count=3 -v` on eleven cases (five helper + six source-derived Manager owner/lock regressions): **33/33 PASS**, no races. Full log `testlog_full.txt`, SHA256 `b10dfb691f934684a6941c3006e1d49218d9036fd5beb0c1ae07fad7755ea94a`.
- `go test -race -count=10 -timeout=120s ./...`: **110/110 PASS**, exit 0, no races. `stress10.log` SHA256 `4e177176c1b46f221279522b2d17dd6bb0de400d6e7c7a01b99adb5bec5c984f`.
- `go vet ./...` exit 0; `gofmt -l *.go` empty on **the isolated local harness only**. No full Product compilation claimed.
- Executed-source ZIP `LIC_A9_PHASE51D_TEST_EVIDENCE.zip`, SHA256 **`603af535f210e470062c2e482a0cfa7829be23321602be47c27593c3fc921779`**, includes `REPRODUCE.sh`, README, raw logs, exact tested helper and explicit stub harness.

**Static GitHub HEAD audit**: 15/15 source invariant assertions PASSED: preserved file identity/owner/link checks, signed `decisionLocked`, method wiring, blocking other mutation paths, helper cancellation, Runtime global admission, changed test fixture, and still-open Worker TOCTOU. These are source assertions, NOT executed genuine Product tests.

## Known residual risks / release blockers

- **Authentic Product Go1.27.2 and genuine signed-V2 Manager.Effect/Accept tests: NOT_RUN**. Do not infer authentic correctness from a source-derived STUB.
- Effect and Accept can now return early if another legitimate process holds the exclusive file lock longer than **2 seconds**. This trades availability for bounded wait and fail-closed security; test real parallel roles, slow external provider callbacks and lock contention before release. Signed V2 same-session acceptance, rollback and high-water journal must remain fully tested.
- Existing **`m.lock` paths still BLOCK indefinitely**, including the `cl.mutate` HTTP request while holding lock, and direct administrative `Manager.lock` or refresh/Concurrency calls from inside an Effect callback. Only direct `Effect` and `Accept` acquisition are bounded here. No claim of universal deadlock freedom.
- Direct `m.Effect` on a locally signed Manager remains different from the process-global `RuntimeEffect` admission policy; audit actual runtime callers instead of silently removing real license state protections. The production runtime should only dispatch side effects through the installed Runtime and durable per-provider outcome fences.
- Worker `queue.Process` performs `CheckRuntime` prior to invoking a transactional handler; **the check is not atomic with eventual external transport**, despite durable pre-attempt uncertainty record. Real PG16/NATS, multi-UID/Authority and provider idempotency tests are necessary. No queue code modified in Phase51-D.
- The full Phase45–51 regression suite, Startup/Redis/NATS and actual signed Authority/PG16 E2E are **NOT_RUN**. Signed easy installer, recovery, rollback, cryptographic release lineage and owner approval PENDING.

**Decision: `PHASE51D_BOUNDED_DIRECT_LOCK_CANDIDATE / ISOLATED_RACE_110_PASS / TRUE_SIGNED_V2_GO127_NOT_RUN / MULTI_PROCESS_AND_WORKER_GATES_PENDING / PRODUCTION_NO_GO`. Keep Draft and DO NOT MERGE, DEPLOY OR ENABLE ACTIONS.**
