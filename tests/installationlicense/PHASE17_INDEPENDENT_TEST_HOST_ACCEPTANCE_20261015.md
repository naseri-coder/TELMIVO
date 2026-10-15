# LIC-A9 Phase17 — Independent test host admission and execution prerequisites

**Date:** 2026-10-15  
**Mode:** Nonproduction host preparation only; no deployment, no production changes.  
**Verdict:** PHASE17_STATIC_HOST_PREFLIGHT_IMPLEMENTED — INDEPENDENT_TEST_HOST_NOT_PROVISIONED — REAL_E2E_NOT_RUN — PRODUCTION_NO_GO.

## Protected source references

- Client Phase16 Draft PR #70: `2f376cf3b916cdbaf4c48e69bc6f23a9a0c3a249`, unmerged.
- Hardened Product PR #61: `d88fdb2e4d34f18e8f9c8313f73db5b14a077d5a`.
- Authority Phase07 Draft PR #55: `bfceaef06d41aa185677a796652d17b07979058a`, unmerged.
- The Phase17 branch only adds one read-only preflight, its 25-unit-test file, this document and updates the existing test-only source inventory. No Production TELMIVO source, SQL, license signer, service, database, `main`, manifest or configuration is changed.

## Why this stage cannot yet certify infrastructure

A read-only inspection of the model-owned Linux container found Debian 13, Python 3.13.5, Go 1.23.2, Git and sufficient minimum memory/storage. But **Docker binary is absent, /var/run/docker.sock is absent**, Podman/PG16 are absent and neither a fresh independent VM nor an approved production-isolated host has been designated. DNS access to GitHub is unavailable locally. This environment is not a qualifying E2E host.

The user-connected remote desktop device `telmivo` is explicitly production and must **not** be used for this phase; the removed Authority VPS is offline. No tool call installed, restarted or deployed anything on either host. Without independent nonproduction infrastructure and independently approved image/cache/source pins, running the eight real Go1.27.2/PG16 E2E cases remains **NOT_RUN**. Lack of an E2E result is not evidence that production licensing works or fails.

## Concrete dedicated test VM specification

