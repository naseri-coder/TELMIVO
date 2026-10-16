# TELMIVO LIC-A9 — Phase08 final cross-repository test-source refreeze

**2026-10-15 / NONPRODUCTION SOURCE PREPARATION ONLY / NO DEPLOY / DO NOT MERGE TO main**

## Intent, immutability and finding

The Phase07 release audit proved that the old Phase02 joint runner was pinned to Authority Phase01 and a pre-hardening Product PR44 source, and its fixture helper silently archived a much older Authority commit. Consequently even a successful old six-plus-two Go/PG16 test result could not certify the actual latest developed code. This phase corrects the source selection while **preserving fail-closed exact-diff controls**. It does not touch any live host or mutate operational Go code.

### Verified source refs

| Area | Reference |
| --- | --- |
| Selected latest Authority runtime / Phase07 PR #55 | `bfceaef06d41aa185677a796652d17b07979058a` |
| Product hardened anti-clone / Phase03 PR #61 | `d88fdb2e4d34f18e8f9c8313f73db5b14a077d5a` |
| Previous Phase02 joint runner PR #60 | `813eca688cfd294160b2746f0f123dc6810a8f98` |
| Previous Phase2L two-Go-test template PR #59 | `11f8d646e6a09ed3e0420b5563ec7bc546575ac5` |
| Older Product PR44 source (no longer candidate) | `3b871473955e6803636bba0e45a843536afb226a` |
| Older archived Authority source (no longer admissible as new fixture) | `68cdb6996f85981c36dd20ecb7435ec010351e9b` |

**Source selection:** This branch begins at exact Product PR61 commit, then imports the reviewed *test-only* files from PR59 and PR60. It is not a merge into either repository's main branch. A new Phase08 checkout HEAD includes these scripts and docs only; no Product `internal/`, `go.mod`, `go.sum`, `cmd/` or deployment bytes are changed relative to Product PR61.

## Corrected architecture

1. `prepare_authority_fixture.py` uses exactly `sha = "bfceaef06d41aa185677a796652d17b07979058a"` for `git archive`; it may no longer extract the older `68cdb...` archive while claiming Phase07 provenance. The tested preparer blob is `816456d387c8e231f1c469c668e01c722e8b5a32`.
2. `run_joint_postgres_e2e.py` now pins selected Authority and Product PR61 exact SHAs. `AUTHORITY_ARCHIVE_SHA` must equal the exact current Authority SHA; `EXPECTED_AUTHORITY_NONRUNTIME_DELTA` is **empty**, so any content change between selected Authority source and fixture archive fails closed.
3. `check_product_checkout` requires an ancestral PR61 commit, a clean worktree, all 11 exact reviewed test/docs paths, and an exact Git blob match for the preparer. Any extra path, missing path, operational Go/runtime mutation, new deploy/config or modified fixture helper fails before Docker execution. No permissive directory-prefix allowance.
4. The prior two cross-repository LIC-A9 Go tests are imported unchanged; `run_joint_docker_only_e2e.py` still requires all eight discovered tests and all eight individual `go test -json` run/pass events, no skip/fail, and disposable Docker+PG16 only. Six legacy tests are **not** equivalent to LIC-A9 anti-clone certification.
5. New `test_phase08_refreeze.py` and refreshed `test_phase02_authority_pin.py` cover explicit source SHA and stale-archive rejection, missing/extra test files, runtime/SQL/deploy drift, dirty checkout, fixture blob tampering and preservation of protected product Go sources.
6. Old Phase02 and Phase2L documentation remains for historical provenance; **this Phase08 document supersedes their source pin selection**, not their previously disclosed NOT_RUN limitations.

The exact expected Phase08 changed-file inventory is the set defined in `EXPECTED_PRODUCT_TEST_DELTA` in `run_joint_postgres_e2e.py`. Final changed-file comparison to PR61 must list exactly those 11 paths, and no others.

## Security separation

