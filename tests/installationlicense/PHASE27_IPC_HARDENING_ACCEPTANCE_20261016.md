# LIC-A9 Phase27 — Linux IPC hardening prototype and candid acceptance (2026-10-16)

**Disposition:** `NONPRODUCTION_SOURCE_PROTOTYPE_AND_TESTS_COMPLETED` / `OWNER_ISOLATION_SECURITY_BLOCKER_REPRODUCED` / `REAL_PRODUCT_INTEGRATION_NOT_RUN` / **PRODUCTION NO-GO**.

This is **not** a secure production broker or approved Multiworker runtime. A same-UID attacker able to mutate the owner directory can still defeat pathname-based exclusivity. Reproduction is documented below and is a blocking result, **not** a PASS.

## Exact source baseline

- Product Phase26 Draft PR #79 HEAD `53ce8ffaa7d34cf6fc6515eb1f368ff6ed6b67e8` — Phase27 branch created from this SHA.
- Authority Phase21 Draft PR #57 HEAD `d6960982e322d4458c7d963bb153f205a1f8fb59` — read-only reference; NOT modified.
- Phase19 signed-session runtime guard retained at blob `c5c81548a1b063825eccc9c3d69cc6006160d4cd`.
- Phase26 architecture contract unchanged. The new IPC proof is a separately nested Go module, not an imported Product package.

## Exact committed and executed isolated Go module

All source files were written and actually executed in a disposable model-owned Linux/amd64 container with **Go 1.23.2**, offline `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`, without network, Docker, PostgreSQL, Telegram, actual Authority or customer keys.

| Path within `tests/installationlicense/phase27_ipc_hardening/` | Git blob SHA matched exactly to locally race-tested bytes |
|---|---|
| `go.mod` | `6c2a322e67adae0fb7259024f2dc7bbf8285b13b` |
| `broker.go` | `c592b8a12a22b173970eb818948920df66f379c1` |
| `broker_test.go` | `785353cfd873bdb88b0ecdec956ef7b18d00d681` |
| `phase27_adversarial_test.go` | `95151ffbda114f124b13719c3761f08cb7b2bdf5` |

The Go runtime source intentionally has a strict `//go:build linux && amd64` boundary; raw Linux x86_64 syscall number 437 accesses `openat2`. This is NOT a cross-architecture/portable implementation. Unsupported kernels/platforms are NOT CERTIFIED. No `go.mod` change at the Product root.

## Security work actually prototyped

- Uses Linux `openat2` with fd-relative `RESOLVE_BENEATH | RESOLVE_NO_SYMLINKS | RESOLVE_NO_MAGICLINKS` on the full root path rather than an ancestor `Lstat` walk. Maintains an open directory FD for per-request replay journal entries with `openat` and `O_NOFOLLOW | O_EXCL`.
- Checks owned mode-0700 directory and owned single-link mode-0600 owner-lock file, nonblocking `flock` across handler lifetime, and mode-0600 Unix socket.
- Obtains *real* Linux `SO_PEERCRED` UID on accepted Unix connections; binds it to an in-memory enrolled worker key and workspace mapping. Each test request is canonical strict JSON with a small 2048-byte frame and a single allowlisted synthetic command; signed Ed25519 claim includes worker/workspace, request ID, nonce, operation and timestamp.
- Denies invalid frame, duplicate JSON, wrong UID enrollment, unauthenticated worker, wrong workspace, expired timestamp, bad Ed25519 signature, unsupported command, duplicate request or nonce.
- Persists *synthetic* request ID and nonce reservations with `O_EXCL` and file/directory `fsync` **before** running a synthetic callback. Concurrent same-ID submissions invoke the callback at most once in the test; after owner restart the journal prevents replay. Ambiguous synthetic callback error returns `UNKNOWN_RECONCILE_REQUIRED`, with no blind retry.
- Concurrent `Close` drains handlers before dropping cooperative owner lock.

