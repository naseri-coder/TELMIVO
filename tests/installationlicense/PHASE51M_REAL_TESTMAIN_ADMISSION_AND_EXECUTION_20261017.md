# LIC-A9 Phase51-M — Real Go TestMain admission bypass remediation

Date: 2026-10-17.
**Disposition: REAL SOURCE BUG FIXED / EXACT GO GUARD RACE PASS / FULL PRODUCT GO1.27.2 + SIGNED V2 + POSTGRESQL16 + NATS NOT_RUN / PRODUCTION NO-GO.**

## Lineage, restrictions and deployment state
- Base: TELMIVO [Phase51-L Draft PR #116](https://github.com/naseri-coder/TELMIVO/pull/116), immutable commit `4e69a847d5f8d5eea985b7e58d865fbb5bdcf12c`.
- New branch: `development/lic-a9-phase51m-native-testmain-direct-bypass-20261017`; this phase does **not** merge, run GitHub Actions, change `main`, touch License Authority, use a server, Termux, Production, database or broker, release secret or installer.
- Genuine Product Go1.27.2 toolchain, PostgreSQL16 and NATS are NOT installed in the isolated model execution environment. Installed Go is `go1.23.2 linux/amd64`; `GOTOOLCHAIN=go1.27.2 GOPROXY=off GOSUMDB=off go version` exits 1: `go: download go1.27.2 for linux/amd64: toolchain not available`. No claim that real Product was compiled.

## Reproduced security issue in actual pre-phase source
Phase51-L tightened the external shell launcher `scripts/ci/check-test-target.sh`, but the genuine `tests/integration/native_test.go` `TestMain` *still performed its older two-field URL check internally*. A direct `go test -tags=integration` does not need to execute the shell script. For example, a PostgreSQL URL whose authority hostname is `127.0.0.1` but whose query appends `&host=production.example` passed the old TestMain predicate. No actual remote connection was made, and whether the DB driver would redirect in practice was not tested; this is an admission-policy bypass, not proven remote access.

## Actual source correction

1. New build-tagged `internal/platform/installationlicense/target_admission_integration.go`, real Git blob **`aeb87864048265c1729d4541c9bfcfe6ceb47d83`**, compiles **only under `-tags=integration`**. The pure, side-effect-free `NativeIntegrationTargetAdmission(lookup)` enforces:
   - Both PostgreSQL URLs have explicit literal loopback `127.0.0.1`, dedicated roles `telmivo_runtime`/`telmivo_migration`, fixed port 5432, exact `/telmivo_test`, nonempty credential and exact `?sslmode=disable`; no driver host/service query override.
   - JetStream URL exactly `nats://127.0.0.1:4222`; optional restore DB pair both on port 5433 with dedicated roles.
   - Rejects contradictory inherited normal DB/NATS URLs, wrong TEST environment variables, PGHOST/PORT/DATABASE overrides, PostgreSQL service/hostaddr/options, and pre-set browser/native signer fixture activation flags.
   - Performs **no network calls, writes, signing or DB work**.
2. Genuine Product Integration TestMain in `tests/integration/native_test.go`, updated Git blob **`0aa60d4890b333613db09d7d68d06f741d079f54`**: replaces older URL validation with `installationlicense.NativeIntegrationTargetAdmission(os.LookupEnv)` and **exits before** `os.MkdirTemp` to create ephemeral V2 identity, `InstallRuntime`, `database.Open` or any PG/NATS query on a rejected target.
3. New exact real source `internal/platform/installationlicense/target_admission_integration_test.go`, Git blob **`fc08b3cf4169d5fe5f8b3dc4f3e50dd6e20824ac`**: two positive cases (primary native target, paired restoration target) and 17 adverse cases (query host/service injection, external DB/broker, wrong DB, role, port, missing password, inherited Prod DSNs/environment/PGHOST/PGDATABASE, missing runtime DB, pre-enabled fixture flags), plus nil env lookup and incomplete restore pair. This is **real standard-library-only Go Product source and tests**, not a Manager or LicenseCrypto stub.

## Actually executed, strictly bounded tests
- An isolated Go1.23.2 module with **byte-identical** two new GitHub blobs (Git blob SHA-1 verified), plus one intentionally minimal untagged package file for build-tag selection; tests exercise the real admission implementation. Separately added a local-only **negative control** that precisely reproduces the earlier TestMain URL predicate, confirming it accepts `&host=production.example` while the new actual Go admission rejects that same URL. No network or DB.
- Ran `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off go test -tags=integration -race -count=20 -timeout=90s -v ./...`: **PASS, 120 top-level test-function executions and 340 named negative subtest executions = 460 named passes**, no Go race observed. Raw log SHA256 **`0a01e4cb65f9941d92d4f57f6d3236b81901ffbffcc613c4cfacd3f8888eae1e`**.
- `go vet -tags=integration ./...` **PASS**. `go list` confirmed integration-only target code is **excluded in normal Go build** and included under `-tags=integration`. Reproduction script PASS.
- Source assertions verified **18/18 structural invariants** across six files on the actual GitHub branch, including new TestMain admission happening before key creation, Runtime installation, or DB connection; existing CI test workflow supplies canonical loopback PostgreSQL/restore and NATS TEST URLs.
- Reproducible ZIP `LIC_A9_PHASE51M_TEST_EVIDENCE.zip`, SHA256 **`5d8040fec85b363eeae35154e25c8dc16e4b25d47c1fc3075b322e2c243e27e7`**, contains exact GitHub source files, local-only negative control and build-tag dummy, raw race/vet/toolchain logs and SHA256 manifest; no keys or customers.

## Honest hard blockers / next decision
1. Real Product `go test -tags=integration -race` **on Go1.27.2**, genuine Ed25519 signed V2 Manager fixture and Authority contract, PostgreSQL16 migration 0019, Redis, NATS JetStream broker ACK-loss, Playwright browser flows, two-process/multi-UID, rolling upgrade and durable UNKNOWN reconciliation: **ALL NOT_RUN**.
2. This guard validates **string targets**, not an authenticated physical test database. Even literal `127.0.0.1` could be forwarded by a tunnel to a remote production host. The user has prohibited use of the existing TELMIVO server; only a separately isolated, disposable test namespace with independently proven PG/NATS identity would permit safe native tests. No such environment was available; do not attempt real DB mutations.
3. Do not merge the chained Phase51 Draft PRs, deploy images, enable Actions or claim installer ready. The new target validator removes a concrete TestMain bypass but not the final Go1.27.2 end-to-end admission blockage.
4. If an approved isolated Go1.27.2+PG16+NATS environment becomes available, run existing `scripts/ci/phase51l_disposable_acceptance.sh` with verified TEST-only DSNs and the new in-process gate, and fix any actual real Product compiler/test failures that surface. Do **not** replace this with more synthetic confirmations.

**Verdict:** `PHASE51M_DIRECT_GO_TESTMAIN_BYPASS_FIXED / EXACT_GO_RACE_460_PASS / REAL_PRODUCT_GO1272_SIGNEDV2_PG16_NATS_NOT_RUN / PRODUCTION_NO_GO`.
