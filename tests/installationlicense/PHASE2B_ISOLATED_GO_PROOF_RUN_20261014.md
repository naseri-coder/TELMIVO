# LIC-A9 Phase 2B — isolated Go execution evidence

**Date:** 2026-10-14. **Environment:** temporary local Go 1.23.2 standalone module, no external modules, database, server, network or production keys.

## Actual executed test

Reconstructed the **exact v1 proof serialization body** from `internal/platform/installationlicense/client.go` into a self-contained temporary Go module with the matching Identity.PublicKey method, then exercised the parameter values and assertions from `proof_contract_vector_test.go` using `go test -count=1 -v .`.

Observed terminal result:

```text
=== RUN   TestM2MProofV1IndependentVector
--- PASS: TestM2MProofV1IndependentVector (0.00s)
PASS
ok  	isolated-proof-test	0.003s
```

- 285 signed-prefix/length-prefixed proof bytes: PASS.
- SHA256 = `9236f56b627a7e6928ec6c70315e7648fdfe5dc8d5dc3e8e0ce7dce533cb31ef`: PASS.
- TEST_ONLY Ed25519 signing/public verification: PASS.
- Tampered proof signature verification: correctly rejected.

**Evidence boundaries:** This is an actual Go execution against an **isolated reproduction of the proof code**, NOT the full TELMIVO module and NOT the exact checked-in Go test executed with Go 1.27. It proves byte-framing consistency of the copied implementation with the independent reference digest; it does not prove source provenance by build or cross-repository compatibility. TELMIVO `go.mod` declares Go 1.27; PR32 Authority declares >=1.25. Main host untouched. No live license, Telegram, production signer, database or new VPS involved.

**Phase verdict:** `ISOLATED_GO_PROOF_VECTOR_PASS`, `COMPLETE_TELMIVO_GO_BUILD_NOT_RUN`, `AUTHORITY_POSTGRES_E2E_NOT_RUN`.

Next quality gate: run exact repository files under a Go 1.27-compatible disposable build environment; then compile and run the cross-repository fixture using disposable PostgreSQL16.
