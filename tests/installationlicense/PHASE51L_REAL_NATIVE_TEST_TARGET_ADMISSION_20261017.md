# LIC-A9 Phase51-L — real native test-target bypass fix and Go1.27.2 admission probe

Date: 2026-10-17.
**Disposition: VERIFIED TARGET-ADMISSION SECURITY FIX / REAL GO1.27.2 + V2 + PG16/NATS INTEGRATION NOT_RUN / PRODUCTION NO-GO.**

## Immutable scope

- Branch starts from Phase51-K Product Draft [PR #115](https://github.com/naseri-coder/TELMIVO/pull/115) commit `8fbf0419b67811d8447183ffaf60ad1d1896dfcf`.
- Only Product review branch `development/lic-a9-phase51l-native-runner-hardening-20261017` was changed. No main, merge, GitHub Actions, Termux, server provisioning, Production, real customers, release signer or remote provider access.
- The model's actual independent local runner has Linux/amd64, `go version go1.23.2`, Python standard library, no Go1.27.2, PostgreSQL, nats-server or Docker. Shell DNS for Go toolchain acquisition is unavailable. Do not infer real Product build coverage.

## Actual security defect reproduced against repository's original code

Old `scripts/ci/check-test-target.sh` exact Git blob `5b85f502cb4f4498b1c5347604a27162f835d2e7` checked only `urlparse(...).hostname` against localhost/postgres and `.path == '/telmivo_test'`. It did not validate protocol, role, port or URI query. Therefore it returned **exit 0** when supplied:

`TEST_DATABASE_URL=postgres://telmivo_runtime:synthetic@127.0.0.1:5432/telmivo_test?sslmode=disable&host=production.example`

and an otherwise valid synthetic TEST_MIGRATION_DATABASE_URL. The query-level host override is a **potential misrouting vector**, depending on driver parsing. No production connection or harmful network request was attempted; this is verified gate-admission failure, not evidence a driver actually redirected a connection.

The patched source rejects the same hostile URL (**exit 2**) before inspecting toolchain or attempting any network I/O.

## Genuine branch source changes

1. `scripts/ci/phase51l_test_target.py` source Git blob **`7ccd54e1b21f645d0869e03503b5e3fbf5d54604`**: pure standard-library, no-IO validation of explicit TEST PostgreSQL URLs with **literal 127.0.0.1**, fixed roles `telmivo_runtime` and `telmivo_migration`, exact DB `telmivo_test`, explicit TCP port 5432, nonempty password and exact `sslmode=disable` query. Rejects unknown URL overrides, multiple '@', wrong scheme/hostname, named remote aliases, fragments, production environment, inherited conflicting normal DB/NATS targets and pre-enabled test signing flags. Optional paired 5433 restore URLs are separately checked. Optional NATS URL must equal exact `nats://127.0.0.1:4222`. Synthetic test values are never printed to logs.
2. `scripts/ci/check-test-target.sh` Git blob **`ece16ffe41babc7fc9600e339f53e386e8859085`**: replaces unsafe inline `urlparse` check with an `exec python3` invocation of the audited validator relative to its own script directory. Existing Makefile `test-integration`, `migration-check` and native workflow already invoke this script, so the existing entrypoint is protected without changing or running GitHub Actions.
3. `scripts/ci/test_phase51l_test_target.py` Git blob **`191c2d76ac5d37140aad297aff43fe3c5b1c8239`**: actual Python unittest suite with two allowed scenarios, 17 hostile URL/env modifications, and 2 independently unsafe restore-input scenarios. Checks precise fail-closed behavior.
4. `scripts/ci/phase51l_disposable_acceptance.sh` Git blob **`3c5ab6f045716e8aacdd1f25dcb47a2a1748f2fd`**: a **separate** opt-in independent native Product test runner, not a server provisioner. Requires `LIC_A9_DISPOSABLE=YES`, `TELMIVO_ENV=test`, `TELMIVO_LICENSE_ENVIRONMENT=TEST`, `SYNTHETIC_MODE=true`, exact loopback TEST_NATS_URL, and passes the tightened target guard. It next verifies local **Go1.27.2** exactly with `GOTOOLCHAIN=local`. Only if valid does it check the localhost PostgreSQL/NATS sockets and run `go test -tags=integration -race -count=1` over genuine Product installationlicense and tests/integration packages with local, offline module dependencies. No automatic server creation, Actions usage, license bypass or Production access. Note: use only on a verifiably disposable namespace/host with no forwarding of 127.0.0.1 into production.

## Actually executed on the current container

- All four GitHub branch source files were reconstructed byte-for-byte into the isolated local test directory; Git blob identities matched the exact hashes above.
- Executed **30 independent Python unittest suite repetitions × 4 top-level functions = 120/120 PASS**, with 17 hostile subtests + 2 restore-negative subtests per repetition. Full log SHA256 `be7ca13a6da30c0361597dcba7012a90aec938f61961ebfd1d76b9c267f349db`. `python3 -m compileall` PASS and both POSIX shell files `sh -n` PASS.
- Executed the old GitHub-identical script against the hostile `host=production.example` URL: **exit 0 (incorrectly admitted)**. Executed the new runner against that URL: **exit 2 (correctly rejected)**, log SHA256 `a190947a52933f69887f9ce01bea47992da3ac36b14fcecfb93e70d6ecfe962e`.
- Executed the new native runner with valid loopback synthetic URLs and explicit opt-in: guard **PASS**, followed by immediate deliberate **exit 3** `REAL_PRODUCT_GO1.27.2_NOT_AVAILABLE: go version go1.23.2 linux/amd64`, log SHA256 `ea4c9d0df347c090639c9be1b6e6f7b52b64c051fccd1704ac3989c9e82b1ba6`. Because version admission occurs **before any socket check or tests**, the script made **no PostgreSQL/NATS connections and no migration**.
- Archive `LIC_A9_PHASE51L_TEST_EVIDENCE.zip` exact local SHA256 `7a6ce9ffbd7363617cc5712a302e92302cb4d99801cdf64bcab0b392b35a97a3`: byte-exact source, negative-control and runner logs, reproducible offline test script and SHA manifest. ZIP integrity and reproduction smoke verified. No credentials/signing material.

## Truthful release blockers and limitations

1. **Genuine Product Go1.27.2 Go test/build, real signed-V2 `Manager.Accept`, PostgreSQL16 Migration 0019, NATS JetStream uncertain receipt and worker recovery tests: NOT_RUN**. This container cannot satisfy required toolchain/services. No override was attempted; do not count wrong-version refusal as a passing Product test.
2. Native target validation **cannot prove** that a localhost TCP port is not an SSH tunnel or forwarded Production endpoint. A genuinely disposable, separately verified PG/NATS host/namespace is required before any future native test execution. Never aim tests at an existing server. The runner performs no database identity attestation.
3. More package compiler failures, V2 fixture incompatibilities, migration conflicts or runtime races may become visible only when the actual Go1.27.2 signed V2 and PG/NATS integration executes. Prior source-only/Go1.23 STUB passes are not substitutes. No guesswork or Production access permitted.
4. Phase51-J/K release artifact guards and unsigned candidate bundle checking do not constitute a signed, authentic easy installer. All Phase51 Draft PRs require independent review and explicit owner merge approval.
5. This step fixes a **real exposed test-target admission bypass**, but does **not** close the overall acceptance phase. **No further increase of production readiness** is asserted.

**Verdict: `PHASE51L_NATIVE_TEST_TARGET_BYPASS_FIXED_AND_REPRODUCED / LOCAL_UNIT_120_PASS / REAL_PRODUCT_GO1272_SIGNEDV2_PG_NATS_NOT_RUN / PRODUCTION_NO_GO`.**
