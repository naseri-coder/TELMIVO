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