# LIC-A9 PHASE32 — Supervisor-Owned Witness Boundary and Actual Linux UID Isolation

Date: 2026-10-16

**Acceptance:** `PHASE32_NONPRODUCTION_LINUX_UID_BOUNDARY_VERIFIED` / `REAL_AUTHORITY_MANAGER_AND_EXTERNAL_PROVIDER_NOT_INTEGRATED` / `PRODUCTION_NO_GO`.

## GitHub ancestry

- Product Phase31 Draft PR #84 HEAD: `bd07153a8b86d3d664c8c8df3d70ef468a3794f2`. Phase32 branch created from this exact commit.
- Authority Phase21 Draft PR #57 HEAD: `d6960982e322d4458c7d963bb153f205a1f8fb59` (read-only; Authority repo untouched).
- All changes confined to `tests/installationlicense/phase32_supervisor_boundary/` and this report.
- No Production host, real key, installation, Product `internal/` runtime, root `go.mod`, SQL migrations, `main` branch, GitHub Actions, or actual customer effects touched.

## Deliverables and exact provenance

| Path | Git blob matching exact locally race-tested bytes |
| --- | --- |
| `tests/installationlicense/phase32_supervisor_boundary/go.mod` | `4c60d6bb276ad46a84fe92a8412aaaab36728f33` |
| `tests/installationlicense/phase32_supervisor_boundary/supervisor.go` | `b0a6e88cee953eb1adc5a78828150c6fc2d6fe61` |
| `tests/installationlicense/phase32_supervisor_boundary/supervisor_test.go` | `2a718d35245c9641fd5cc338522c611e18b2007f` |

Separate nested Go1.23 Linux-only module uses only Go standard-library packages and the Phase31 locally persisted witness via local `replace`. This is an **executable proof** but **NOT** integrated with Product's real Go1.27 `Manager.Effect`.

## Technical design actually exercised

- Runs a disposable privileged **root supervisor** that alone can open a root-owned mode-`0700` witness directory; Client/Owner Unix UID `25111` is dropped to an actual unprivileged process, and outsider UID `25112` is denied.
- A different **root-owned** IPC namespace is mode `0750` with group `25111`; the supervisor creates a Unix socket mode `0660` root:25111. The authorized unprivileged Owner can *connect* but cannot unlink/replace the listener, create an alternate listener inside its directory, or unlink/truncate the witness files.
- Receives actual kernel `SO_PEERCRED` UID from each accepted Unix connection; enforces configured UID plus a bounded (2048-byte) **canonical JSON**, test Ed25519 issuer signature, installation/owner/session/epoch, short validity and a strict `FENCE_SYNTHETIC_EPOCH` vs `APPLY_SYNTHETIC_RECORD` command allowlist. Any shell, payments, trading or arbitrary callback command is rejected.
- The root-side `Phase31.Witness` applies strictly monotonic epoch fencing. Every synthetic Apply rechecks the actual current persisted epoch at the same witness file lock as recording the entire *synthetic* effect. Duplicate request IDs are denied; concurrent distinct OS processes yielded one winner.
- **Supervisor restart is fail-closed:** its in-memory `readyEpoch=0` disallows an old Owner's previously signed Apply after restart until a **new** signed Fence at the next epoch succeeds. Previous epoch requests are rejected after that takeover. The Phase31 witness's own persistent epoch survives restart.
- Private test signer keys are generated at runtime in the disposable parent test process. Only already-signed wire envelopes are passed to unprivileged subprocesses through their environment; no real secrets are used.

## Real executed verification, source matched to GitHub

Environment: disposable Linux/amd64 container with euid=0 for the test harness only, Go `1.23.2`, kernel Linux `6.18.44`, offline `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`.

