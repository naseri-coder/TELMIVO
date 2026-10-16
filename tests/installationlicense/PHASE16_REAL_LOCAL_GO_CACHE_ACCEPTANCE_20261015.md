# LIC-A9 Phase16 — Real local-Go offline module-cache integration proof

**Date:** 2026-10-15. **Mode:** disposable nonproduction *local* Go test only. **Production verdict: NO-GO.**

## Frozen baseline

- Phase15 Draft PR #69 HEAD: `1b886f6c0d99ede7274149bc1dd938bbee31ce15`.
- Protected TELMIVO hardened Product PR #61: `d88fdb2e4d34f18e8f9c8313f73db5b14a077d5a`.
- Protected Authority PR #55: `bfceaef06d41aa185677a796652d17b07979058a`.
- All changed paths on the Phase16 branch remain under `tests/installationlicense/`. No changes to Go runtime, `go.mod`, `go.sum`, live containers, SQL, GitHub Actions, secrets, production Telmivo host or `main`.

## Why Phase16 is more than another mocked success

In Phase15, only Python unit tests had proven the local SHA256 module-cache staging function; nothing had confirmed that **the real Go toolchain can compile an imported external module using the staged cache after `GOPROXY=off`**.

The new opt-in `phase16_real_go_offline_cache_probe.py` performs that exact integration with **Go 1.23.2 installed locally** and temporary test-only inputs:

1. Build an entirely synthetic `example.org/lic-a9-local-dep@v1.0.0` Go module zip, mod and info in a temporary **file://** local module proxy; this does not contact the Internet.
2. Initialize a temporary consumer Go module that requires that dependency and calls an exported function.
3. Invoke real `go mod tidy` with `GOPROXY=file://...`, `GOSUMDB=off`, `GOTOOLCHAIN=local`, isolated temp module/build cache to fill the module cache from local bytes only.
4. Run the **actual Phase15 implementation** `approved_module_cache_digest` and `stage_approved_module_cache` against this temporary source. SHA256-check the staged file list and byte contents. **The hash is self-generated for this synthetic laboratory fixture and is not independently approved release custody.**
5. Disable dependency retrieval using `GOPROXY=off` and run actual `go test -mod=readonly -count=1 -v .` against the temporary consumer, with `GOMODCACHE` pointing at the staged cache. Demand a real `TestOfflineCache` PASS record.
6. Run the identical Go test with a **fresh empty** `GOMODCACHE`, still `GOPROXY=off`, and assert a **nonzero exit**: no silent network fallback, no false green result.
7. Report `SYNTHETIC_LOCAL_GO_CACHE_INTEGRATION_PASS` (never `PRODUCTION_READY`), delete all test modules and caches using a `TemporaryDirectory`. The Go subprocess environment is reduced to basic local variables and explicitly required offline Go variables; no production or cloud/Docker credentials are forwarded.

## Actually executed observed results

| Actual operation | Result | Evidence |
| --- | --- | --- |
| Local file-backed Go module proxy setup | PASS | Temporary, self-constructed dependency; no external network |
| Go mod tidy / module cache preload | PASS | Real installed Go 1.23.2 |
| Phase15 exact code SHA256 source/stage verification | PASS | 8 staged cache files; temporary content pin |
| Real `go test -mod=readonly -count=1 -v .` with staged cache and `GOPROXY=off` | **PASS** | `TestOfflineCache`, package PASS |
| Real Go test with empty isolated cache and `GOPROXY=off` | **BLOCKED as expected** | Nonzero exit, no default proxy |
| Previously implemented unit suites | **89/89 PASS** | Phase09 9, Phase10 12, Phase12 AST 20, Phase13 offline policy 26, Phase15 module cache 22 |
| Full private checked-out TELMIVO Python suite | **NOT_RUN** | This container lacks full authenticated private source checkout |
| Eight real combined Authority/Product Go1.27.2/PostgreSQL16 E2E scenarios | **NOT_RUN** | No Docker socket/image/real PG16; installed Go is 1.23.2 |

The actual committed probe file Git object matches the locally executed bytes:
`tests/installationlicense/phase16_real_go_offline_cache_probe.py`: `9025db7f81fa486d440def3b054f1c5ee5a3ff58`.

The modified Phase11 AST inventory was also verified locally at Git blob `13ea873fec3ff03c242169cd190aa91e7e89b86e`; the optional probe is syntax-checked before Docker in any future complete source checkout. Exact changed-path allowlist is extended from 27 to **29 literal test/doc paths**; strict regression expectation changes accordingly. The probe is *not* included in the unconditional `test_*.py` discovery pattern, as the actual Docker-only suite does not require a host Go installation.

Local evidence SHA256:
- Executed real Go probe JSON: `a3a2d144601864fba5b6550d57705ee973a64c68537c0e667830083902f2a86b`.
- Re-executed 89 Python unit test log: `9a3d9f7871e050db13766ff5632568ea7aec2c5169413c46bbb5665643f8ca74`.

## Reproduce on an authorized disposable workstation

```sh
cd tests/installationlicense
PYTHONDONTWRITEBYTECODE=1 python3 -B phase16_real_go_offline_cache_probe.py

PYTHONDONTWRITEBYTECODE=1 python3 -B -m unittest -v \
  test_phase09_host_requirements test_phase10_image_identity \
  test_phase11_python_source_gate test_phase13_offline_go_policy \
  test_phase15_reviewed_module_cache
```

This is a **standalone Go module-cache mechanism check**. It does not download real TELMIVO libraries, run Authority Go code, test the real eight E2E lease/clone cases, authenticate an external module supply-chain custodian, validate Go 1.27.2 compatibility, or prove any business logic correctness. It is an honest new layer of evidence under the production NO-GO, not an invitation to use the main TELMIVO VPS.

## Mandatory unfinished acceptance gates

- Obtain an **owner-approved independent disposable test host** with exact private Product and Authority checkouts; local Docker Unix socket, independently approved immutable Go1.27.2 and PostgreSQL16 image IDs, and a full externally reviewed/preloaded real module cache whose SHA is independently retained.
- Run source AST and **full actual Python suite** against the final clean Product HEAD, including real Docker mock runner tests; fix any exact failures before proceeding.
- Execute **eight real Go1.27.2/PG16 anti-clone and lease/restart E2E cases** using current pinned runner; confirm all individual `run/pass` JSON events, no network downloads, no unexpected source mutation and successful PostgreSQL cleanup.
- Complete full LIC-A9 race/replay/clone fault matrix, independent disaster recovery, Ubuntu easy-installer validation and owner-signed production release review separately.

**Final:** `PHASE16_LOCAL_REAL_GO_OFFLINE_CACHE_PASS` / `LOCAL_UNIT_89_PASS` / `FULL_PRIVATE_PYTHON_SUITE_NOT_RUN` / `REAL_GO1272_PG16_E2E_NOT_RUN` / `PRODUCTION_NO_GO`.

**Stop at Phase16. Do not merge, deploy, use real signer keys, operate on TELMIVO production infrastructure or claim full license release readiness.**
