# LIC-A9-PHASE-37 — Durable Synthetic Owner Outcome Journal Acceptance (2026-10-16)

**Decision:** `PHASE37_TEST_CODE_AND_LINUX_STUB_RACE_VERIFIED` / `REAL_PRODUCT_GO127_PHASE37_TESTS_NOT_RUN` / `REAL_MULTIPROCESS_OWNER_FENCING_NOT_CERTIFIED` / **PRODUCTION_NO_GO**.

## Stage-entry prerequisites: what was actually met vs deferred

- **PASS — real Product functional evidence from Phase36:** The user's Termux `go1.27.2 android/arm64` executed **10/10 actual Product `Manager.Effect` Phase36 tests PASS** on Product SHA `168f46a5326ce7ecb5b8e696bad1c95529cb9d47`; output `ok .../internal/platform/installationlicense 0.304s`. This verified stage-entry feasibility; it is not a separate Linux race test.
- **PASS — branch ancestry:** Phase37 is based on the exact Phase36 Draft PR #89 HEAD `168f46a5326ce7ecb5b8e696bad1c95529cb9d47`; Authority Phase21 PR #57 `d6960982e322d4458c7d963bb153f205a1f8fb59` was NOT modified.
- **PASS — Linux kernel prerequisite only:** Phase36's independent Linux Go1.23 hardlink-capability proof constructed all five attack fixture hardlinks with 30 repeated race-instrumented test runs. **The genuine Product's five hardlink defenses are still NOT_RUN on Linux Go1.27.**
- **BLOCKED — real production prerequisites:** Real Authority-signed V2 worker identity/lease, PostgreSQL16 two-repo 8+ E2E, independent Linux Go1.27 full Product `-race` and all five Product hardlink assertions, remote provider epoch fencing, durable owner/worker systemd deployment, DR, independent security approval and release keys remain NOT_RUN/NO-GO. These are mandatory for release but NOT a license to use Production server for stage-entry experiments.

## Changed files (Product test-only; no production runtime changes)

| Path | Git blob SHA, checked against locally executed source |
| --- | --- |
| Modified `internal/platform/installationlicense/phase36_real_owner_relay_test.go` | `9c0a7e9496a9180aea7716dc6299b22c0c9f3a88` |
| New `internal/platform/installationlicense/phase37_durable_owner_journal_test.go` | `099443c8cb46205ca0c5411ccf34917663f6512e` |
| New `internal/platform/installationlicense/phase37_durable_owner_journal_integration_test.go` | `243c89130a6d646e8cded507c2728a055bfd8626` |

The preexisting Phase36 Unix-socket relay's `process` now checks the optional test-only `phase37Journal` and reserves the **full canonical signed claim digest keyed by request ID** **BEFORE** invoking **actual `Manager.Effect`** (once the Go1.27 tests run on the real Product). The Phase36 in-memory-only reservation continues unchanged when no durable test journal is configured. No live customer operation was added.

The Phase37 journal uses:
- Owner-private test directory, `openat(O_NOFOLLOW)` and owned mode-`0600` files; cooperative `flock` lock per journal; exclusive `O_CREAT|O_EXCL` request reservation; `fsync` on file and directory. If a record becomes corrupted or truncated, reads fail closed. It never removes reservations or silently retries an uncertain effect.
- A domain-separated Ed25519 signature on a **synthetic** provider receipt bound to request ID, full claim digest, provider ID and result `CONFIRMED`. Forged receipt rejected; conflicting receipts cannot override previous confirmations. Confirmation provides **test-only evidence**; it is not a real settlement or external provider acknowledgment.
- Reopened journal after process crash still denies duplicate request ID even if the earlier synthetic callback returned uncertain status. Distinct journal handles and eight concurrent reservation contenders have a single winning reservation in the local cooperative locking model.
- Actual child subprocess killed by `os.Exit(77)` immediately after persistent fsync-backed reservation; parent reopens and sees `RECONCILE_REQUIRED` instead of silently authorizing a retry.

## Actual executed verification (be precise about the model)

