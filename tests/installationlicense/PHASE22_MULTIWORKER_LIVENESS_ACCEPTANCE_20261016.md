# LIC-A9 Phase22 — Multiple Workers, Signed Runtime Sessions, and Refresh Leader

**Date:** 2026-10-16. **Mode:** Draft security regression + isolated deterministic reproduction. **Production verdict: NO-GO**.

## Frozen source and explicit scope

- Product Phase19 Draft PR #74 source parent `a9e09c8e0029fa0d35e6e06ed6da0fe640c672b8`. Runtime security fix in `internal/platform/installationlicense/state.go` blob `c5c81548a1b063825eccc9c3d69cc6006160d4cd`.
- Refresh leader method in `internal/platform/installationlicense/concurrency.go` blob `9b27feb2fae155ddd7c3000272c58f1e7ba84ae3`, **unchanged**.
- Authority Phase21 Draft PR #57 `d6960982e322d4458c7d963bb153f205a1f8fb59` and Authority Phase20 PR #56 remain unchanged. This stage does **not** merge either repository.
- No production `.go` source, service settings, API, migrations, Go module manifests, deployment, network, bot, DB, signing keys or `main` altered. Only a new Go **test**, this document, and two exact test-source freeze inventories are changed.

## Security invariant vs. availability: actual source behavior

The signed V2 lease includes a **single `RuntimeSessionID`**. `Open(cfg)` generates a fresh `runtimeSession` for every Manager; `decisionLocked()` **requires exact signed session equality** before allowing `Effect()`. The shared installation directory has **one persisted V2 lease**, and `acquireRefreshLeader()` uses an exclusive nonblocking `flock` on `refresh-leader` across worker processes. `Poll()` permits only the leader to refresh.

For two independent Managers sharing a directory, with a signed token issued to the leader:
- leader session: signed lease valid -> `Decision=VALID_ONLINE` -> protected effect possible;
- follower session: different from signed runtime -> `Decision=RESTRICTED/CONCURRENCY_REFRESH_REQUIRED` and protected effect refused;
- while leader lock held: follower cannot become refresh leader to independently obtain its signed lease;
- after signed generation increments for follower, a leader still holding its original session now becomes restricted under the shared state.

**Confirmed contract conflict:** a single shared signed session/lease plus one elected refresh leader is incompatible with authorizing two independent worker sessions simultaneously. This is a Product **multiworker liveness / operational availability blocker**, *not an observed Production outage*, and a deliberately restrictive security behavior, **not** an argument to loosen the anti-clone session check.

## Actually executed local evidence

An **offline, self-contained Go 1.23.2** test harness reused two exact original GitHub function bodies as embedded source copies:
- `decisionLocked`: 2358 bytes, independent FNV-1a64 `ab76adf66a7aa692` both in exact GitHub source extraction and local compiled copy.
- `acquireRefreshLeader`: 630 bytes, independent FNV-1a64 `54fbd34ddb54f194` both in exact GitHub source extraction and local compiled copy.

Local test module path `example.invalid/telmivo-lic-a9-phase22-synthetic`, no external modules. The imported license-cryptography policy, claim verification, local persistence and fresh process sessions were deliberately **synthetic stubs**, not actual Ed25519/DB implementations. **Never treat this harness as a real Product runtime package or authenticated Go1.27 test.**

Actual command:

```sh
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -race -count=1 -v ./...
```

Four real Go test cases under `-race`, **4/4 PASS**:
1. Distinct worker session stays RESTRICTED while leader signed session is VALID, follower effect never runs, and nonblocking `flock` returns ErrPending.
2. Moving signed session from worker A to worker B reverses which worker is authorized; no simultaneous authorization.
3. 30 parallel mocked protected effects allow only the source-matching session, with **zero unexpected follower execution**.
4. Real Linux `flock` rejects dual leadership; releases allow leader takeover.

**These PASS results substantiate the source-derived incompatibility, not multiworker support.**

## New source-level signed regressions (NOT_RUN against full Product checkout)

`internal/platform/installationlicense/phase22_multiworker_session_contract_test.go` Git blob `ac92d01bb8c77ee5ad65ad61ee2cbc71db670a63` is Go-formatted and uses **the existing Phase19 test-only Ed25519 signer fixture** to exercise actual signed V2 lease/acceptance and one installation directory with two independently opened managers. Four test functions check:
- Follower denied commercial effect even when leader lease valid.
- Exclusive refresh-leader lock and follower restricted state coexist.
- Granting signed generation 2 to follower restricts the old leader.
- Concurrent leader/follower effects never leak into restricted worker.

The file was compared byte-for-byte against the independently gofmt-formatted local test source and matched the GitHub blob. **Compilation and actual `go test -race` of this new file against the complete Product source remain NOT_RUN** because the isolated available toolchain is Go 1.23.2 while the Product module requires Go 1.27.0, and the complete private dependency/cache/release test host is not available. Source-level behavior in synthetic harness is independently exercised, but the signed test fixture remains unexecuted in the full package. No false PASS claim.

The modified `tests/installationlicense/run_joint_postgres_e2e.py` exact changed-path fence now lists **37 paths** relative to the hardened Product PR61, including exactly 3 explicitly reviewed `internal/platform/installationlicense/` paths (one runtime fix from Phase19, Phase19 test, Phase22 test). `test_phase08_refreeze.py` maintains that regression allowlist, rejects missing or extra paths, and still cannot authorize Production.

## Required next engineering decision (do not bypass)

Three architectural options need **owner-approved nonproduction design and integration tests**, not a quick silent fallback:

1. **Enforce single active effect-owning process**: every licensed external effect must execute through one properly signed leader, while other roles are genuinely read-only or dispatch through an authenticated local broker. Avoid a covert cluster topology that silently declines legitimate workers.
2. **Independent per-worker signed leases**: redesign Authority concurrency accounting and Product state/high-water storage so independent process sessions each hold distinct signed capabilities; every grant/revocation/replay/clone fence must be enforced by Authority. A shared single `state.json` cannot store distinct simultaneously valid V2 sessions.
3. **Explicit role isolation and one-installation-per-worker**: allocate separately registered installation identities only if product commercialization and licensing allow it; avoids treating clones as a legitimate license cluster.

Do **not** let a worker execute protected side effects merely because another worker's token or refresh leader is valid. Do **not** revert the Phase19 signed-session fence. Do **not** invent a bypass for this release.

Must run the **real full Go 1.27.x `-race` Product suite**, signed Phase19/22 tests, multi-process restart and leader-handoff tests, eight actual Authority×Product/PostgreSQL16 E2E cases, independent recovery and actual signed installer rehearsals, and resolve Product `main` divergence before a final owner-approved release.

**Disposition: `PHASE22_MULTIWORKER_CONFLICT_REPRODUCED_SOURCE_STUB_RACE_4_PASS` / `SIGNED_PHASE22_PRODUCT_GO_TESTS_NOT_RUN` / `REAL_GO127_PG16_E2E_NOT_RUN` / `MULTIWORKER_LIVENESS_UNRESOLVED` / `PRODUCTION_NO_GO`.**

**STOP. Draft only, no merge, no Production connection or deployment.**
