# LIC-A9-PHASE-36 — Real-Manager Owner Relay Test + Actual Linux Hardlink Capability Probe

Date: 2026-10-16  
**Decision: TEST CODE AND LINUX KERNEL PROBE VERIFIED; REAL PRODUCT PHASE36 Go1.27 EXECUTION PENDING; MULTI-WORKER PRODUCTION NO-GO.**

## Exact GitHub lineage
- Product parent: Phase35 Draft PR #88 HEAD `24c350eb84901c96e87d9325fe4b6fcaae240cdd`, which contains real user-executed Android Go1.27.2 evidence: 8/8 genuine Phase34 Manager.Effect tests PASS, full Product license package PASS **with 5 hardlink SKIP**, go vet PASS.
- License Authority Draft PR #57 HEAD `d6960982e322d4458c7d963bb153f205a1f8fb59` (unchanged, review-only).
- No Production host, License VPS, Authority repo, Product `main`, Github Actions, services, real credentials, external business effects or GitHub merges.

## Exactly three source deliverables

| New file | Git blob SHA identical to locally executed code |
| --- | --- |
| `internal/platform/installationlicense/phase36_real_owner_relay_test.go` | `4a041d1a801661d96a303efdde9545dd30c3a212` |
| `tests/installationlicense/phase36_linux_hardlink_probe/go.mod` | `6b22709c1d16c15ba5a46b678b3d442924c10d2c` |
| `tests/installationlicense/phase36_linux_hardlink_probe/hardlink_linux_test.go` | `5184628ad8822e4c6841e2c54027f9a363c00a20` |

All new files are test-only; root Product `go.mod` remains Go1.27 and unchanged. No `Manager.Effect` Product runtime code or License Authority code modified.

## Phase36 Owner relay: genuine Product test source

The new test file is in the **actual Product `installationlicense` package**, not a separate standalone mock package in GitHub. It uses the prior Phase19 disposable signed-V2 fixture and constructs a **TEST-ONLY in-process Unix socket relay**. The actor without the active V2 signed session is expected to fail when calling `Manager.Effect` directly; the Owner with the valid signed session can execute only a synthetic, tenant-restricted callback via its own **actual `Manager.Effect`**.

Ten authored cases `TestLICA9Phase36*` cover:
1. Follower direct Effect denied by V2 runtime-session check.
2. Owner relay calls Manager.Effect for a signed synthetic request.
3. Cross-tenant validly signed request denied by test domain policy.
4. Unsigned request mutation denied.
5. Wrong runtime session even with valid test-peer signature denied.
6. Duplicate same request ID denied.
7. Unknown synthetic outcome reserves the request and rejects blind retry within the same process.
8. After a new signed-V2 generation transfers session ownership, old owner relay fails closed.
9. Eight concurrent requests for identical request ID execute only one synthetic callback.
10. Foreign test signing key rejected.

Test-only Ed25519 worker identity, in-memory per-request reservations, no persistent crash recovery, no real Customer RBAC or provider call. **This is one owner and an in-process follower with real Unix-socket frames**; it does NOT implement or prove multiple independent Worker OS processes, real supervisor peer UID isolation, separate database per tenant, durable idempotency, or external provider exactly-once execution. The prior Phase22 multiworker liveness gap remains OPEN; forwarding is a candidate architecture, not production availability.

Android compatibility improvement: Unix test socket now uses private `os.MkdirTemp("", "lic36-")` and short filename `owner.sock`, rather than a potentially overlong `t.TempDir` path. Socket frame is bounded by `io.LimitReader`; server exits and registers all goroutines before Wait.

### What was ACTUALLY run for the Owner relay
- A disposable **local Go1.23.2 Linux/amd64 stub Manager** module compiled the byte-exact new Go test source and exercised its signed Unix IPC protocol. `go test -race -count=15 -timeout=120s ./...`: **15/15 complete suites PASS**, 10 named test functions per suite = 150 local tests, no races; `go vet ./...`: PASS.
- Local runner log SHA256: `92c58d73f1aa8a12237927f769271d15882b2c96c0d3c4c1214a6d400e4c5240`.
- This stub provided `Manager`, `Open`, `Accept`, `Effect` and signed test-lease helper with compatible signatures, but was **not the actual Product implementation**. Real V2 cryptographic admission / real Manager recheck is **NOT_RUN for Phase36** until the user executes the committed test under Go1.27 on their Android/Termux device or on isolated Linux.

## Real Linux syscall capability test — independently executed

The new standalone Go1.23 `phase36_linux_hardlink_probe` package uses the **actual Linux kernel/filesystem** to generate a hardlink for each of the five named protected files: `identity.json`, `state.json`, `highwater.json`, `lock`, `refresh-leader`. It verifies `os.SameFile` and `Fstat.Nlink == 2`, and separately asserts that `O_NOFOLLOW` refuses symlink opening.

- Environment: isolated Linux/amd64, `go version go1.23.2`, no Production, network, service or real secrets.
- `go test -race -count=30 -timeout=90s ./...`: **PASS**, 30 repetitions of each named Linux kernel test (all five subcases each time), no races.
- `go vet ./...`: **PASS**.
- Full verbose log SHA256 `512327b33f16e26bcbbe5ef7525b7ed3607e4ff527139d096f45c6487743a23f`.
- All test/module Git blob SHAs re-fetched from GitHub and matched locally tested bytes exactly.

**Important limitation:** This actual Linux probe verifies **kernel ability to create hardlinks**, NOT actual TELMIVO Product `Manager.Effect`/hardlink-denial logic on Linux. The five genuine Product hardlink assertions were SKIP on Android and remain **NOT_RUN on a full Linux Go1.27 Product checkout**. Do not conflate a kernel test PASS with Product security acceptance.

## Next acceptance gate (NOT performed)

On user's already configured **nonproduction Termux Go1.27.2** clean clone at this Phase36 exact reviewed SHA:

```sh
cd ~/telmivo-license-audit-src
git fetch origin development/lic-a9-phase36-real-owner-relay-linux-hardlink-proof-20261016
git switch --track origin/development/lic-a9-phase36-real-owner-relay-linux-hardlink-proof-20261016
git status --short
git rev-parse HEAD
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -count=1 -timeout=180s -v -run '^TestLICA9Phase36' ./internal/platform/installationlicense
```

The user must confirm the exact HEAD and test output. `go test` runs only test code, no servers or Actions. On Android no race certification can be asserted. Follow-up full Linux Go1.27+ `-race` package test plus actual Product hardlink cases, Authority/PG16 multiworker E2E and independent security review remain required.

## Final status
- Linux actual kernel/filesystem hardlink prerequisites: **PASS**.
- Signed IPC test logic under isolated **stub Manager**: **PASS**.
- Signed IPC through **real Product Manager.Effect**: **NOT_RUN/UNVERIFIED**.
- Linux Go1.27 actual Product Hardlink defense with race: **NOT_RUN/UNVERIFIED**.
- Concurrent independent Owner/Worker service availability, durable domain effects, external providers: **BLOCKED**.
- Production: **NO-GO**.

STOP after Phase36; keep Draft; do not merge, deploy, run Actions, use Production server or recreate deleted License server.
