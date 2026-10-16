# TELMIVO LIC-A9 Phase19 — Runtime-session replay/clone safety patch

**2026-10-16 | SECURITY REVIEW REQUIRED | DRAFT PR / NO-GO**

## Frozen source and explicitly changed scope

- Phase18 source parent: Product Draft PR #73, `d12a80d10575b3cef8ae9d6b824f89dfcdbf193f`.
- Hardened Product runtime ancestor: Draft PR #61, `d88fdb2e4d34f18e8f9c8313f73db5b14a077d5a`.
- Authority candidate: Draft PR #55, `bfceaef06d41aa185677a796652d17b07979058a` (unchanged).
- **Intentional exception to the previous test-only freeze:** modify exactly `internal/platform/installationlicense/state.go` and add `internal/platform/installationlicense/phase19_session_fencing_test.go`; also extend two strict test-only source-fence files and add this report. No other runtime source, deployment, SQL, Go dependencies, signing authority, production host, real license key or main branch is altered.

## Confirmed local authorization-boundary issue

Source-reviewed `state.go` created a random `m.runtimeSession` for every new `Open` (new process instance), and signed V2 license claims include `x.RuntimeSessionID`. The original `decisionLocked` checked cryptographic verification, monotonic generation and token expiry but **did not compare `x.RuntimeSessionID` against the current `m.runtimeSession`** when authorizing product effects. Thus, a restarted or cloned installation with the same signed V2 state could pass `Effect` *before* attempting an Authority concurrency refresh; this weakens the expected anti-clone boundary. This is a **local-source security defect confirmed under an isolated, stubbed verifier model**, not a demonstrated compromise of a production system.

### Actual executable regression evidence

Exact original `decisionLocked` and `Effect` function bodies were extracted from Product PR61 state.go Git blob `f8889a94b284f8cc5627dcc1f59a4c3fcbc95b8b` into a standalone Go1.23.2 module; the stubbed verifier intentionally supplied a valid verified signed-V2 lease result. The source logic itself was preserved, while storage/cryptographic verification and locking adapters were replaced with local test-only stubs:

- **Before patch:** original runtime session authorized; a different runtime session also authorized the callback with the previous lease. The security test **FAILED as expected** (`TestReopenedSessionMustBeRestrictedBeforeRefresh`).
- **After patch:** four Go source-level tests, including original-session allowance, stale-session refusal, new-session refresh allowance, and 20 parallel leader/clone effects, **4/4 PASS under `go test -race -count=1`**. These are not production Go1.27.2 tests and do not invoke the real Authority/PG16 or real end-to-end signature verification.
- The exact regression source change applied to Product runtime is one guard after `m.verify`:
```go
if x.Version == 2 && x.RuntimeSessionID != m.runtimeSession {
    return Decision{State: "RESTRICTED", Reason: "CONCURRENCY_REFRESH_REQUIRED", RefreshDue: true}, ErrRestricted
}
```
- Actual Product `state.go` Git blob after patch: `c5c81548a1b063825eccc9c3d69cc6006160d4cd` (from `f8889a94b284f8cc5627dcc1f59a4c3fcbc95b8b`).
- New **real-package** Go regression test file `phase19_session_fencing_test.go` creates disposable Ed25519 root/signer keys and signed V2 leases in memory, tests post-restart refusal, simulated fresh signed-generation recovery, old-generation replay refusal, and 12-way concurrent effect fencing. Git blob: `3fe0316182760ccf6504b7f36e7be8cd19dacd77`. This new full-package Go regression source is **NOT_RUN** against the complete actual Product checkout; its final Go1.27.2 execution is mandatory.

## Revised exact source-pinning invariant

The Product source delta against frozen runtime PR61 is now exactly **35 explicitly listed paths**, including **only two reviewed `internal/` exceptions**, namely the security guard in `state.go` and its new Go test. The other 33 paths remain the previously approved tools/tests/docs plus this acceptance document. No broad runtime directory allowlisting is permitted. `test_phase08_refreeze.py` now strictly checks this exact exception and asserts a missing security-fix path fails. `run_joint_postgres_e2e.py` retains complete delta equality and known fixture-preparer SHA pin. All Product head/image/module pins for the actual Docker-only E2E remain mandatory.

The available isolated 20-file Python tooling suite was run again after applying the exact reviewed 35-path fence changes: **190 tests, 190 PASS, zero errors**, including the new negative source-fence regression. The updated `run_joint_postgres_e2e.py` and `test_phase08_refreeze.py` local Git blob IDs match GitHub:
- `8819a631f3f0b960906715bdfa53880e5b377ff9`
- `a326b9d6893fed95189e426fb83873fa3be33c38`.

**Important:** This Python unit suite is a complete suite of the isolated `tests/installationlicense/` tooling subtree, not the whole Product/Authority application Go/DB test suite.

## Mandatory remaining release blockers

1. On a **separately approved disposable nonproduction** Linux host, obtain complete exact private Product and Authority checkouts. Run Product Go1.27.2 standard and `-race` suite including the new full-package Go tests; compile all code and run real concurrency/lease/restart tests.
2. Explicitly test the availability tradeoff introduced by session fencing for *legitimate multiple bot/worker processes sharing one installation directory*: old session must fail closed, but product can only restore authorized operations after its own freshly signed lease. Repair legitimate multi-process leader/follower behavior if tests reveal a deadlock/liveness problem; **do not silently weaken the session comparison**.
3. Run all eight real LIC-A8/A9 cross-repo Go1.27.2+PostgreSQL16 E2E `run/pass` cases with valid independently reviewed image/module/checkout pins and Docker cleanup evidence. Stale Authority idempotency replay, primary/clone races, renewal, terminal revocation, lost-response retry and post-rollback replay matrix remain unproven in the real environment.
4. Complete independent disaster recovery, release signing, installer QA, external code review and owner human approval before any deployment. Any failure remains a block.

**Outcome:** `PHASE19_STALE_SESSION_EFFECT_REPRODUCED`, `PHASE19_RUNTIME_GUARD_PATCHED_IN_DRAFT`, `LOCAL_GO_SOURCE_HARNESS_4_PASS_RACE`, `PYTHON_TOOLING_190_PASS`, `ACTUAL_PRODUCT_GO1272_FULL_SUITE_NOT_RUN`, `REAL_CROSSREPO_PG16_E2E_NOT_RUN`, `PRODUCTION_NO_GO`.

**Keep Draft. No merge, no deployment, no real Production keys/database/server, and no Phase20 actions.**
