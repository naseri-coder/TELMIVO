# LIC-A9 A2-03 — PostgreSQL per-service identity routing and exclusive credential admission

2026-10-17 · **DRAFT ONLY / NO DEPLOY / NO-GO**

**Verdict:** `A2_03_SOURCE_ROLE_ROUTING_STAGED / ZERO_IO_DSNS_TESTED / AUTHENTICATED_PG_ROLE_CUTOVER_NOT_DONE / PRODUCT_GO1272_PG16_NATS_NOT_RUN / PRODUCTION_NO_GO`

## Provenance, boundaries and lineage

- Parent: TELMIVO [A2-02 Draft PR #121](https://github.com/naseri-coder/TELMIVO/pull/121), exact SHA `9a7b3f97ffea69fb3273e6d3f4ad35ab2c6c20a3`.
- Branch: `development/lic-a9-a2-03-db-role-admission-20261017`. No merge, no Actions run, no changes in `main` or Authority, no real server/Termux/production access and no customer or release private key exposure.
- Core finding: `runtime.Run` previously handed `c.DatabaseURL` to all executables. Simply adding two credentials into the same shared `.env` would expose BOTH secrets to both processes, and switching `app.worker_mode` GUC is NOT an authorization boundary.

## Actual proposed source changes

1. `internal/platform/config/dbrole.go` (byte-exact offline source Git blob **`233ad312c7eedcbb01ee5e3b1a18af5235852d5c`**): maps `api`, `main-bot`, `group-bot`, `downloader-bot`, `customer-bot-runtime` to `telmivo_api`; maps `worker`, `media-worker` to `telmivo_worker`. Strict per-process PostgreSQL DSN validation: explicit username, nonempty password/host/port/database, unambiguous SSL mode and production TLS; rejects libpq driver query URL override for host, hostaddr, user, service, role, options, etc. No network access.
2. `internal/platform/config/config.go`: new `DatabaseRoleSplitRequired` toggled only with explicit `TELMIVO_DB_ROLE_SPLIT=required`; otherwise existing legacy read path remains. In strict mode fail closed if PG ambient environment overrides, another service DB secret, or migration DB credential appears in the same process; partial dedicated-role login with split disabled also rejects.
3. `internal/platform/database/database.go`: new `OpenForExpectedRole(ctx,dsn,expected)` asserts PostgreSQL **server** `current_user` and role flags on **each physical pgxpool AfterConnect**, including reconnects. Rejects wrong role, SUPERUSER, BYPASSRLS, INHERIT, CREATEROLE, CREATEDB, REPLICATION, no-login or ANY role membership that could permit SET ROLE privilege escalation. Error messages do not display credentials. Existing `Open` remains compatible with legacy/test/migration while cutover is staged.
4. `internal/platform/runtime/runtime.go`: validates own DSN at process startup BEFORE loading license or connecting to external services; chooses role-attested database pool only in required split mode. No background fallback to shared `telmivo_runtime` when explicit mode enabled.
5. `compose.production.yaml` in this UNMERGED review branch: tenant-facing processes receive ONLY `TELMIVO_TENANT_ENV_FILE`; worker and media-worker override with ONLY `TELMIVO_WORKER_ENV_FILE`; migration/grants use ONLY `TELMIVO_MIGRATION_ENV_FILE`. Each Compose variable is **required** (fail before service startup). No role credentials stored in Git. **This source change will break any old deployment without three properly provisioned files and roles; that is intentional fail-closed behavior, NOT release approval.**
6. `scripts/production/a2_03_role_env_gate.py` (exact Git blob **`b188a04a0e47e36fecc61844a243f6a061938213`**): offline validation of three disjoint env files using existing `preflight.load_env` (0600), exact role logins, production TLS, distinct passwords, no migration secret in service env or libpq environment override; never prints secrets. `scripts/production/preflight.py` now rejects legacy shared-credential preflight and checks 3 env paths. This checks configuration **only**, not DB privileges.
7. `internal/platform/config/dbrole_test.go` and `scripts/production/test_a2_03_role_env_gate.py`: numerous negative cases (improper role, query override, duplicate SSL, role inheritance, file sharing/permission, secret sharing and migration credential leakage). `tests/integration/a2_03_split_role_admission_test.go` stages native signed TEST V2 + PG16 proof that legacy login cannot impersonate API/Worker or admin roles. **Real Product Integration NOT_RUN**.

## Actually executed, with precise limits

- Exact byte-for-byte Go validator `dbrole.go` (Git SHA above) was copied into a **standalone Go1.23.2 standard-library module** with **local-only, separately authored negative tests**. `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -race -count=20`: PASS. `go vet ./...` PASS. Go raw log SHA256 **`6b132b65b3bcfed094b833e7454760c25ad041c97f50763cb8b8b9b57e412dba`**. This does NOT compile full `config.go`, database pgxpool, Product or native signed V2.
- Actual GitHub Python gate and matching test sources byte-exact verified, with **a clearly labelled locally reconstructed minimal `preflight.load_env` stub** in the isolated test module (NOT full production preflight). Twenty `PYTHONDEVMODE=1 -W error` unittest runs of 5 methods = **100/100 PASS**, including 15 adversarial subcases per iteration. Log SHA256 **`2bc75934c44552a85d0c46e04efe9b51018fe54466942606be59d54e4348f677`**.
- An initial **real test defect** was detected and corrected before acceptance: the env-permission test incorrectly expected `ValueError`, while the existing production preflight contract raises `PreflightError`. New exact GitHub test blob **`81d3a2f68c6b5aeecbfe29dfed44d488c7a1970b`**, matching the passing local test bytes.
- **28/28** static structural assertions on exact current GitHub branch files PASS: mapping, no legacy fallback under explicit split, DB AfterConnect server role checks, separate Compose env_file overrides and production preflight gate. Structural assertions do not constitute runtime PostgreSQL isolation.
- Offline toolchain probe confirms Go `go1.23.2`; requesting `Go1.27.2` exits nonzero `toolchain not available`; Docker, PostgreSQL server/client and NATS server unavailable. No remote DB access.
- Reproducible bounded evidence ZIP `LIC_A9_A2_03_TEST_EVIDENCE.zip`, SHA256 **`9e586ce5337cf8178c5ba20ff728161ad1f43ded422980280e89b786df940835`**, size 8,702 bytes, verified ZIP + per-file integrity manifest. Includes exact tested standalone sources, local test stub **clearly disclosed**, logs and dependency probe; no real secrets.

## Unresolved hard blockers / acceptance for A2-04

1. Actual **PostgreSQL16 roles `telmivo_api`, `telmivo_worker`** have **NOT been created, granted least privileges or connected** to. Server `current_user`, role memberships/SET ROLE denial and real pgxpool AfterConnect are **NOT_RUN**.
2. Existing blanket `GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO telmivo_runtime` still remains; FORCED RLS on three journal tables still **NOT ENABLED**. API/Worker separation is **a source candidate only**, not an independently verified DB least-privilege boundary.
3. Journal `workspace_id` in migration 0020 comes from **syntactically valid envelope claims**, not authenticated producer identity; do NOT use it as a trusted RLS predicate until producer-to-tenant binding independently verified.
4. Genuine Product Go1.27.2 full build/race, signed TEST V2 Manager, multi-workspace PG16 migrations 0020, API vs Worker grants and privilege matrix, NATS JetStream ACK-loss/UNKNOWN recovery, Docker Compose effective env-file resolution, production preflight end-to-end, and rollback: **ALL NOT_RUN**.
5. `compose.production.yaml` now references three **unprovisioned** env files on this Draft branch. The new production preflight deliberately blocks old shared-role installs. **Do not merge or deploy** until passwords, roles, native tests and precise environment cutover have been approved and demonstrably passed.
6. Next: A2-04 must provision **isolated disposable** PostgreSQL16 service-role fixtures (no Production or user Termux), validate SQL grants/role memberships and multi-workspace RLS, and then review source refactors needed for true least privilege. Do not auto-merge stacked Draft PRs.

**Final: `A2_03_CANDIDATE_PREPARED / EFFECTIVE_PG_ROLE_SPLIT_NOT_PROVEN / PRODUCTION_NO_GO`.**
