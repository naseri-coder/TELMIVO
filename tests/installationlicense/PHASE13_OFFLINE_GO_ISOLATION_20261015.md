# LIC-A9 Phase13 — Offline Go preflight isolation and exact acceptance

**2026-10-15. Nonproduction / Draft only. NO release approval.**

**Disposition:** `PHASE13_OFFLINE_GO_POLICY_UNIT_PASS` / `FULL_PRIVATE_PYTHON_SUITE_NOT_RUN` / `REAL_GO127_PG16_E2E_NOT_RUN` / `PRODUCTION_NO_GO`.

## Source and stage lineage
- Starting TELMIVO Phase12 Draft PR #66: `ec1ee9db229f3c3f24accb973ed55e98ba581c05`.
- Protected hardened TELMIVO client source: PR #61 `d88fdb2e4d34f18e8f9c8313f73db5b14a077d5a`.
- Protected Authority source: PR #55 `bfceaef06d41aa185677a796652d17b07979058a`.
- No mutation to any `main`, installed client Go code, Authority Go code, operator, SQL schema, configuration, deployment, test keys, live bot, customer data or production machine.

## Current environment and blocker

The full authenticated private Product checkout could not be obtained in the model-owned local Linux runtime: network DNS for GitHub is unavailable. Docker, Podman and PostgreSQL 16 executables are absent; host Go is 1.23.2 whereas the disposable Docker test targets Go 1.27.2. The planned **complete checked-out Python suite has NOT_RUN**. The eight real Go1.27.2/PostgreSQL16 cross-repository lease/cloning E2E cases likewise **have NOT_RUN**. No real Telegram, Docker, Production server or database was contacted.

## Verified code defect

The Phase12 Docker-only runner executed its Go-version check, Go compilation and Go-test discovery with Docker `--network bridge` before PostgreSQL startup. Bridge mode can permit network egress and module downloads, contradicting the test-only offline execution objective. Only the final E2E stage explicitly set `GOPROXY=off` and `GOSUMDB=off`. This is a source-policy weakness, **not evidence of actual network access or exfiltration**.

## Changes, all under test-only scope
1. Add `phase13_offline_go_policy.py`, a pure, deterministic policy validator requiring `--network none` for Go version/compile/discovery and only `container:lic-a9-joint-<12 lowercase hex>` for the final Go test. Reject bridge, host, arbitrary container IDs and network/privilege/port-publishing aliases.
2. Require exactly one `--network`, `--pull=never` and exactly one copy each of `GOTOOLCHAIN=local`, `GOWORK=off`, `GOTELEMETRY=off`, `GOPROXY=off` and `GOSUMDB=off`. Reject duplicate, missing, malformed or override aliases (`--network=...`, `--net=...`, `--env=...`, `--cap-add=...`, `--privileged=...`, `--publish=...`).
3. Update `run_joint_docker_only_e2e.py` to validate all constructed Go Docker argv **before subprocess**, and use network `none` for Go-version/compile/discovery; final Go test continues to share only its isolated PostgreSQL network namespace. Explicit immutable image IDs and independent exact client SHA from Phase10 remain mandatory, as do the temporary PG16 DB and cleanup guarantees.
4. Update `test_joint_docker_only_e2e.py` fixture network expectations, and add mock assertions that preflight never uses Docker bridge or an online Go module source. **Full runner/mock suite NOT_RUN** locally without private source checkout.
5. Update `phase11_python_source_gate.py` required inventory and Phase08 exact-change test. Exact Product SHA diff allowlist now includes **24 literal files** under `tests/installationlicense/`; 3 new entries are this report, the offline policy and its new test module. No dynamic prefix waiver was introduced.
6. `test_phase13_offline_go_policy.py`: **26 local Python tests**, including argument/type confusion, extra Docker network and env aliases, unexpected capabilities, mistaken Go proxy settings and published ports.

## Actually executed evidence, independently reproducible

Four Python 3.13.5 unit suites were run on exact byte-identical GitHub Python sources in the model-owned local environment with no Docker or network:

| Suite | Actual result | Executed file Git blob |
| --- | --- | --- |
| Phase09 static host preflight | 9/9 PASS | `cc39702bcc862129457924321508ee877d867058`, `2e88ee643d7c5a41a95a47f1d40b1573a5e1c015` |
| Phase10 immutable Docker image admission | 12/12 PASS | `f75b191520e4f2d3d7275a318daf9aacae3cf511`, `17176e79d5db150cd0231da53baa28dbc457b0fa` |
| Phase12 enhanced executable-main AST | 20/20 PASS | `502d1bea684b9da4b08ef06d5767d6bbc7048d7f`, `6e29936bce55dc67b89b45c751c477e0c9f20b06` |
| **New Phase13 offline Go network policy** | **26/26 PASS** | `f5fe8a34e577edfe0cd83468c156cbc61e6d71ea`, `853ac0106d5843078fd067038c0efdbd945d5cb8` |
| **Total limited local unit test executions** | **67/67 PASS, zero failed** | Log SHA256: `9771f501fc720cfec07fc706067c090941a9d6ac2e0675e165f40c93b8418743` |

