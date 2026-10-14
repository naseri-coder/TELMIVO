# LIC-A9 Phase 2A — Local execution evidence (2026-10-14)

## Actual execution
- Executed **8 Python contract-guard tests**, all PASS, using a local reconstructed copy of the current contract-checking logic and tests. Python 3.13.5. No production access or live APIs.
- Python syntax compilation of the reconstructed checker and tests passed.
- Tested old Authority SHA refusal, missing route, changed Ed25519 proof order/namespace, missing idempotency header, broken fixture Python syntax and duplicate SHA assignment.

## Explicit limitations
- The actual private GitHub PR47 checkout was not available as a complete clone in this isolated runner; these tests are **local source-logic reproductions**, not authoritative test-suite evidence for the exact GitHub tree.
- Available Go = 1.23.2, but the selected PR32 Authority requires Go >= 1.25.
- A disposable PostgreSQL 16 database was unavailable. Therefore **Go build and cross-repository activation/refresh/offline E2E were NOT executed**.
- Direct GitHub cloning from this runner could not resolve github.com. GitHub connector reads/writes are available but do not represent checkout/build execution.
- An attempt to publish an executable local-only E2E shell runner was stopped by the tool's safety control; it was **not added**.
- Main TELMIVO host/VPS was not touched. No new server, persistent database, live Telegram credentials, production key or deployment was created.

## Verified current state
- PR47 source guard is checked in at tests/installationlicense/contract_guard.py, plus 8-unit-test suite and source-integrity checks.
- PR32 selected Authority SHA is 68cdb6996f85981c36dd20ecb7435ec010351e9b.

**Verdict:** STATIC_LOGIC_TESTS_REPRODUCED_8_OF_8_PASS; EXACT_GITHUB_CHECKOUT_EXECUTION_UNVERIFIED; GO_POSTGRES_CROSSREPO_E2E_NOT_RUN.

## Next action
On a true isolated temporary runner with both private checkouts, Go >= 1.25 and disposable PostgreSQL 16, execute the exact PR47 guard + fixture preparation, compile the legacy fixture against PR32, fix actual API drift in test-only code, then run full E2E. Never turn a missing prerequisite into a pass.
