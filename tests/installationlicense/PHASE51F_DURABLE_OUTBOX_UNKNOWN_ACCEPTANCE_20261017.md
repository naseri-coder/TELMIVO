# LIC-A9 Phase51-F — Durable JetStream UNKNOWN quarantine; no blind outbox replay

Date: 2026-10-17. Status: **DRAFT CODE CANDIDATE / SOURCE_CONTRACT_PASS / GENUINE PG16+NATS+PRODUCT_GO1.27.2 NOT_RUN / PRODUCTION NO-GO**.

## Repository lineage and scope

- Parent: TELMIVO Phase51-E Draft PR #109, immutable head `86d755eb375e328cde7ef774e7ab33f15a102e67`.
- Review branch: `development/lic-a9-phase51f-durable-outbox-unknown-20261016`. Only this review branch was modified.
- No GitHub Actions, merge, main changes, production/surrogate customer database, private release keys, Termux, real Authority/provider operation or server provisioning.
- Product `go.mod` declares `go 1.27.0`; requested acceptance target is Go1.27.2. Local isolated model container has Linux/amd64 **Go1.23.2 only**. The attempt `GOTOOLCHAIN=go1.27.2 GOPROXY=off GOSUMDB=off go version` failed: `toolchain not available`. PostgreSQL, nats-server and Docker are not installed in this isolated runtime. No full Product or genuine Broker/DB integration was run.

## Proven production-source repair — code staged, NOT operationally released

1. New migration `db/migrations/0019_outbox_unknown_delivery.sql`, Git blob **`d5bf42119ec0ad178658c7e79df24e01dc9b35c0`**:
   - Adds `delivery_state` (`READY`, `UNKNOWN`, `PUBLISHED`), `delivery_attempted_at`, and indexes for READY vs operator-reconciled UNKNOWN.
   - Backfills **historically acknowledged** rows (`published_at IS NOT NULL`) to PUBLISHED, with `delivery_attempted_at=published_at`, before adding consistency constraints.
   - Trigger prevents `UNKNOWN->READY`, `READY->PUBLISHED` without a prior durable UNKNOWN, any `PUBLISHED->READY` transition, and deletion of an UNKNOWN record. Guards READY->UNKNOWN attempt counter, owner and lease fields. No automatic reopening of unknown delivery is implemented.
   - It is an additive migration in Git source only; **PostgreSQL DDL and data backfill have NOT been executed**. All old Relay versions MUST be stopped before a future authorized migration. Mixed-version dispatch would defeat the guarantee.
2. `internal/platform/queue/nats.go`, Git blob **`6943f5bd294235dc2721f8966c6ec76dbd45f9ac`**:
   - `Relay` selects at most **one** READY event, using `FOR UPDATE SKIP LOCKED`.
   - Inside the same transaction, atomically marks it `UNKNOWN` with owner, attempted timestamp, +1 attempt count, and a lease; **commits UNKNOWN before any JetStream publish**. A failed/ambiguous DB claim commit causes **no broker I/O**.
   - `Client.Publish` remains guarded with `installationlicense.RuntimeEffect` from Phase51-E; `Nats-Msg-Id` remains the original event ID.
   - Only a successful JetStream PublishMsg acknowledgment authorizes a CAS SQL transition from UNKNOWN to PUBLISHED using the same owner; missing DB acknowledgment remains UNKNOWN until reconciliation.
   - On **any** publish error (including timeout, lost broker ack, revoked Runtime), returns new opaque `ErrOutboxDeliveryUnknown` and leaves the UNKNOWN row in the DB; **removes previous automatic lease-release and backoff that could resend a published message**.
   - This provides an **at-most-one automatic publish attempt per outbox event under the updated code/schema**, not end-to-end exactly-once delivery and not proof that broker or downstream consumer never duplicates. It can intentionally quarantine a pre-marked event that never actually went onto the wire (availability trade-off).
