# LIC-A9 Phase35 — Android Hardlink Fixture Failure: Security-Preserving Remediation

Date: 2026-10-16

Status: **LOCAL_FIXTURE_HELPER_LINUX_RACE_PASS**, **TERMUX_RUNNER_SELFTEST_14_OF_14_PASS**, **ACTUAL_ANDROID_RETEST_PENDING**, **ANDROID_HARDLINK_SECURITY_PROOF_UNVERIFIED**, **PRODUCTION_NO_GO**.

## Trigger: genuine Android Go1.27.2 Product test output

The user ran the pinned Phase35 Termux runner against exact HEAD `8900b2ec8051f64b8c74cad2c5365cc309df0cfa` on `go version go1.27.2 android/arm64`, in a clean development checkout.

Actual output:
- `real_phase34_manager`: `ok github.com/naseri-coder/TELMIVO/internal/platform/installationlicense 0.256s`. **Real eight Phase34 Manager.Effect signed test cases ran and the targeted package test process passed.** No mock or fake Go driver was involved in this user-run.
- `full_license_package`: **FAILED** (as initially coded). In `hardlink_security_test.go`, five attempted `os.Link` security fixtures for `identity.json`, `state.json`, `highwater.json`, `lock`, `refresh-leader` returned **permission denied** before reaching the actual security assertion.
- `go vet`: **NOT_RUN** because the runner is fail-fast.

**Important:** A failure to *create* a hardlink attack is not proof that `Manager.Effect` or the license file hardlink guard is defective. Conversely, it is also **not proof that licensing hardlink security has passed** on Android. A real Linux hardlink denial test remains mandatory.

## Minimal change; no runtime code modification

Only test code and Termux runner affected; all changes are stacked onto **existing PR #88**, no new PR, no main merge.

1. New `internal/platform/installationlicense/phase35_hardlink_fixture_test.go` (Git blob `ce5218cdc5ecb90fa2adb2e79100ffffaff8609f`): `phase35RequireHardlinkFixture(t,source,alias)` first constructs the **genuine original attack**. If this succeeds, all existing fail-closed runtime assertions still run unchanged. If it fails:
   - Linux and other non-Android targets: **FAIL**, exactly as before.
   - Android with errors other than `EPERM` or `EACCES`: **FAIL**.
   - Android with permission denial: probe `os.Link` between **two unrelated freshly created 0600 file paths on the same Go test filesystem**. If ordinary links work, **FAIL** rather than hiding a discrepancy with the protected licensing file.
   - **ONLY IF** the independent ordinary link is also denied with `EPERM`/`EACCES`: `t.Skipf` with explicit marker `LIC_A9_ANDROID_HARDLINK_FIXTURE_UNAVAILABLE`.
2. Existing `internal/platform/installationlicense/hardlink_security_test.go` (new Git blob `b38bdd9ec0e4ccde3dcc7f048066990b70d57431`): replace the **two fixture-construction sites** with the helper. All five per-file protected-effect hardlink checks and refresh-leader deny checks remain intact after a successfully constructed link. No production implementation changed.
3. Termux runner `tests/installationlicense/phase35_execution_envelope/run_termux_manager_tests.sh` (new Git blob `f672dba9f62c1311f9a282e7b47967dbece18067`): run the **full Product package test suite with `-v`** so explicit skips are in retained logs. If any test skipped, output a **PARTIAL** result, e.g. `LIC_A9_TERMUX_PARTIAL_FUNCTIONAL_PASS_ANDROID_HARDLINK_SKIPPED_NOT_RACE_NOT_PRODUCTION`, and **never** label it unqualified full functional PASS. Still invoke `go vet` only if the full package's actual Go test command succeeded.
4. `tests/installationlicense/phase35_execution_envelope/test_termux_manager_runner.py` (new Git blob `d525f414f1b62055467ef207db71ad8c9d284969`): adds skip-path negative acceptance check; existing fail-closed controls retained.

## Verification genuinely executed

- Linux/amd64 Go1.23.2 disposable standalone local module using the exact new helper Go source: `go test -race -count=1 -v ./...` **PASS**, with a real cross-directory scratch-file Hardlink constructed and verified by `os.SameFile`; `go vet ./...` **PASS**. This checks the helper's ordinary Linux positive execution path but **does not compile or run the full Product Go1.27 package**.
- Bash `bash -n run_termux_manager_tests.sh`: **PASS**.
- Python `test_termux_manager_runner.py`: **14/14 PASS** with mock Go driver, includes the new proof that a skipped security case emits `PARTIAL` instead of full `PASS`. Mock driver runs are **not real Product functional tests**.
- Git blob hashes of the Go helper and runner files were independently checked against locally executed source content using `git hash-object`; SHA verified from GitHub.
- Actual Android re-execution of the updated full suite and actual vet is **PENDING USER DEVICE EXECUTION**, not claimed.

## Explicit remaining risk

The five Android hardlink attack assertions may become **SKIP**, not PASS, because the device kernel/SELinux forbids constructing the attack. The alternative ordinary-source probe is an environment capability observation, not a replacement for the security proof. On a separate **trusted isolated Linux Go1.27+** machine with normal hardlink filesystem semantics, all five hardlink tests must execute and PASS without skips, as part of later Production release acceptance. No Race Detector on `GOOS=android`.

Do not use Product Production server, removed Authority VPS, GitHub Actions, release key or live external effects. Keep PR Draft and Production NO-GO.
