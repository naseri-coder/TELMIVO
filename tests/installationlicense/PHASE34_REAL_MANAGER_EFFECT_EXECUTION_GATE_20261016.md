# LIC-A9 Phase34 — Genuine Product Manager.Effect Regression Tests / Execution Gate

Date: 2026-10-16  
**Status:** `PHASE34_REAL_PRODUCT_TESTS_AUTHORED_AND_SOURCE_VERIFIED` / `GO127_REAL_EXECUTION_BLOCKED` / `ACTUAL_E2E_NOT_RUN` / **PRODUCTION NO-GO**.

## Exact ancestry and changes

- Parent: TELMIVO Product Phase33 Draft PR #86 HEAD `70a73705d7f7de4acec09e3aa5a6d2e1fd03464d`.
- Authority Phase21 Draft PR #57 HEAD `d6960982e322d4458c7d963bb153f205a1f8fb59` (read only).
- Actual Product Go1.27 module: `go.mod` blob `b9a1ccbfc3b7c765cc42017892cc566e6fd62400` with `go 1.27.0`.
- Actual Product implementation: `internal/platform/installationlicense/state.go` blob `c5c81548a1b063825eccc9c3d69cc6006160d4cd`.
- Existing synthetic Ed25519-signed V2 fixture: `internal/platform/installationlicense/phase19_session_fencing_test.go` blob `3fe0316182760ccf6504b7f36e7be8cd19dacd77`.

**New:** `internal/platform/installationlicense/phase34_real_manager_effect_test.go` (Go package `installationlicense`), Git blob `d32c4dac201fcb78216b633830a6d70e37b7f0b5`, **byte-identical** to the locally parsed/gofmt-checked test file.

This is the first phase in this recent sequence to add a regression file directly to the **actual Product installation-license Go package** instead of extending a separate mock broker module. **However, authoring a genuine package test is NOT executing it.**

## Eight new real-code tests awaiting execution

The tests instantiate the **real `Manager` type** using the preexisting Phase19 synthetic signed-V2-claims fixture. They call the **real `Manager.Effect` method**, rather than a separate mocked `EffectGate`.

1. `TestLICA9Phase34RealManagerEffectDispatchesOnlyWithSignedV2Session`: permitted callback exactly once for test-signed valid process session.
2. `TestLICA9Phase34RealManagerEffectBeforeActivationDenied`: unactivated install cannot dispatch.
3. `TestLICA9Phase34RealManagerEffectRejectsCanceledContext`: callback not called after cancellation.
4. `TestLICA9Phase34RealManagerEffectPropagatesFailureWithoutAutoRetry`: an unknown synthetic callback result propagates without internal retry.
5. `TestLICA9Phase34RealManagerEffectDeniesRestartedSessionUntilNewSignedV2`: restarted session cannot reuse old V2 token; new signed generation/session permits execution; former owner is denied.
6. `TestLICA9Phase34RealManagerEffectRejectsExplicitRestrictedState`: restricted installation state prevents dispatch.
7. `TestLICA9Phase34RealManagerEffectDeniesExpiredTestLease`: simulated TEST-only clock beyond V2 lease validity denies callback.
8. `TestLICA9Phase34RealManagerEffectConcurrentFollowerNeverDispatches`: competing Manager instances sharing installation directory cannot reuse one process's signed session; follower callbacks remain zero under concurrent requests.

All keys are random in-memory **test-only** Ed25519 keys; no real Authority, Production material, exchange, payments, Telegram API or database touched. No changes to real `Manager.Effect` implementation. This is regression coverage pending Go1.27 verification, NOT an actual broker-to-Manager IPC integration.

## Verification ACTUALLY performed

**PASS — syntax/format/integrity:**
- Local Go **1.23.2** `gofmt -w` on new test file, followed by `gofmt -l`: no formatting errors.
- Local Go1.23 `go/parser` `parser.ParseFile(...,parser.AllErrors|parser.ParseComments)`: succeeded; parsed eight `TestLICA9Phase34*` functions. Local parser audit output SHA256: `e6de9047effd95862cfd8195c9c01c3e7a7b45853e2b68c34cc97adb25659b6e`.
- Locally calculated `git hash-object` matches freshly refetched GitHub blob `d32c4dac201fcb78216b633830a6d70e37b7f0b5` exactly.
- **20/20 static source-contract checks** against actual GitHub Product code, Phase19 signed fixture, new test file and `go.mod`. Checks cover real `Effect` signature, `m.lock`, context validation, pre-callback `decisionLocked` enforcement, V2 process session mismatch denial, signed-V2-test fixture availability, random runtime session generation, Product concurrency requirement and no outgoing network calls in the new tests. Source-text/structure checks are NOT executable Go tests.

**BLOCKED — real Go execution:**
- Local toolchain: `go version go1.23.2 linux/amd64`.
- Reproduced command: `GOTOOLCHAIN=local GOPROXY=off go test -race -count=1 -run '^TestLICA9Phase34' .` against a temporary module declaration matching root `go 1.27.0` (not a full Product checkout). **EXIT CODE 1**, diagnostic: `go.mod requires go >= 1.27.0 (running go 1.23.2; GOTOOLCHAIN=local)`. Diagnostic log SHA256: `42946e3355b424885bf5041f6f7044238b03e6d484f000e23c04fc916b56d185`.
- Direct `git ls-remote https://github.com/naseri-coder/TELMIVO.git HEAD` from execution container also failed DNS lookup for github.com. GitHub connector successfully fetched source and wrote changes, but cannot make Go1.27 toolchain and cached dependencies appear in the local runtime.

**Therefore: 0/8 Phase34 genuine Product runtime tests executed, 0/8 verified runtime PASS, all eight pending. No compile/build/race claims for the Product.**

## Exact commands required for real acceptance, when genuine Go1.27 and dependencies are available

From a **disposable nonproduction checkout** of this Phase34 branch (NEVER the Production host, no Actions):

```sh
go version                                    # must be >= go1.27.0
go test -race -count=1 -timeout=180s -run '^TestLICA9Phase34' ./internal/platform/installationlicense
go test -race -count=1 -timeout=180s ./internal/platform/installationlicense
go vet ./internal/platform/installationlicense
```

Required eventual PASS evidence: all eight tests executed against actual Go1.27 Product package, full license package test suite including Phase19/22, no races, precise build/toolchain versions, immutable HEAD SHA, and independent real Authority/PostgreSQL16 V2 E2E tests. Test output alone cannot prove secure multi-process, systemd FD isolation or external provider fencing.

## Non-waivable release blockers

- **REAL PRODUCT TEST EXECUTION NOT_RUN:** Go1.27 required; local host Go1.23.2, no direct network/download access.
- **BROKER TO ACTUAL MANAGER.EFFECT NOT_CONNECTED:** Phase33 IPC remains test-only and mock-wired; new real package test exercises direct method calls only **when run later**. Do not claim an operational IPC-integrated license owner.
- **REAL AUTHORITY SIGNED LEASE / PG16 E2E NOT_RUN:** Fixture is a self-signed disposable test issuer, not TELMIVO License Authority.
- External payment/trading/Telegram effect provider fencing, real Customer RBAC/quotas, durable PostgreSQL domain ledger, secure hostile same-UID/out-of-process isolation, independent DR and installer all **NOT_VERIFIED**.
- Root module version and `main` divergence need independent release security review. No Main merge or server changes allowed.

**Phase34 code delivery/source audit ACCEPTED; Phase34 runtime integration acceptance BLOCKED, Production NO-GO. STOP after Phase34.**