- `go test -race -count=1 -timeout=80s -v ./...`: **14/14 Go test functions PASS**; no reported race; full test run took approximately 26 seconds.
- Included actual unprivileged subprocesses (not impersonated UID fields), nonmatching kernel UID refusal, root-owned path unlink/rename denial by UID25111, forged signed frame rejection, wrong installation, expired grant, replay, post-restart quarantine, root-owned socket permissions, mode-0777 refusal, stale epoch refusal, and four parallel child processes competing for one synthetic effect.
- `go test -race -count=2 -run '^TestP32RealWorkerUIDAndSignedFence$' -timeout=40s -v ./...`: **2/2 repeat runs of the selected real UID / signed-claim test PASS**.
- `go vet ./...`: **PASS**; `gofmt -l supervisor.go supervisor_test.go`: empty (**PASS**).
- The exact GitHub file blobs were re-fetched and compared to `git hash-object` for each of the three local files tested: **3/3 byte-equal**.
- Full suite log SHA256: `17e7ca20dd30509bfbb3d167d19f4310e7abaaef603a26518940858789be9993`.
- Two-run selected test log SHA256: `582ce4a35885290e9baf1cf468e0f8bd3ce7be3eb4a0ab1b773433674547a820`.

**Negative execution notes:** The *first* attempt to run actual UID subprocesses failed because `go test` places its child test binary under a root-only build directory. The test harness was repaired to expose the **same race-instrumented executable** in a disposable, traversable directory; then the complete 14/14 suite succeeded. A later attempt to run a whole-suite `-count=2` was interrupted by its enclosing execution timeout and is **not claimed PASS**. The selected repeated identity test above was successfully verified. An earlier full-suite run (before a cleanup refinement) also passed, but is not used as the final-source evidence. No real service-manager multiuser environment was configured persistently on the user's host.

## What is genuinely resolved, and what is still blocked

**PASS in test model:** The specific Phase31 hostile **unprivileged same-UID Owner** path-reset attack now fails because the Owner is no longer the OS principal owning/modifying the witness directory or Socket namespace. Crucially this is a *change to trust boundaries*, not a claim that `flock` magically prevents pathname attacks. The tests use actual Linux UID credentials and privileges.

**NOT CERTIFIED:**

1. The trusted root supervisor itself, privileged path ancestors, filesystem/mount namespace, root compromise, supply-chain privileges, Linux service-manager lifecycle, actual `systemd` socket activation/FD handoff, shared group enrollment, credentials across multiple hosts, and operator-independent high-water witness custody. Root-owned temporary test directories are **NOT** a hardened production filesystem and the code checks neither all parent components nor kernel syscall-exploitation risks.
2. The test Ed25519 signer is **NOT** the real TELMIVO License Authority V2 lease. This does **NOT** invoke the genuine Product `Manager.Effect`, runtime session anti-clone decision, real worker RBAC/entitlements/quotas, or DB transaction/outbox.
3. The witness and its synthetic Apply are **on one local filesystem**, not a genuinely independent remote authority, quorum, exchange, payment processor or Telegram sender. No remote provider is required to enforce the epoch. Old-owner network packets already in flight and external at-most-once settlement remain **NOT_TESTED**. This does **NOT** establish exactly-once delivery.
4. A malicious process with root access or writable trusted ancestor could manipulate the witness despite this boundary. Trusted supervisor compromise remains out of scope.
5. Full real Product Go1.27 and Authority/PG16 cross-repository integration, 8+ E2E scenarios, signed real installer, independent disaster recovery, main divergence resolution, and release approval are **NOT_RUN**.

## Decision

**Phase32 restricted root-supervisor proof and exact-source local tests: PASS.**
**End-to-end Owner/Worker Runtime integration: BLOCKED / NOT_RUN.**
**Real external Provider fencing, independent remote witness, real Authority signed lease: NOT IMPLEMENTED.**
**PRODUCTION RELEASE: NO-GO.**

Stop after Phase32. Keep Draft; do not merge to main, run Actions, touch servers or deploy.
