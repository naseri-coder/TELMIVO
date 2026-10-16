# LIC-A9 Phase25 — Canonical Source Reconciliation / Acceptance (2026-10-16)

**Disposition: SOURCE_GRAPH_AUDIT_PASS / MAIN_DIVERGENCE_DOCUMENTED / RELEASE_NO_GO.**
Scope: GitHub-only, nonproduction, *no* automatic merge, deployment, signing, live host or GitHub Actions.

## Canonical, exactly pinned heads

| Repository | Main SHA | Approved-as-review-input (NOT a release) | Difference from main |
| --- | --- | --- | --- |
| `naseri-coder/TELMIVO` | `f8b7962b83760f780e1c96bc3b80872223dedaee` | Phase24 PR #77 `255a55123015a0ad3d5efe1ab9a233e678e5db03` | candidate 139 ahead / 2 behind; merge base `a708193c75b06895c8d5bfdd8e8b585a93863a30` |
| `naseri-coder/TELMIVO-LICENSE-AUTHORITY` (private) | `c078f55f55a0446d0a6ff49fb6bc0111a217ae42` | Phase21 PR #57 `d6960982e322d4458c7d963bb153f205a1f8fb59` | candidate 166 ahead / 0 behind |

These SHAs are a **dated metadata pin**, not a signed provenance receipt and not permission to merge.

## Verified PR dependency graph

Product chain, all OPEN/DRAFT/UNMERGED:

1. PR #74 Phase19 HEAD `a9e09c8e0029fa0d35e6e06ed6da0fe640c672b8`, base `d12a80d10575b3cef8ae9d6b824f89dfcdbf193f`, 5 changed paths (including runtime `internal/platform/installationlicense/state.go`).
2. PR #75 Phase22 HEAD `24ef50684148a2a434f0dbde8c81017c6c0e244c`, base exactly PR #74 HEAD, 4 paths.
3. PR #76 Phase23 HEAD `34a1459c3f44a7395f3044040d707c53644443fc`, base exactly PR #75 HEAD, 6 paths.
4. PR #77 Phase24 HEAD `255a55123015a0ad3d5efe1ab9a233e678e5db03`, base exactly PR #76 HEAD, 5 paths; these 5 are test/docs/tooling paths, not production `internal/` runtime paths.

Authority chain, all OPEN/DRAFT/UNMERGED:

- PR #55 Phase07 HEAD `bfceaef06d41aa185677a796652d17b07979058a`, base `bce3490234224e6156c9fdfeaf28b43d189e767f`.
- PR #56 Phase20 HEAD `d947d2829f9980117a1ef5dfd2082d4613151fa0`, base exactly PR #55 HEAD, 2 paths.
- PR #57 Phase21 HEAD `d6960982e322d4458c7d963bb153f205a1f8fb59`, base exactly PR #56 HEAD, 4 paths.

**Known provenance staleness:** Authority PR #57's source-graph snapshot pins Product Phase19 PR #74, not newer Product Phase24 PR #77. Do **not** mistake that Phase21 cross-repository snapshot for verified combined Phase24 compatibility. This Phase25 report records both heads and the dependency gap but does not rewrite immutable Phase21 evidence.

## Product main-only commits — decision

Two main-only commits, oldest first:

- `11cc000eb9e84de1ec6946ad133f674259fe3094` — adds `tools/license-owner-offline-age-key`, intended for owner-device offline age recovery identity.
- `f8b7962b83760f780e1c96bc3b80872223dedaee` — adjusts that same tool to accept verified Android Termux owner recovery paths and exception for an app-owned 0771 parent.

Reverse `Phase24...main` GitHub comparison: **2 commits, exactly 1 changed path** (`tools/license-owner-offline-age-key`). This does not intersect the 5 changed paths of Phase24. No same-path conflict is identified in this narrow comparison; **this is NOT a tested three-way merge, no guarantee of semantic compatibility, and no resolution of key-custody review**.

**Decision:** preserve both commits on main; do not discard/rebase/cherry-pick/auto-merge them into the Phase24/25 chain. Independently review local owner key custody, Termux path canonicalization, directory permissions, symlink and TOCTOU defenses, and clean-room tests before proposing any merge. Separate tests and review are mandatory because this tool deals with recovery private-key handling. Main remains unmodified.

## Tests ACTUALLY performed during Phase25

- Connected GitHub live metadata + changed-path inventory + compare endpoints: **64/64 read-only structural assertions PASS** for PRs #74–#77 and #55–#57, head/base/draft/open/unmerged/path count, product and authority ahead/behind, and no Phase24 main-only path overlap.
- Local isolated Python3.13.5: `python3 -B -m unittest -v test_phase25_source_reconciliation.py` — **2/2 unittest methods PASS**; the second contains **30/30 negative/tamper subcases**. These are **snapshot-policy tests only**, not full repository/source build tests.
- Exact locally tested Python test file was pushed as `tests/installationlicense/test_phase25_source_reconciliation.py`; verified GitHub git-blob SHA **`cb75d76990e489276f734d0211bbc77ca7c51b36`**, identical to locally tested `git hash-object`. Local SHA256 **`cd5a020ae8f0a5c68a6c554fdecec37bdb316bb2755d6b3a3c373693ce1082b9`**.
- Tested isolated environment has Go 1.23.2 and Python 3.13.5, but no full authenticated dual source checkout, no available PostgreSQL16/Docker, and GitHub DNS is unavailable to the local shell. **No real Product Go1.27 full suite, Go race integration, broker/Authority integration, or Go/PostgreSQL16 cross-repo E2E executed during Phase25.** Previous Phase24 PR reports are historical evidence, not rerun PASS in this phase.
- Branch changes: this Phase25 Python snapshot test and this documentation file only. No application runtime/security guard/broker, SQL, release installer, signatures, main, or Production changes.

## Required blocking gates (no waivers)

1. **Product main-only owner recovery helper**: independent security review and controlled future merge plan, without leaking owner keys or relaxing custody.
2. **Runtime architecture**: the Phase24 broker is test-only and **NOT integrated** with real `Manager.Effect`, tenant/business RBAC, signed per-session leases, or worker lifecycle. Phase26 must freeze the design before Phase27+ implementation.
3. **Full real Go**: pinned exact full Product Go1.27.x and Authority Go suites and `-race` still **NOT_RUN** on an approved independent test machine.
4. **Cross-repo real E2E**: 8+ Go1.27/PostgreSQL16 genuine test scenarios, worker leader failover and effect fencing still **NOT_RUN**.
5. **DR and signed easy install**: independent two-cluster restore, authenticated offsite monotonic witness, signed Ubuntu install/upgrade/rollback and verified release provenance still **NOT_VERIFIED/NOT_RUN**.
6. Independent security code review, dependency/commit provenance, branch protection and **explicit owner release approval** are still **PENDING**.

## Phase25 acceptance verdict

**PHASE25_SOURCE_GRAPH_RECONCILED_FOR_REVIEW: PASS** (single canonical current Product and Authority review inputs, exact PR ancestry, documented main-only commits, frozen nonproduction scope).

**PHASE25_PRODUCTION_READY: FAIL / NO-GO.** None of the outstanding gates have been waived. The phase closes as an *audit-only* milestone, not a completed deployment or a verified full integration.

**STOP at Phase25. Next Phase26 requires a separate instruction.**
