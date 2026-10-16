# LIC-A9 Phase35 — Termux Go 1.27 Environment Blocker Mitigation (2026-10-16)

**Disposition:** `MOBILE_REAL_PRODUCT_FUNCTIONAL_LANE_AUTHORED` / `LAUNCH_GUARD_13_OF_13_PASS` / `PHASE34_REAL_TEST_EXECUTION_NOT_RUN` / **PRODUCTION_NO_GO**.

### Problem and verified alternative
- Product `go.mod` requires Go **1.27.0**. The isolated execution runtime only has Go **1.23.2** and fails DNS/downloads for Go1.27, so it **cannot** execute the eight real Phase34 Manager tests.
- Official Termux golang package source lists **3:1.27.1** and GitHub CLI has an official community installation path `pkg install gh` for Android. The user's Android/Termux device is therefore a **real non-server execution option**, not an assumed successfully installed machine.
- Read-only import audit against the actual Product source on Phase35 found `internal/platform/installationlicense` and `licensecrypto` directly use the Go standard library only; the targeted package test does not require a live PostgreSQL, Redis, NATS or external provider. The full Product dependency graph and Authority E2E remain separate.
- Go Race Detector does **NOT** officially support `GOOS=android`. The Termux runner is deliberately **functional-only without `-race`**. It does not claim Production acceptance or replace a future isolated Linux Go1.27 race test.

### New code in existing Draft PR #88

1. `tests/installationlicense/phase35_execution_envelope/run_termux_manager_tests.sh`
   - Git blob **`08acb620a492c3f0ae338168d20af7e1f7bf4401`** (exact locally tested bytes).
   - Requires explicit disposable checkout consent, Termux prefix, reviewed immutable full HEAD SHA, clean Phase35 branch, Go >= 1.27, correct Product module, and **three pinned genuine Go source blob hashes** including real `Manager.Effect` implementation and signed V2 fixture.
   - Rejects Production, Actions and external side effects, disables Go network downloads, runs **real** Phase34 signed-V2 Manager tests, then full installationlicense package tests and vet. Captures protected logs and SHA256SUMS. Does NOT change Go module version or run any real provider effect.
2. `tests/installationlicense/phase35_execution_envelope/test_termux_manager_runner.py`
   - Git blob **`e689f44fdac9d4713061c0527ba20eadd1aac67b`** (exact locally tested bytes).
   - **13/13 adversarial Python guard self-tests PASS** using a deliberately **fake Go driver**. Checks reject invalid environment/branch/SHA, Go1.23.2, bad module, dirty checkout and unsafe side effects; mocks expected command dispatch and propagates failure. **These are runner security tests, not the eight Product Go tests.**
   - Independent repeat of all 13 tests: **PASS**; `bash -n`: **PASS**.
   - Repeated test log SHA256: `8b9b08f4a08e05490d8073547993401d6c9aab7ef727a7970e25fb6714033640`.

### Practical on-device workflow (never on Production)

In user's **own** Android Termux, with internet for installing official packages and cloning private repo:

```sh
pkg update -y
pkg install -y golang git gh
gh auth login             # if not yet authenticated; never paste token in chat
go version                # must display go1.27+
cd "$HOME"
gh repo clone naseri-coder/TELMIVO telmivo-license-audit-src
cd telmivo-license-audit-src
git fetch origin development/lic-a9-phase35-offline-go127-execution-gate-20261016
git switch --track origin/development/lic-a9-phase35-offline-go127-execution-gate-20261016
export LIC_A9_DISPOSABLE=YES
export LIC_A9_EXPECT_HEAD=<THE_FINAL_REVIEWED_40_CHAR_PR88_HEAD>
bash tests/installationlicense/phase35_execution_envelope/run_termux_manager_tests.sh
```

Replace the placeholder HEAD with the **actual final SHA** of Draft PR #88 displayed on GitHub; do not type angle brackets literally into a shell. Use a new, clean clone distinct from any Product working directory. The script refuses mismatched SHA. If Go package cache is missing, it refuses downloads and reports a blocker; preparation must be separately reviewed, not silently relaxed. Collect/redact logs before sharing; never share GitHub login secrets.

### Remaining non-waivable gates
- **Eight real Phase34 Product Go tests remain 0/8 NOT_RUN in this chat runtime.** Neither the user phone nor any Go1.27 installation has been operated remotely.
- A passing Termux functional suite would **not** prove Race Detector, full Go1.27 Linux build, real Authority V2 license issuer and PostgreSQL16 E2E, actual Owner IPC-to-Manager connection, remote provider fencing or signed installer.
- No GitHub Actions, Production host, deleted Authority VPS, actual customer effects, server, `main` merge or deployment.

**Obstacle mitigated:** an independent Android Go1.27 execution path exists, is version-pinned, security-tested and checked into existing PR. **Actual Product functional verdict is pending execution on the device; Production NO-GO.**
