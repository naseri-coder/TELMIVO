# LIC-A9 Phase10 — Immutable-source and Docker-image admission gate

**2026-10-15 — test-only development; NO Production, NO main merge, NO real license credentials.**

**Acceptance:** `PHASE10_OFFLINE_PROVENANCE_GATE_PREPARED`; `REAL_GO127_PG16_CROSSREPO_E2E_NOT_RUN`; `PRODUCTION_NO_GO`.

## Inherited exact source and reason for this phase

- Authority Phase07 candidate: `bfceaef06d41aa185677a796652d17b07979058a` (Draft PR #55, unmerged).
- Hardened TELMIVO product runtime parent: `d88fdb2e4d34f18e8f9c8313f73db5b14a077d5a` (Draft PR #61, unmerged).
- Phase08 cross-repo refreeze test source: `f7ba7f1bc2e8f531786aa2bcb0b13dcd1354e9e6` (Draft PR #62).
- Phase09 disposable PostgreSQL runner: `dbf6b334e0ae16c267acd69f2eb83060eac46c4e` (Draft PR #63, this development branch's ancestor).

The Phase09 runner already required clean, reviewed Phase07 Authority source, a PR61 ancestor and **14 allowed test/docs paths**, but checked only the *names* of those paths. A change to the content of an already allowed test or runner file can retain the same changed-file set. It also executed version-labelled Docker images `golang:1.27.2-bookworm` and `postgres:16`, whose tags can be redirected to different bytes. These are specific insufficient-proof findings, not evidence of an actual attacker or compromise.

## Implemented controls

1. `phase10_image_identity.py`: strict lower-case Docker config identity of shape `sha256:<64 hex>`, exact 40-hex commit SHA, and a no-shell locally inspected `docker image inspect --format '{{.Id}}'` identity check. Refuse missing, wrong, corrupt or tag-valued identity. This tool does not generate approved pins from the candidate itself.
2. `run_joint_docker_only_e2e.py`: takes three new out-of-band reviewed CLI arguments:
   - `--expected-product-head`: exact independently reviewed final TELMIVO checkout commit (not `main`, branch, or runtime PR61 ancestor).
   - `--expected-go-image-id`: reviewed local Docker image ID for `golang:1.27.2-bookworm`.
   - `--expected-pg-image-id`: reviewed local Docker image ID for `postgres:16`.
3. After the old source/clean-worktree/allowed-delta checks, the runner calls `check_checkout(product, expected_product_head)` **before Docker**. Absent/wrong pins and changed HEAD refuse. It inspects both preloaded local image labels and matches them to the independently supplied IDs before starting any container.
4. All Go and PG16 `docker run` invocations from the real runner now use the **immutable approved image ID** rather than the mutable tag, and include `--pull=never`. Both command builders now require an explicit keyword-only `image` argument validated as a 64-digit SHA-256 Docker image ID: there is no tag-default fallback, and a direct call with a mutable tag is refused. The existing local Docker Unix-socket and private PG16 `--network none`, ephemeral `PGDATA` tmpfs and cleanup constraints remain unchanged. Docker will not implicitly fetch or switch an image to satisfy the test.
5. `test_joint_docker_only_e2e.py`: updated mocked runner test scenarios to model image inspection and assert exact HEAD check, no pulling, unchanged image IDs, precontainer refusal on incorrect pin, and exact cleanup.
6. The exact changed path allowlist now comprises **17 literal files** under `tests/installationlicense/`; `test_phase08_refreeze.py` expects exactly 17. No `internal/`, `go.mod`, `go.sum`, `deploy/` or production mutation.
7. Added `test_phase10_image_identity.py`: **12 real isolated Python3.13.5 unittest cases** executed locally, validating strict pin formats, image comparison, changed-tag refusal, missing-image failure and no shell execution. Source/test Git blob values have been matched byte-for-byte to GitHub:
   - `phase10_image_identity.py`: `f75b191520e4f2d3d7275a318daf9aacae3cf511`.
   - `test_phase10_image_identity.py`: `17176e79d5db150cd0231da53baa28dbc457b0fa`.

## Reproducible isolated local test

```sh
cd tests/installationlicense
PYTHONDONTWRITEBYTECODE=1 python3 -B -m unittest -v test_phase10_image_identity
```

Observed result **12/12 PASS**, no Docker, no PostgreSQL, no network, no real Go project built. The complete branch Python test suite, including the updated Runner mock tests, still requires a checked-out authenticated private GitHub source and is **NOT_RUN in this working container**. The original Go1.27.2+PG16 eight-case cross-repo tests remain **NOT_RUN**. A mock PASS does not certify LIC-A9.

## Future execution procedure — no guessed trust pins

Preconditions: an **explicitly approved disposable nonproduction host** with trusted local Docker Unix socket, preloaded reviewed Go1.27.2 and PG16 images, matching independently retained SHA256 image IDs, exact Authority PR55 checkout, exact Phase10 Product branch HEAD, no environment variables addressing a real Authority or TELMIVO server, and reviewed test-only `prepare_authority_fixture.py` blob.

Independently obtain and verify the three values with the owner/custodian before running. Never source them from untrusted messages in the candidate bundle, from `git rev-parse HEAD` of the unreviewed checkout alone, or by blindly accepting whatever `docker image inspect` returns.

```sh
python3 -B tests/installationlicense/run_joint_docker_only_e2e.py \
  --authority /absolute/path/to/authority-at-approved-commit \
  --product /absolute/path/to/checked-out-phase10-candidate \
  --acknowledge-disposable-host \
  --expected-product-head REVIEWED_EXACT_40_HEX_COMMIT \
  --expected-go-image-id sha256:INDEPENDENTLY_REVIEWED_64_HEX_GO_IMAGE_ID \
  --expected-pg-image-id sha256:INDEPENDENTLY_REVIEWED_64_HEX_PG_IMAGE_ID
```

Those illustrative argument placeholders are deliberately **not** valid hashes: the program must refuse them. An independent image-config ID validates bytes relative to the review pin but is **not** a signed registry manifest, software supply-chain attestation, or proof that an entire Docker daemon is trustworthy. Retain source commit, manifest digest, registry attestation, expected architecture and compiler module integrity evidence separately. Docker tags must be downloaded and reviewed **before** test execution on an isolated machine, not implicitly pulled by the runner.

GitHub inspection of the final candidate also verified the exact 17-path Product PR61 ancestry diff and nine additional source-level checks concerning the mandatory immutable-image arguments; those structural checks do not establish actual Docker execution.

## Remaining non-waivable blockers

- Real private checkout of Authority and Product plus exact full Python runner test execution: NOT_RUN here.
- Independent human review and independent custody of the out-of-band Product HEAD and image SHA pins: NOT_APPROVED.
- Real Go1.27.2 build plus 8 real individual A8/A9 Go `run` and `pass` events with PostgreSQL16 and TLS: NOT_RUN.
- Client Go1.27.2 race unit suite, Authority Go1.25+ unit/PG16 operator and full clone/replay/race matrix: NOT_RUN.
- Actual Ubuntu installer upgrade/rollback, age disaster restore, independently signed latest pin / offsite recovery, owner release signing, security review: NOT_COMPLETED.

**No real Docker image IDs have been fabricated, inferred or passed off as reviewed. No installer, DB, Production VPS, private owner key, live Telegram bot or `main` was touched. Phase10 strengthens the future test entrypoint but cannot certify its current behavior on a non-existing Docker host.**
