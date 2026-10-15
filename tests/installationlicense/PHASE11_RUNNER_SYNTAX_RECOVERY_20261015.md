# TELMIVO LIC-A9 Phase11 — Broken test-runner source repair and fail-closed AST preflight

**2026-10-15 — DEVELOPMENT ONLY / NO PRODUCTION / NO `main` MERGE**

**Disposition:** `PHASE11_RUNNER_DUPLICATE_TAIL_REMOVED` / `AST_GUARD_UNIT_13_PASS` / `REAL_GO127_PG16_E2E_NOT_RUN` / `PRODUCTION_NO_GO`.

## Pinned immutable parents

- Phase10 TELMIVO test runner PR #64 HEAD `1911eed0d081301f5dac28fb4df8d0d61b5ec2a5`, Draft/unmerged.
- The executable hardened Product runtime remains PR #61 SHA `d88fdb2e4d34f18e8f9c8313f73db5b14a077d5a`; **no Product runtime code changed**.
- The Authority candidate remains PR #55 SHA `bfceaef06d41aa185677a796652d17b07979058a`; no Authority files or services changed.
- Immutable reviewed external Product checkout HEAD and Go/PG image SHA256 requirements introduced in Phase10 are preserved.

## Proven Phase10 source blocker (not a hypothetical vulnerability)

Direct GitHub inspection of `tests/installationlicense/run_joint_docker_only_e2e.py` at Phase10 HEAD exposed **two `if __name__ == '__main__'` entries**, with roughly 68 stale invalid lines after the first entrypoint. The trailing text began `, './authority'],` and was followed by an older copy of compiler, Docker and cleanup code. Python cannot parse this source; **none of the earlier mock-source checks or isolated image-ID unit tests established that the complete Phase10 runner was importable or executable**. The previous production readiness decision therefore correctly remains **NO-GO**.

The Phase11 branch deletes **only** the invalid appended tail (~3,624 bytes) and preserves the canonical function and security barriers, leaving precisely one final `if __name__ == '__main__': raise SystemExit(main())` at the end of the file.

## Permanent regression control

Added `phase11_python_source_gate.py` and `test_phase11_python_source_gate.py` under `tests/installationlicense/`:

1. Use `ast.parse` **without importing/executing** source to inspect every `*.py` in the test tooling directory.
2. Require a fixed inventory of the 13 essential Python tools and regression files. Detect missing tools, syntax errors in any file, symlink substitution and unsafe test-source directory.
3. For the actual Docker-only Runner, require **one and only one terminal top-level** Python `if __name__ == '__main__'` guard. This prevents a recurrence even if malicious or accidental trailing text is syntactically valid.
4. Reject before Docker: the Runner performs `audit_python_sources(product / 'tests/installationlicense')` after exact independently reviewed Product HEAD validation and before its Docker context inspection. Test mocks are explicitly adjusted; a new test asserts a syntax gate failure never calls Docker.
5. Update literal test-only source allowlist from **17 to 20** paths, adding exactly the two new AST files and this report; retain exact ancestor, clean worktree and file-diff equality guards.

## Actual executed evidence vs not executed

Executed in the isolated model-owned local Python 3.13.5 environment, using two newly committed files with **byte-for-byte matching Git object hashes**:

- `phase11_python_source_gate.py`: Git blob `5030924f3731e45f6b1d88322deeca3a25ceb9d0`.
- `test_phase11_python_source_gate.py`: Git blob `c919ad1e7428f87c92fcc61436216778a5be20c1`.
- Command: `PYTHONDONTWRITEBYTECODE=1 python3 -B -m unittest -v test_phase11_python_source_gate`.
- **13/13 PASS, zero failures.** Includes reproducing the exact malformed `, './authority'],` tail; a valid but unauthorized statement after `__main__`; duplicate/missing main guards; invalid syntax in other tests; missing/symlinked files; safe Unicode and read-only refusal.
- Local test log SHA256: `1153273d580e0d70fc0ad07d21f1a10e1e70537920b9b1f218cac6da3a852258`.
- GitHub connector also verified Phase11's Runner now has a single terminal entrypoint, no stale tail, and AST gate is ordered before Docker. This is a static check; **the full exact private Branch Python suite was NOT_RUN** because no complete private checkout is mounted in the model-owned local container.
- Actual Phase11 Source Gate running *over every final GitHub test Python file* is likewise **NOT_RUN** until a checked-out private Product tree is available locally.
- The genuine eight-Go-test joint client/Authority run with Go1.27.2, TLS and PostgreSQL16 remains **NOT_RUN**. Docker is unavailable in the isolated container. No placeholder reports are labelled actual E2E.

## Follow-up acceptance on a separately approved disposable host

1. Exact Authority PR55 and Phase11 Product clean Git checkouts, local trust anchor / manifest and reviewed Go/PG image IDs; no actual TELMIVO server paths, production secrets or remote Docker engine.
2. Run `python3 -B tests/installationlicense/phase11_python_source_gate.py --test-source-dir tests/installationlicense`; demand exit 0 (syntax-only, not permission to release).
3. Run `PYTHONDONTWRITEBYTECODE=1 python3 -B -m unittest discover -s tests/installationlicense -p 'test_*.py' -v`; repair any unit mock/compatibility failures inside exact test scope before proceeding.
4. Preload **independently reviewed** module caches and immutable Docker images, then run the pinned Docker-only eight-test Go1.27.2 + PG16 E2E using reviewed exact Product HEAD and image IDs. Require all eight individual `run` and `pass` JSON events and cleanup.
5. Independently review source transformations, PostgreSQL configuration, host network isolation and supply-chain policy; full user-product+Authority security/race fault matrix, signed independent recovery, production installer and release signing remain separate mandatory gates.

**No GitHub Actions were triggered; no `main` merge, production DB write, Terraform, deploy, server access, private owner key, Telegram operation, release, Go build or Docker container was used.** This phase corrects the known Python test-runner blocker, but is **not** the final licensing product acceptance.
