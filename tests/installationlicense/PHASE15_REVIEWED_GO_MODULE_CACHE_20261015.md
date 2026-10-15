# LIC-A9 Phase15 — Independently reviewed offline Go module cache

**Date: 2026-10-15. Development branch only. Draft / NO Production / NO main merge.**

## Scope and actual source bug

Phase14 TELMIVO Draft PR #68 commit \`6a3e4f1917a82afa0f63bad10b5825f8ac44aad6\` is the exact parent; hardened product runtime is still PR61 \`d88fdb2e4d34f18e8f9c8313f73db5b14a077d5a\`. Selected Authority source remains PR55 \`bfceaef06d41aa185677a796652d17b07979058a\`.

**Found a deterministic Go compilation blocker in the previous Docker-only Runner**: it always created an **empty** temporary \`go_cache/mod\` path, set \`GOMODCACHE\` to that path in its Go container, and demanded \`GOPROXY=off\` in network-isolated mode. The Authority go.mod requires pgx/v5 and other third-party dependencies. The Product go.mod additionally requires NATS, Redis and other modules. Consequently a real compilation could not load any uncached external dependencies without independently staging those bytes. The Phase13 documentation promised a preloaded cache, but the runner did not actually accept or stage one. This is a source-level problem, **not a claim that an actual Go+PostgreSQL Docker run failed**.

## Exact nonproduction remediation

1. Add \`tests/installationlicense/phase15_reviewed_module_cache.py\`: deterministic, SHA256 content/relative-filename inventory of a **fully preloaded owner-controlled Go module cache**. Scope is limited to 100,000 regular singly-linked, protected owner-owned files and 2 GiB. Reject unsafe directory/file types, symlinks, hardlinks, group/other-writable data, unknown paths, empty/non-module caches, changed bytes and unexpected stages.
2. Require independent out-of-band \`sha256:<64 lowercase hex>\` approval. The tool does **not** generate an approved pin from the candidate checkout or imply that a self-reported checksum is an external signature. The owner/reviewer must obtain and retain the expected cache digest from a separately reviewed trusted artifact.
3. Only after source and the separately provided digest match, copy the approved module bytes to disposable \`go_cache/mod\`; make this temporary copy writable by the ephemeral Go test UID and verify the staged bytes against the same digest. Reject and delete a mismatched temporary stage; never write to the approved source cache.
4. Add mandatory CLI options to \`run_joint_docker_only_e2e.py\`: \`--reviewed-module-cache /absolute/preloaded/nonprod/modcache\` and \`--expected-module-cache-sha256 sha256:<approved 64 hex>\`. Syntax, Product checkout, user-approved image/HEAD pins remain mandatory. A cache with an invalid pin fails **before Docker context inspection**; invalid/missing staged modules fail **before any Go/PG container starts**.
5. Preserve \`GOPROXY=off\`, \`GOSUMDB=off\`, \`GOTOOLCHAIN=local\`, image IDs, \`--pull=never\`, \`--network none\` compilation, the isolated PostgreSQL final namespace, positive eight-test JSON evidence requirement and cleanup.
6. Update Docker mocked test fixtures to supply the new mandatory options and mock the stage in successful runs, and add two cases showing invalid pin/missing reviewed stage refuses before any Docker **container**.
7. Extend strict Product PR61 exact allowlist from 24 to **27 test/docs paths**; update AST inventory to include the cache reviewer and its tests; update existing exact-diff regression count. No Go/Authority runtime, database, service, production config, main branch, key, or deploy file changed.

## Actual isolated test evidence

Local Python 3.13.5, five true Python unit suites on byte-identical GitHub files (the tests use isolated synthetic module data; **not** a genuine commercial cache or remote DB):

| Suite | Count | Result |
| --- | ---: | --- |
| Phase09 non-invasive host scanner | 9 | PASS |
| Phase10 immutable Docker image policy | 12 | PASS |
| Phase12 executable AST entrypoint | 20 | PASS |
| Phase13 Go offline argv policy | 26 | PASS |
| **Phase15 reviewed module-cache file/byte policy** | **22** | **PASS** |
| **Total local unit test executions** | **89** | **PASS, zero failed** |

Executed \`PYTHONDONTWRITEBYTECODE=1 python3 -B -m unittest -q test_phase09_host_requirements test_phase10_image_identity test_phase11_python_source_gate test_phase13_offline_go_policy test_phase15_reviewed_module_cache\`.

Exact GitHub blob IDs, matched locally: new cache implementation \`e5df2f52b7b92f1ee1bb7ae0a48259b5e0eb881e\`, new cache tests \`499a0a1c72456fbf5e6b60b79259e113f03dad10\`, updated AST gate \`dad7c379757b23e15c552943725c87e2e48e54ce\`.

The separately modified complete Docker runner/mock suite, full private Product Python tests and actual Go1.27.2+PG16 runtime integration were **NOT_RUN in this session**: no complete authenticated private checkout was available in the model-owned container, and local Docker is absent. Do not interpret 89 Python unit test successes as eight actual LIC-A9 lease/clone scenarios.

## Genuine E2E execution procedure / blocking dependencies

- One **independently authorized disposable nonproduction Linux host**, not the live TELMIVO VPS, with private Product+Authority exact checkouts, local Docker Unix socket, approved immutable Go1.27.2 and PostgreSQL16 image IDs, and a separately authenticated approved module-cache directory with an independently reviewed SHA256 inventory pin.
- The Go module cache must actually contain all dependencies used by the selected fixture and product (not just a synthetic \`pgx\` example). Prepare it in an approved isolated supply-chain workflow, verify checksums and provenance, and record its bytes and SHA256 separately from the candidate being evaluated.
- Execute the exact bytecode-disabled Python AST gate and full checkout unittest suite. Record failures without adjusting security constraints. A full Python PASS is mandatory before proceeding to Docker.
- Invoke the Docker-only Runner using the three existing independent source/image pins **plus** the two new required module-cache arguments. Any missing module/incorrect pin fails closed with no automatic network fallback. Observe actual Go1.27.2 compile and list, PostgreSQL16 boot/restore and eight named individual Go \`run/pass\` JSON events.
- Measure DB cleanup and preservation of the real source, then independent anti-clone/adversarial review, signed installer, disaster recovery, owner signing and Production release approvals remain separate **NO-GO** gates.

## Explicit disposition

\`PHASE15_REVIEWED_CACHE_IMPLEMENTED\`; \`PHASE15_MODULE_CACHE_UNIT_22_PASS\`; \`LOCAL_PYTHON_UNIT_TOTAL_89_PASS\`; \`FULL_PRIVATE_PYTHON_SUITE_NOT_RUN\`; \`REAL_GO_PG16_E2E_NOT_RUN\`; \`PRODUCTION_NO_GO\`.

**Stop at this phase, keep Draft, do not merge to main or touch any live TELMIVO service.**