**Local disposable Linux/amd64 runtime has Go `1.23.2`.** Product root module requires Go `1.27.0`, and the sandbox cannot download that toolchain. The tests were compiled and executed in an **isolated Go1.23 module** with a **compatible STUB Manager** (NOT genuine TELMIVO `Manager`).

- `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -race -count=1 -v -timeout=120s ./...`: **PASS**, the tested module contains 10 Phase36 functions + **9** Phase37 functions (19 named test functions). Phase37's crash child is an additional subprocess, not an additional named test function.
- `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -race -count=20 -timeout=120s ./...`: **20/20 complete suites PASS**, 19 named test functions per suite, no reported data races, `ok phase37.local/test-stub 1.998s`. Log SHA256: `d0ddc65d02ff9add28e0249dc206f6a3ea974dfce1ef05008b2abd92e66ccf6c`.
- `go vet ./...`: **PASS**.
- `gofmt -l phase36_real_owner_relay_test.go phase37_durable_owner_journal_test.go phase37_durable_owner_journal_integration_test.go`: **no output**, formatted.
- **3/3** GitHub blobs matched `git hash-object` of exactly the locally tested three files.
- **0/9 genuine Go1.27 Product Phase37 tests executed in this chat**. The Go1.27.2 Android Termux environment exists on the user's phone but is NOT remotely controlled by this chat.

The **nine** new Phase37 test functions are:
1. Real-Manager-in-Product-package signed synthetic effect after fsynced reservation (real execution pending).
2. Unknown callback result, simulated reopened journal, no blind retry.
3. Independently signed synthetic receipt, verified confirmed status.
4. Forged provider receipt rejected.
5. Request ID reuse across tenants denied and claim mismatch fails closed.
6. Truncated journal record rejected.
7. Eight concurrent handles on one journal, one winner.
8. Old Owner denied by authentic `Manager.Effect` after signed V2 handoff (real execution pending).
9. Actual child process abrupt exit after fsync and parent recovery as `RECONCILE_REQUIRED`.

## Known high-risk limitations (NO-GO)

1. This is **TEST-ONLY** code under `*_test.go`. There is no durable ledger in the runtime Product Owner process, no Authority-issued authenticated worker credentials, and no integrated real multi-UID/systemd IPC service.
2. `flock` is cooperative; **same-UID hostile directory writer can replace the lock pathname**. No supervisor-owned, distinct OS UID secure journal/service boundary is established here. The prior Phase32 root-supervisor proof is independent test code, **not deployed**.
3. No transaction across `Manager.Effect`, the journal and any external provider; **exactly-once external delivery is NOT proved**. Result may remain `RECONCILE_REQUIRED` after crash. No default automatic retry or unsigned manual clear is authorized.
4. Receipt keys are synthetic, not operational or independently held provider credentials. Test-only signed V2 licenses are from disposable in-memory signers.
5. Phase22 simultaneous multiworker liveness remains **OPEN**; previous/phase37 owner tests show fail-closed behavior, not production worker availability. Kernel, UID identity, remote replay, TOCTOU, independent provider epoch and PostgreSQL16 behavior need genuine cross-process/crossrepo E2E.

## Required next user-device evidence (NOT YET RUN)

A clean nonproduction checkout of the exact Phase37 branch and immutable HEAD on Android/Termux `go1.27.2`:

```sh
cd ~/telmivo-license-audit-src
BRANCH=development/lic-a9-phase37-durable-owner-outcome-journal-20261016
git fetch origin "$BRANCH"
git switch --track "origin/$BRANCH"
git status --short
git rev-parse HEAD
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -count=1 -timeout=180s -v -run '^TestLICA9Phase37' ./internal/platform/installationlicense
```

Confirm the branch HEAD first; stop if local modifications or SHA diverge. Mobile successful tests establish real Product **functional behavior**, not Linux `-race` nor provider delivery. Stop after Phase37; no main merge, server, deleted license VPS, deployment or GitHub Actions.

**Phase37 test implementation + local Linux stub race verification: ACCEPTED. Genuine Go1.27 Product Phase37 functional acceptance: PENDING. Production: NO-GO.**
