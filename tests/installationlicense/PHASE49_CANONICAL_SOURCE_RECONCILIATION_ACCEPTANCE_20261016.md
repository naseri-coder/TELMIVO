# LIC-A9 Phase49 — canonical source reconciliation and no-merge integration candidate

Date: 2026-10-16

**Status:** PRODUCT CANDIDATE SOURCE-CONVERGED / GIT ANCESTRY DIVERGED / RELEASE NO-GO. This document is evidence for a *draft development review*, not authority to install, deploy, merge, trigger Actions, or touch customer data.

## Immutable source lineage

| Source | Git ref | Immutable SHA |
|---|---|---|
| Product Phase48 parent | TELMIVO PR #102 (DRAFT / UNMERGED) | `3a637d149f3d66169c272008409b2452d80783d6` |
| Phase49 initial content reconciliation | `development/lic-a9-phase49-canonical-source-reconciliation-20261016` | `1196099d4e13f5061b8f9198842c15dccd94ef88` |
| Product runtime `internal/platform/installationlicense/runtime.go` | Phase48 + Phase49 | Git blob `eb80737092f6d40cd9b1acfb96ef7c4033c5e30c` |
| Product owner-only recovery utility | latest Product `main`, carried to Phase49 | Git blob `3d82585795fba5a0140da05025712a13df435af4`, mode `100644` |
| Authority candidate | TELMIVO-LICENSE-AUTHORITY PR #57, Phase21 DRAFT / UNMERGED | `d6960982e322d4458c7d963bb153f205a1f8fb59` |
| Product main (reviewed snapshot) | `main` | `f8b7962b83760f780e1c96bc3b80872223dedaee` |

Product parent stack: Phase45 PR #99 -> Phase46 PR #100 -> Phase47 PR #101 -> Phase48 PR #102 -> Phase49. All are **DRAFT/UNMERGED**. The Authority repository's Phase21 PR #57 remains a separate read-only candidate; no Authority source is changed in this phase. The older Authority PR #47/#48/#49 competing offline verifiers were previously consolidated through canonical Phase01 PR #50; do not merge the superseded alternatives.

## Exact reconciliation performed, and limitations

- `main` had exactly two commits absent from the Phase48 branch, `11cc000eb9e84de1ec6946ad133f674259fe3094` and `f8b7962b83760f780e1c96bc3b80872223dedaee`. Both exclusively create/modify `tools/license-owner-offline-age-key`. No overlapping source path with Phase48's `runtime.go`.
- The source-converged Phase49 branch was made from Phase48 HEAD and a **single-parent** Git commit attached the **identical main blob SHA** `3d82585795fba5a0140da05025712a13df435af4` in the same path/mode, without running that custody tool.
- Git comparisons: Phase48...Phase49 reconciliation commit: ahead 1/behind 0, exactly **one added path**. `main...Phase49`: ahead 241/behind 2 (still **DIVERGED IN HISTORY**); *content parity on this path does not remove ancestry divergence*. A later independently reviewed clean rebase/integration remains mandatory; do not force-push existing branches, and do not merge to either `main`.
- No executable runtime source, Go module, product startup code, database SQL, Authority code, secrets, installed binaries or workflows were modified in Phase49. Final Phase49 report commit adds this Markdown report only.

## Independent source-only checks — executed

Actual GitHub connector reads of repo, PR, file and compare data, followed by local JavaScript/V8 structural assertions against the returned bytes (not just a copied PR description): **25/25 PASS, zero failures**.

Of these, **19 positive assertions** checked Phase48 unchanged runtime blob; byte-identical main custody tool; pinned Product/Authority review heads, DRAFT/UNMERGED; immediate Phase48 ancestry; one-path Phase49 delta; explicit Product divergence and Authority main relationship; Product Go 1.27 declaration; runtime atomic binding publication/read, default restriction, foreign Manager denial, cancellation and delegation to `Manager.Effect`; plus owner-key utility's local/SSH/root guard strings.

**6/6 negative tamper mutations REJECTED** by the same source-only policy mirror: remove atomic pointer, remove installation store, remove consistent binding load, remove foreign Manager prohibition, remove pre-install restriction, or replace `Manager.Effect` with direct callback. These are **syntactic guardrail checks, not executed Go semantics, not crypto verification, and not a formal security proof**.

Read-back GitHub blob IDs:
- Runtime `eb80737092f6d40cd9b1acfb96ef7c4033c5e30c` = Phase48 canonical runtime unchanged.
- Owner-key tool `3d82585795fba5a0140da05025712a13df435af4` = current reviewed main byte-exact.

## Runtime security review: non-waivable open issues

1. Phase48 intentionally allows `WithManager(ctx,m)` **before** global `InstallRuntime`; verify all runtime callers and whether any real effect can execute in this window. Policy must be explicitly scoped to isolated test/owner processes or rejected before production boot.
2. `InstallRuntime(nil)` then `InstallRuntime(valid)` is allowed. This is a deliberate in-process re-installation path, **not immutable revocation**. Prove the authorization and lifecycle contract for caller-controlled reinstallation, including restart/clone and rollback.
3. Replacing a Manager pointer does **not** revoke a previously admitted in-flight external effect; independently prove durable owner/UID fencing, lease/session/recovery epoch validation, outbox idempotency and provider receipts. Exactly-once external side effects are **not proven**.
4. Compile/test real Product and inspect all entrypoints; Phase45-48 isolated STUB Manager Go1.23 Race Detector tests must not be misrepresented as genuine Product Go1.27.2 E2E results.
5. Independently verify custody tool behavior on a real **owner-controlled local** device before relying on it. It was imported but **not executed**, no age identity was created or accessed. Never run on Production or SSH.

## Non-waivable next admission gates

- Genuine TELMIVO Product Go **1.27.2** module (declares `go 1.27.0`), authentic signed V2 lease and Phase45-48 tests, full startup, lint/vet and native Linux `-race`: **NOT_RUN**.
- Authority real Go **1.25+** (declares `go 1.25.0`) test/build, verified M2M signing and state machine: **NOT_RUN**.
- Real nonproduction Linux multi-UID Supervisor + durable IPC/owner journal + crash/failover/replay/hardlink checks: **NOT_RUN**.
- Real disposable PostgreSQL16 multiworker Product↔Authority E2E and external provider outcomes, dual encrypted restore rehearsal, signed easy installer: **NOT_RUN**.
- Git topology now content-reconciled for the two Product main commits, but the final independent review/rebase, cryptographic provenance pin and clean full-build evidence: **PENDING**.

Local available toolchain for this execution was Linux/amd64 Go1.23.2; GitHub access via connector worked but shell GitHub DNS was unavailable. No claim of Go1.27.2, PostgreSQL16, Linux multi-UID or deployment execution is made.

**Decision: PHASE49_SOURCE_CONVERGENCE_AND_25_STATIC_CHECKS_PASS / MAIN_ANCESTRY_REBASE_PENDING / REAL_GO_PG16_E2E_NOT_RUN / PRODUCTION_NO_GO.**

**Do not merge, deploy, enable Actions, create/restart servers or use real signer keys.**
