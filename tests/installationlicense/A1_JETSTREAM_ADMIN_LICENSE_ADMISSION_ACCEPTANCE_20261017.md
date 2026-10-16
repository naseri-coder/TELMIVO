# LIC-A9 A1 — JetStream administrative and broker-effect admission remediation

Date: 2026-10-17

**Status:** `A1_JETSTREAM_ADMIN_SOURCE_PATCHED / ACTUAL_RUNTIME_EFFECT_GO123_RACE_PASS / 19_STATIC_GITHUB_PASS / PRODUCT_GO1272_NATS_SIGNED_V2_NOT_RUN / PRODUCTION_NO_GO`

## Provenance, restrictions and affected audit findings

- Parent: TELMIVO LIC-A9 Phase51-N [Draft PR #118](https://github.com/naseri-coder/TELMIVO/pull/118) immutable HEAD `b1db302534d3724e1115169b4a06000bb2623ec2`.
- Candidate branch: `development/lic-a9-a1-jetstream-license-admin-fence-20261017`.
- Covers audit findings **F-01** unauthorized JetStream `AddStream` and **F-02** Consumer creation/bind before a signed runtime effect admission.
- No changes to main, Authority, GitHub Actions, Termux, active infrastructure, real JetStream/PostgreSQL, private keys, merging or deployment.

## Actual Product source changes

1. `internal/platform/queue/nats.go`, Git blob `815b4e5d77608dbed7b93fb5d83c5aa21b37822d`:
   - `Client.Ensure(ctx context.Context)` puts `JS.AddStream` **inside** `installationlicense.RuntimeEffect(ctx,...)`, preventing broker stream provisioning with an absent, terminally restricted or revoked installed Manager.
   - `Client.consume` puts `JS.ConsumerInfo`, conditional `JS.AddConsumer` and `JS.PullSubscribe` inside **one** signed protected callback. It does not create or bind Consumers if RuntimeEffect denies admission.
   - `sub.Fetch` is protected by a separate per-fetch RuntimeEffect, eliminating the signed-license revocation window between the existing advisory `CheckRuntime` and the actual broker call. A restricted runtime waits rather than fetching.
   - `m.AckSync`, `m.NakWithDelay` and the DLQ-write plus `m.Term` branch use signed RuntimeEffect callbacks; an expired/revoked license cannot issue fresh queue acknowledgment/disposition commands. After handler commit but before an ACK, denial can cause *redelivery*, which the existing inbox and UNKNOWN idempotency rules must reconcile; no automatic replay of external handlers was added.
   - Previously guarded `Client.Publish`, durable outbox UNKNOWN-before-Publish fence, and JetStream duplicate-ID policy are unchanged.
2. `internal/platform/runtime/runtime.go`, Git blob `0e557b7a4e1f9fb6f93434ebe3f91e1a0482c96b`: startup now calls `q.Ensure(ctx)`. The **specific** `ErrRestricted` denial is not treated as a fatal Startup error, preserving restricted read-only diagnostic routes; all other errors still abort.
3. `internal/platform/queue/a1_jetstream_admission_test.go`, Git blob `41e019af968897190a0b0c47730b8e4e03395b05`: genuine Product child-process tests staged for absent and terminal runtime with `Client.JS=nil`; if a broker admin call escapes the fence it would panic. Includes `Ensure`, `Consume`, `ConsumeExternal`. **NOT_RUN against the full Product**.
4. `internal/platform/queue/a1_signed_v2_broker_admission_integration_test.go`, Git blob `bccb49f941fb874bfa26b406e5cf135ff62985f7`: **integration-tag-only** signed-V2 fixture and zero-network fake JetStream: admit one `AddStream` and one `ConsumerInfo` with a real ephemeral signed TEST-V2 Manager, then `InstallRuntime(nil)` and assert zero new broker calls after revocation. No real broker or persistent signer. **NOT_RUN against Product** due to missing toolchain/dependencies.

## Actual locally executed tests

- Exact current Product `internal/platform/installationlicense/runtime.go` Git blob `f9b580b63cb484dd1428df5580fbcc045341d3f8` copied byte-for-byte into standalone Linux Go1.23 module. Its `RuntimeEffect` and `InstallRuntime` are real Product source, but the Manager is a clearly labeled **TEST STUB**, **not** genuine Ed25519 signed V2.
- Independently executable callback tests model eight JetStream effects (`AddStream`, `ConsumerInfo`, `AddConsumer`, `PullSubscribe`, `Fetch`, `Ack`, `Nak`, `Term`) through real RuntimeEffect; verify all denied before installation, all admitted under test stub valid state, all denied after terminal revocation, and that revocation drains an admitted callback without interruption. No real broker or database contacted.
- `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off go test -race -count=10 -timeout=90s -v ./...`: **PASS**; `go vet ./...`: **PASS**. Log SHA256 `028c3b3115c02d545a9587650b005e2fcfa239877c5e629fec7938e1d76316b9`.
- 19/19 actual GitHub source structural assertions PASS: verify `AddStream/ConsumerInfo/AddConsumer/PullSubscribe/Fetch/Ack/Nak/Term` are scoped by RuntimeEffect, restricted startup branches, no signed fixture import in ordinary queue source and retention of `Publish`/outbox fences. Structural checks ≠ Go1.27 Product compilation.
- Verified offline ZIP `LIC_A9_A1_TEST_EVIDENCE.zip`, SHA256 `a98d4301ac6ea78ecdc424cd523a5fe287099e4abeeba9b2cd18e8383c9f4ae3`, contains byte-exact runtime.go, stub disclosure, test code, logs, reproduction, hashes and static source audit JSON.

## Hard limitations / honest residual risks

1. **Real TELMIVO go1.27.2 compiler build, queue package unit/integration tests, real signed-V2 queue fixture, genuine NATS2.x, PG16, JetStream consumer/stream state and revocation under transport delay are NOT_RUN.** Installed Go1.23.2 and no NATS module cache/Docker. Do not claim P0 E2E resolution.
2. `queue.Open` still performs a NATS connection at startup even in restricted mode, before the guarded `Ensure`. This candidate blocks **JetStream administrative mutations/consumer registration/acknowledgment**, not every socket handshake. Deciding whether restricted mode must suppress even NATS transport connections requires a separate Startup architecture change, preserving `/health/license` diagnostics and activation lifecycle. Do not overclaim complete zero-network-effects.
3. `Client.Ensure` can still report an existing stream configuration conflict, and future multi-role startup may need read-before-create compatibility checks. Do not silently replace/change an existing JetStream stream in Production.
4. Unsubscribe remains an exit cleanup action for a bound durable and is deliberately not wrapped in RuntimeEffect, to allow resource release after revocation; it should not delete the durable. Confirm in real NATS.
5. Dynamic license state while starting a signed worker is guarded; however observing it on live JetStream, verifying shutdown/ACK races and the effect of holding a filesystem lock during NATS commands remain pending.
6. Do **not** merge/activate Actions/Production until real Go1.27.2 + genuine signed Manager + disposable NATS/PG testing and independent review succeed.

**Verdict:** `A1_SOURCE_CANDIDATE_PENDING_NATIVE_PRODUCT_AND_NATS_E2E`; release remains **NO-GO**.
