# LIC-A9 Phase45 — Runtime default-deny before Manager installation

Date: 2026-10-16. **SECURITY BUG IDENTIFIED AND BRANCH-ONLY REMEDIATION CANDIDATE. PRODUCTION NO-GO; GENUINE PRODUCT GO1.27 INTEGRATION NOT_RUN.**

## Exact ancestry and blast radius

- Parent Phase44 Draft PR #98: `7005bb6712d1250d99266f2f110500cc28c96a3c`.
- New Phase45 branch: `development/lic-a9-phase45-runtime-default-deny-20261016`. No merge to `main`.
- **This stage modifies genuine Product runtime code** (`internal/platform/installationlicense/runtime.go`) plus a regression test (`phase45_runtime_fail_closed_test.go`). Unlike Phases38–44, it is NOT merely a test-only change.
- No Authority code, database, external provider, bot business modules, root Go module, GitHub Actions or Production server was touched.

## Verified original fail-open behavior — exact Product source

Original `internal/platform/installationlicense/runtime.go` Git blob: `55a390411117028e4fd7c35b29bb5ff824d2c826`. The **same exact source bytes** were compiled unchanged in an isolated Linux Go1.23.2 component harness with an explicitly fake compatible Manager. A regression witness in an uninitialized process, without `InstallRuntime` or `WithManager`, showed:

1. `RuntimeEffect(context.Background(), callback)` returned success **and invoked the callback** without checking an actual Manager.
2. `CheckRuntime(context.Background())` returned **nil** in the same uninitialized state.
3. `HTTP(context.Background(), stubHTTPClient, request)`, routed through `RuntimeEffect`, invoked the synthetic RoundTripper without a Manager.

The baseline `go test -race` witness **PASS** means it successfully **demonstrated the old fail-open semantics**; it does NOT mean those semantics were safe. Baseline log SHA256: `ffef0c5901f18631a714974f2eab35f964ae28705df13bd9dc804145b873ae93`. No external HTTP or real business effect occurred.

**Risk characterization:** an unguarded caller that invokes `RuntimeEffect` before `InstallRuntime` might perform a protected effect before runtime authorization is established. This is a source-level authorization gap and not a demonstrated exploit of an actually deployed TELMIVO instance. Explicit `RequireContext` was already restrictive; the vulnerable situation is an unbound context before installation.

## Surgical Product runtime candidate

Changed `runtime.go` Git blob **`5b9941aa16560dc27314cc627b6efc0ce94e8515`**, matched exactly to local tested source.

1. `InstallRuntime` publishes `runtimeRequired=true` **before** installing the global Manager pointer, so a concurrent caller in the transition fails closed instead of remaining unguarded.
2. A single `runtimeForEffect(ctx, globallyRequired, global)` requires either an explicitly bound Manager or a successfully installed global one; **absence of a Manager returns `ErrRestricted`** even before installation.
3. `RequireContext` with no explicitly bound Manager is still denied (cannot use a global fallback).
4. Cancellation is checked first; existing `Manager.Decision` and `Manager.Effect` remain the authorities for authorized contexts.
5. `CheckRuntime`, `RuntimeEffect` and its `HTTP` wrapper now all use the deny-by-default resolver. Genuine Authority bootstrap client `Client.request` uses its own explicit raw HTTP client, so it is not wired through this RuntimeEffect wrapper, but **application-wide bootstrap and caller compatibility still require full Product integration review**.

New `phase45_runtime_fail_closed_test.go` Git blob **`e5a10da3cbe99b6ee842e9591e4a60de711e9cdf`**, matched byte-for-byte with locally executed tests. It references the actual Product Phase19 signed V2 fixture when built in the real package. **That genuine Product fixture has NOT been executed for Phase45**.

## Actually executed local evidence (Linux/amd64 Go1.23.2)

Only the **exact real `runtime.go` bytes**, the exact new Phase45 test bytes and a **compatible stub Manager/Phase19 fixture** were compiled in a standalone disposable Go1.23.2 module; no Go1.27 toolchain available here.

- Baseline original source `go test -race` expected bypass witness **PASS**, no races; exact original Product runtime blob verified.
- Patched source `go test -race -count=10 -timeout=90s -run '^TestLICA9Phase45'` **PASS**, seven named test functions per iteration (one runs as a subprocess-child under the parent harness), 11.119s, tool exit 0, no reported race. SHA256 full 10-repeat log: `e98c018ef0ebda43724b20c418a61192faefe1c9df178412fb08c5b2730ec181`.
- Patched source `go test -race -count=1 -v ./...`: **7/7 named PASS**, 2.028s, log SHA256 `f10bc6f140d70454b7d4be5c28e1ef7a0dd01c72e0a970df2fda00b377211d26`.
- `go vet ./...` **PASS**, `gofmt` clean, exact Git file blobs matched independently.
- Checks: missing runtime and installed-nil Manager blocked, explicit context requirements, canceled context, restricted Manager, permitted test-signed valid Manager, and a **new OS child process** verifying `RuntimeEffect`, `CheckRuntime`, and `HTTP` cannot fail open at bootstrap.

## Mandatory acceptance before merge

**DO NOT MERGE this runtime change yet.** Unlike a test-only security contract, this alters actual Product bootstrap behavior. It requires:

- Genuine Product Android Go1.27.2 Phase45 tests with actual signed Phase19 V2 fixture and `go vet`.
- Full Product suite / compatibility scan of all callers that may have depended on unguarded pre-`InstallRuntime` behavior, especially startup health/read-only endpoints and Authority activation bootstrap.
- Native Linux Go1.27+ actual Product `-race` and root multi-UID integration, hardlink tests and end-to-end Authority/provider checks.
- Independent security/code review and staged rollout approval.

**PHASE45_REAL_RUNTIME_SOURCE_SECURITY_CANDIDATE_VERIFIED_IN_GO123_COMPONENT / AUTHENTIC_PRODUCT_GO127_NOT_RUN / PRODUCTION NO-GO.** PR remains Draft; no merge, Actions, server or deployment. Stop after Phase45.
