# LIC-A9 Phase 2A — Fixture repair and honest execution boundary

**2026-10-14 — NOT A PRODUCTION OR CROSS-REPO E2E PASS.**

## Work completed on PR47 branch

1. Found and fixed a **real syntax corruption** in `tests/installationlicense/prepare_authority_fixture.py` (duplicate text added during the earlier SHA substitution). The fixture now has a single CLI-gated entry point, explicitly verifies the selected PR32 exact source commit via `git rev-parse --verify`, then uses `git archive` to prepare a TEST-ONLY copy outside both worktrees. Rewrites happen only in that disposable extraction, never in the original Authority source.
2. Aligned `contract_guard.py` with the repaired Python source form `sha = "<40hex>"` and list-form `git archive` invocation. Previously the guard expected a different syntax and would incorrectly reject the fixed preparer.
3. Updated the six synthetic regression cases to the new pinned format. The six isolated Python unittest cases pass in the assistant's local test container.
4. Added `test_fixture_source_integrity.py`: three additional guards check Python syntax, exactly one correct SHA assignment and absence of auto-execution on import.
5. Confirmed from GitHub that Authority M2M doc and TELMIVO client's nine signing fields/routes/header names agree at the **static contract** level. The source proof SHA is PR32 `68cdb6996f85981c36dd20ecb7435ec010351e9b`.

## What is still NOT executed or qualified

- No full clone of both GitHub repositories to a single build environment was performed here.
- The local container has Go 1.23.2, whereas the Authority source README requires Go >=1.25, and no isolated PostgreSQL 16 test instance is available there. The separate live TELMIVO VPS was NOT used as a disposable test runner; License Authority VPS was deleted by owner.
- The old E2E template was built around an earlier LIC-A7 source. Its internal imports, migration semantics, method signatures and mock clock substitutions may be incompatible with PR32; they must be compiled and triaged against the **exact** selected source without editing Production code to fit stale test assumptions.
- The three new fixture integrity checks were added to GitHub and logically inspect AST, but were **not** executed by the official suite in the current turn. The earlier six isolated synthetic tests were executed and passed.
- There are no new production keys, no real Telegram/Drime calls, no server creation, no DB writes and no release/deploy.

## Next safe execution sequence

1. Use an actual disposable x86_64 runner with Go >=1.25, a temporary PostgreSQL 16 and **both GitHub repos** at their exact SHA/branch, with network access only to fetch open-source Go modules and private GitHub source under owner authorization.
2. Run `python3 -m unittest discover -s tests/installationlicense -p 'test_*guard.py' -v` and `python3 -m unittest discover -s tests/installationlicense -p 'test_fixture_source_integrity.py' -v`; execute `contract_guard.py --authority-root <PR32-checkout> --product-root <PR47-checkout>`.
3. Prepare the test-only fixture in a temporary folder, compile using the current Go toolchain, record any PR32 vs legacy mock-API mismatch **without forging PASS**.
4. Resolve incompatibilities in test harness first; run genuine Authority + Product activate/refresh/revoke/offline scenarios against synthetic ephemeral DB. Report actual test count/failures/skips and source SHA.

**Verdict:** `PHASE2A_FIXTURE_SYNTAX_REPAIR_DONE` / `STATIC_CONTRACT_GUARDS_PASS_6_OF_6` / `CURRENT_SOURCE_GO_POSTGRES_E2E_PENDING`.