Run locally:
```sh
cd tests/installationlicense
PYTHONDONTWRITEBYTECODE=1 python3 -B -m unittest -v \
    test_phase09_host_requirements test_phase10_image_identity \
    test_phase11_python_source_gate test_phase13_offline_go_policy
```

**These 67 are unit-test executions using synthetic inputs, not Go client/Authority E2E tests.** The new guard and unit test files have matching GitHub blobs. GitHub structural verification of the full modified runner is separate and does not establish Python importability or a real PostgreSQL run.

## Remaining mandatory acceptance gates

- Authenticated, **owner-authorized disposable** (not main TELMIVO) Linux environment with clean exact Product and Authority checkouts.
- Independently sourced/pinned `golang:1.27.2-bookworm` and `postgres:16` image IDs and pre-reviewed Go module download/cache provenance. **Preload the necessary module cache before execution**; compile/discovery deliberately cannot access an online module proxy. A missing module must fail, never silently fetch.
- Execute the checked-out full Python unittest suite with bytecode disabled; fix any actual import/mock test failures inside the test-only scope.
- Run the actual eight targeted Go1.27.2/PostgreSQL16 E2E tests, retain individual `run`/`pass` Go JSON records, and verify isolated PostgreSQL cleanup. Then complete the independent LIC-A9 anti-clone fault/race/replay matrix.
- Signed production installer, independent disaster-recovery/custody keeper, security review and explicit human release approval remain separate blocked gates.

**Do not merge, deploy, sign, or report production readiness.** This is an offline test-runner security improvement only.


---

## Phase14 — actual E2E execution decision (2026-10-15)

**NO-GO: `PHASE14_HOST_BLOCKED`; do not claim real Go/PostgreSQL test execution.** This audit is an appendix rather than an additional tracked file so the exact **24-path** Product PR61 source-diff allowlist remains valid without changing code.

**Pinned parents:** Draft PR #67 TELMIVO `14d72d2b0466eed19aef462e5fa0aa8284068474` (still Draft, unmerged), hardened Product PR #61 `d88fdb2e4d34f18e8f9c8313f73db5b14a077d5a`, Authority PR #55 `bfceaef06d41aa185677a796652d17b07979058a`.

**Observed model-owned isolated host:** Python 3.13.5; Git installed; Go 1.23.2. Docker CLI **ABSENT**, Docker socket **ABSENT**, Podman **ABSENT**, PostgreSQL (`postgres`, `initdb`, `psql`) **ABSENT**, `age` **ABSENT**; GitHub DNS **FAILED**. Authenticated complete private Product/Authority checkout **ABSENT**. This environment is explicitly not a designated production Authority host or TELMIVO server. Exact PR67 SHA returned **zero PR-triggered GitHub Actions workflow runs** in the checked query; no CI workflow was launched here.

**Executed instead (not substitute E2E):** the preserved exact-file Phase09/10/12/13 Python policy test suites were rerun on local isolated copies (9+12+20+26 = **67/67 PASS, 0 FAIL**). These are narrow local unit/synthetic tests, already covered in Phase13; no real Go/PG16 integration behavior was executed or certified. Observed log SHA256: `9544127248e79ba369f84c09dc168b2d6586e6ed93487cc2e373048c9a5015b7`. Read-only host preflight evidence SHA256: `606072c5bb1c9a8252180ee1e804233417bd19a99320319c3221b75ec38fb16b`.

**Not executed:** full exact private branch Python unittest suite; Phase11 AST scan over all current private branch files; Go 1.27.2 compilation of real combined Authority/client; Docker PostgreSQL16 startup; 8 real LIC-A8/A9 test `run/pass` events; full lease/clone/replay/fault matrix; independent offsite/last-known-good keeper; Ubuntu live installer and signed release. They all remain **NOT_RUN/NOT_CERTIFIED**.

**Required next real proof:** an independently designated, owner-authorized **disposable nonproduction** Linux workstation with clean exact git checkouts (Authority PR55 SHA and Product final approved Phase14 SHA), local Unix Docker socket, independently approved immutable Go/PG image IDs, preloaded reviewed Go module cache, no production secrets, host/port publication or remote Docker context. Use the existing `phase11_python_source_gate.py` then `PYTHONDONTWRITEBYTECODE=1 python3 -B -m unittest discover -s tests/installationlicense -p 'test_*.py' -v` on a clean checkout; only if PASS, invoke `run_joint_docker_only_e2e.py` with real independently approved source/head/image pins and `--acknowledge-disposable-host`. Record actual all-eight `run/pass` events and PostgreSQL cleanup before release planning. Never invent pins, weaken the offline checks, or run this on the main TELMIVO VPS.

**Phase14 verdict:** `ENVIRONMENT_DIAGNOSIS_COMPLETE` / `LIMITED_LOCAL_UNIT_67_PASS` / `FULL_PYTHON_SUITE_NOT_RUN` / `REAL_CROSSREPO_E2E_NOT_RUN` / `PRODUCTION_NO_GO`.
