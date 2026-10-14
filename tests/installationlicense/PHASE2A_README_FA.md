# LIC-A9 — Phase 2A: source contract preflight (no VPS)

**2026-10-14 | NONPRODUCTION ONLY.** The product is under development from scratch; no customers, no permanently provisioned License Authority server, and no real Telegram/authority credentials are needed.

## Current authoritative source selection

- Authority: private `naseri-coder/TELMIVO-LICENSE-AUTHORITY`, PR32 exact SHA `68cdb6996f85981c36dd20ecb7435ec010351e9b`.
- Product: this TELMIVO feature branch based on `feature/lic-a9-anti-cloning-client-v0.1`.
- The previous TEST-only fixture pinned outdated Authority SHA `392fc89087d8dac001d1709d0f7973c2f28a8515`. It now pins the chosen PR32 SHA. This **updates source selection only**; do not infer E2E compatibility from that edit.

## Zero-server contract preflight

On a developer's own machine with **both private repositories** checked out, detach the Authority checkout at exact PR32 SHA (never use a production checkout). The check performs no network access or side effects except reading repository files and calling read-only `git rev-parse HEAD`:

```sh
python3 tests/installationlicense/contract_guard.py \
  --authority-root /absolute/path/to/TELMIVO-LICENSE-AUTHORITY \
  --product-root /absolute/path/to/TELMIVO
python3 -m unittest discover -s tests/installationlicense -p 'test_contract_guard.py' -v
```

The contract guard requires exactly the selected Authority SHA, verifies the four v1 M2M route declarations, `TELMIVO_INSTALLATION_REQUEST_V1` domain, exact nine signed field ordering in the product `proof` builder, six essential client HTTP markers, the fixture SHA pin, and the use of `git archive` to extract the pinned authority.

Six isolated unittest scenarios: expected static source shape, old Authority SHA refused, missing route refused, signed-field reordering refused, changed signature domain refused, missing Idempotency-Key refused.

**Limit:** this check is static. The 6 self-contained tests passed in an isolated Python 3.13 container using synthetic source snippets; they do NOT prove actual Go builds, Authority handler acceptance, cryptographic E2E or PostgreSQL migrations. Full original-source static preflight is available via the command above and must be run on clean checked-out sources.

## Next remaining Phase 2A gate

Run a **real disposable joint Authority + Product Go test** against a throwaway PostgreSQL 16 instance with synthetic signing keys and mock TLS, first resolving compile/fixture incompatibilities introduced since the old SHA. The existing `authority_e2e_test.go.template` references a legacy fixture transformed with string substitutions including a mock clock; it is **not yet evidence of PR32 cross-repo E2E PASS**. Report compile errors and failing assertions precisely instead of changing production behavior to make tests pass.

Do not start any VPS, contact live Telegram, issue real licenses, push private keys to GitHub, or claim production readiness. Commit each actual fix in a scoped PR.
