# LIC-A9 Phase 2I — PostgreSQL Docker non-root entrypoint compatibility

**Scope:** tests only, offline source verification; no Actions, no production host, no database creation.

## Confirmed compatibility risk

The Phase 2H runner uses the official `postgres:16` image with `--cap-drop ALL` and `--security-opt no-new-privileges`, but previously omitted `--user`. The official PostgreSQL Docker entrypoint runs as root by default and calls `exec gosu postgres` to change UID during startup. With `CAP_SETUID/CAP_SETGID` removed, this default privilege transition can fail even before any test starts.

**Upstream primary references:**
- https://github.com/docker-library/postgres/blob/master/docker-entrypoint.sh — root branch invokes `exec gosu postgres`.
- https://github.com/docker-library/docs/blob/master/postgres/README.md — official `--user` support.
- https://github.com/docker-library/postgres/blob/master/16/bookworm/Dockerfile — PostgreSQL image prepares the postgres-owned data directory.

## Minimal fix and scope

In `tests/installationlicense/run_joint_docker_only_e2e.py`, the PostgreSQL `docker run` options now explicitly include `--user postgres --cap-drop ALL`. This starts as the image's existing unprivileged account, avoiding the root/gosu path while **retaining** the zero-capability fence. Existing `--network none`, no published ports, `no-new-privileges`, disposable image database, SHA provenance gates, six-test evidence checks and verified cleanup remain unchanged.

Added regression checks in `tests/installationlicense/test_joint_docker_only_e2e.py` to assert that the generated PostgreSQL `docker run` invocation has `--user postgres`, `--cap-drop ALL`, network `none`, no `-p`, and no `--privileged`.

## Evidence / limits

- Exact code paths and upstream entrypoint behavior reviewed against authoritative source.
- The regression assertions have been **added** to the repository, but the full current Python suite has **not been executed in this turn**. The change must not be labeled tests-PASS without execution.
- No Docker Engine exists in this independent container; **PostgreSQL container startup and all six real joint E2E tests remain NOT_RUN**.
- Only a disposable independent host may execute the runner. Never use live TELMIVO or an Authority production database.

**Status:** `PHASE2I_NONROOT_DOCKER_ENTRYPOINT_REVIEWED`, `REAL_DOCKER_POSTGRES16_E2E_NOT_RUN`.
