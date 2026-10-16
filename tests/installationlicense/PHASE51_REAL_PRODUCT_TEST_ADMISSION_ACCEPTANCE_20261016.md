# LIC-A9 Phase51 — genuine Product/Go1.27 admission audit and executable regression candidate

Date: 2026-10-16.
Disposition: **SOURCE-REVIEWED / OFFLINE COMPONENT-RACE PASS / AUTHENTIC PRODUCT GO1.27.2 NOT_RUN / GENUINE V2 INTEGRATION NOT_RUN / PRODUCTION NO-GO**.

## Immutable provenance / constraints

- Parent is TELMIVO LIC-A9 Phase50 Draft PR #104, HEAD `410a44f9f324ec7734e354a154a512db6bb68b0f`. Branch: `development/lic-a9-phase51-real-product-test-admission-20261016`.
- TELMIVO `main` and License Authority remain untouched. No merge, GitHub Actions, Android/Termux, production/test VPS, credential or customer side effect.
- Product `go.mod` declares `go 1.27.0`. Required patched execution target for this roadmap is Go **1.27.2**.
- The available disposable Linux/amd64 model container has only `go version go1.23.2 linux/amd64`; `GOTOOLCHAIN=go1.27.2 GOPROXY=off GOSUMDB=off go version` produced exit 1, `go: download go1.27.2 for linux/amd64: toolchain not available`. DNS lookups for the Go mirror and github.com failed, and `git ls-remote` could not resolve GitHub. We did not run real Product on Go1.27.2 and make no claim otherwise.

## Two real Product source repairs in this DRAFT candidate

1. `internal/platform/installationlicense/state.go`: public context-bound free function `Effect(ctx,fn)` previously used `FromContext(ctx)` then invoked `m.Effect(ctx,fn)` directly. Even if the process runtime had been terminally restricted, a valid signed Manager injected into Context could bypass **the Phase50 process-wide guard**. The compatibility function now delegates to `RuntimeEffect(ctx,fn)`, preserving the common fail-closed policy. Exact Git blob of candidate: `f7e241efec9b479dec8318bf48b6f42dfe50f6b4`.
2. `internal/platform/installationlicense/http.go`: HTTP Middleware previously used `m.Decision()` alone to admit mutation handlers, even when a global `InstallRuntime(nil)` terminally restricted the process. Middleware now also requires `CheckRuntime(r.Context())`; when it fails, the HTTP decision is explicitly `RESTRICTED / RUNTIME_RESTRICTED`. Exact Git blob `2e9030b2807830386cc6d211b24c71e43ca6ac64`. Restricted liveness/read-only and acknowledged webhook exceptions are unchanged. This is **not** proof that every webhook/worker effect is separately fenced.

These changes are not release-authorized. Direct calls to the `(*Manager).Effect` method can still bypass the *global in-process* lifecycle policy if a caller chooses to bypass `RuntimeEffect`. Such calls require an explicit security/code-owner audit; local Manager cryptographic verification and process global state are distinct defenses.

## Genuine TEST-only signed V2 test sources staged but NOT_RUN

New `internal/platform/installationlicense/phase51_real_manager_admission_test.go`, exact Git blob `5363c97bb7c62ff06e1f7a67832c3a63b3e0dfc8`:
- Calls existing real `phase34LicensedManager(t)` fixture, which generates ephemeral Ed25519-signed V2 claims using the authentic Product `phase19TestSignedConcurrentInstallation(t)` fixture.
- Fresh-process cases: signed Manager injected before global runtime installation **denies** `Effect`, `RuntimeEffect` and a commercial POST; after global installation `Effect` and POST are admitted; after `InstallRuntime(nil)`, the same still-cryptographically-valid Manager cannot revive process effects or HTTP mutation.
- Separate release-blocking test `TestLICA9Phase51RealManagerNestedCheckMustNotDeadlock` executes a signed `RuntimeEffect` callback containing `CheckRuntime(ctx)`, with a 3-second child-process deadline. This is designed to **fail** if authentic `Manager.Effect` self-deadlocks, not to invent a PASS. It was compiled/exercised only with a compatible STUB Manager in this environment; authentic Go1.27 run is **NOT_RUN**. Do not waive a failure.

## Concrete independent Linux kernel evidence for the nested-lock risk

The byte-exact Product `state.go` uses `syscall.Open(... O_NOFOLLOW ...)`, `syscall.Flock(fd,LOCK_EX)` in `m.lock`; `Manager.Effect` evaluates the callback **inside** `m.lock`; `CheckRuntime` calls `m.Decision` which invokes the same `m.lock` on a **separately opened descriptor**. This has a deterministic Linux advisory flock self-deadlock risk if nested in one callback.

