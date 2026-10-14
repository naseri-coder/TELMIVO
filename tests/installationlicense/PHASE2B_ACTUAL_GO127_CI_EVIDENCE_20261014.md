# TELMIVO LIC-A9 — Actual Go 1.27 source test completed

**Evidence date:** 2026-10-14
**Scope:** TELMIVO client-source Phase 2B only. No Authority / Postgres E2E.

## Authoritative independent CI evidence

- GitHub Actions run: https://github.com/naseri-coder/TELMIVO/actions/runs/38619032922
- Run ID: `38619032922`; job ID: `116041444296`.
- Event: `push`; **exact tested TELMIVO commit:** `72549405711a6011c1a6573a3b638712880ca701`.
- GitHub final state: `completed` / **`success`**.
- Hosted runner: Ubuntu, Go **1.27.0 linux/amd64**, installed from the repo's actual `go.mod` using `actions/setup-go@v5`.
- Actual command: `go test -count=1 -v ./internal/platform/installationlicense/...`.
- Two actual Go packages passed: `internal/platform/installationlicense` and `internal/platform/installationlicense/licensecrypto`.
- **19 top-level Go tests + 6 nested codec subcases** passed, including `TestM2MProofV1IndependentVector`.
- Actual Python test commands ran 4 suites: **8 + 3 + 7 + 12 = 30 tests**, all passed.
- The workflow used `permissions: contents: read` and `actions/checkout@v4` with `persist-credentials: false`. No Authority secret, deployment, production database or main server access was used.

## Correct boundary of the result

The **actual TELMIVO product Go license client** has now compiled and passed its real unit tests with its declared Go version. This supersedes prior inability to run the full client module locally on Go 1.23.2.

The following are still **NOT RUN**:
- Go compile/test from **the selected Authority PR32 checkout** at `68cdb6996f85981c36dd20ecb7435ec010351e9b`, paired with TELMIVO;
- old test fixture compiled with both source trees at the pinned commits;
- isolated PostgreSQL 16 migrations and license activation, refresh, revoke, concurrency and timeout end-to-end tests;
- deploy, release-signing/installer and production licensing.

**Verdict:** `PRODUCT_GO127_LICENSE_CLIENT_UNIT_TESTS_PASS`; `PYTHON_SOURCE_CHECKS_30_OF_30_PASS`; `CROSSREPO_AUTHORITY_POSTGRES_E2E_NOT_RUN`.

The workflow is scoped to a one-time `push` path for its own file, to avoid repeated jobs on subsequent documentation commits and respect Actions quota.
