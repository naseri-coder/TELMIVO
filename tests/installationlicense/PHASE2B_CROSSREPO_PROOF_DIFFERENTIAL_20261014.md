# LIC-A9 — Cross-repository Go request-proof differential (2026-10-14)

## Verified result — isolated, TEST ONLY

Two real Go function bodies were read from GitHub and reproduced without edits in a disposable, **standard-library-only** Go module:

| Source | Exact source | GitHub blob SHA |
|---|---|---|
| Product | `internal/platform/installationlicense/client.go` on TELMIVO PR47 development branch | `47827f35721daf21994a6eba4f901c7eccea0ae5` |
| Authority | `internal/m2m/proof.go` on PR32 commit `68cdb6996f85981c36dd20ecb7435ec010351e9b` | `6dad2e0b2226673e33fd519cdd09550aaaf83fbc` |

The production `proof()` implementation was tested against the Authority `SigningInput()` implementation. The only extra source was synthetic, non-secret `Identity` plumbing and seven independent Go table scenarios.

Observed disposable Go 1.23.2 command:

```text
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -count=1 -v .
=== RUN   TestSignedInputsIdentical
=== RUN   TestSignedInputsIdentical/activate
=== RUN   TestSignedInputsIdentical/refresh
=== RUN   TestSignedInputsIdentical/deactivate
=== RUN   TestSignedInputsIdentical/concurrency
=== RUN   TestSignedInputsIdentical/status
=== RUN   TestSignedInputsIdentical/UTF8
=== RUN   TestSignedInputsIdentical/binary
--- PASS: TestSignedInputsIdentical (0.00s)
    --- PASS: TestSignedInputsIdentical/activate
    --- PASS: TestSignedInputsIdentical/refresh
    --- PASS: TestSignedInputsIdentical/deactivate
    --- PASS: TestSignedInputsIdentical/concurrency
    --- PASS: TestSignedInputsIdentical/status
    --- PASS: TestSignedInputsIdentical/UTF8
    --- PASS: TestSignedInputsIdentical/binary
PASS
ok   wireproof
```

For every case, compared **byte-for-byte** the two serialized signing inputs, signed the client bytes using a disposable deterministic Ed25519 test key, verified the signature using the Authority bytes and checked that altering the request body invalidates that signature. Status uses an empty Idempotency-Key; query and empty body participate in the signing input. The UTF8/binary cases validate signature framing only; they do not imply the Authority accepts these as valid business JSON.

## Reproducible next-run artifacts

- `run_crossrepo_proof_differential.py` extracts both Go function bodies and the Authority `Proof` struct and domain from **pinned, clean** repository checkouts, then creates a temporary Go-only module. It disables Go proxy/toolchain downloads and refuses unknown source SHAs or dirty worktrees.
- `wireproof_differential_test.go.txt` is the standalone Go test harness with seven scenarios. The harness needs Go >= 1.23 only because it compiles isolated proof functions; it is **not** a TELMIVO module build.

With both repos available at selected SHA and a clean checkout:

```sh
python3 -B tests/installationlicense/run_crossrepo_proof_differential.py \
  --authority /absolute/path/TELMIVO-LICENSE-AUTHORITY \
  --product /absolute/path/TELMIVO \
  --product-sha <ACTUAL_PRODUCT_40_HEX_COMMIT_SHA>
```

**Truth boundary:** Seven actual isolated tests PASSED using manually materialized exact function bodies extracted from the two GitHub blob contents. The just-committed Python extractor + Go harness have **not** been independently executed from two complete private repo clones. No claim is made of full TELMIVO Go 1.27 compilation or PostgreSQL 16 E2E activation. No VPS or production database was accessed, no real license issued, no credentials created or stored.

**Result:** `WIRE_PROOF_BYTE_SERIALIZATION_COMPATIBLE_7_OF_7`; `SOURCE_CHECKOUT_RUNNER_PENDING`; `GO_1_27_PRODUCT_MODULE_BUILD_PENDING`; `LICENSE_ACTIVATION_POSTGRES_E2E_PENDING`.