3. New `internal/platform/queue/outbox_fence.go` with three security-reviewable SQL constants, exact Git blob **`86936c5db80c0d718367e61e3e1bf491e512fd72`**, used by the production `Relay`.
4. New `internal/platform/queue/phase51f_outbox_fence_test.go` exact Git blob **`8677362c8488b7dbf4318143ffe3b4e8663ed521`**. Deterministic SOURCE / MODEL cases covering crash-before-publish, accepted message/lost ACK, network timeout, revocation after claim, failed receipt persistence, and acknowledged terminal transition. The model is not a live broker.
5. New `db/migrations/phase51f_outbox_migration_test.go` exact Git blob **`42ed0c05639695125055a00af348b6ee74eb6bd4`** checks the **actual embedded SQL bytes** and backfill-before-constraint/trigger order. Does not execute SQL in PostgreSQL.
6. `tests/integration/native_test.go` now expects **19** migrations rather than 18; exact Git blob **`31dd4f20f90bbc9696ffe1bae870abe2833b925c`**. Without this change `TestMigration` would predictably fail after introducing 0019. Existing `TestOutboxLease`, `TestDownloadQueue` and `TestJetStreamRecovery` still require native regression.
7. New `tests/integration/phase51f_outbox_integration_test.go`, exact Git blob **`67e6e0ab33619726c4c94f7df96e305714248342`**, uses the existing strict disposable `TEST_DATABASE_URL` / `TEST_MIGRATION_DATABASE_URL` harness, calls `owner.Migrate`, commits a synthetic READY->UNKNOWN row, verifies expiry does not make it eligible, asserts trigger rejection of UNKNOWN->READY and PUBLISHED->READY, and models acknowledged UNKNOWN->PUBLISHED. **This PG integration test is NOT_RUN** in this session. It does not reproduce real broker acknowledgment loss.

## Actually executed offline tests; exact scope of PASS

A disposable Go1.23.2 module was assembled with **byte-exact GitHub source files**:
- `internal/platform/queue/outbox_fence.go` `86936c5db80c0d718367e61e3e1bf491e512fd72`;
- `internal/platform/queue/phase51f_outbox_fence_test.go` `8677362c8488b7dbf4318143ffe3b4e8663ed521`;
- `db/migrations/embed.go` `381665ad5afd0b1abd5fa48393be336a42890520`;
- `db/migrations/0019_outbox_unknown_delivery.sql` `d5bf42119ec0ad178658c7e79df24e01dc9b35c0`;
- `db/migrations/phase51f_outbox_migration_test.go` `42ed0c05639695125055a00af348b6ee74eb6bd4`.
The full Go syntax/format of the new Go1.27-only integration test source was locally parsed by `gofmt -l` against the **exact Git blob** `67e6e0ab33619726c4c94f7df96e305714248342`, without compiling or connecting to PG.

Executed `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off go test -race -count=10 -timeout=120s -v ./...` across **two isolated actual-source packages**: PASS; **40 top-level test invocations**, plus **50 named scenario subtest invocations**, zero Race Detector failures. Actual output log SHA256 **`c10d986956c006e3a9859388faadf12b02d9bee38b35487ef0d661aa2e687a89`**. `go vet ./...`: PASS (empty output, SHA `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`). `gofmt -l` empty on tested sources. These are **Go source/contract and in-memory model tests**, not a real DB/NATS integration, not the full nats.go package build, and not genuine signed V2.

The separate exact GitHub-head source review had **23/23 static structural assertions PASS**, including SQL state guards, migration backfill ordering, claim Commit before Publish, no automatic lease-release retry branch, owner-fenced confirmation, Phase51-E RuntimeEffect, and existing native migration-count update. This is not executable integration evidence.

Evidence bundle: `LIC_A9_PHASE51F_TEST_EVIDENCE.zip`, with runnable offline source, exact Git blob SHA manifest, raw logs and toolchain probe.

## Explicit remaining blockers (NOT_RUN / NO-GO)

- Real Product Go1.27.2 `go test -race ./...`, actual PostgreSQL16 0019 migration execution/backfill/trigger enforcement, JetStream PublishMsg lost-ACK/timeout/crash-after-claim under real broker, existing Queue/Integration/Startup worker suites: **NOT_RUN**.
- No automatic reconciliation or re-publish of quarantined UNKNOWN events exists. **Do not** treat an expired lease as delivery proof; an operator needs independently corroborated broker/consumer evidence and explicit owner-approved per-event resolution. Ambiguous outcomes that never reached NATS will remain quarantined until handled.
- The migration must **not** be applied while any old Relay process is running or able to publish. Old workers ignore `delivery_state` and could replay events. Deploy sequencing, migration transaction/locks, old running roles and read permission tests require controlled disposable/native evaluation.
- Unknown preservation must be assessed with storage retention, NATS dedup window, downstream inbox/consumer uniqueness, max-delivery, RLS, and incident workflows. The prior `queue.Process` admission-to-provider TOCTOU remains outside the new Outbox. Direct `Client.Publish` bypasses the outbox ledger but remains runtime-licensed; it needs independent consumer/idempotency review.
- No merge to `main`, Production, Actions, License Authority changes, signing-key use, or real provider effects occurred.

**Verdict:** `LIC_A9_PHASE51F_DURABLE_UNKNOWN_QUARANTINE_SOURCE_READY / OFFLINE_RACE_MODEL_PASS / PG16_JETSTREAM_PRODUCT_GO127_SIGNEDV2_NOT_RUN / PRODUCTION_NO_GO`.
