# TELMIVO LIC-A9 Phase09 — Real joint E2E attempt, local capability audit and PG16 boot-fence

**Date:** 2026-10-15  
**Mode:** isolated local capability diagnosis, test-only GitHub development; NO production or main merge.  
**Final:** `PHASE09_REAL_E2E_BLOCKED_NO_DOCKER`; `PHASE09_PG16_DISPOSABLE_TMPFS_PATCH_PREPARED`; `PRODUCTION_NO_GO`.

## Frozen source candidates

- Phase08 test runner / hardened product: TELMIVO Draft PR #62 `f7ba7f1bc2e8f531786aa2bcb0b13dcd1354e9e6`.
- Product operational source parent: PR #61 `d88fdb2e4d34f18e8f9c8313f73db5b14a077d5a`.
- Authority exact runtime pinned by the runner: Draft PR #55 `bfceaef06d41aa185677a796652d17b07979058a`.
- Product and Authority `main`, production servers, real PostgreSQL, private keys and production bot/DB are unchanged.

## Environment actually inspected

Isolated model-owned Linux 6.18.44 x86_64 working container:
- Go installed: `go1.23.2 linux/amd64`.
- Python installed: `3.13.5`; `git` and `openssl` present.
- `docker`, `podman`, native PostgreSQL server/client (`postgres`, `initdb`, `pg_ctl`, `psql`) and `age`: NOT INSTALLED.
- DNS/network fetch of `proxy.golang.org`: `Could not resolve host`; no container images or toolchain could be downloaded.
- This is **not** the TELMIVO server. No attempt was made to reach its Docker socket, production files or database.
- Important nuance: the **Docker-only runner does not require host Go 1.27.2 or host PostgreSQL**; it requires local, isolated Docker with reviewed `golang:1.27.2-bookworm` and `postgres:16` images and access to exact private Git checkouts. Lack of Docker is the first concrete blocker here.

The exact new `phase09_host_requirements.py` GitHub source blob `ee023fe4491a7451867e83f275d969ed4c628f62` was copied **byte-identically** and executed locally, producing `BLOCKED_NO_DOCKER_CLIENT`, exit code `3`, with no sensitive inherited environment variables detected. This scanner **does not invoke docker/Go/Git/network**, and its verdict never claims E2E or Production authorization.

## Source-level issue discovered and repaired

Prior Docker runner launched the official `postgres:16` image with `--user postgres` and `--cap-drop ALL` and *default* PGDATA, where the default image volume might not be writable by the unprivileged user on first launch. The official PostgreSQL Docker image documents that data directory ownership must match the running user. This was an **untested first-boot risk**, not a confirmed container failure.

The revised `run_joint_docker_only_e2e.py` adds a pure `disposable_pg_command` builder that **rejects unapproved container names**, and binds the unprivileged PostgreSQL process to an explicitly temporary in-container data directory `PGDATA=/tmp/lic-a9-postgres-data`, mounted on `--tmpfs /tmp:rw,nosuid,nodev,size=256m,mode=1777`. The container remains:
- `--user postgres`, `--cap-drop ALL`, `--network none`,
  `--security-opt no-new-privileges`, resource limited and `--rm`.
- No host database DSN, production socket, published port, persistent named volume, or host credential mount.
- The suite retains all eight exact `go test -json` run/pass proof checks and stops the ephemeral PostgreSQL container after testing.
- Existing Go/Authority/client business logic and signing/lease/clone rules are **unchanged**.
- PGDATA/tmps are an argv-level mitigation; the image **must still boot successfully under real Docker to be accepted**.

The existing `test_joint_docker_only_e2e.py` now tests the constructed argv, invalid names and mocked test sequence. Those additional branch tests are **written but NOT_RUN on the actual private checkout** due to its unavailability in the isolated local container.

## Actually executed tests and evidence

1. `phase09_host_requirements.py` and `test_phase09_host_requirements.py` were independently materialized into local files with **exact same Git blob** as their new GitHub counterparts (`ee023fe4491a7451867e83f275d969ed4c628f62`, `2e88ee643d7c5a41a95a47f1d40b1573a5e1c015`). Run: `PYTHONDONTWRITEBYTECODE=1 python3 -B -m unittest discover -s ... -p test_phase09_host_requirements.py -v` → **9/9 PASS**.
2. A separate local ephemeral Go 1.23.2 module generated **eight synthetic test stubs with the same Go test names** as the frozen runner. Real `go test -list` and `go test -json -run` executed: **8/8 individual run/pass events observed**, package pass observed. This tests Go output/event formatting and selection only. It is **not** any of the real Authority/Product eight E2E tests, does not compile their actual runtime code, and does not involve PostgreSQL16.
3. GitHub confirms Phase09 changes are limited to test code, test utilities and documentation. Source SHA/diff confirmation is separate from functional Go/DB E2E.

## Actual joint-E2E gates (NOT_RUN)

- Exact Authority PR55 and Product Phase09 clean Git checkout available in same isolated Linux host: NOT_READY locally.
- Approved local Docker Unix socket, reviewed image builds, non-production host designation: NOT_READY locally.
- Go1.27.2 container compilation against real Authority and hardened Product: NOT_RUN.
- Six legacy A8 + two targeted LIC-A9 tests using real PostgreSQL16/TLS API: **NOT_RUN**.
- Full LIC-A9 clone/restart/replay/race and adverse failures, client `go test -race`, Authority operator PG16 checks: NOT_RUN.
- Two-cluster age disaster restore, independent offsite keeper, signed Production release, Ubuntu full installer: NOT_RUN/NOT_CERTIFIED.

## Next non-production acceptance path

On a **separately approved disposable host**, checkout exact Git SHAs and run the Phase08/Phase09 Python unittest module in isolation, followed by the Docker-only runner with `--acknowledge-disposable-host` and no Production variables. Confirm image provenance, PostgreSQL16 starts with no root privilege under the temporary PGDATA, compile the actual joint fixture, and collect all eight Go individual `run` and `pass` records, package pass and complete automatic DB cleanup. If any of those fail, record exact error and correct only the authorized test-only/compatibility scope. Do not run on main TELMIVO live VPS, merge to `main`, or authorize Production based on this phase.

**Acceptance:** Source-level hardening and isolated preflight unit tests completed; REAL Go1.27.2/PG16 E2E remains `BLOCKED_NO_DOCKER_CLIENT`. No future release certification is inferred.
