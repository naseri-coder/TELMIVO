# LIC-A9 Phase47 — Installed Runtime Manager Overrides Untrusted Context Manager

Date: 2026-10-16.

**Disposition:** PRODUCT RUNTIME AUTHORIZATION BUG REPRODUCED IN EXACT CODE BYTES; BRANCH-ONLY REMEDIATION CANDIDATE WITH LINUX GO1.23 RACE COMPONENT REGRESSION PASS; GENUINE PRODUCT GO1.27 NOT_RUN; PRODUCTION NO-GO.

## Ancestry, scope and deployment ban

- Parent Phase46 Draft PR #100: exact HEAD `1bd16b71874529970f56f2de23a5956f2b393638`.
- Dedicated Phase47 development branch `development/lic-a9-phase47-global-runtime-authority-20261016`; **do not merge to any `main`**.
- This stage changes **real Product runtime code** at `internal/platform/installationlicense/runtime.go` and adds one Product `*_test.go`. It changes no GitHub Actions, Authority code, databases, real provider, Product root `go.mod`, secrets, Production server, or release installer.
- Existing Phase45+46 work remains stacked as parents; prior Production NO-GO remains binding.

## Precise newly discovered authorization gap

The Phase45 candidate correctly changed pre-installation behavior to default deny. However, its `runtimeForEffect(ctx, globallyRequired, global)` selected `FromContext(ctx)` **before** the globally installed Manager and fell back to `global` only when the Context lacked a Manager.

A real application can call `InstallRuntime(nil)` after configuration/Authority startup failure, or `InstallRuntime(officialManager)` after a successful bootstrap. A different, context-injected valid Manager could then be preferred instead. This meant that `RuntimeEffect`, `CheckRuntime` and `HTTP` could be routed through **an unauthorized Manager instance**, bypassing the process-wide installation restriction.

The flaw was reproduced in an **isolated component harness** compiling byte-exact unchanged **Phase45 Product** `runtime.go` (Git blob `5b9941aa16560dc27314cc627b6efc0ce94e8515`) and the actual Product HTTP wrapper, with explicitly compatible stub Manager objects:
- `InstallRuntime(nil)` followed by `WithManager(ctx, foreignValidManager)` allowed synthetic callback execution and `CheckRuntime` success.
- `InstallRuntime(officialManager)` followed by `WithManager(ctx, differentValidManager)` admitted execution through the *foreign* Manager.
- Prior source `go test -race` baseline reproduction **PASS means the bypass witness succeeded**, not that old behavior was secure. Baseline log SHA256 `29ee4796483216cdf7d52eb54ea4fcebc0daab5caff65da4c9d86e52b187d5f9`.
- This is a **source-level authorization flaw** and is **not** proof of an exploit of a live TELMIVO deployment.

## Surgical remediation in real runtime.go

New Git blob `9b72adb763eb1e4705d407a6711942793a465123`; independently fetched and compared byte-for-byte with exactly the locally race-tested file.

Within `runtimeForEffect`:
- If the process-wide guard has been installed (`globallyRequired=true`), **`global=nil` denies** even when Context presents a valid locally signed Manager.
- If Context contains a Manager different by pointer identity from installed `global`, **deny**.
- Otherwise return the installed global Manager, which performs its real local signed-lease and state verification through `Decision` / `Effect`.
- Before process-wide installation, an **explicit context-bound Manager** can still be used by a deliberately isolated worker subject to that Manager's signed-lease verification; unbound Context remains deny-by-default (Phase45 behavior).
- Canceled Context and `RequireContext` rules retain their earlier priority.

**Caveat:** publishing/replacing an installed Manager is currently two separate atomic values (`runtimeRequired` and `runtimeManager`), not a process-wide serialized generation/epoch transaction. An `InstallRuntime(nil)` call cannot retroactively cancel an effect already admitted on a previous valid Manager snapshot. Operational revocation and external effect fencing still require real lifecycle integration, durable journal and an independent provider/Authority boundary.

## New six-scenario test file

`internal/platform/installationlicense/phase47_global_authority_test.go` Git blob `1ecff4d23b4e498d128389658e975898b7d311a8`.

Test uses **six fresh-process subprocess scenarios**:
1. Nil installed global refuses otherwise-valid different Context Manager and prevents `RuntimeEffect`, `CheckRuntime` and synthetic HTTP RoundTripper.
2. Valid official installed Manager refuses different valid Context Manager; permits same instance and ordinary authorized global use.
3. Changing installed global to nil takes precedence over an old valid Context Manager.
4. Legitimate explicitly bound signed V2 Manager is usable **before** global installation.
5. Canceled Context remains denied before callback.
6. 16 goroutines issuing 60 attempts each with a foreign Context Manager remain denied during 100 installed-global nil/valid transitions; Race Detector checks memory safety.

The test source is built in the true Product package with the real `phase19TestSignedConcurrentInstallation` fixture when genuine Product Go1.27 is available. That actual build/test is **NOT_RUN** here.

## Test evidence ACTUALLY executed — Linux/amd64 Go1.23.2

Go is `go1.23.2 linux/amd64`, `CGO_ENABLED=1`; `go.dev` and `proxy.golang.org` cannot be resolved, so this isolated model container cannot obtain Go1.27. No production servers or Android device were used.

The component module uses **exact original/modified real Product `runtime.go` bytes** and unchanged real Product `http.go`, plus exact Phase45/46/47 regression files and a **clearly labeled stub Manager/Phase19 lease fixture**. No claim of true Product cryptographic integration.

- New Phase47 cases, `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off go test -race -count=3 -timeout=180s -v -run '^TestLICA9Phase47' ./...`: **6 child scenarios × 3 repetitions = 18/18 PASS**, **exit 0**, no race warnings, `ok ... 19.222s`. Complete current log SHA256 `87bc5bc07b3aab82c90a3a0b6424013864911a9f6d1a00e4016e955c09952055`.
- Combined isolated Phase45+46+47 component tests, `go test -race -count=1 -timeout=180s -v ./...`: **PASS, exit 0**, `ok ... 14.157s`. Complete current log SHA256 `6faad20b1f45896a985285c018b78354d66201d2fcca37db15b99b1d9d627729`.
- `go vet ./...` PASS, `gofmt -l` produces no unformatted Go files, both new Git blobs exact match.

## Mandatory acceptance gates / stop

- **Genuine TELMIVO Product Go1.27.2 Phase45–47 signed V2 integration**, full installationlicense package and application startup tests: **NOT_RUN**.
- Native Linux Go1.27 root/multi-UID race, genuine Product hardlink tests, Phase43–44 security tests with actual Manager, independently signed Authority boot challenge and receipt, PostgreSQL16 E2E and verified provider idempotency: **NOT_RUN / NOT_IMPLEMENTED**.
- Because this **alters real runtime security behavior**, **do not merge** until genuine Product integration, authorized bootstrap compatibility and independent security review pass.
- No Production server, deleted License VPS, GitHub Actions, live credentials or customer effects.

**PHASE47_REAL_RUNTIME_GLOBAL_AUTHORITY_CANDIDATE_VERIFIED_IN_STUB_COMPONENT / GENUINE_GO127_PRODUCT_NOT_RUN / PRODUCTION_NO_GO. STOP AFTER PHASE47.**