# TELMIVO LIC-A9 Phase 02 — canonical cross-repo E2E readiness and blocked execution

**Date:** 2026-10-15  
**Mode:** Nonproduction, GitHub-only changes to TELMIVO test tooling, no deployment or live host contact.  
**Verdict:** `PHASE02_SOURCE_PIN_COMPATIBILITY_FIX_PASS`; `REAL_GO1272_POSTGRES16_E2E_NOT_RUN`; `PHASE02_FULL_ACCEPTANCE_BLOCKED`.

## Why Phase02 was blocked before Go could run

The existing Phase2L runner required Authority exact source `939d14800c602dd9c31ff23fdf6e0688a440d4c2`, while Phase01 canonical intake consolidation completed as an unmerged Draft PR #50 at `d4cff712d39e21f934afb0f43ee36a376f1e5345`. The runner's diff gate also allowed exactly two nonruntime files between archival `68cdb6996f85981c36dd20ecb7435ec010351e9b` and the earlier Authority pin. This made the canonical Phase01 head fail before a compiler or database could be invoked.

## Exact source pins / scope

| Item | Immutable selection |
| --- | --- |
| Authority Phase01 Draft PR #50 | `d4cff712d39e21f934afb0f43ee36a376f1e5345` |
| Archived Authority Go fixture source | `68cdb6996f85981c36dd20ecb7435ec010351e9b` |
| Product anti-clone client PR #44 | `3b871473955e6803636bba0e45a843536afb226a` |
| Product Phase2L test-only source parent PR #59 | `11f8d646e6a09ed3e0420b5563ec7bc546575ac5` |
| Product module requirement | Go 1.27.0 (`go.mod`) |
| Docker-only runner image candidate | `golang:1.27.2-bookworm` and `postgres:16` |

The archival Authority fixture remains pinned to its original SHA and may be used only if the delta to the selected Authority head is exactly the eight reviewed nonruntime/test/tool/document paths. Do not change the archive SHA merely to make a runner pass.

## Changes on this draft branch

1. `run_joint_postgres_e2e.py` (also imported by the Docker-only runner) now pins Authority to the exact Phase01 PR #50 head. The default delta allowlist includes exactly the 8 reviewed file paths; it does not broadly whitelist `tools/`, `tests/`, `internal/` or `docs/`.
2. Existing `git diff --name-only` **exact-set equality** remains; missing or additional paths fail closed. The existing clean working tree and exact SHA checkout fences stay enabled.
3. New `test_phase02_authority_pin.py` covers canonical exact SHAs, eight-file approved synthetic Git delta, missing/extra runtime Go/SQL/deploy/tool delta denial, and dirty/mismatched checkout denial.
4. No `go.mod`, Go runtime code, schema, wire protocol, signing rules, credentials or production script changed in either repository.

## Evidence observed in this execution

- GitHub `compare_commits(68cdb... -> d4cff...)` returned **ahead 16, behind 0, exactly 8 changed paths**: three docs, one Go test, two Python tests, two release tools. No Authority runtime Go, database migration or deployment path changed.
- GitHub `compare_commits(3b871... -> this Phase02 development branch)` confirmed test-only product changes, not TELMIVO runtime source changes.
- Nine source-integrity/static checks using real GitHub refs and branch content **9/9 PASS**; the runner source contains the exact pinned Authority SHA and eight expected paths.
- Nine local **independent synthetic Git fence regressions**, using standard Python and a disposable on-container Git repository, **9/9 PASS**. This demonstrates the allowlist semantics but is **not execution of the entire GitHub branch's unittest suite**.
- Local available Go toolchain: `go1.23.2`. Docker CLI, PostgreSQL executable/service and network DNS to obtain external official toolchains are unavailable in the local execution environment. **No real Go1.27.2 + PostgreSQL16 source build / Authority M2M / Product anti-clone E2E test started**.
- Phase2L targeted test template still has two real scenarios to execute: `TestLICA9CrossRepoRealLeaseAndRestart` and `TestLICA9CrossRepoClonedGenerationFenced`, plus six LIC-A8 smoke cases. Previous Python/Go stub results cannot certify either real anti-clone behavior or full LIC-A9 matrix.

## Phase02 acceptance gate

| Requirement | Result |
| --- | --- |
| Canonical Phase01 head accepted by exact diff pin | PASS (GitHub static verification) |
| Change-fence rejects extra runtime/SQL/deployment paths | PASS (synthetic) |
| Product runtime source unchanged | PASS (GitHub changed-path comparison) |
| New branch complete Python suite run | NOT_RUN |
| Go1.27.2 full-source compile against real Authority + product | NOT_RUN |
| Real disposable PostgreSQL16 migrations and M2M TLS requests | NOT_RUN |
| Eight real Go -json run/pass event proofs + cleanup | NOT_RUN |
| Race/replay/clone/recovery matrix acceptance | NOT_CERTIFIED |
| Production and live TELMIVO changes | NONE |

**Gate:** `PHASE02_FULL_ACCEPTANCE_BLOCKED_BY_RUNTIME_AVAILABILITY`. No claim of two real A9 tests passing. No merge to `main`, no production authorization.

## Safe continuation requirement

Use an independently authorized disposable Docker-capable Linux host with locally verified source checkouts, a local Docker Unix socket (never `DOCKER_HOST=ssh://...`), exact Go1.27.2 and PostgreSQL16 images, temporary data directory, and isolated synthetic credentials. Run the pinned Docker-only suite. Review actual compile/DB errors and fix only scoped test/runtime issues on development branches, without relaxing integrity or anti-clone restrictions. If the runner yields all eight individual Go run/pass events, complete the additional A9 adversarial matrix; two tests alone do not approve the full release.
