# LIC-A9 Phase51-E — Bounded Authority admin lock and final JetStream runtime fence

Date: 2026-10-16.
Verdict: **SOURCE-PATCHED / EXACT HELPER RACE PASS / REAL PRODUCT GO1.27.2 + SIGNED V2 + NATS/PG16 E2E NOT_RUN / PRODUCTION NO-GO**

## Scope / immutable provenance

- Parent TELMIVO Phase51-D DRAFT PR #108, commit `0d50f0b3168918189e5dabf4d8967217a47a885c`.
- Branch-only development: `development/lic-a9-phase51e-authority-admin-lock-and-publish-fence-20261016`. No Merge; no writes to Product/Authority main, production services, secrets, owner Termux or GitHub Actions.
- Product still declares `go 1.27.0` in go.mod; user roadmap requires Go1.27.2. Disposable model container is Linux/amd64 Go1.23.2 and cannot resolve GitHub/Go toolchain download DNS. **Full Product Go1.27.2 was NOT_RUN.**

## Genuine repository code changes

1. New `internal/platform/installationlicense/administrative_lock.go` Git blob **`f11f1e8f689fa1925164c4281d8419369fa68e90`**. `m.administrativeLock(ctx, fn)` calls existing Phase51-D `m.lockModeWithWait(ctx, LOCK_EX, 2*time.Second, ErrPending, fn)`, retaining physical Linux file ownership/mode/inode checks and the original exclusive lock through the entire callback. Context cancellation terminates a waiting admission; a busy lock returns `ErrPending` without executing or blindly retrying the administrative callback.
2. `internal/platform/installationlicense/client.go` patched blob **`5bd3fba676f3b54dee80e594c4b62618ab3f5642`**. Five direct blocking Manager lock call sites in Authority client `mutate`, `Activate`, `registered`/Deactivate, `Refresh`, `Status` now use the bounded administrative lock. This includes **outgoing HTTP under the same locked callback**: pending idempotency operation is durably written *before* request; signed envelope acceptance is still under the lock. Network timeout remains 15s; other concurrent administrative/effect callers may fail closed instead of waiting indefinitely. No signed protocol or idempotency-key semantics changed.
3. `internal/platform/installationlicense/concurrency.go` patched blob **`da39a0b7a2dfbfe06d61b2cd5ddee9e830441605`**: `Concurrency(ctx)` uses bounded administrative lock when preparing the signed per-session concurrency request. No secret/Authority source or signed token policy changed.
4. `internal/platform/queue/nats.go` patched blob **`163d133ab970a04e478e8ce0fd85dadb86807f05`**. `Client.Publish` now places the whole final `PublishMsg` inside `installationlicense.RuntimeEffect(ctx, fn)`, rechecking the installed, signed Manager and holding Phase50's in-process effect lease. The earlier `Relay` license preflight is not a durable permission: revocation could previously happen between that check and JetStream I/O. The existing `Nats-Msg-Id`, relay lease, SQL receipt and retry contract are **unchanged**. This change may cause previously ungated direct Publish callers to fail closed until they explicitly install Runtime, and can hold the signed Manager lock for the entire broker operation.
5. New authentic-Product test source `internal/platform/installationlicense/phase51e_administrative_lock_test.go`, blob **`0ea9c62fe79cc1d89c2e9a4f45560382c5a0cb23`**, includes the existing genuine ephemeral Ed25519-signed V2 TEST-only fixture `phase34LicensedManager(t)` with an in-memory HTTP transport. It tests that all six relevant administrator paths fail on canceled/busy entry with **zero Authority HTTP calls**, with signed lease still `VALID_ONLINE` and no new Pending state. Also includes native Manager lock admission/cancellation/pending tests. **This complete Product fixture was NOT_RUN in the available environment.**
6. New actual queue package test source `internal/platform/queue/phase51e_publish_guard_test.go`, blob **`c38525e2c3a9f1401b854c8195d4ca2b39e564b9`**: separate fresh-process default-deny and terminal `InstallRuntime(nil)` cases call `Client.Publish` with `JS=nil`. If the protected transport callback is executed, this intentionally panics. **This real queue package test was NOT_RUN without Go1.27/dependencies.**