1. Owner separately approves a **disposable, isolated** Linux VM on independent infrastructure (Ubuntu 24.04 LTS or Debian 13). Recommend 4 vCPU, 8 GiB RAM, 40 GiB disk; hard minimum enforced by static preflight: 2 vCPU, 4 GiB RAM, 10 GiB free scratch space. No mount, subnet route, remote Docker endpoint, token, database, PostgreSQL volume, signing key or live Telegram bot from Production may be reachable.
2. Install trusted, pinned Docker Engine packages **only on this approved VM**, with the owner reviewing the official [Ubuntu install guide](https://docs.docker.com/engine/install/ubuntu/) or [Debian 13 guide](https://docs.docker.com/engine/install/debian/). The Docker group and Docker Unix socket are effectively host-privileged, so do not grant unreviewed accounts access. No Docker over TCP. Review daemon security separately: https://docs.docker.com/engine/security/.
3. Use two clean, separate authenticated checkouts of the exact Authority and Client commits. Independently approve the **full final Phase17 Client SHA**, not merely the PR61 ancestor. Read-only check `git rev-parse HEAD` and `git status --porcelain --untracked-files=all` in both checkouts, but do not trust the checkout as the only source of its own approval pin.
4. Preload an externally reviewed, content-pinned Go1.27.2 Docker image, PG16 Docker image, and the *complete real* dependency module cache. Preserve independent Docker image SHA256 ID records and reviewed SHA256 cache inventory. The runner must refuse absent/uncertain values and must not fetch missing modules online.
5. Review that Docker is bound only to an authorized local Unix socket. Docker tag or CLI presence alone is **not** evidence that a daemon is safe, a host is isolated from Production, or the actual images are certified. The preflight intentionally never queries the daemon or self-certifies the VM.

## Exact static host preflight

From the **approved** disposable Product checkout run:

    PYTHONDONTWRITEBYTECODE=1 python3 -B \
      tests/installationlicense/phase17_disposable_host_preflight.py \
      --scratch-dir /absolute/disposable/workspace \
      --docker-socket /var/run/docker.sock

The checker examines OS, vCPU/RAM/disk, Python/Git/Docker CLI, real local Unix-socket file/mode, absolute scratch path and **only the names** of selected inherited Production/Docker credential environment variables. No external subprocess, Docker daemon query, service start, remote network call, local credentials read or production host mutation.

Exit code **3** means BLOCKED; exit code **2** means only *STATIC_PREREQUISITES_PRESENT_EXTERNAL_APPROVAL_REQUIRED*, **not approval to start Docker or authorize a release**. The report deliberately excludes hostname, exact address, environment variable values, source secrets and license keys.

This environment's actual preflight result was exit **3**, with exactly:
- `DOCKER_CLI_NOT_INSTALLED`
- `LOCAL_DOCKER_SOCKET_MISSING`

The 25 new local Python tests passed and covered supported/unsupported OS, resource thresholds, absent binaries, missing/world-writable/unsafe Unix sockets, sensitive inherited environment names, byte-safe reporting, false green hosts, disallowed scratch directories and no subprocess/network execution.

## Owner-authorized future E2E sequence (NOT executed in Phase17)

On a truly independent test host after its separate approval:

    PYTHONDONTWRITEBYTECODE=1 python3 -B \
      tests/installationlicense/phase11_python_source_gate.py \
      --test-source-dir tests/installationlicense

    PYTHONDONTWRITEBYTECODE=1 python3 -B -m unittest discover \
      -s tests/installationlicense -p 'test_*.py' -v

    git status --porcelain --untracked-files=all

Any failure, source drift or unexpected modification blocks the next step. Only if the complete Python suite succeeds, the human reviewer separately accepts the Docker isolation/image/cache/host attestations, and verified test-only modules are present, can the **existing** `run_joint_docker_only_e2e.py` be invoked with reviewed `--authority`, `--product`, `--expected-product-head`, `--expected-go-image-id`, `--expected-pg-image-id`, `--reviewed-module-cache`, `--expected-module-cache-sha256` and `--acknowledge-disposable-host`. No placeholder SHA is valid. Demand eight individual authentic Go `run`/ `pass` events, complete PG16 startup and cleanup, and real anti-clone fault evidence. Erase ephemeral test infrastructure only after evidence review.

The full production release also requires independent recovery, live installer verification, signed release authorization and additional security review. None is authorized by this phase.

## Observed Phase17 acceptance matrix

| Gate | Status |
| --- | --- |
| Read-only nonproduction host preflight code | IMPLEMENTED |
| Exact tested source and GitHub file SHA | VERIFIED |
| 25 adversarial unit tests | 25/25 PASS |
| Current model-owned host inspection | BLOCKED: Docker CLI/socket missing |
| Genuine independent VM provisioning | NOT_DONE |
| Human approval of independent host | PENDING |
| Actual private repository checkouts | NOT_READY locally |
| Full checked-out Python suite | NOT_RUN |
| Eight real Go1.27.2/PostgreSQL16 crossrepo E2E | NOT_RUN |
| Signed installer/recovery/production readiness | NO-GO |
| Real server, live data, signing keys or `main` mutations | NONE |

**Stop at Phase17.** Do not mark it as actual infrastructure provisioning or advance to Phase18 until the independent host is designated and its prerequisites are genuinely satisfied.

---

## Phase18 — attempted full Python suite and real Go/PG16 E2E (2026-10-15)

**Decision: BLOCKED, do not certify E2E or advance to release.** This section records actual execution rather than creating more simulated “passed E2E” tests.

**Exact source parents:** TELMIVO Phase17 Draft PR #71 = `36b3fb75e8817908aa0083b84f1509d7142eb801`; hardened Product runtime PR #61 = `d88fdb2e4d34f18e8f9c8313f73db5b14a077d5a`; Authority Phase07 Draft PR #55 = `bfceaef06d41aa185677a796652d17b07979058a`. All remain unmerged. No deployment, build of the real source, production signing, live user DB or main-branch edits took place.

**What was executed in isolated model-owned Linux:**
- The six existing Python unittest groups (Phases 09, 10, 12, 13, 15, 17) were re-executed: **114 tests PASS, 0 failures**. These are real executions of limited local tests, not full commercial client/Authority runtime verification.
- Direct GitHub connector retrieval enabled *16 out of 20* required Python source/tool/test files to be placed in a disposable local checkout. Every one of the 16 exact files matched its GitHub **Git Blob SHA**, and **16/16 passed `ast.parse`**. The four required absent files are `run_joint_docker_only_e2e.py`, `run_joint_postgres_e2e.py`, `test_joint_docker_only_e2e.py` and `test_joint_runner.py`.
- An actual `PYTHONDONTWRITEBYTECODE=1 python3 -B -m unittest discover -s . -p 'test_*.py' -v` on the **incomplete** test checkout **returned exit 1**: 115 discovery results, **114 successful tests and 1 import error** from `test_phase02_authority_pin.py` because `run_joint_postgres_e2e` was not locally materialized. Its real Go/Python runner test cases were therefore **not loaded**, not “passed”.
- The complete Python source inventory gate correctly returned **SOURCE_DENIED / PYTHON_SOURCE_INVENTORY_INCOMPLETE (exit 3)**. It was not bypassed and no valid pass assertion was created.
- Local nonproduction host gate returned **BLOCKED_NONPRODUCTION_HOST_PREREQUISITES (exit 3)** due precisely to `DOCKER_CLI_NOT_INSTALLED` and `LOCAL_DOCKER_SOCKET_MISSING`. Go locally is 1.23.2, not the required Docker Go 1.27.2; genuine PostgreSQL16 is absent.
- The 8 real cross-repository Go1.27.2/PG16 integration cases including lease, restart and anti-clone **NOT_RUN**. Source download via direct GitHub DNS failed locally; the connected GitHub plugin was read-only for source verification. No Production device was used.

**Evidence stored with the response:** `LIC_A9_PHASE18_CURRENT_STATUS_FA.md` and `LIC_A9_PHASE18_CURRENT_EVIDENCE.zip` include the raw 115-discovery run showing the one ImportError, 114-unit recheck log, 16 Git Blob inventory, source gate, host gate and SHA256 manifests. They are **local model-produced evidence**, not CI artifacts or test results from an independent production-equivalent host.

**No test-only executable runtime files changed in this Phase18 PR.** Append this audit only to this already allowlisted document, preserving exactly **32 changed test/docs paths** against hardened PR61 and the Phase17 source-fence integrity. No files outside `tests/installationlicense/`, no `go.mod`, `go.sum`, `internal/`, `deploy/`, DB, secrets or `main` changed.

**Blocking prerequisite before claiming Phase18 complete:** an owner-approved, isolated, independently reviewed disposable host with local Docker Unix socket, clean actual private Product and Authority checkouts at exact SHAs, externally reviewed Go/PG16 image IDs and full module cache. Run full source AST and all Python tests until PASS, then **8 genuine named Go run/pass events and successful PG cleanup**. Only those events can qualify the integration. Continue the security/fault matrix afterward; do not relabel the current partial result as E2E PASS.

**Verdict:** `PHASE18_114_UNIT_PASS` / `PHASE18_PARTIAL_CHECKOUT_16_OF_20_VERIFIED` / `FULL_PYTHON_SUITE_INCOMPLETE_1_IMPORT_ERROR` / `REAL_GO1272_PG16_E2E_NOT_RUN` / `PRODUCTION_NO_GO`.