- No direct Docker access to TELMIVO production host, no `DOCKER_HOST` remote endpoint, no preexisting database DSN or customer credential environment. The runner enforces a disposable local Unix Docker socket and private test-only PostgreSQL container.
- Go build/test and PG16 container use synthetic credentials; tests must run on **an explicitly authorized isolated host**, not the active TELMIVO VPS.
- The legacy Host-Go runner is deprecated for LIC-A9 security proof. The current Docker-only runner is the **intended** future executable full eight-test entrypoint. No completion claim is made without actual run/pass event logs, code/compiler diagnostics, and independent state review.
- Initial source fixture selects entire exact Authority HEAD, but the test harness may rewrite module imports and test-only clocks in its **temporary extracted copy**, not in the actual Authority source. These fixture transformations themselves must be independently reviewed against the current Authority APIs before accepting test results.

## Tests and observed evidence

- GitHub connector checked Phase07 Authority PR55 and Product PR61 exact immutable heads, their unmerged status and current Phase02 PR60/Phase2L PR59 templates.
- Direct GitHub diff on this branch proved the new delta contains only reviewed `tests/installationlicense/` paths. Static source checks for final commit SHA pins, exact clean-worktree/diff fence, pinned fixture hash, unchanged original A9 template, current Authority archive and unchanged Product operational code passed.
- Python synthetic source-pin tests, created for this phase, are **not a substitute for compiled Go**. The complete branch's unittest suite was NOT_RUN. A separately authored independent local mirror of the exact diff/ancestor/clean-worktree algorithm was executed in disposable Git repositories: **13/13 PASS, 0 failures**; local log SHA256 `c26e0f5adab8db26375b743c7dc95b4bbdd1ef920c58ed24d51768626b6ebc8b`. This demonstrates only guard semantics, not complete checked-out source operation.
- The current execution environment has Go `go1.23.2` and no Docker CLI, PostgreSQL16 or authenticated access to the private source through a local Git checkout. **The real Go1.27.2/PG16 cross-repository tests were NOT_RUN**.
- Full Authority Go>=1.25, Product Go1.27.2, operator PG16, two-cluster age restore, authenticated independent keeper and Ubuntu installer/release qualification remain blocked by the same nonproduction infrastructure limits as Phase07.

## Reproducible next authorized isolated-host execution

After **explicit designation of a disposable nonproduction Linux host** and secure checkout of both private repositories (never the TELMIVO production host), make one clean checkout of Authority exact `bfceaef...` and one clean checkout of this final Phase08 Git SHA.

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -B -m unittest discover \
  -s tests/installationlicense -p 'test_*.py' -v

PYTHONDONTWRITEBYTECODE=1 python3 -B \
  tests/installationlicense/run_joint_docker_only_e2e.py \
  --authority /absolute/path/to/pinned-authority \
  --product /absolute/path/to/pinned-phase08-product \
  --acknowledge-disposable-host
```

Verify Docker only via a local Unix socket, use pinned Go1.27.2 and PG16, preserve per-test `run` and `pass` evidence and automatic container cleanup, refuse missing/duplicate/skip/fail Go event records. Then run *full* LIC-A9 fault matrix, Go `-race`, and independent security review. **Do not sign or install a Production release on the strength of these tests alone.**

## Acceptance table

| Gate | Status |
| --- | --- |
| Authority Phase07 exact SHA selected in fixture and both runners | IMPLEMENTED (GitHub verified) |
| Product hardened PR61 source is branch ancestor; no runtime diff | IMPLEMENTED (GitHub verified) |
| Exact 11-file test-only product allowlist + clean worktree | IMPLEMENTED (negative test fixtures committed) |
| Previous stale Authority archive can no longer qualify | IMPLEMENTED |
| Independent local synthetic exact-source Git fence mirror | 13/13 PASS (separate reimplementation) |
| Real complete Python source-fence suite on final branch | NOT_RUN (private checkout inaccessible locally) |
| Real Go1.27.2 + PG16 eight named test run/pass events | NOT_RUN |
| Full Go unit/race and independent anti-clone security fault matrix | NOT_RUN |
| Production/main modifications | NONE |
| Independent release authorization | NO-GO |

**Status:** `PHASE08_SOURCE_REFREEZE_PREPARED`; `PHASE08_REAL_JOINT_E2E_BLOCKED`; `PRODUCTION_NO_GO`. Complete *source-preparation work*, but do **not** mark actual real E2E testing or product publication complete.
