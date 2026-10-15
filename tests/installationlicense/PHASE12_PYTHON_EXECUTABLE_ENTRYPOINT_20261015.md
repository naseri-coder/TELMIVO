# TELMIVO LIC-A9 Phase12 — Executable Python entrypoint gate and honest test-suite evidence

**2026-10-15 | TEST-ONLY GitHub branch | Draft / NO-GO**

## Frozen lineage and intended task

Parent PR #65 Phase11 `3381ce2c140a6cf9476cab8f9caa91470d3a29ca`. Intended LIC-A9 Authority `bfceaef06d41aa185677a796652d17b07979058a` (Draft #55) and hardened TELMIVO client `d88fdb2e4d34f18e8f9c8313f73db5b14a077d5a` (Draft #61) remain unchanged.

This phase attempted to acquire the complete authenticated private Product checkout and run the full Python source/test suite from PR #65. That checkout is **not available in the local model-owned Linux container**. `git ls-remote https://github.com/naseri-coder/TELMIVO.git HEAD` returned `Could not resolve host: github.com`. Local Docker, podman, PostgreSQL binaries, and Docker socket are also unavailable. Therefore full-branch Python tests and real Go1.27.2/PostgreSQL16 eight-case cross-repository E2E are **NOT_RUN**, not PASS. The user-owned Production TELMIVO VPS was intentionally not accessed.

## New verified source defect

The Phase11 AST syntax gate merely recognized a single terminal `if __name__ == '__main__'` block; a syntactically legal terminal `pass` or `raise SystemExit(0)` could satisfy the gate while never running the Docker E2E entrypoint. Such a false-positive would materially undermine a real E2E acceptance report. No evidence of exploitation was found: this was a provable validator gap, not a production incident.

## Exact scoped repair

1. `phase11_python_source_gate.py` now checks AST semantics of the terminal guard: it must contain **exactly** `raise SystemExit(main())`, with a zero-argument call to a top-level function named `main`, no `else`, no additional statement, no alternate function, no wrong-argument call and no exception cause. Existing syntax checks for all test Python files, single terminal guard, required inventory and symlink rejection remain.
2. `test_phase11_python_source_gate.py` updates the valid synthetic fixture to the canonical executable main and adds **seven** negative checks: inert `pass`, false success exit, wrong function, extra statement, alternative `else`, fake arguments, and missing top-level `main`.
3. `run_joint_postgres_e2e.py` exact Product tested-path allowlist grows from **20** to **21** only by adding this Phase12 documentation. `test_phase08_refreeze.py` now requires exactly 21. No operational `internal/`, `go.mod`, `go.sum`, `deploy/`, SQL, bot, signing or installer paths changed. The Phase11 Runner still calls the enhanced gate before any Docker command.
4. The parsed Python source gate remains **syntax/entrypoint validation only**, not a signature verification, proof of independent runner content, source root trust or proof of executing a real Docker/Go test.

## Tests truly executed on exact GitHub files

Local Python 3.13.5 using isolated synthetic inputs, no subprocess container/service side effect, no Production credentials or database:

| Suite | Actually executed | Source provenance |
| --- | --- | --- |
| Phase09 static host preflight | 9/9 PASS | `cc39702bcc862129457924321508ee877d867058` + `2e88ee643d7c5a41a95a47f1d40b1573a5e1c015` |
| Phase10 Docker image identity | 12/12 PASS | `f75b191520e4f2d3d7275a318daf9aacae3cf511` + `17176e79d5db150cd0231da53baa28dbc457b0fa` |
| Enhanced Phase11 AST executable entrypoint | 20/20 PASS | `502d1bea684b9da4b08ef06d5767d6bbc7048d7f` + `6e29936bce55dc67b89b45c751c477e0c9f20b06` |
| **Total local test executions** | **41/41 PASS** | Git `hash-object` byte-for-byte matched corresponding GitHub file blobs |

Commands executed:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -B -m unittest -v \
  test_phase09_host_requirements \
  test_phase10_image_identity \
  test_phase11_python_source_gate
```

The local combined test log is included in the isolated evidence ZIP. **Do not report these as 41 distinct commercial anti-clone E2E tests; these are actual narrow unit tests, many using synthetic fixtures.**

## Trust boundaries and next genuine acceptance

- Direct GitHub compare must show exactly 21 reviewed test/docs paths against the hardened Product PR61 source, including this new document, without unexpected runtime changes.
- To execute the complete exact private branch Python suite, a clean authenticated Product checkout must be materialized on an **owner-authorized disposable workstation**, not the TELMIVO Production VPS. Validate the exact Product HEAD, run `python3 -B -m compileall -q tests/installationlicense` and `PYTHONDONTWRITEBYTECODE=1 python3 -B -m unittest discover -s tests/installationlicense -p 'test_*.py' -v`, record each failure and make only scoped test fixes in a new Draft PR.
- Run the Go1.27.2 + PostgreSQL16 real joint Docker suite only with independently approved image IDs, clean Authority SHA and exact Product HEAD; require 8 individual genuine `run` / `pass` events, PostgreSQL cleanup, runtime lease/clone verification and no skips or auto-certs.
- Full LIC-A9 race/adversarial security matrix, real independent disaster recovery and signed-production release/installer remain separate **NO-GO** gates.

**Disposition:** `PHASE12_EXECUTABLE_ENTRYPOINT_GAP_CLOSED`; `LOCAL_UNIT_41_PASS`; `FULL_PRIVATE_PYTHON_SUITE_NOT_RUN`; `REAL_CROSSREPO_GO_PG16_E2E_NOT_RUN`; `PRODUCTION_NO_GO`.

Do not merge or deploy; no user server, real database, Production environment, keys, signed release, automation or GitHub Actions were touched.
