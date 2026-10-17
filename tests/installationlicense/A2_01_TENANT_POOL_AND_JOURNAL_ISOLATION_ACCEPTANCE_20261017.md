# LIC-A9 / A2-01 — Single-connection authorization and journal isolation prerequisites

Date: 2026-10-17.

**Disposition:** `TENANT_TX_AUTH_SOURCE_FIXED / JOURNAL_RLS_SOURCE_NO_GO_CONFIRMED / PYTHON_EXACT_SOURCE_TESTS_100_PASS / NATIVE_GO1272_POSTGRES16_NOT_RUN / PRODUCTION_NO_GO`.

## Immutable lineage and execution constraints

- Parent: TELMIVO [A1 Draft PR #119](https://github.com/naseri-coder/TELMIVO/pull/119), HEAD `db81785811715a68f7043731d1bc9d27c3aed265`.
- Review branch: `development/lic-a9-a2-tenant-auth-same-tx-20261017`.
- No merges to main, production, GitHub Actions, Termux, TELMIVO server, real customers, secrets, provider, PostgreSQL or NATS.
- Current disposable execution environment is Go1.23.2 Linux, with **no Go1.27.2/PG16/NATS/Docker**. Real Product build and native DB integration not attempted.

## Actual deterministic source defect fixed: tenant admin authorization pool starvation

The previous `internal/platform/database/database.go` `Store.Tenant` started a transaction with `s.Pool.BeginTx`, then checked workspace membership with `tx.QueryRow`. If the user was not a workspace member and `adminAllowed=true`, it invoked `s.IsAdmin(ctx,user)`, which calls **`s.Pool.QueryRow` outside the still-open transaction**. The Product's own `PoolConfig` explicitly permits `pool_max_conns=1` (and has a unit test for it). With the only connection held by `Tenant`, the out-of-transaction administrator lookup waits for an unavailable second connection and may only fail on context cancellation/timeout. This is a directly proven resource-allocation defect in source; the actual PostgreSQL stall remains untested here.

Patch in `internal/platform/database/database.go`, Git blob `9ce1a6cc11abff6c57da03e337e5b6a8568a2ed3`:
- One `tx.QueryRow` makes both membership and conditional `adminAllowed AND EXISTS(platform_admins)` decisions on the **same transaction connection** and statement snapshot.
- Explicit `adminAllowed=false` still prevents non-member platform administrators accessing another workspace.
- Fail closed on an error or false authorization, only then call `database.Scope(ctx,tx,workspace)` and application callback.
- No changes to platform admin grants, RLS trust, database schema, migrations, payments or license authority.

New `tests/integration/a2_tenant_single_connection_test.go`, Git blob `0c73981c4201b451282d542f7970bb31513da605`: staged **real PostgreSQL16 Integration test** in the already existing isolated signed-V2 TEST TestMain environment. It explicitly migrates the disposable test DB, opens an additional `pgxpool` with **MaxConns=1**, uses synthetic platform administrator/member/stranger fixtures, checks four allowed/denied scenarios and exact `app.workspace_id` transaction scope. If the old code remained, the non-member admin case would block until its three-second context timeout. **NOT_RUN** in this environment.

## Confirmed RLS and GRANT gaps — intentionally NOT declared fixed

Actual original schema `db/migrations/0001_core.sql` includes:
- `outbox_events` with nullable `workspace_id`;
- `inbox_events` with only `consumer,event_id,created_at` and no `workspace_id`;
- `dead_letter_events` with `consumer,event_id,error_code,payload,created_at` and no `workspace_id`.

Across the 19 source migrations inspected, there is **no ENABLE/FORCE RLS + explicit tenant policy** for these three journal tables. `infrastructure/postgres/grants.sql` grants `SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public` to `telmivo_runtime`. Current Relay and Process run via the shared runtime pool and need both cross-workspace worker operations and tenant-scoped processing. Simply enabling `workspace_id` RLS would strand Relay; adding a GUC `app.is_worker=true` switch **would not establish a privilege boundary**, because the same runtime DB role can set that untrusted GUC. Both paths were rejected as unsafe shortcuts.

New `scripts/ci/a2_journal_isolation_audit.py`, exact Git blob `ec9be4c7458e62e1995e5c4cefa43c4c14278cbb`: read-only standard-library source audit. It reports the three absent journal RLS prerequisites, two missing workspace columns and blanket runtime-table GRANT, emits `NO_GO_DB_ISOLATION_GAPS` with exit status 2; if source prerequisites are later present it outputs only `STRUCTURAL_PRECONDITIONS_ONLY_REVIEW_REQUIRED`, never claims authorized isolation. No schema mutation or DB connection. Its counterpart `scripts/ci/test_a2_journal_isolation_audit.py`, exact Git blob `02a0bb2d85db9e193f0f22959a912699788478d0`, tests both positive structural fixtures and adversarial negative gates.

## Actually executed vs pending

- **Actually executed:** Python unit tests from the **two byte-for-byte exact GitHub source files** verified via Git blob SHA-1. `python3 -W error -m unittest ...`: **5 tests × 20 process repetitions = 100/100 PASS**, no actual DB. Raw log and SHA256 retained.
- **Actually executed:** audit CLI against a **scoped fixture** made of the relevant current GitHub `CREATE TABLE` statements and exact grants.sql: exit **2** and `NO_GO_DB_ISOLATION_GAPS` as required. This SQL fixture is **not the full 19-migration checkout**, and it was NOT executed as SQL.
- **Actually executed:** separate current GitHub source-based structural checks for single transaction connection, authorized-before-Scope sequencing, one connection in staged native test and current schema/grants (5/5 authorization-specific checks pass; 3 RLS gaps found). Source review is not Product compilation.
- **NOT_RUN:** real Product Go1.27.2 package tests/race, native PostgreSQL16 single-pool admission, grants/RLS behavior, worker/relay concurrent cross-tenant queries, migrations, role cutover, NATS, Authority and browser E2E.
- Reproducible bounded local ZIP `LIC_A9_A2_01_TEST_EVIDENCE.zip`, SHA256 **`e3ab03ada521d0c651419a114ec6bbfe2b761579a62a487e4b3224de5d904852`**, includes exact script/test, synthetic schema slice, raw stdout, source disclosure and integrity hashes. It contains no confidential materials.

## Required subsequent work: A2-02 transactionally safe worker/tenant isolation

1. Inventory real reads/writes against all three journal tables including malformed-envelope global DLQ, cross-tenant Relay and historical UNKNOWN records. Determine whether legacy journal provenance can be mapped to workspace without guesses.
2. Stage **separate PostgreSQL roles/credentials** for tenant-facing API and background queue workers; minimum grants and no BYPASSRLS/SET ROLE elevation. Do not rely on `app.worker_mode` session GUC with a shared runtime login.
3. Design forward-only additive migration with workspace provenance for Inbox and Dead Letter. Preserve historical rows, UNKNOWN quarantine and durable idempotency; do not fabricate or silently discard missing workspace IDs.
4. Only with verified worker-role cutover add enforced FORCE RLS tenant policies, a tightly scoped worker policy and revoke blanket DML grants. Validate malformed/deadletter records, old workers stopped, upgrade and rollback behavior.
5. Use disposable PostgreSQL16 seeded with two workspaces and multiple roles: explicitly show cross-tenant reads/writes fail under tenant role, worker Relay/Consume succeed, financial tables cannot be accessed by overprivileged role, pool_max_conns=1 administrator flow succeeds, and data loss/replay never occurs.
6. Independent Go1.27.2 Product and Authority tests, static/grants checks and peer review required before any merge.

**No Go1.27.2 environment was available. This phase intentionally does not enable unsafe journal RLS or grant revocations that would break live workers. Draft-only / PRODUCTION NO-GO.**
