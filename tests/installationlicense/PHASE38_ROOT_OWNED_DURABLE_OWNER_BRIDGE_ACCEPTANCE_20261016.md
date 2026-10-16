# LIC-A9 PHASE 38 — Root-Owned Durable Synthetic Owner Bridge

**Date:** 2026-10-16  
**Scope verdict:** `LINUX_REAL_MULTI_UID_SUPERVISOR_JOURNAL_PROOF_PASS`  
**Product Go1.27 real Manager integration:** `NOT_IMPLEMENTED/NOT_RUN`  
**Release:** **PRODUCTION NO-GO**.

## Source ancestry, admissions and constraints

- Product parent is Phase37 Draft PR **#90**, SHA `0c6b39700c8e3cfa4ec31c33fcfe80b6760e14ad`.
- Phase37 **nine real Product `Manager.Effect` tests** were independently executed on the user's Android/Termux Go1.27.2 at that exact parent SHA: **9/9 PASS**. This is a stage-entry functional prerequisite, NOT a Phase38 production integration result.
- License Authority separate Draft PR **#57**, SHA `d6960982e322d4458c7d963bb153f205a1f8fb59`, is unchanged and review-only.
- Stage38 introduces a NEW **standalone, test-only** Go1.23 module in `tests/installationlicense/phase38_supervisor_durable_bridge/`. It is NOT imported by Product runtime; root `go.mod`, Product `internal/` and Authority remain unchanged.
- No `main` merge, Go Actions, Production service/host, deleted license VPS, credentials, DB, live provider operation or installer were touched.

## Exactly tested GitHub-delivered files

| Path under `tests/installationlicense/phase38_supervisor_durable_bridge/` | Git blob SHA; exact local tested-byte match |
| --- | --- |
| `go.mod` | `d8888e69d5ba4adb281745494b043be553533ae5` |
| `bridge.go` | `69b37480c05637ee3bea7e6b551ecf0437372e8f` |
| `bridge_test.go` | `1dfc93c59a2c595a32b21bf16450fab2cf8cedf1` |

All 3 files were independently refetched from GitHub after upload and matched the local `git hash-object` of the *actually executed* source. Temporary encoded transfer segments were deleted; they do not remain in the review diff.

## Test-only Linux security boundary implemented

