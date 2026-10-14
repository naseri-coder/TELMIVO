# LIC-A9 Phase 2B — verified toolchain gate and independent M2M signature vector

**Date:** 2026-10-14. **Scope:** GitHub-only, no production services, no new VPS.

## Verified facts

- Exact TELMIVO development-branch `go.mod` declares **`go 1.27.0`**.
- Existing Authority PR32 README declares **Go 1.25 or newer**. Joint build therefore needs a **Go 1.27-compatible** toolchain, not simply Go 1.25.
- Current isolated runner has Go 1.23.2 and Python 3.13.5. It cannot build the actual TELMIVO module under its declared Go version.
- Current isolated runner cannot resolve `github.com` through Git; private-repository checkout is unavailable. PostgreSQL 16 disposable instance was not present.
- No test run against a complete exact checkout of both repositories occurred in this phase. No cross-repo activation, refresh, revoke or expiry E2E passed.

## Actual source improvement

New unit test `internal/platform/installationlicense/proof_contract_vector_test.go` is a network-free `package installationlicense` test. It invokes the real product `proof()` function with a deterministic TEST-ONLY Ed25519 seed (00..1f), immutable request fields, and a reference digest calculated outside Go using Python SHA256 and Ed25519 public-key derivation. It expects a 285-byte serialized proof and SHA256:

`9236f56b627a7e6928ec6c70315e7648fdfe5dc8d5dc3e8e0ce7dce533cb31ef`

It additionally checks signature verification and tamper rejection. This fixes a gap in API-signing contract coverage by pinning exact byte framing; it does not require PostgreSQL.

**Evidence limitation:** Python independently calculated the reference vector, but the checked-in Go unit test **has not been executed** in the complete TELMIVO Go 1.27 module. Do not report `GO_TEST_PASS` or `AUTHORITY_E2E_PASS` based on vector generation.

## Next safe action

A disposable runner with Go 1.27-compatible toolchain and source checkout of both private repos should first execute:

```sh
go test ./internal/platform/installationlicense -run '^TestM2MProofV1IndependentVector$' -count=1
python3 -m unittest discover -s tests/installationlicense -p 'test_contract_guard.py' -v
python3 -m unittest discover -s tests/installationlicense -p 'test_fixture_source_integrity.py' -v
```

Only after these pass, add a **disposable PostgreSQL16** test database and compile the exact cross-repo fixture. The fixture imports older Authority symbols; handle current-source API drift only in the disposable test harness and present failures explicitly. Do not use the active TELMIVO VPS as a test database, auto-deploy, or request another permanent server.

**Verdict:** `INDEPENDENT_PROOF_VECTOR_ADDED_UNRUN` / `JOINT_TOOLCHAIN_GO_1_27_REQUIRED` / `REAL_E2E_BLOCKED_BY_RUNNER_PREREQUISITES`.
