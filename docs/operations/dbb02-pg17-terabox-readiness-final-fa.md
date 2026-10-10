# TELMIVO DBB-02 — PostgreSQL 17 & TeraBox official API preflight / گزارش نهایی

**Result: `TELMIVO_DBB02_PG17_ISOLATED_RESTORE_PASS_TERABOX_API_PENDING`**  
**Publication:** development-only branch stacked on TELMIVO Draft PR #45; no merge, deploy, scheduler, upload or production backup.  
**UTC date:** 2026-10-10.

## Baseline

- Repository: `naseri-coder/TELMIVO`.
- Immutable starting commit: `35a15e44e53f34f39d409612096b3aa18521d888` on `feature/db-backup-nonprod-spool-v0.1` (Draft PR #45).
- Main TELMIVO's actual production PostgreSQL version was previously read-only observed as 17.11, database `telmivo`, size 15,038,131 bytes, 117 app tables. **No live database accessed in DBB-02.**
- License Authority is a separate repository/host; this stage does not modify its services, production grants or backup policy.

## Original failure and successful retry

First disposable PostgreSQL17 attempt hit a transient start-up race: `pg_isready` observed the initial temporary server before the PostgreSQL Docker entrypoint finished its one-time initialization. The temp instance was cleaned up; nothing persisted.

Retry required an explicit Docker initialization-complete log marker **and** a successful SQL health probe on the final socket. Isolated `postgres:17-bookworm` local image was already available; **no image pull**.

## Verified test environment

- Test host: existing TELMIVO server `217.60.62.241`, but wholly isolated, short-lived Docker test instance, **NOT** production DB container `telmivo-prod-postgres`.
- Test Docker command restricted with `--network none`, no port publishing, `--cpus=0.50`, `--memory=240m`, `--pids-limit=70`, `--security-opt no-new-privileges`, temporary tmpfs for PostgreSQL data, synthetic fixtures only.
- No secret reads, production environment file access, service restarts or Postgres live connection.
- Verified exact GitHub head at time of execution: `4aba8594f5af44be8066a55a97a596594d8cb704`; after final evidence-doc commit, the child tree will differ only by this report.
- Tested the committed shell file `tests/backup/run_isolated_pg17_drill.sh` on the actual GitHub checkout, not an unreleased reconstruction.
- Runtime PostgreSQL: **17.11**.

## Concrete results

| Test | Result |
|---|---|
| PostgreSQL 17.11 isolated launch | PASS |
| `pg_dump -Fc` synthetic PostgreSQL 17 source | PASS |
| `pg_restore --exit-on-error` to separate empty synthetic PostgreSQL 17 DB | PASS |
| Equality of synthetic user-state and audit-intent data hashes | PASS |
| Synthetic binary payload identity | PASS |
| Custom-format dump nonempty | PASS: 3,702 bytes |
| Entire previous focused Python suite | PASS: **23 / 23** |
| Real age encryption/decryption combined with PostgreSQL 17 | **NOT_RUN** |
| Real TeraBox OAuth, upload, download, limits, object lock | **NOT_RUN** |
| Actual Production database backup / timing performance | **NOT_RUN** |
| Main Production DB access or service mutation | **NONE** |

The dump bytes/hash are non-authoritative fixture evidence, not a production size estimate or signed recovery receipt.

The disposable container and synthetic scratch source checkout were removed after successful tests; no project files on Production checkout changed.

## TeraBox official API readiness

Official source:
https://www.terabox.com/integrations/docs?lang=en

App integration requires separately issued `client_id`, `client_secret`, `private_secret` in addition to an ordinary user account. Token validity and account-specific API/upload domain resolution require secure design. Free-account API eligibility, quota, automation rates, S3 API, immutable versions and deletion-resistant retention **are not verified**. The operator must first create the normal free account and report only nonsensitive actual quota and whether Open Platform access is granted.

**Do not use cookie extraction, unofficial scraped endpoints, a browser password in a script, or real user database files for tests.**

TeraBox is an optional **additional encrypted copy** until all integrity, independence, custody, retention and recovery gates are qualified. Do not downgrade License Authority R5-G/R5-F anti-rollback policy.

## Remaining delivery gates

1. Dedicated PostgreSQL17 **candidate + age** end-to-end isolated drill; this stage proved PG17 `pg_dump/pg_restore` and retains earlier PG16 full crypto proof, **not** a PG17 full crypto proof.
2. Validate secure input and permission handling in the backup candidate before any authorization to read production.
3. Approve non-production TeraBox API onboarding; account alone is not sufficient.
4. Once official client credentials are obtained, separate authorization for a TEST-only ciphertext upload/readback/decrypt and quota evaluation.
5. Design retention with no deletion before qualified independent read-back/restore; main target 10-minute DB-only, License daily only after licensing DB provisioned with independent security journal durability.
6. No scheduler, deploy, merge, production credentials, keys or actual recurring backups until explicitly authorized.

**Production backup and restore qualified: FALSE.**  
**TeraBox connected: FALSE.**  
**Production services changed: FALSE.**