A root-owned supervisor creates and exclusively opens a private mode-0700 journal root and a separate mode-0750 root-owned Unix-socket directory (group = approved worker's real Linux GID). The listener is mode-0660 root:worker-GID.

- Tests drop an actual child process to **UID/GID 25111** for authorized Worker IPC and to **25112** for denied outsider IPC. The supervisor uses kernel `SO_PEERCRED` UID, not a client-supplied UID.
- Signed, canonical, size-bounded and time-bounded synthetic Ed25519 claims include installation, owner, session, monotonically increasing epoch, action and tenant/actor/resource for synthetic intent only. Unknown/forged/cross-tenant operations fail closed.
- The **supervisor itself**, not the Worker UID, owns `_root.lock`, `epoch.json`, request reservations and provider evidence. `O_NOFOLLOW`, owner/mode/inode validation, `flock`, exclusive request files, file fsync and directory fsync implement an offline, local durable test journal. Same UID as Worker cannot unlink/replace journal or socket namespace in the tested Linux permissions model.
- A signed `FENCE` requires precisely `currentEpoch + 1`; any unconfirmed persistent reservation blocks handoff. `APPLY` durably records a synthetic intent and responds `RECONCILE_REQUIRED`: **no remote callback, payment, Telegram message or exchange order is executed**. Repeated IDs cannot create a new effect intent.
- `CONFIRM` requires independently signed **mock** provider receipt bound to the original claim hash, request ID, provider ID and result `CONFIRMED`. Forged proof is refused; only a complete set of confirmed outcomes permits new epoch handoff.
- A supervisor restart quarantines old signed sessions via an **in-memory fresh-fence gate**. A real separate root supervisor subprocess was terminated abruptly by `os.Exit(77)` after a Worker obtained the result of a fsync-backed journal reservation. The trusted parent verified the crashed socket's owner and dead listener, removed the stale root-owned socket pathname, reopened the journal and proved old-session apply denied and next epoch blocked by the unresolved outcome.
- No automatic retry or permissive conflict reset.

## Actually executed evidence, not inferred

Host: disposable model-owned Linux/amd64 kernel 6.18.44, `go version go1.23.2 linux/amd64`, **euid=0 only inside test container**, offline (`GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`). This is NOT a Product server.

The final exact-byte source was compiled with `-race`; the 13 named `TestP38*` functions consist of **11 substantive security tests** and **2 child/helper entry functions**. The tool enforces a short external wall clock, so one combined verbose full-suite invocation **TIMED OUT in its enclosing tool before completion**, and is **NOT a full-suite PASS**. The suite was then split into two disjoint, individually completed runs:

1. Group A, `go test -race -count=1 -timeout=90s -v -run '^(TestP38Child|TestP38PermittedWorkerUidAndDurableReserve|TestP38KernelRefusesUnapprovedUID|TestP38UnprivilegedWorkerCannotReplaceRootJournal|TestP38SignedTenantAndForgedKeyDenied|TestP38PendingBlocksHandoffUntilSignedReceipt)$' ./...`: **PASS**, exit 0, including real separate-UID IPC and proof-bound handoff.
2. Group B, `go test -race -count=1 -timeout=90s -v -run '^(TestP38RestartRetainsPendingAndBlocksNewEpoch|TestP38ConcurrentDistinctUIDWorkerProcessesHaveOneWinner|TestP38CorruptJournalFailsClosed|TestP38MispermissionedRootRejected|TestP38StaleOrReplayFenceDenied|TestP38CrashSupervisorChild|TestP38AbruptSupervisorExitDoesNotPermitBlindRetry)$' ./...`: **PASS**, exit 0, including 8 separate UID Worker contenders, corrupt-journal denial and true `os.Exit(77)` root supervisor process crash.
3. `go vet ./...`: **PASS**. `gofmt -l bridge.go bridge_test.go`: empty, **PASS**. GitHub blob SHA: **3/3 exact matches**.
4. Group A stdout/stderr SHA256 `091f63e527cc24b0572bd5b5d25f98534e366741bcde10febd7927674866b63e`.
5. Group B stdout/stderr SHA256 `f30af8a9f142d0a3690443d53c14ec23ca7bdab9823f1cd9031830e54a5a6063`.

**This verifies Linux multi-UID *synthetic* service behavior only, not a real TELMIVO license grant, domain authorization, root process hardening, an external provider or actual Product `Manager.Effect`.**

## Non-waivable release blockers

1. **Real Product Manager NOT CONNECTED:** `bridge.go` is a separate test-only module and does not invoke the genuine Product Go1.27 `Manager.Effect`. Phase36/37 genuine direct Manager tests succeeded only in their own test-only relay; they do not mean a real supervisor now runs Product.
2. **Actual credentials NOT USED:** all Ed25519 keys and sessions are synthetic, test generated. No Authority-issued Worker identity, immutable trusted public roots, revocation protocol, RBAC, quota enforcement or real cross-repo signed V2 lease in this module.
3. **Real remote fencing NOT PROVEN:** `RECONCILE_REQUIRED` represents only a local synthetic durable record, not remote exactly-once effect delivery. No authenticated real provider callback/receipt and no PostgreSQL16 transaction, outbox, independent highwater network witness, replication, disk powerfail verification or DR.
4. **Root supervisor trust, ancestors and deployment NOT CERTIFIED:** a hostile privileged/root process or untrusted writable ancestor is outside this proof. No verified service-manager lifecycle, systemd socket activation, linux capability/confinement design, remote/network partition, TPM or FS hardening. Crash recovery requires a **trusted operator** to verify and remove the stale socket; the program does not bypass it automatically.
5. **Phase22 multiworker liveness OPEN:** valid workers can submit a bounded synthetic request to a single Owner, but this does NOT establish reliable concurrent Business/Production worker execution, actual customer ownership and availability.
6. **Linux Go1.27 Product hardlink 5 cases and full Product race suite NOT_RUN:** Android Go1.27.2 had five hardlink test `SKIP` due kernel restrictions, and no Android `-race`. This separate standalone Go1.23 Linux module does not satisfy that release gate.
7. Actual two-repo Authority/PG16 eight-scenario E2E, signed easy installer, root-key custody reconciliation, manual review and release approval remain blocked.

**PHASE38 OFFLINE LINUX MULTI-UID SECURITY PROOF: PASS in tested scope. PRODUCT RUNTIME INTEGRATION: NOT_RUN. PRODUCTION: NO-GO.** Keep Draft. Stop after Phase38.
