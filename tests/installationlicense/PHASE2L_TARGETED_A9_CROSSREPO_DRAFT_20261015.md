# TELMIVO LIC-A9 Phase 2L — targeted real cross-repository anti-cloning E2E candidate

**2026-10-15 | Candidate / non-production only | No real Docker/Go1.27.2/PostgreSQL16 run yet**

## Frozen source and contract

Starting source branch: `feature/lic-a9-anti-cloning-client-v0.1` commit `3b871473955e6803636bba0e45a843536afb226a`. The Authority fixture is pinned to merged PR32 head `939d14800c602dd9c31ff23fdf6e0688a440d4c2` with the original archive delta guard; fixture preparer blob `be27fbdf5871ee54e35ef9302d2749a67b1725e9` is unchanged.

Product `installationlicense.Client.Concurrency(ctx)` sends a canonical, PoP-signed `/v1/licenses/concurrency` request. Authority `m2m.Server` verifies it and `postgres.Store.ConcurrencyRefresh` manages generation ratchet and stale-clone rejection. Both paths are used in the new test Go template; no fake Authority implementation.

## Two targeted real-DB tests, not six legacy LIC-A8 tests

`tests/installationlicense/authority_lic_a9_crossrepo_test.go.template` is copied into **only the disposable Go fixture** following `prepare_authority_fixture.py` (which is unchanged). The imported bytes are SHA-pinned.

1. `TestLICA9CrossRepoRealLeaseAndRestart`: real TLS Authority, real scoped PostgreSQL fixture and signed LIC-A8 bootstrap, then a **RequireConcurrency** product client must deny a legacy-only lease with exact reason `CONCURRENCY_REFRESH_REQUIRED`; it obtains the real signed v2 concurrency lease (generation 1), restarts with a distinct runtime session and ratchets to generation 2 with a different lease ID.
2. `TestLICA9CrossRepoClonedGenerationFenced`: obtain signed concurrency lease, **copy synthetic identity/state/highwater into a second disposable directory**, advance the genuine installation's generation, then the clone's stale request must return terminal `ErrRestricted` with exact reason `CONCURRENT_INSTALLATION_CONFLICT`; PostgreSQL must retain winning generation 2, persist `CLONE_SUSPECTED` and exactly one durable `clone_conflict_detected` audit intent; the clone's `Manager.Effect` callback must **never execute**; leader's locally verified grant remains usable. This does not attempt to bypass PoP or use a real installation identity.

These tests depend on actual PostgreSQL16 and product⇄Authority HTTP/TLS signing, not merely JSON codec assertions. They may still reveal integration defects during first actual run; no PASS is claimed for either test.

## Fail-closed runner

The existing Docker-only runner now:
- Imports only the exact SHA-pinned new Go template into the synthetic fixture, refusing unexpected bytes/destination.
- Compiles all Go test source and confirms **all eight** names (six LEGACY + two A9) **before** creating a test DB.
- Executes all eight in one isolated Go1.27.2 invocation against disposable PostgreSQL16, requiring precisely one `run` and one `pass` JSON event per named test plus package pass. Missing, duplicate, skipped, failed or malformed evidence refuses success.
- Preserves product checkout SHA protection, local Docker-only socket, database `--network none`, unprivileged `postgres` container, read-only product mount, no host port, temporary cache, and **verified cleanup**.
- Emits distinct `LIC_A8_LEGACY_CROSSREPO_GO1272_PG16_SMOKE_PASS` and `LIC_A9_CROSSREPO_TARGETED_ANTICLONE_2_TESTS_PASS` **only** after real execution and cleanup; always includes `LIC_A9_FULL_SECURITY_MATRIX_NOT_CERTIFIED` to avoid treating two tests as comprehensive security certification.

## Execution evidence this turn

On an independent container with Go 1.23.2, Git and Python, but no Docker/PostgreSQL:
- `gofmt` parsed/reformatted the new Go test template.
- Actual synthetic Go `go test -list` and `go test -json` exercised eight **stub** tests to verify discovery and JSON gates. **This is not execution of the two real Go/PG16 scenarios.**
- All **47/47 local Python/synthetic tests passed**; exit 0; the new regression guard also checks exact denial reason, Authority ratchet and durable conflict intent. No full-source Go test binary was compiled.
- SHA256 of the clean 46-test log: `b3c7d8a78df8cca707b27e2ffc5e4949d2d5b9296d078f96fb9d3b00ebde655f`.
- Exact Git blobs of these locally tested files:
  - runner: `bc3ce48fe89436d540e5f8e3344f56df7a96efe0`
  - regression tests: `b0bd6ce3db13a84b2b2fd2d1dbb85b7fa21d11cd`
  - new Go E2E template: `e9846d589990210074c8980321c8cb168cc0fb6f`

## Release boundary

**This PR is DRAFT and must NOT merge solely on the local Python result.** The Go1.27.2 compiler against both actual full source trees and real PostgreSQL16 E2E have **NOT_RUN**. First execute in a separate authorized, disposable Docker host; fix actual compile/DB failures on that fixture without relaxing gates. These two tests also do not yet cover all LIC-A9 cases (simultaneous same-generation races, signed epoch and recovery transitions, backlog/lock limits, tampered key PoP, supersession). Full matrix remains unapproved; Authority PR32 remains draft/unmerged.

Verdict: `PHASE2L_TARGETED_A9_CROSSREPO_CANDIDATE_PREPARED`; `LOCAL_47_OF_47_PASS`; `REAL_A9_GO1272_PG16_E2E_NOT_RUN`; `NO_PRODUCTION_MUTATION`.
