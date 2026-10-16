# LIC-A9 Phase51-N — Explicit signed TEST identity for native acceptance (DRAFT / NO-GO)

Date: 2026-10-17.

**Disposition: SECURITY REGRESSION FIXED IN SOURCE; VERIFIED EXACT-BYTE OFFLINE Go/Python TESTS PASS; FULL PRODUCT GO1.27.2, REAL SIGNED V2, PostgreSQL16, NATS, PLAYWRIGHT NOT_RUN; PRODUCTION NO-GO.**

## Exact starting point and restrictions

- Parent: [TELMIVO LIC-A9 Phase51-M Draft PR #117](https://github.com/naseri-coder/TELMIVO/pull/117), immutable parent HEAD `0749e50c675ec03e7c08bfcc46073e599dc0b0c4`.
- Review branch: `development/lic-a9-phase51n-browser-native-compile-recovery-20261017`.
- No Merge, change to Product/Authority main, GitHub Actions execution, Termux, real production license secrets, live server, remote database, broker or provider action. Native workflow source was updated as a **candidate only** and **NOT executed**.
- In this isolated model sandbox the actual installed Go toolchain is `go1.23.2 linux/amd64`; `Go1.27.2` cannot be downloaded here because DNS is unavailable. Docker, PG16, NATS Server unavailable locally.

## Reproducible security finding (not a bogus compile claim)

The Phase51-M in-process `NativeIntegrationTargetAdmission` required correctly formatted loopback PostgreSQL/NATS URLs, but treated `TELMIVO_ENV`, `TELMIVO_LICENSE_ENVIRONMENT` and `SYNTHETIC_MODE` as **optional**: it only rejected these values when present and nonmatching. Thus a direct `go test -tags=integration` with all three absent and valid test DSNs passed the admission predicate before a temporary Ed25519 signing root was created. The old code's behavior was reproduced with a **real Go1.23.2 test**: three missing-field subcases failed the intended security assertion (expected red control), with **no DB or network calls**.

An early suspicion that `context` was unused in `phase51i_browser_startup_test.go` was checked and **disproved**: that import is used by `m.Effect(context.Background(),...)`. No unjustified import deletion or compile-fix claim was made.

## Actual source changes in the review branch

1. `internal/platform/installationlicense/target_admission_integration.go` Git blob **`b1aa49a637457edab439b6cb26f6f9f43062a594`**: requires exact explicit `TELMIVO_ENV=test`, `TELMIVO_LICENSE_ENVIRONMENT=TEST`, `SYNTHETIC_MODE=true` before processing DB/NATS URLs or any native test side effects. Missing or empty values reject with `NATIVE_TEST_EXPLICIT_TEST_IDENTITY_REQUIRED`. The existing strict DSN/restore/provider restrictions are preserved.
2. `internal/platform/installationlicense/target_admission_integration_test.go` Git blob **`acb6c7b1c41ec72ef661b251cade296c3c1c68fd`**: positive original real Go source test matrix now declares the three TEST flags. New `phase51n_explicit_test_identity_integration_test.go` Git blob **`42b2dd635a1c4f611f1b56e7d8ece7fc47e0b18d`**: deliberately omits or empties each required field and asserts rejection.
3. `internal/platform/installationlicense/fixture_integration.go` Git blob **`d1aa10db2ab2e7862d02a0ea257bafc107b5fe6e`**: the *native test* ephemeral genuine signed V2 Manager entrypoint now independently refuses missing or wrong explicit TEST, TEST-license or synthetic mode. This is a build-tag-only test signer, not a product production bypass. The distinct browser-test fixture's own strict TEST criteria remain.
4. `internal/platform/installationlicense/phase51h_signed_v2_integration_test.go` Git blob **`56a42ee2a149f71c843c76a6aaaf6996d3597c6c`**: authentic V2 fixture unit-test source now sets `SYNTHETIC_MODE=true` for the positive case and adds explicit missing/wrong TEST-identity negative scenarios. **NOT_RUN against actual Product Manager/codec**.
5. `scripts/ci/phase51l_test_target.py` Git blob **`2f4404474fd99d967e37fcaf07c3855c5b629a7e`**: the preexisting shell-controlled test-target guard now applies the *same* three explicit TEST flags before checking PostgreSQL/NATS. `scripts/ci/test_phase51l_test_target.py` Git blob **`b786eab62ffb52944267d9718abe9ba5a7ae878b`** adjusts its valid synthetic environment. New `scripts/ci/test_phase51n_explicit_test_identity.py` Git blob **`16fd393bc5cf7ba956153bccebb5e8986f662070`** tests missing, empty and mismatched-case values.
6. `.github/workflows/migration-check.yml` Git blob **`6bb51a85ae45718106b4da6ca5a2215f2e486ac0`**: **source-only change**, not an Action run. Future native job explicitly defines the three flags, pins setup-go **exactly 1.27.2**, and uses two disposable `postgres:16-bookworm` test service images instead of Postgres 17, matching planned PG16 acceptance. Existing browser step still builds an ordinary API binary and **does not yet certify the distinct signed TEST-V2 browser API**.

## Actually executed evidence — do not overinterpret

- **Genuine Go validator source**, no LicenseCrypto/Manager stub: byte-exact GitHub Go admission code + tests in a standalone stdlib-only Go1.23.2 module. `go test -tags=integration -race -count=10 -timeout=120s -v ./...`: **60 top-level Go test PASS, 230 named Go subtests PASS**, Race Detector did not identify a race. `go vet -tags=integration` PASS. Full log SHA256 **`f6dc6f2c0d682b8cfec635181826ad820dfc9ffac46dc02c679ffc897d8a3bf6`**.
- **Actual Python gate source**, identical to three GitHub blobs: 20 independent processes running 7 Python unittest methods each: **140/140 PASS**, including explicit-TEST-denied cases, hostile host overrides and restoration validation. Full log SHA256 **`afaec6fb39e2ee9bd962244ac2dd319006ac0df91d32f5247ee1be453dc5f975`**.
- **Original (red) Go negative control:** before the patch, the native validator accepted each absent identity flag (3 intentionally failing assertions). Old admission behavior was observed; this was not a Product E2E test.
- **Actual previously staged Phase51-L Shell runner** with synthetic loopback TEST DSNs and explicit opt-in returned **exit code 3** on the Go1.27.2 exact-toolchain check: `REAL_PRODUCT_GO1.27.2_NOT_AVAILABLE: go version go1.23.2 linux/amd64`. The runner performed no PostgreSQL/NATS connection and no migration.
- 15/15 static structural assertions over nine GitHub files passed, including locked TEST-only signer, target guards, Go1.27.2/PG16 workflow source (not executed), and verbatim blob hashes of executed Go/Python source.
- Archive `LIC_A9_PHASE51N_TEST_EVIDENCE.zip`, exact ZIP SHA256 **`f2d1c9fd3183bf7a78da636510801feb45124fcfe27b56ddb3360608ca7986a5`**, verified with unzip and a reproducible offline script. No production keys or customer data.

## Mandatory blockers to close before merge or release

1. Real TELMIVO **Go1.27.2** full Product build, real signed V2 `Manager.Accept` and all Phase45–51 package/Integration race tests: **NOT_RUN**.
2. Real isolated **PostgreSQL16 Migration0019**, dual-role SQL constraints, PG16 NATS2.11 Redis7 native worker recovery, ACK loss/UNKNOWN reconciliation: **NOT_RUN**.
3. Real separately compiled TEST-only browser V2 API and Playwright journeys: **NOT_RUN**. Existing native workflow's browser step remains incompatible with new signed V2 fixture until future authorized end-to-end adaptation.
4. No user Production environment or main TELMIVO server can be used. An explicit TEST string alone does not prove a physical port is not a forwarded/tunneled production endpoint; verify separate isolated service identity before running schema-changing native tests.
5. No normal-release signer, installer or Docker image has been proven release ready. No real supplier origin signature was validated.

**Verdict:** `LIC_A9_PHASE51N_EXPLICIT_TEST_IDENTITY_PATCHED / EXACT_GO_RACE_290_PASS / PYTHON140_PASS / SIGNED_V2_GO127_PG16_NATS_BROWSER_E2E_NOT_RUN / PRODUCTION_NO_GO`.
