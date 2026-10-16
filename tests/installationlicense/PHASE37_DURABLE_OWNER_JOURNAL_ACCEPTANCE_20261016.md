# LIC-A9-PHASE-37 — Durable Owner Request/Outcome Journal (Test-Only)

Date: 2026-10-16

**Disposition:** `PHASE37_SOURCE_COMMITTED_AND_STUB_RACE_TESTS_PASS` / `GO127_REAL_PRODUCT_MANAGER_RETEST_NOT_RUN` / `PRODUCTION_NO_GO`.

## Immutable ancestry and admission prerequisites

- Product parent: Phase36 Draft PR #89 HEAD `168f46a5326ce7ecb5b8e696bad1c95529cb9d47`.
- Authority Phase21 Draft PR #57 HEAD `d6960982e322d4458c7d963bb153f205a1f8fb59`; read-only, unchanged.
- **Real Product Go1.27 on user's Termux Android:** previous exact Phase36 branch at parent SHA ran `go test -count=1 -v -run '^TestLICA9Phase36'` with **10/10 PASS** against actual Product Manager.Effect (user-provided device output; source blob `4a041d1a801661d96a303efdde9545dd30c3a212`), recorded in PR #89 conversation.
- **Phase35 real Android Product tests:** 8/8 real Phase34 Manager tests PASS; entire installationlicense package PASS **with 5 Android hardlink SKIPs** and `go vet` PASS. Android `GOOS=android` does not support the official Go race detector.
- **Linux kernel-only hardlink capability proof:** Phase36 standalone module ran `go test -race -count=30` PASS with actual five hardlink creations; it does **not** run Product's five security assertions.
- **Not met:** a complete Linux Go1.27 Product checkout with five *actual* hardlink assertions and Race Detector, multi-host Owner identity isolation, live Authority signed V2 lease and eight+ PostgreSQL16 crossrepo E2E, real provider fencing. These remain release BLOCKERS, not "waived" prerequisites.

## Exactly three changed source files

| Path | Git blob SHA (byte-matched against actually executed local fixtures) |
| --- | --- |
| Modified `internal/platform/installationlicense/phase36_real_owner_relay_test.go` | `9c0a7e9496a9180aea7716dc6299b22c0c9f3a88` |
| New `internal/platform/installationlicense/phase37_durable_owner_journal_test.go` | `099443c8cb46205ca0c5411ccf34917663f6512e` |
| New `internal/platform/installationlicense/phase37_durable_owner_journal_integration_test.go` | `243c89130a6d646e8cded507c2728a055bfd8626` |

This is entirely **Product _test.go code**: no Product runtime change, database migration, actual Provider call, Product root module change or change to `Manager.Effect`.

## What the nonproduction ledger proves

- Optional Phase37 journal attached to Phase36 test Owner relay writes a **durable reservation BEFORE** calling actual `Manager.Effect`. Phase36's existing in-memory test path remains a separate compatibility branch when no journal is supplied.
- A private 0700, process-owned test directory stores each canonical intent by hashed request ID; `O_EXCL`, `O_NOFOLLOW`, `Fstat(Nlink=1/owner/mode)`, cooperative Linux/Android `flock`, and file+directory fsync fence duplicate reservations across handles. A corrupt/partial record fails closed; no automatic deletion, reset or blind retry.
- The reservation binds full canonical Phase36 signed-claim fields via domain-separated SHA256; a cross-tenant claim cannot reuse the same ID. Two different IDs may coexist.
- An **independent synthetic Ed25519 provider key**, generated at test runtime, may sign an outcome receipt binding request ID, full claim digest, provider ID, and CONFIRMED result. Forged, mismatched and corrupt receipts cannot make an unknown outcome become confirmed. Confirmed requests also remain non-replayable.
- An actual separate test **child process** calls `reserve`, durably syncs it, then exits with `os.Exit(77)`; a later parent inspection sees `RECONCILE_REQUIRED` and cannot reserve again.
- The Owner test relay still checks ephemeral signed test-worker messages; during an active session it invokes `Manager.Effect` only after reservation and limited mock tenant policy. Old Owner after signed V2 handoff remains restricted in real Product tests once run.
- This is **NOT exactly-once external execution**. Same-UID malicious writer can still mutate local journal pathnames; a supervisor-owned distinct-UID filesystem/IPC design (Phase32) is a future deployment dependency. The test relay and test signer are not a real service or independent witness. A crash after reservation before invoking the callback can cause deliberate indefinite reconciliation pending, not automatic retry.

## Actual execution evidence with precise limits

Local disposable Linux/amd64, `go version go1.23.2`, no network/Production, **stub Manager** implementing the same method signatures to compile and execute the byte-identical new tests:

- The **exact three GitHub Go blobs**, combined with only a local stub Manager fixture, passed `go test -race -count=20 -timeout=160s ./...`: **20/20 complete iterations PASS**, with 19 named checked-in phase36/phase37 test functions per suite (**380 test-function executions**), zero races reported.
- `go test -race -count=1 -v -timeout=100s ./...`: **19/19 PASS**.
- `go vet ./...`: **PASS**; `gofmt -l` empty, no formatting problems.
- SHA256 repeated race log: `4a7ab6a6d03a64bfcce5a5a2e994119aa7169ccc21b7c8bbf8187fa875b6b490`.
- SHA256 verbose log: `472d3faa29f1fc908ebb8301225196137392430409700bd4bd14f2ecd441bce5`.
- Separate exploratory **16 Phase37 test functions** (not checked into this PR) were run with `go test -race -count=20` PASS in the same disposable stub setting. **Do not mistake exploratory tests for the nine new checked-in tests, or either stub result for actual Product Go1.27 execution.**

**CRITICAL: 0/9 new genuine Go1.27 Phase37 tests have run against Product Manager at this stage.** The Phase36 10/10 genuine Go1.27 PASS belongs to the prior immutable parent SHA and is independent evidence only.

## Required next acceptance gate on user Termux (NOT_RUN as of this report)

Use a clean, verified Phase37 checkout from GitHub at this PR's immutable HEAD (shown in GitHub after creation), with the user's existing genuine `go1.27.2 android/arm64`:

```sh
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -count=1 -timeout=180s -v -run '^TestLICA9Phase37' ./internal/platform/installationlicense
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -count=1 -timeout=180s -v -run '^TestLICA9Phase36' ./internal/platform/installationlicense
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go vet ./internal/platform/installationlicense
```

Do not run on Product production server or the deleted Authority host. Do not claim `-race` Android compatibility. If local source SHA or Go version differs, stop.

## Release decision

**Phase37 test implementation and local stub/race contract: PASS. Genuine Product functional Phase37: PENDING USER GO1.27 EXECUTION.** Linux full Product hardlink/race, real Authority V2/PG16 E2E, distinct UID supervisor, durable real database/outbox, post-crash independent reconciliation/provider fencing and high-availability multiworker liveness remain **BLOCKED**.

**Production NO-GO. Keep Draft; no `main` merge, no Actions, no server/deployment, no next phase without user request.**
