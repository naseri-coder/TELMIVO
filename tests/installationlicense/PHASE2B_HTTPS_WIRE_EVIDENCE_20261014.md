# LIC-A9 — Local HTTPS wire interoperability evidence, 2026-10-14

## Executed (disposable local standard-library Go 1.23.2 fixture)

Both sides of the v1 request-signature protocol were represented in one isolated Go module:

- Client: source-derived `proof()`, `(*Client).request()`, `StrictJSON()`, with necessary synthetic `Identity`, `Operation` and narrow state/reply type scaffolding.
- Authority: source-derived `SigningInput()`, `VerifyProof()`, `Proof` and request domain.
- Temporary `httptest.NewTLSServer`, random **test-only** Ed25519 installation identity; no persistent service, no database and no external internet connections.

Relevant original GitHub blobs inspected:
- Product `internal/platform/installationlicense/client.go`: `47827f35721daf21994a6eba4f901c7eccea0ae5`.
- Authority `internal/m2m/proof.go` at PR32 `68cdb6996f85981c36dd20ecb7435ec010351e9b`: `6dad2e0b2226673e33fd519cdd09550aaaf83fbc`.

Actual local terminal commands:

```text
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -count=1 -v ./...
PASS
ok   lic-http-wire-fixture 0.027s

GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -race -count=1 -v ./...
PASS
ok   lic-http-wire-fixture 1.125s
```

Covered **5 legitimate HTTPS request subcases**: activate, refresh, deactivate, concurrency and status, and **4 adversarial/reliability cases**: request body tamper rejected; correctly signed but expired timestamp rejected; consecutive network requests issue independent nonces; duplicate JSON response fields rejected. The disposable harness itself passed with Go race instrumentation.

## New reproducible GitHub test assets

1. `tests/installationlicense/run_crossrepo_https_wire.py`: pins both checked-out commits, rejects uncommitted/untracked files, extracts the **exact Go declarations** from both checked-out repositories, creates a disposable Go 1.23 standard-library module and executes the HTTPS wire tests with `GOPROXY=off` and `GOTOOLCHAIN=local`.
2. `tests/installationlicense/https_wire_test.go.txt`: 5 HTTPS methods and 4 negative/reliability checks.

To execute after cloning **both private repos** and checking out exact commits:

```sh
python3 -B tests/installationlicense/run_crossrepo_https_wire.py \
  --authority /path/to/LIC-A9-PR32-checkout \
  --product /path/to/TELMIVO-PR47-checkout \
  --product-sha <EXACT_40_HEX_TELMIVO_COMMIT_SHA>
```

The tool **does not run the complete TELMIVO Go 1.27 application**. Crucial distinction: the **standalone local reproduction was run**, whereas the newly committed exact-source Git-checkout extractor **has not yet been executed from two complete private Git clones**. Do not conflate these sources of evidence.

## Outstanding actual blockers

- Full TELMIVO module requires Go 1.27; current disposable runner only has 1.23.2.
- Complete private GitHub clones and disposable PostgreSQL 16 are not available in this isolated environment.
- The historic Go/SQL-backed joint fixture may require test-only adaptation to PR32 and must not be reported as a successful activation test.
- No live Authority, PostgreSQL, Telegram, paid infrastructure, installer or main TELMIVO VPS was changed or touched.

**Verdict: LOCAL_HTTPS_REQUEST_WIRE_9_SCENARIOS_PASS (also -race); EXACT_REPOSITORY_EXTRACTION_RUN_PENDING; FULL_GO127_PLUS_POSTGRES_E2E_NOT_RUN.**
