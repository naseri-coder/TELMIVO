# LIC-A9-PHASE-42 — Portable Genuine-Manager Signed Boot & Durable Journal Acceptance

Date: 2026-10-16

**Current disposition (updated after user execution):** `REAL_PRODUCT_ANDROID_GO127_PHASE42_8_OF_8_FUNCTIONAL_PASS` / `STANDALONE_GO123_STUB_RACE_TESTS_PASS` / `LINUX_GO127_ROOT_RACE_NOT_RUN` / **PRODUCTION NO-GO**.

## Phase entry and observed obstacle

- Parent Product Phase41 Draft PR #95 immutable HEAD `e1bbe1e9341119039797badad6058ce968c8204d`.
- Current isolated Linux/amd64 toolchain: `go1.23.2`; environment cannot resolve `go.dev` over DNS, so the genuine Product `go1.27.0` module cannot be compiled under native Linux/amd64 here.
- User already ran genuine Product installation-license tests successfully on **Android/Termux Go1.27.2**, including 10/10 Phase36 and 9/9 Phase37; those runs are evidence for previous phases, **not** this one.
- Phase42 narrows the next functional question: can the actual Product `Manager.Effect` be invoked only after a correctly signed **test-only** fresh boot and correctly scoped request, with persistent no-blind-retry and authentic signed V2 session revocation, using tests that work on Android and Linux without root privileges?
- This is **not a replacement for Phase41's native root Linux/amd64 Go1.27+ Race Detector acceptance**.

## Single new test-only source

`internal/platform/installationlicense/phase42_real_manager_portable_challenge_test.go` contains **eight** named `TestLICA9Phase42*` tests and ephemeral, disposable crypto/journal test helpers. No runtime Product source, root `go.mod`, Authority, provider effects or production service is modified.

Expected exact Git source:
- `git hash-object` Git blob `6725706247cef9c357b1398e9377b3601e1811aa`.
- SHA256 `98d13504b954c46e354dd469ef12020462a2d28f1690064f96cf1222b4dc1f0e`.
- The transfer helper verifies both before staging or committing.

The test gate signs two independently domain-separated Ed25519 objects: (1) a **process-ephemeral random 256-bit** boot grant bound to runtime session, epoch and expiry; (2) a scoped request bound to the signed boot nonce and current test epoch. It uses the existing Phase19 **genuinely signed disposable V2 lease fixture**, Phase37 fsync-backed disposable journal and ultimately **real Product `Manager.Effect` when run in the genuine Product package on Go1.27**. Both successful and unknown calls remain **test-only**; no third-party provider acknowledgment or deployed token issuer exists.

Eight tests:
1. No synthetic effect before a valid signed fresh-boot grant; one permitted callback after real Manager, same-ID replay denied.
2. Valid future FENCE pre-signed for a previous boot denied after simulated Supervisor boot restart.
3. Real signed V2 lease handoff revokes former Manager, even after a prior correct boot.
4. Forged grant, expired signed grant, cross-tenant request, foreign key and altered signed claim all fail closed.
5. Reopened durable journal refuses same-ID retry after unknown outcome, even with fresh nonce and epoch.
6. Twelve simultaneous same-ID signed requests result in one journal reservation and one Manager callback.
7. Cryptographic signature binds boot nonce and runtime session (mutation fails validation).
8. **Reopening the actual Manager with a NEW runtime session and accepting signed generation-2 V2** requires a fresh boot, rejects old signed grants and old requests, permits a new synthetic callback, and rejects stale old Manager dispatch.

The test-only gate does **not** assert a separate OS-UID Worker, true root-owned supervisor or production distributed consensus. Its internal test-only boot epoch is not backed by a production Authority counter; distinct request IDs in an unresolved epoch require future global reconciliation and provider fencing.

## Actual isolated Linux verification

The **byte-exact Phase42 Go file** was compiled in a standalone Go1.23.2 Linux/amd64 test module alongside the previously tested Phase36+37 journal fixtures with a **compatible, explicitly fake STUB Manager**. This is not real Product execution:

- `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -race -count=20 -timeout=120s -run '^TestLICA9Phase42' ./...` — **20/20 complete suites PASS**, eight named tests per suite, no reported races.
- Log SHA256 `511399b7f190b30123fe349d9d60b270da4c7cfb6c6c879f4eec0de3d6bac734`.
- `go vet ./...` **PASS**, all Go source gofmt-clean.
- This **STUB** result checks local test contract and syntax only, not full Ed25519 V2 Product cryptographic validation.

## Genuine Go1.27.2 Android/Termux acceptance — EXECUTED AND PASS

The guarded phase42 transfer package **was executed** by the user in the disposable Product clone `~/telmivo-license-audit-src`. The script:
1. Verify official branch and expected immutable head, clean checkout, Go1.27+ Android/Termux, no Production/Actions, and both Git blob hash/SHA256 for exactly one new Go test file.
2. Run `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -count=1 -timeout=180s -v -run '^TestLICA9Phase42' ./internal/platform/installationlicense` **against the actual Product Manager implementation**, before committing.
3. Reject any skipped/failing tests, run `go vet`, and only then commit/push to the Phase42 branch with no merge.
4. Report the SHA and logs for independent PR evidence review.

**Confirmed real Product functional acceptance:** the user supplied the complete Android/Termux execution transcript (2026-10-16) after running `LIC_A9_DISPOSABLE=YES bash TELMIVO_LIC_A9_PHASE42_APPLY_AND_TEST.sh`.

- **All eight named `TestLICA9Phase42*` Go tests returned PASS**, no reported SKIP or FAIL, final `PASS` and `ok github.com/naseri-coder/TELMIVO/internal/platform/installationlicense 0.273s`.
- `go vet` completed successfully before the script committed and pushed; no reported error.
- GitHub user commit after test: `4dfdf442c065650dbbf9e246ef70fbdd3531f8e5` on `development/lic-a9-phase42-portable-real-manager-boot-journal-20261016`. The terminal printed `LIC_A9_PHASE42_REAL_MANAGER_ANDROID_8_OF_8_PASS_PUSHED`, with evidence directory `/data/data/com.termux/files/home/telmivo-license-audit/lic-a9-phase42-5xZITe`. This path is on the user's phone; the files inside have not been independently opened.
- The GitHub **branch HEAD and PR #96 HEAD** were independently fetched and matched the user commit. Exact Product test file Git blob is `6725706247cef9c357b1398e9377b3601e1811aa`, matching the hash of the locally prevalidated source.
- Scope is authentic Product `Manager.Effect` with synthetically signed fixtures and a disposable journal **on Android Go1.27.2**. This is NOT the native Linux root multi-UID `-race` gate and NOT provider/Authority production certification.

## Release blockers

- Native Linux/amd64 Go1.27 actual Product `Manager.Effect` under real root-owned multi-UID Phase41 bridge with CGO Race Detector: **NOT_RUN**.
- Product complete Go1.27 Linux security hardlink tests, real Authority boot-challenge grant and signer isolation, PostgreSQL16 genuine E2E, externally attested provider idempotency, operational Supervisor install/service and independent audit: **NOT_RUN/NOT_IMPLEMENTED**.
- Test-only Ed25519 keys, scopes and journal are NOT customer identity, live Authority or provider settlement.

**PRODUCTION NO-GO. Keep every PR Draft. No GitHub Actions, Merge, server or deployment. Stop after Phase42.**