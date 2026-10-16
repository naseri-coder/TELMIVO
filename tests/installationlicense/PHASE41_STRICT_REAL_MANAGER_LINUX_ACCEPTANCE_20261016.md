# LIC-A9 Phase41 — Strict Fresh-Boot Fencing in the Real-Manager Linux Test Path

Date: 2026-10-16. **Test-only code complete / isolated Linux Go1.23 stub race tests PASS / real TELMIVO Product Go1.27 Linux root race NOT_RUN / PRODUCTION NO-GO.**

## Precise lineage and scope
- Parent: TELMIVO Phase40 Draft PR #94, HEAD `04e220b05b2ebe5888ab4c90fce52c7278a0ceea`.
- Target development branch: `development/lic-a9-phase41-strict-real-manager-20261016`.
- No runtime Manager implementation, root Product `go.mod`, existing supervisor service, Authority source, Production server, GitHub Actions or `main` merges. All code edits are `*_test.go` plus a standalone offline **test-runner** shell script.

## Exact 4 changed files and SHA provenance

| File | Verified Git blob of local tested bytes |
|---|---|
| Modified `internal/platform/installationlicense/phase39_real_product_supervisor_test.go` | `c35ebd60f8cfe1e5596bae2b42bdd62cf8abf12c` |
| Added `internal/platform/installationlicense/phase41_strict_product_supervisor_test.go` | `84f9af29f71d4b8298cab953f59be65827e77fbd` |
| Added `tests/installationlicense/phase41_execution/run_real_manager_linux41.sh` | `63dc4aa14af1897a379ecc29e0ea03eec2c7ea1d` |
| This report `tests/installationlicense/PHASE41_STRICT_REAL_MANAGER_LINUX_ACCEPTANCE_20261016.md` | see committed GitHub file |

## What this stage fixes — only in the Product **integration test**

Previously Phase39's optional root-owned bridge-to-`Manager.Effect` fixture started the Supervisor with Phase40's fresh-boot challenge **disabled**. Thus Phase39's test source could pass while not validating the recorded **future signed FENCE replay** weakness discovered in Phase40.

- The shared Phase39 helper now offers `phase39FixtureOpenMode(t, strictBoot)`. Existing Phase39 tests keep the same behavior through `phase39FixtureOpen(t)`, which delegates with `strictBoot=false`.
- New Phase41 Linux-tagged tests (**`linux && lic39 && lic41`**) always create the disposable Product integration fixture with `strictBoot=true` **before** registering the optional Effect gate and before Supervisor Start.
- Phase41 has 5 tests. They intend to assert: old/missing boot challenge cannot fence before a real Manager effect; pre-signed future FENCE becomes invalid after Supervisor restart; unknown journal state prevents rearm despite a correct fresh challenge; genuine signed V2 session revocation still blocks Manager.Effect; real outsider Linux UID and signing tampering cannot bypass root-owned worker admission.
- The callback increments an in-memory counter only. Its `RECONCILE_REQUIRED` wire response is **never** a validated external provider completion. No actual commercial operation exists.

## ACTUALLY executed verification — isolated Linux/amd64 Go1.23.2, UID0 and separate kernel UIDs

The model's local sandbox cannot download/access a Go1.27 toolchain (DNS/network unavailable; installed toolchain `go1.23.2 linux/amd64`). To verify the new Product-file test **source syntax and IPC test contract** before commit, its exact bytes were executed in a disposable Go1.23 module with a **compatible, explicitly fake STUB Manager**, plus the exact Phase40 nested bridge files.

- `go test -tags='lic39 lic41' -race -count=1 -timeout=180s -v -run '^TestLICA9Phase41' ./...`: **5/5 PASS**, `ok ... 20.261s`, exit code 0. Full log SHA256 `c804559963f507b34423d953644b3e5e4af0ef242f9accf0bd1b4f797cebb16f`.
- Phase39 compatibility: `go test -tags='lic39 lic41' -race -count=1 -timeout=180s -v -run '^TestLICA9Phase39' ./...`: **7/7 PASS**, including one no-op worker child helper, `ok ... 16.218s`, exit code 0. Log SHA256 `a01872bbfa9eda91b76fe7c234af84f3f884f156290c2ed548e92df760f37b6b`.
- `go vet -tags='lic39 lic41' ./...` **PASS**. All modified/new Go source files are gofmt-clean.
- A combined `-race` invocation displayed PASS for all 12 tests and final `PASS`, but the host execution request timed out without a confirmed tool exit status; **not counted as an independently accepted full-suite completion**.
- Stage40 standalone root multi-UID bridge all-26 `-race` tests PASSED on that earlier immutable Phase40 code; Phase41 does NOT modify that bridge's code.

## Linux Go1.27 real Product acceptance runner — built, NOT executed

`tests/installationlicense/phase41_execution/run_real_manager_linux41.sh` is a standalone CLI gate for a hypothetical **fully isolated disposable Linux/amd64 UID0** test system with Go>=1.27, native CGO Race Detector and full Product clone. It refuses by default unless **both** `LIC_A9_DISPOSABLE=YES` and `LIC_A9_ROOT_LINUX_TEST_APPROVED=YES` are set, refuses Product/Authority Production and Actions, verifies exact reviewed Git HEAD and clean repository and 3 Git source hashes, then executes:
1. Complete Phase38-40 nested bridge `go test -race` under native Linux.
2. Five `TestLICA9Phase41` tests under genuine Product package and Go1.27 with temporary modfile, `-tags=lic39,lic41`; ensures all five PASS and none SKIP.
3. Full Product installationlicense package `go test -race`; rejects any real Product Linux hardlink test SKIP.
4. Product `go vet`; logs SHA256 audit manifest.

The runner was **syntax checked with `bash -n`** and its no-approval fail-closed entry was tested locally. No genuine Go1.27 Linux test has run; the success sentinel must NOT be claimed.

## Unresolved NON-WAIVABLE blockers

- There is **NO actual Go1.27 Linux root `Manager.Effect` execution**. The preceding Product Phase37 Go1.27.2 Android functional PASS is independent and cannot certify `-race` or multi-UID root Linux security.
- Real Authority signed boot-nonce issuance/challenge verification, independent crypto boundary, real multiworker liveness, persistent PostgreSQL16 fencing/outbox, external provider epoch/idempotency, independently signed results, Product real hardlink test Linux Go1.27 and production deployment are NOT implemented/NOT_RUN.
- The Phase40 strict-boot gate remains opt-in in the **standalone test bridge**, now forcibly enabled only by the tagged Phase41 Product integration fixture. It is **NOT enabled in TELMIVO production runtime**. Phase41 is test coverage, not an operational fix.
- No new Production server requested or created. No main merge, GitHub Actions, deployment, live license signer or customer effects.

**Decision: PHASE41_TEST_HARNESS_STRICT_BOOT_AND_STUB_LINUX_RACE_PASS / GENUINE_PRODUCT_GO127_LINUX_ROOT_NOT_RUN / RELEASE_NO_GO. Stop after Phase41.**