**Not implemented**: actual per-service-user Unix access, live systemd socket/FD handoff, audited worker key enrollment/rotation/revocation, actor/resource/installation binding of IPC message, server-side real Product tenant/RBAC/entitlement/quotas, `Manager.Effect` integration, real provider outcome ledger, monotonic epoch witnesses or production worker failover. Same-UID workers were used in the successful local tests, so these tests do not prove multiple OS identities can operate the interface.

## ACTUALLY EXECUTED tests (exact committed Go file blobs)

- `go test -race -count=1 -v ./...`: **19/19 Go test functions PASS**, no reported Go data race. Includes worker signature, UID mismatch, cross-workspace denial, request/nonce replay, 2048-byte bounded frame, symlink rejection, cooperative owner competition, ambiguity and journal across restart.
- `go test -race -count=25 ./...`: **25/25 complete suites PASS**, 475 test-function executions.
- `go vet ./...`: **PASS**.
- Go module and all three Go source blobs matched GitHub HEAD exactly after push.
- First-pass suite log SHA256: `48d42c99d2c3cdd96ceb90ed6a3983b45b8b8f635faa08889a5ce32e6b477572`.
- Stress run log SHA256: `6c5b7259d98dd3a01a9c77c96bca0dc0e572cbf93701ba26237a23c940c3945b`.

## Reproduced critical isolation limitation

An **additional disposable exploratory adversarial test**, deliberately NOT counted in the 19 canonical success cases and NOT committed to Product, ran with `go test -race`:

1. Start a cooperative Owner.
2. As another actor with the same OS UID and write access to its 0700 directory, unlink `owner.lock` and `broker.sock`.
3. Open a second Owner on the **same directory path**; the operating system admits a new separate owner lock inode despite the first owner still holding the old one.

**Observed:** Second `Open` succeeds. Log SHA256 `a83ed1ef6a7db2be38bbed31cdec58eae4ac6b6231e314bb200b13a98508fd58`. The exploratory test was written to PASS when it demonstrated the defect; this does **not** indicate a secure multi-owner design.

This shows why `openat2` and `flock` individually are not sufficient as proof against a hostile same-UID directory writer. Current ephemeral Unix socket `0600` inside mode-0700 directory also blocks distinct normal Unix UIDs from connecting; a real multi-UID service/socket activation or trusted FD handoff needs to be designed and tested separately. **No implementation or Production deployment is authorized until process identity and directory trust separation are established and independently verified.** Avoid claiming secure same-UID isolation. Potential approach: supervisor-owned immutable routing/ownership boundary with systemd socket activation or verified namespace/FD handoff; select after independent host threat-model validation, never through permissive shared-directory fallback.

## Remaining hard gates

1. **Owner isolation: BLOCKED / reproduction** — prove exclusive owner under genuine distinct service UID/peer credentials and robust socket namespace/FD ownership; handle malicious same-UID path mutation or explicitly exclude same-UID untrusted principals with enforceable isolation.
2. IPC command needs exact Phase26 required fields and binding (installation, actor, domain resource, payload digest) with authoritative server-side enrollment/authorization.
3. Real `Manager.Effect`, signed V2 session, live cancellation/leader handoff, effect outcome reconciliation: **NOT_RUN**.
4. Full exact Product Go1.27/Authority Go race suites, eight real Go/PostgreSQL16 cross-repo E2E, independent DR, signed easy installer: **NOT_RUN**.
5. Independent security review, reconciliation of two Product `main` owner-key commits and explicit release approval: **PENDING**.

## Guardrails

This phase adds only four nested Go proof files and this report; **no Product runtime files, Authority, SQL, production container, `main`, GitHub Actions, real signer, database or TELMIVO host touched**.

**Phase27 research/prototype completed. Secure operational owner isolation remains a release-blocking FAIL; Production NO-GO. STOP after Phase27.**