## Actually executed standalone offline evidence — precise limits

Disposable `/mnt/data/lica9_phase51e_component` contains:
- **byte-exact real Product `administrative_lock.go`**, `git hash-object` **`f11f1e8f689fa1925164c4281d8419369fa68e90`** matching GitHub.
- **byte-exact real Product Phase51-D `flock_admission.go`**, Git blob `48eb4d88a6587946a92ea47ec45f086b0b969e61`.
- An explicitly **SOURCE-DERIVED STUB Manager** for the physical `lockModeWithWait` checks. It is **not** full Product `state.go`, signed V2, real Authority `Client` or JetStream.
- 15 isolated tests for exclusive Flock, wait timeout, cancellation, busy->ErrPending without callback, callback holding the actual lock, file hardlink and symlink refusal, ownership invariants and invalid arguments.
- Executed `go test -race -count=2 -timeout=90s -v ./...` => **30/30 PASS**. `go test -race -count=5 -timeout=180s -v ./...` => **75/75 PASS**, no detected race. `go vet ./...` exit 0 and `gofmt -l` returned no files.
- Full stress log SHA256 **`2392c0c872d13753287a724bade5ca405f2dcc9250caad251e7ffb27e290b339`**.
- `LIC_A9_PHASE51E_TEST_EVIDENCE.zip` SHA256 **`2f598e49e2ebed5e6401ad3fa65adf4527a5be93276634098b91ef13531da085`**, contains `REPRODUCE.sh`, full raw logs, hash manifest, byte-exact testable helper and clearly labeled Stub. No production keys or customer data.

**GitHub exact-branch static audit**: **22/22 source assertions PASS** across 9 source/test files, including six bounded administrative lock call sites (five in client.go, one in concurrency.go), real `decisionLocked` and journal ordering, final `PublishMsg` inside RuntimeEffect, NATS Msg-ID and durable SQL uncertainty marker. These are **structural/source checks only**.

## Release blockers and remaining unsolved issues

1. **REAL Product Go1.27.2 / genuine signed V2 admin + full Phase45–51 package Race Detector + queue package tests = NOT_RUN.** Neither a standalone helper test nor static matching is a substitute. Cannot claim final integration success.
2. **Ambiguous JetStream publish outcomes remain unsafe to blindly retry.** `queue.Relay` currently releases the outbox lease and applies backoff on *any* Publish error. A network error or lost broker acknowledgment does **not** prove no message was accepted; `Nats-Msg-Id` has finite deduplication, so unknown-result attempts could be duplicated after its window. A separate design/release gate is required for durable `UNKNOWN/RECONCILE` and operator intervention. This phase deliberately did NOT mutate PostgreSQL schema or transactional rollback/retry paths without real PG16/NATS evidence.
3. `queue.Process` preflight vs external handler still has a TOCTOU gap unless every actual provider-side transport is individually guarded via `RuntimeEffect` and durable attempt receipts. `Client.Publish` protects final NATS effects only; it is not universal provider fencing. Recursive Publish from inside an already-held Manager.Effect can hit Phase51-D's 2s fail-closed lock deadline.
4. `Client.mutate` and `Client.Status` **still hold the exclusive Manager file lock during an outgoing HTTP Authority request** (client timeout 15s). Phase51-E bounds other callers' *entry waits*, not the duration of a callback once admitted. Network timeout, cancellation, idempotency response uncertainty and multi-process refresh liveness require genuine integration coverage.
5. Authenticated Authority Product↔Go1.25+/PostgreSQL16 signed session leasing, NATS DLQ, multiple UID/IPCs, provider receipts, releases/easy installer, restart/rollback and independent security review are **NOT_RUN**. No branch is approved for production.

**Decision:** `LIC_A9_PHASE51E_ADMIN_LOCK_SOURCE_PATCHED_AND_NATS_GATE_STAGED / EXACT_HELPER_RACE_75_PASS / SIGNED_V2_PRODUCT_GO127_NOT_RUN / BROKER_UNKNOWN_OUTCOME_RELEASE_BLOCKER / PRODUCTION_NO_GO`.
