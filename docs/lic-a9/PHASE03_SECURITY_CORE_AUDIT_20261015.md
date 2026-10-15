# TELMIVO LIC-A9 — Phase 03 Security Core (nonproduction candidate)

**Date:** 2026-10-15  
**Mode:** Source-level audit + targeted protective patch + test-only fixtures.  
**Decision:** `PHASE03_HARDLINK_AND_NIL_MANAGER_SOURCE_FIX_PREPARED`; `PHASE03_SECURITY_CORE_FULL_ACCEPTANCE_BLOCKED`.

## Frozen source references

- TELMIVO anti-clone client / Phase03 source parent: `3b871473955e6803636bba0e45a843536afb226a` (PR #44).
- License Authority selected Phase01 consolidated parent: `d4cff712d39e21f934afb0f43ee36a376f1e5345` (Draft PR #50).
- Cross-repo Go1.27.2 + PostgreSQL16 test runner with PR50 pin: TELMIVO Draft PR #60 head `813eca688cfd294160b2746f0f123dc6810a8f98`, stacked on Draft PR #59.
- Authority original nonruntime fixture baseline: `68cdb6996f85981c36dd20ecb7435ec010351e9b`.
- No protocol, lease claims, PostgreSQL migration, Root signer custody, operator role/credential, or server deployment change in Phase03.

## Source audit observation / rationale

Current TELMIVO installation authority client uses secure local directory ownership/0700, non-following leaf opens (`O_NOFOLLOW`), Fstat regular-file/UID/permission checks, and process-wide `flock` for persisted identity, lease, monotonic highwater and refresh leader. Previously **none of the lock/read/leader file Fstat guards required link count = 1**.

A second pathname (same-filesystem hardlink) to a protected inode is therefore accepted, even though a second pathname is unnecessary for a legitimate installation. On systems permitting that inode alias, the file can be accessed through that additional pathname while the main installation path still appears safe. This is **defense-in-depth local hardening**, not evidence that signed license validation could be bypassed or that the host is compromised; an adversary with the installation UID may already possess extensive privileges. This patch intentionally fails closed rather than pretending to prevent all same-UID compromise.

A separate robustness issue in `NewClient(nil)` was an unguarded dereference of `m.cfg.URL`. A missing manager must produce `ErrState`, not a panic or uncontrolled restart.

## Changes (scoped to TELMIVO; Authority source unchanged)

1. `internal/platform/installationlicense/state.go`: require Fstat `Nlink == 1` for lock inode and all persisted reads (`identity.json`, `state.json`, `highwater.json`). Existing `O_NOFOLLOW`, mode, UID and size checks retained.
2. `internal/platform/installationlicense/concurrency.go`: require single link on `refresh-leader` inode before attempting nonblocking leadership lock.
3. `internal/platform/installationlicense/client.go`: `NewClient(nil)` returns `nil, ErrState`.
4. `internal/platform/installationlicense/hardlink_security_test.go`: **test-only synthetic fixtures** check 4 persisted file/lock aliases, restricted decision, protected `Effect` callback never executing, refresh leader denial, and nil-manager construction.
5. This audit document records explicit acceptance limits and remaining security work.

The protections are additive. They do not change permitted license states, validity intervals, signing protocol, concurrency generation ratchet, response parsing, rate limits, DB, release tooling, deployment files or customer data.

## Evidence and limitations

- GitHub exact-source inspection: `state.go`, `concurrency.go`, `client.go`, Authority `internal/m2m/server.go` and existing lifecycle tests were reviewed.
- Newly committed Go regression test was independently parsed and formatted by local `gofmt` using Go1.23.2 with no syntax errors or formatting changes. Its local Git blob `e00698d1006e3673ba0775e3b4e85aa76bf9b4e7` matches the exact GitHub test blob.
- Local runtime lacks Go1.27.x, Docker and PostgreSQL16, and has no external DNS; **the target repository Go unit/race/integration suites were NOT executed**. `gofmt` is **not** a compiler, unit test, security certification or functional runtime proof.
- Read-only/static changed-file checks, where performed, do not certify behavior. Testing of hardlink protection under the actual production target UID/filesystem and concurrent state updates remains pending.
- Authority PR #50 and client PR #44 remain unmerged; Phase02 PR #60 still has no actual joint Go/PG16 E2E PASS.

## Security-core remaining acceptance matrix

| Gate | Result |
| --- | --- |
| Read/lock/leader hardlink rejection code reviewed and committed to development | PATCH_PRESENT / pending Go execution |
| Nil-manager fail-closed constructor | PATCH_PRESENT / pending Go execution |
| Go1.27.2 `go test`, `-race`, vet and full product regression | NOT_RUN |
| Authentic Authority signing and verified update/rollback E2E with PG16 | NOT_RUN |
| 8-case joint cross-repo suite (six A8, two A9) and broader A9 adversarial matrix | NOT_RUN |
| Migration 0001–0008 and audit history permission/rollback correctness | HISTORIC_ONLY / fresh verification pending |
| Real signed release/authenticated provenance and independent security approval | NOT_APPROVED |
| Production changes / `main` merge | NONE |

## Cross-PR integration discipline

This Phase03 development branch is intentionally based on **TELMIVO PR #44** rather than combining it with unexecuted Phase02 PR #60. Because PR #60's `check_product_checkout` protects `internal/` bytes, this patch will correctly be refused by the old exact-product pin until a later explicit **joint E2E source refreeze** selects the Phase03 head and revises the protected checkout policy with exact review. **Do not weaken that protection now.** That follows the incomplete Phase02 test gate, not bypasses it.

## Phase03 release decision

**`PHASE03_SECURITY_HARDENING_CANDIDATE`; `REAL_GO1272_UNIT_RACE_NOT_RUN`; `REAL_A9_CROSSREPO_E2E_NOT_RUN`; `PRODUCTION_NO_GO`.**

A correct next validation requires a disposable Go1.27.2 environment with controlled module dependencies, the original TELMIVO fixtures and `go test -race ./internal/platform/installationlicense/...`. Then run complete two-repo TLS+PostgreSQL16 E2E with jointly pinned actual Authority/Product commits, and review source under a human security gate before any merge or release.
