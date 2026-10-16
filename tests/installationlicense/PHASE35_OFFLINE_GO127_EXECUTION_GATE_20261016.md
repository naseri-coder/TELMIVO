# LIC-A9 Phase35 — Offline Go1.27 Product Test Execution Envelope

**Date:** 2026-10-16  
**Result:** `OFFLINE_EXECUTION_GUARD_12_OF_12_PASS` / `REAL_PRODUCT_GO127_TESTS_0_OF_8_NOT_RUN` / `PRODUCTION_NO_GO`.

## Verified GitHub ancestry

- Product Phase34 Draft PR #87 HEAD: `9a0cdfbc8b6dd4e11ea8f4e3ad96feba99c58259`.
- License Authority private Phase21 Draft PR #57 HEAD: `d6960982e322d4458c7d963bb153f205a1f8fb59` (read-only; no changes).
- Product `go.mod`: `go 1.27.0`, existing blob SHA `b9a1ccbfc3b7c765cc42017892cc566e6fd62400`.
- Phase34 test in the actual Product package remains on this branch: `internal/platform/installationlicense/phase34_real_manager_effect_test.go` at blob `d32c4dac201fcb78216b633830a6d70e37b7f0b5`. It has 8 authored real-Manager test functions and is **NOT EXECUTED**.
- Product `main`, Authority `main`, existing Go1.27 dependency lockfile, Product runtime, installations, databases and servers remain untouched.

## What was delivered

Only two new files under `tests/installationlicense/phase35_execution_envelope/` plus this acceptance report.

| File | Exact GitHub blob SHA matching local executed bytes |
| --- | --- |
| `run_real_manager_tests.sh` | `35c3260c89764083c704548cf4f42e1a005a56ed` |
| `test_run_real_manager_tests.py` | `3949c980d4f8328236b0caa118e54a732c069ff1` |

The runner is a **fail-closed, opt-in and offline-only test harness**, NOT a cloud host provisioner, Go version installer, pipeline, production integration, license signer, server action or background task.

### Runner contract

- Requires `LIC_A9_DISPOSABLE=YES` as an explicit disposable test-environment opt-in. Refuses `GITHUB_ACTIONS=true`, `APP_ENV=production`, `ENVIRONMENT=production` and `LIC_A9_ALLOW_SIDE_EFFECTS=YES`.
- Demands an existing clean Git checkout of `github.com/naseri-coder/TELMIVO` at a **known reviewed 40-char `LIC_A9_EXPECT_HEAD`**, on a `development/lic-a9-phase35-*` branch. Refuses `main`, detached checkout, dirty state, wrong SHA, missing signed V2 fixture, missing Phase34 test file and root `go.mod` version mismatch.
- Requires Go `1.27+`, Linux, CGO for `-race`. Exports `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=off` to block toolchain/module downloads. Dependency cache must already exist in the disposable test environment; missing modules are a BLOCK, not permission to contact external servers.
- With all gates satisfied, runs the **actual Product** `go test -race` eight-test group, the full `internal/platform/installationlicense` package `go test -race`, then `go vet`. If any fail, the shell exits without an execution PASS marker; all command output is preserved outside the source checkout in a private temporary log directory, with SHA256SUMS on total success.
- The script does not deploy or merge anything; never executes financial, Telegram or exchange provider effects. Existing test fixtures use disposable in-memory Ed25519 signers, not actual Authority Production keys.

### Local execution evidence — 12 real harness tests, **not Product tests**

Local isolated Linux/amd64 container: `go version go1.23.2 linux/amd64`, Python3, GNU Bash, Git. No resolvable external network.

`python3 -B tests/installationlicense/phase35_execution_envelope/test_run_real_manager_tests.py` (executed on the same source subsequently committed) returned **12/12 PASS** in 0.354s, including:

- explicit disposable consent missing → exit 78, **no Go tests started**;
- `GITHUB_ACTIONS=true` and production APP_ENV → exit 78;
- Go1.23.2 reported by either fake Go driver **or the actual local Go1.23.2 compiler** → exit 78, no test dispatch;
- immutable HEAD mismatch, dirty worktree, wrong branch `main`, wrong module, side-effect flag → exit 78;
- simulated Go1.27 driver (explicitly a **mock executable**) verifies that exactly two `go test -race` commands and `go vet` would be invoked in order; **this is NOT a compiled or run Go1.27 Product test**;
- simulated failed Go test returns nonzero and no success marker.

Local verification also executed `bash -n run_real_manager_tests.sh` **PASS** and Python `py_compile` **PASS**. Exact refetched GitHub blobs matched `git hash-object` locally **2/2**.

SHA256 evidence:
- selftest stdout+stderr: `340fd7ea811fec8781e98a44cf12c00991c1a1032dc223ab31b328f8db5464c4`;
- runner source: `97b4e44726e69aa0192788bd4ccfb465da29c6fa064a44d00c4f561b46e1976a`;
- Python self-tests source: `8a39e061a34ff63678173416a2e9de18e0da7b7054931b9e1c26e4824921eb3e`.

**Explicit negative evidence:** Go1.27 is not installed in the available isolated execution container. Searching normal toolchain locations found only Go1.23.2; Github.com/go.dev/proxy.golang.org DNS resolution is unavailable from the local container; there is no permitted Production VPS fallback, and Actions are disallowed. We did **NOT** run the actual eight Product Go1.27 tests or the full Product Go1.27 license-package race/vet suite.

## Future real acceptance procedure (NOT performed now)

In a verified **independent, nonproduction, clean Linux Git checkout** on the exact reviewed Phase35 HEAD with a trusted Go1.27+ toolchain and pre-cached Go modules:

```bash
# Execute only in a disposable nonproduction checkout; NEVER in Production.
export LIC_A9_DISPOSABLE=YES
export LIC_A9_EXPECT_HEAD=<EXACT_REVIEWED_PHASE35_HEAD_SHA>
bash tests/installationlicense/phase35_execution_envelope/run_real_manager_tests.sh
```

Store and review the resulting exact-head/Go-version log and SHA256SUMS. Do not confuse a mock-runner test with real Product Go `PASS`. Never provision the deleted License Authority server or reuse the Product Production server just to make this run.

## Outstanding release-critical blocks

1. **Go1.27 actual Product execution:** **0/8 Phase34 tests run**; same for full package `go test -race` and `go vet`. The new runner does NOT solve missing Go1.27/toolchain/dependency cache.
2. **True broker-to-real-Manager integration:** Phase33 IPC still routes to a mock gate. Phase34 Go tests call the real Manager directly only when run later. No real customer operations are routed through this architecture.
3. **Authority genuine V2 lease / PostgreSQL16:** signed test fixture is **not** the Authority server or its operational cryptography; real cross-repository eight-scenario E2E remains NOT_RUN.
4. **Hostile UID/path and remote provider epoch fencing:** Phase32 is a limited local root-supervisor proof, not systemd/remote witness implementation. No actual payment/Telegram/exchange fencing, domain RBAC or durable PostgreSQL outbox.
5. **Final security review and installer:** not performed; Production is **NO-GO**.

**Phase35 runner authoring, 12 safety self-tests, exact-source SHA: PASS. Genuine Go1.27 functional/race acceptance: BLOCKED. Production NO-GO. STOP after Phase35, keep PR Draft.**
