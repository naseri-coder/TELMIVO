# LIC-A9-PHASE-31 — Durable Cross-Process Witness / External Fencing Boundary

Date: 2026-10-16
Disposition: **OFFLINE_GO_MODULE_AND_VERIFIED_TESTS_ACCEPTED** / **HOSTILE_SAME_UID_WITNESS_RESET_REPRODUCED** / **REAL_REMOTE_EXTERNAL_FENCING_NOT_IMPLEMENTED** / **PRODUCTION_NO_GO**.

## Source authority and exact ancestry

- Product Phase30 Draft PR #83, parent HEAD: `e13c50b77ba0e1d66f1c8f35835c40776c2c045e`.
- Authority Phase21 Draft PR #57, HEAD: `d6960982e322d4458c7d963bb153f205a1f8fb59` (read-only reference; no Authority changes).
- Phase31 is stacked only on the Phase30 Product development branch. Previous Phase28/29/30 **test-only** modules are required through relative local Go module replaces; root Product `go.mod`, `internal/` runtime and databases remain unchanged.
- No GitHub Actions, main merge, Production deployment, removed Authority server, main TELMIVO VPS, real signing keys, real provider accounts or data migrations.

## New committed code — real Go, Linux-local and deliberately NONPRODUCTION

Paths within `tests/installationlicense/phase31_durable_witness/`:

| File | Actual Git blob, matched to executable source |
| --- | --- |
| `go.mod` | `0ebd17fff5d50ab6e4d8ec56509f3a1d81298b61` |
| `witness.go` | `c4dd0e85fc2adcbcd36cf0a6bfbf482fb5da099c` |
| `witness_smoke_test.go` | `9e293ee9700d65577df5afe15b9a5dfbf9ca3f76` |

- A supervisor-trusted private mode-`0700` directory stores a persistent single monotonic `epoch.json`; cooperative `flock` and `openat(O_NOFOLLOW)` guard each transaction; writes use exclusive temporary entry, file fsync, fd-relative rename, and directory fsync.
- `Fence(ctx, next)` requires **exactly next=current+1**, so duplicate/skip/downgrade attempts fail closed. This implements the Phase30 `Fencer` interface inside a new, standalone test module; it replaces the previously process-local `TestProvider` with a **shared file-backed local object** for the test.
- `Apply(ctx,epoch,requestID)` rechecks the **latest witness epoch at the same local lock boundary as the full synthetic effect**. The entire synthetic effect is an append-only, fsynced journal record; no arbitrary callback, network call, exchange/payment/Telegram action, actual provider settlement or remote side effect occurs.
- Request IDs are hashed to distinct O_EXCL filenames. Missing/corrupt/mispermissioned state and records fail closed. A previously applied request cannot be reused at a later epoch.
- Reopen after restart preserves epoch and applied records. Two separately launched OS processes sharing this local witness can compete for Fence or an Apply; only one wins. A terminated child after a successful durable write does not grant permission to replay it.
- Phase30 `Supervisor.Acquire` integration is *actually tested* with this new witness and **test-signed mock owner grants**; Phase29 mock-signed pending outcome reconciliation is tested with the witness in the **extended local suite**, not in the committed smoke test.

## Actual executed evidence and source provenance

Environment: isolated model-owned Linux/amd64 runtime, Go `1.23.2`; `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`; no live assets or GitHub Actions.

**Exact GitHub-committed test suite (10 named functions):**

`go test -race -count=1 -run '^TestP31' -timeout=120s -v ./...`: **10/10 PASS**, no reported race; log SHA256 `d3fb12d00148623bf6def7f0b5307ba4eafd2ce76378b5179ed9dd98a47729a1`.

`go test -race -count=8 -run '^TestP31' -timeout=140s ./...`: **8/8 repeated complete smoke runs PASS** = 80 smoke test-function invocations; log SHA256 `1fbaab2539dba9fb766a330ba4fcd450945971b5cd1a5b4f24ebd89b816a7bbb`.

**Extended exploratory local suite:** An additional `witness_test.go` with 30 named functions was exercised alongside the committed smoke suite: `go test -race -count=1 -v -timeout=140s ./...` **40/40 PASS**; log SHA256 `db51431df7208c95e0b957f5da39f9f288d896238848875a13f04337b1253eea`. **The 30-function additional file is NOT committed in this PR**; it is separate local supplementary evidence and must not be represented as a GitHub-committed executable test. A standalone verification archive may be supplied separately.

`go vet ./...`: **PASS**; `gofmt -l witness.go witness_smoke_test.go witness_test.go`: no output. All **three** committed Go source/module Git blobs re-fetched after push match the actual locally executed bytes via `git hash-object`.

The committed Go witness has no internet, root Product database, real signed Authority issuance, or external business integration.

## Critical reproduced security failure — never conflate test PASS with security PASS

**`TestP31KnownSameUIDDirectoryReplacement_SECURITY_BLOCKER` (in the committed GitHub smoke tests) intentionally demonstrates a vulnerability.** A hostile actor able to write in the witness root with the same OS UID can unlink `witness.lock` and `epoch.json` and reopen a new, independent lock inode with a reset epoch of zero. The test passes because it correctly *reproduces the failure*, not because it proves isolation.

**This is a release-blocking vulnerability.** File-backed `flock`, `O_NOFOLLOW`, `O_EXCL` and fsync do not make a same-UID writable pathname an independent trust boundary. The Phase28 supervisor-owned socket/FD ownership model must be adapted to a genuine independently administered witness: dedicated UID/mount namespace and kernel-controlled paths/FDs; independent secure administrator keys and failover policy; real peer identities/authorization; safe bootstrap and state recovery; hostile-UID tests; non-cooperating former Owner.

## Actual external-fencing and delivery limits

- The witness is **local, on one host and one filesystem**. It has no network protocol, independent machine, consensus/quorum, external signing service, immutable remote highwater, hardware attestation, or true remote business provider. It is NOT a distributed fencing certification.
- `Apply`'s append-only file **is itself the entire synthetic effect**, so its serialization does not prove atomicity across an external provider's request, settlement, loss of response, or delayed old-owner network send. A real exchange/payment/Telegram provider might not accept epoch tokens or enforce provider-side fencing.
- Real Product `Manager.Effect`, signed V2 Authority session, actual RBAC/tenants/quotas, independent PostgreSQL16 operation ledger, service-manager multi-UID separation, secure epoch DR and eight genuine Product×Authority E2E tests remain **NOT_RUN / BLOCKED**.
- If a remote witness advances first but the local Phase30 Supervisor commit fails, the test **fails closed** until reviewed recovery; it must not guess or automatically decrement either side.
- This does NOT claim Exactly Once for anything beyond a local synthetic durable record, nor prove fsync's guarantees under physical power-loss conditions on all storage.

## Acceptance

**Phase31 offline prototype implementation and committed smoke tests: PASS.**

**Same-UID hostile-state reset: REPRODUCED SECURITY BLOCKER.**
**Genuine independent remote witness and external provider epoch fencing: NOT IMPLEMENTED / NOT TESTED.**
**Full Go1.27 Product + Authority/PostgreSQL16 real cross-repo 8+ E2E and production release: NO-GO.**

STOP after Phase31. No Phase32, no main merge, no server, no deployment, no Actions.
