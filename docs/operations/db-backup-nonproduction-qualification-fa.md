# TELMIVO — Database backup non-production candidate (FA)

**Status: NON-PRODUCTION TEST CANDIDATE; NOT A DEPLOYMENT, PRODUCTION BACKUP OR TeraBox QUALIFICATION.**

## Scope and boundary

- Project: \`naseri-coder/TELMIVO\`, main production database \`telmivo\` on PostgreSQL **17.11**. Previous read-only observation: **15,038,131 bytes**, **117 application tables**, on 2026-10-10. This was a *database size*, not a measured compressed dump.
- Licensing Authority is a separate private repository/host. Its running PostgreSQL 16/main currently has only the default \`postgres\` database and no observed application tables. No license Production backup was taken.
- Operator's **future target policy**: main TELMIVO **every 10 minutes** (review 5/15m), **database only**; License Authority **daily** after provisioning, with independent durability of security mutations so daily database snapshots do not permit unsafe rollback.
- No media, downloader artifacts, Redis/NATS, app secrets, TLS keys, or host files are included. These may need their own disaster-recovery plan; this candidate does not claim whole-service recovery.
- No credentials, production keys, live data, schedules, cloud accounts or TeraBox integration are included.

## Candidate files

- \`scripts/backup/db_age_spool.py\`: explicit-only local encrypted spool, optional PostgreSQL \`pg_dump -Fc\` streaming to **age** without plaintext dump stored in the spool. \`--source postgres\` is **dangerous and not authorized for Production**. It requires \`--acknowledge-live-read\` *as a technical fence only* and additionally requires a separate operational approval, compliant secret handling and an approved backup recipient. The flag **does not grant permission** by itself. License source blocked.
- \`scripts/backup/estimate_capacity.py\`: offline calculator for 5, 10 or 15 minutes, daily License and a nominal retention-point model. No scheduling, upload or deletion.
- \`tests/backup/test_db_age_spool.py\` and \`tests/backup/test_estimate_capacity.py\`: 23 synthetic tests, including corruption detection, manifest flags, non-overwrite, private paths, input validation and interval arithmetic.

Only local hash/length match is established by the spool. \`manifest.json\` always keeps \`remote_uploaded=false\`, \`external_readback_verified=false\`, \`semantic_restore_qualified=false\` and \`recovery_pin_committed=false\`. Do not mistake these for recovered-system proof.

## New isolated integration evidence — 2026-10-10

Runtime: non-root \`licstage\` on **13.140.91.176**, in a newly created private scratch directory. An isolated, disposable PostgreSQL **16.15** cluster bound **only to a Unix-domain socket** using a nonproduction synthetic \`telmivo\` database; PostgreSQL 16/main service not accessed or modified.

- Exact GitHub candidate commit tested: \`776a17ccbc59dc1f912f3019e0ea70f654381c0f\`.
- Source checked out using an independent \`git clone --depth 1 --single-branch\`. Source SHA verified before execution.
- Actual **pg_dump (PostgreSQL 16.15)** piped through actual **age 1.1.1** by invoking \`db_age_spool.py capture\` with an **isolated local Unix-socket PGHOST**, dummy empty PGPASSFILE and newly created TEST-only age identity. No password or real secret exported.
- Produced **3,025-byte** encrypted fixture, then decrypted with \`age\`, restored into a distinct synthetic \`telmivo_restore\` database, compared fixture-table hashes for synthetic activation/revocation/retirement and pending/finalized intent rows: **PASS**.
- \`db_age_spool.py inspect\` matched ciphertext SHA256 and length: **PASS**.
- Corrupting one ciphertext byte caused **both local SHA256 inspection and actual age authenticated decryption to reject**: **PASS**.
- The GitHub branch's own synthetic test suite: **23/23 PASS**, exit 0.
- Isolated PostgreSQL was stopped by an automatic trap after validation. No real Production PostgreSQL, Telegram, main TELMIVO host, TeraBox, licensing keys or service configuration touched.

### Interpretation

This proves the **specific candidate path on PostgreSQL 16**, actual age authenticated ciphertext, isolated restore and negative tamper rejection. It does **not** prove PostgreSQL 17 compatibility, full production dataset backup/restore, restore completeness of dependencies, real TeraBox integration, or unattended scheduling. A dedicated isolated PostgreSQL 17 drill is still required before proposing deployment. No real backup timer has been enabled.

## TeraBox gate

TeraBox is at most an **extra encrypted copy**, not yet a qualified immutable or independent anti-rollback vault. Official API onboarding, free-account API eligibility, quotas, automated retrieval, exact-version semantics, retention/immutability and independent restoration all remain unqualified. No unofficial cookie automation, plaintext upload or account password in scripts.

## Explicit next-stage gates

1. Review source and secret/execution fences; refine hardening and error-handling, especially a dedicated production-only authorization and operational credential grant. Current source is a test candidate, not installation instructions.
2. Qualify the same candidate with isolated PostgreSQL **17** and actual \`age\` in a fully synthetic environment.
3. Confirm official TeraBox API access and perform a separately authorized encrypted synthetic upload/read-back test.
4. Define safe scheduling (main target 10m, License daily only after DB provisioned) and bounded storage with retention protecting last verified backup. Do not delete until independent read-back and restore qualification exist.
5. Formal release/rollback approval and production read-only backup performance evaluation are separate authorizations.
6. License Authority production anti-rollback and external mutation journal remain mandatory despite daily snapshot preference.

**Final local verdict:** \`TELMIVO_DB_BACKUP_REAL_CRYPTO_ISOLATED_PG16_PASS\`.

**Deployment status:** FALSE. **TeraBox connected:** FALSE. **Production backup enabled:** FALSE.
