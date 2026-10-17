# LIC-A9 / A2-02 — Durable journal workspace claims, legacy provenance quarantine and RLS cutover gate

Date: 2026-10-17

**Disposition:** `A2_02_ADDITIVE_JOURNAL_SCHEMA_AND_SOURCE_PATHS_STAGED / SOURCE_EXACT_GO_RACE_30_PASS / PYTHON_AUDIT_100_PASS / AUTHENTICATED_PRODUCER_AND_ROLE_SPLIT_NOT_DONE / REAL_GO127_PG16_NATS_NOT_RUN / PRODUCTION_NO_GO`.

## Immutable lineage and execution restrictions

- Base: [TELMIVO A2-01 Draft PR #120](https://github.com/naseri-coder/TELMIVO/pull/120), exact parent HEAD `8bba3062c754296f6de29110e6da5e10354e24fe`.
- Review branch `development/lic-a9-a2-02-journal-provenance-20261017`.
- **No Merge, no GitHub Actions execution, no Production/Termux/Main TELMIVO server, no actual database mutation, no Authority changes, no operator release key or customer data.**
- Local independent Linux toolchain is Go1.23.2 + Python3; Go1.27.2 / PG16 / NATS / Docker unavailable.

## Source findings and patch

1. **Schema 0020:** new `db/migrations/0020_journal_workspace_provenance.sql`, exact Git blob `68f1c365dbaefcbe56c4a563a2a20680844a9e37`, adds **nullable** `workspace_id uuid REFERENCES workspaces(id)` to `inbox_events` and `dead_letter_events`. Original `(consumer,event_id)` primary keys, immutable deduplication semantics, old nullable rows, original Outbox table and migration 0019 UNKNOWN quarantine are unchanged. No backfill, `NOT NULL`, drop, delete, grant modification, RLS enabling or role switch.
2. **Process paths:** `internal/platform/queue/process.go`, exact Git blob `f0beab020122575281de2e8e8ebaf0eb7dd1f4a9`, records the **claimed** workspace for a syntactically valid event in new Inbox and external-attempt DLQ inserts. On duplicate Inbox key, reads known durable `workspace_id` and denies a differing non-NULL value with `database.ErrForbidden`; a legacy NULL keeps its previous no-replay/dedupe behavior and is NOT filled. If the pre-external DLQ uncertainty fence collides, rejects the attempt without executing the external handler. Successful external confirmation deletes the fence only when workspace matches.
3. **NATS DLQ paths:** `internal/platform/queue/nats.go`, exact Git blob `4c70483d7bdb246d6b6320768328b4b4af5d90ca`, records `workspace_id` for syntactically valid envelopes and **NULL** for malformed. When duplicate DLQ entry exists, only allows broker Term retry if its stored workspace claim equals the current one; does not overwrite historical row or infer a workspace from an untrusted payload. Existing `RuntimeEffect` fencing from A1 remains. No new broker or provider retries.
4. **Critical trust limitation:** a valid UUID shape is *not* authenticated producer ownership. Therefore `workspace_id` here represents the **CLAIMED** envelope workspace, NOT a verified tenant authorization fact. A2 role cutover and final RLS MUST establish independent producer-to-workspace authenticity before using this value to authorize any query.
5. **Audit updated:** `scripts/ci/a2_journal_isolation_audit.py` (Git blob `dad075ca1388aff2f16c76a8afbdf1fb9d060ee9`) now recognizes forward-only `ALTER TABLE ... ADD COLUMN workspace_id` in addition to original `CREATE TABLE`. Its test `test_a2_journal_isolation_audit.py` (blob `71ad72c57dcaaea00bc160b0fc055c0cf1b769b1`) confirms the missing-column warnings are gone while all three missing RLS policies and broad runtime grants still force **NO-GO**.
6. **Staged Product tests:** `db/migrations/a2_02_provenance_migration_test.go` (blob `05bc4ac75cb1f7f414620955f85af831c6ae8a26`): source-only Go contract ensuring each journal independently gains the new column, no dangerous rewrite/RLS. `tests/integration/a2_02_journal_provenance_test.go`: build-tagged native PG16/real signed TEST-V2 test for known-workspace Inbox, same-key deduplication, different-tenant collision denial, legacy NULL Inbox/DLQ preservation, and successful external uncertainty fence lifecycle; **NOT_RUN** on Product.
7. **Future separate worker role:** read-only `infrastructure/postgres/a2_02_readonly_role_inventory.sql` and `infrastructure/postgres/A2_02_ROLE_CUTOVER_CONTRACT.md` specify separate credentials/role admission tests and independent producer authentication. No new login, `SET ROLE`, `GRANT` or `BYPASSRLS` privileges are activated; code startup still uses one shared `DATABASE_URL`. This is intentionally a **partial A2 cutover**, not an insecure mock RLS boundary.

## Real bounded tests executed in isolated model sandbox

- Exact GitHub SQL blob `68f1c365...` and exact Go source-contract test blob `05bc4ac...` replicated byte-for-byte in stdlib-only Go1.23.2 `embed.FS` sandbox: `go test -race -count=30 -timeout=100s -v ./...`: **30/30 PASS**, no race detector issue. `go vet ./...` PASS. Raw log SHA256 **`a204fa7535b53feb9c0e8960204c50cc86bef3b878a3e3224e0eb11ae4fc6d7e`**.
- Deliberate mutation **removed the Dead Letter ADD COLUMN** in an independent copy; the exact Go contract **failed with nonzero exit**, proving it requires both tables independently. Negative-control log SHA256 `957008829687f127ce0d64f4c12958c536387fa4903eb8c5f59fb27feaaa7040`.
- Two exact GitHub Python audit/test blobs, checked byte-identical: `PYTHONDEVMODE=1 python3 -W error -m unittest` **5 tests × 20 processes = 100/100 PASS**. Log SHA256 **`6b66c179055b1e4f845482fad0fe458aba872e5e7850b747db40759f9a8dcc38`**.
- Actual Python audit CLI run on a **synthetic, source-derived three-table excerpt from the real 0001 migration plus exact current grants and actual new 0020**: exit **2**, status `NO_GO_DB_ISOLATION_GAPS`, 0 columns missing, **3 RLS policies still missing**, runtime blanket DML grants still present. This is NOT a full real PG schema, and does not authenticate tenant claims. Log SHA256 `60802d90173eee738390e01c7032bb44aa2f8baabafa7172f20f6bcc39e30b45`.
- Actual GitHub source audit: **28 structural assertions**, including one early false-positive caused by counting the text `NOT NULL` in a **comment**, corrected by inspecting executable SQL lines; effective findings consistent with 0020 and test/source checks. Static checks are not a substitute for PostgreSQL execution.
- Local evidence ZIP `LIC_A9_A2_02_TEST_EVIDENCE.zip` exact SHA256 **`b39bab0125ec87638ee06e7f745560904109f19ffea12d4933423756fc7f6b43`** verified. Contains only **four byte-exact GitHub source files**, a clearly labeled synthetic schema excerpt, current grant excerpt, raw logs, negative control and hashes; full Product queue source not compiled in this offline harness. No private signer, database credentials or customer data.

## Blocking acceptance criteria: NOT_RUN / NO-GO

1. Build all actual Product Go1.27.2 modules including modified Process, NATS queue and genuine signed-V2 Manager and run all package/race suites: **NOT_RUN**.
2. Execute 0020 and native multi-workspace Inbox/DLQ test on a **verified disposable PG16** with real independent tenant and worker credentials, exercise authentic producer binding, valid/malformed envelope and historical migration/revert: **NOT_RUN**.
3. Move API/Worker/Relay to separately authenticated PostgreSQL roles with no inherited role escalation, test allowed and denied privilege matrices and change routing; only then enforce FORCE RLS on journals and revoke blanket grants. **NOT DONE**.
4. Reconcile historical NULL provenance and UNKNOWN broker outcomes independently; never backfill from untrusted event JSON or requeue UNKNOWN based solely on elapsed lease time.
5. Run NATS JetStream ACK-loss, crash/restart, redelivery, broker Term, worker stop/start and end-to-end Product/Authority/Installer tests: **NOT_RUN**.
6. Production release remains forbidden; keep all PRs Draft/unmerged and do not run Actions or deploy.

**Final verdict:** `A2_02_SOURCE_PREPARED_WITH_UNTRUSTED_CLAIM_WARNING / ROLE_SPLIT_RLS_PENDING / PRODUCT_PG16_NATS_GO127_NOT_RUN / PRODUCTION_NO_GO`.