A separate, executed **Linux/amd64 Go1.23.2 kernel-level test** made two independent descriptors to the same file, took the first exclusive flock, confirmed the second did not acquire while the first was held, and confirmed it acquired after unlock. `go test -race -count=3 -v -timeout=40s ./...`: **3/3 PASS**, log SHA256 `f2b4ca5a79559f0f251a3ec41273326e6bf4ce5847ad51de73860203a57ddcee`. This proves **kernel lock semantics**, not the authentic V2 Manager regression's outcome.

## Actually executed local offline Product-component smoke checks

A disposable local Go1.23.2 module compiled:
- Byte-identical **real Product Phase50** `runtime.go`: Git blob `edd96d3e8da065c990c03dea4d3fb35d69802a90`.
- Byte-identical **patched Product Phase51** `http.go`: Git blob `2e9030b2807830386cc6d211b24c71e43ca6ac64`.
- Only the small patched `Effect` compatibility wrapper extracted from `state.go`, NOT the complete authentic `state.go`.
- An explicitly compatible **STUB Manager** with synthetic `VALID_ONLINE`; NO Ed25519 V2 verification; and a local stand-in for the genuine signed fixture in the Go integration test source.

One source smoke test run under `go test -race -count=1 -v ./...` passed, with the genuine-test source compiled against the stub, **NOT against the true Product Manager**; local log SHA256 `b2f3f1590e944dde0f75ddc120054e6754295df583c88151349307ae5b2da05c`. `go vet ./...` PASS, `gofmt -l` on tested Product Go bytes clean. The compatibility test run three times in **three separate fresh Go test processes** (rather than `-count=3`, which correctly fails after a terminal process-wide restriction): **3/3 PASS**, combined log SHA256 `8a3be090af81075aa1a1edd04d913dd1edf859ceb49e63d86951c224defa71a1`.

The initial `-count=3` in a **single process** had a reproducible FAIL in repetitions 2/3 because Phase50 intentionally makes `InstallRuntime(nil)` terminal. This is a test isolation issue, not proof of release success; the fresh-process rerun resolves this local fixture issue without weakening runtime security.

## Source audit (read from actual exact branch GitHub files)

**13/13 structural assertions PASS**, strictly static/source-only, across six actual repository files:
- `state.go` new guarded free Effect and Manager's file-lock effect execution;
- `runtime.go` process default-deny/terminal nil and admission drain;
- `http.go` runtime guard/terminal restriction plus acknowledged webhook exceptions;
- `internal/platform/runtime/runtime.go` installs global Manager before DB/Redis/NATS initialization (static ordering only);
- `internal/platform/queue/process.go` and `queue/nats.go` call `CheckRuntime` before handlers/external dispatch.

**Open security/compatibility questions** (not certified by static ordering):
- `queue.Process` executes an external handler **after** the runtime check, which is a TOCTOU window unless the handler itself fences the external effect using `RuntimeEffect` and durable idempotency evidence.
- `Manager.Effect` holds a lock while executing its callback. Nested `CheckRuntime` may deadlock; synchronous `InstallRuntime` inside a callback also remains forbidden; callbacks that never finish can indefinitely delay revocation completion.
- Product bootstrap source order is correct, but **full startup with DB/Redis/NATS and real signed fixtures has NOT been compiled/run**.
- Middleware's health/Webhook-acknowledgement exemptions are intentional; every real business side effect in handlers still needs an audited gate.
- Phase45–50 legacy tests were **NOT_RUN** in the genuine Go1.27 Product. Neither Linux root multi-UID IPC nor PG16 Product/Authority E2E was run.
- Identity, signed bundle/install, external provider receipts and Production remain outside this phase.

## Mandatory acceptance to exit NO-GO

1. On an independently authorized **disposable nonproduction** Linux host with Go1.27.2 and dependencies, pin this exact branch HEAD and run `go test -race -count=1 -timeout=180s ./internal/platform/installationlicense/...`, `go vet ./...` and full Product `go test ./...`. Do not use `-count=3` on process-global lifecycle tests that do not spawn child processes.
2. Run `TestLICA9Phase51RealV2RuntimeAndMiddleware` using the actual Ed25519/TEST V2 fixture and demand PASS.
3. Run `TestLICA9Phase51RealManagerNestedCheckMustNotDeadlock`; **any timeout is a release-blocking FAIL**. Fix the callback/file-lock contract without removing the durable authorization lock or relaxing clone/lease fencing; then rerun.
4. Audit all direct Manager.Effect uses, workers, queue redelivery/outcome idempotency, and HTTP/Webhook side effects.
5. Independently complete Authority Go1.25+, PostgreSQL16, Linux root multi-UID, two-process fencing/recovery, signed easy-install and rollback gates. Independent owner approval for any later merge/deployment.

**Decision: PHASE51_CODE_CANDIDATE_REGISTERED / SOURCE_STATIC_13_PASS / KERNEL_FLOCK_RACE_3_PASS / OFFLINE_STUB_RACE_PASS / AUTHENTIC_PRODUCT_GO127_AND_SIGNED_V2_NOT_RUN / REAL_NESTED_MANAGER_GATE_PENDING / PRODUCTION_NO_GO**.
