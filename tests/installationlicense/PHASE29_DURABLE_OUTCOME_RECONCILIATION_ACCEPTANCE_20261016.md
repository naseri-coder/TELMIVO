# LIC-A9 Phase29 — Durable Outcome Reservation and Reconciliation Proof

Date: 2026-10-16
Status: **NONPRODUCTION PROTOTYPE ACCEPTED / REAL RUNTIME LEDGER NOT INTEGRATED / RELEASE NO-GO**

This stage is a **real, compiled and race-tested offline Go implementation** of an append-only local *synthetic* intent/outcome journal, wired to the Phase28 **mocked final-effect Gateway interface**. It DOES NOT implement an Authority licence server, payment/trading/Telegram transaction, actual Provider settlement, signed installation session, production database or real Product `Manager.Effect` integration.

## Exact source ancestry

- Product parent: Phase28 Draft PR #81, HEAD `f671b85cf993097a3de4f48c85e984e07cc361e3`.
- Authority review-only: Phase21 Draft PR #57, HEAD `d6960982e322d4458c7d963bb153f205a1f8fb59` (unchanged).
- Phase28 `effectgate` module code Git blob `907a7a4a8a57c144e97ba19000704e6f56d731e4`; current Phase29 `go.mod` has explicit **local Go `replace`** into this exact sibling nested test-only module.
- Product `main` and Authority `main` unchanged. Product's Phase19 signed-runtime-session clone fence remains authoritative and unchanged.

## Code delivered

Files **only** under `tests/installationlicense/phase29_outcome_ledger/`:

| File | Git blob SHA (byte-equal to local tested file) |
| --- | --- |
| `go.mod` | `e9de9f478acabea8f52a782abe0856a88414d35d` |
| `ledger.go` | `96260b0128048706580464427ea95eb68224e10a` |
| `ledger_test.go` | `0548d0d9074504e052380269b113c00e46964af0` |

`go.mod` contains `replace telmivo.local/lic-a9-phase28-effect-proof => ../phase28_effect_gate_proof`. This is valid only inside the checked-in `tests/installationlicense/` tree; the root Product application **does not import this module**.

### Local journal contract

1. Accept finite synthetic `SYNTHETIC_RECORD_EFFECT` intents with nonempty `request_id,worker_id,workspace_id,actor_id,resource_id`. Canonical JSON records bind all these fields. No generic shell, real payment, trade or Telegram effect exists.
2. `Reserve` takes the cooperative file `flock`, creates immutable request-ID-hashed entry with `openat(O_CREAT|O_EXCL|O_NOFOLLOW)` under a private fd-opened directory, performs `fsync(file)` and `fsync(directory)` **before** handing control to the synthetic effect. Concurrent reservations yield one winner; duplicates or cross-workspace ID reuse are denied. On incomplete/uncertain durability the retained journal entry **is never automatically removed**.
3. `Inspect` returns `RECONCILE_REQUIRED` for every reserved intent lacking accepted outcome evidence. Missing/corrupt data **fails closed**. Reopening the ledger after process restart does not renew intent permission.
4. `Confirm` requires a **test-only Ed25519-signed provider outcome** whose signature covers domain separator, request ID, full canonical intent SHA-256, provider operation ID and `CONFIRMED` result. It stores a distinct immutable receipt and fsyncs file/directory. Tampered, unsigned or different-key records, attempted history rewrite and cross-tenant proof reuse fail. Only identical already-confirmed evidence is idempotently accepted.
5. This intentionally creates **at-most-once synthetic reservation**, NOT exactly-once external settlement. There is **no** automatic resend after timeout/crash, no treatment of an unknown result as success and no assumption that a third-party platform honors the idempotency key.

### Security prerequisites and limitations

- Test root must be private mode `0700` and owned by the caller. `openat` and `O_NOFOLLOW` help with individual file paths, but the root's **ancestor path trust/secure supervisor FD handoff** and same-UID directory attack require the Phase28 multi-UID architecture and independent end-to-end review. A malicious same-UID writer may replace `_journal.lock`; `flock` alone does **NOT** fence attackers or a stale owner.
- The Go1.23 test-only signed receipt simulates **an independent provider**; it is not a real exchange, merchant, broker, or Telegram attestation. Never transplant the test signing key generator into production.
- Persistent filesystem files with fsync are *not* a PostgreSQL transactional outbox, cross-provider commit log or a formally verified power-loss guarantee; no physical power-failure disk test was run. File corruption yields a fail-closed/manual-reconciliation state, not self-repair.
- `Manager.Effect` remains mocked through Phase28's `effectgate.EffectExecutor`; **the genuine Product `internal/platform/installationlicense/state.go` was not modified or executed in this phase**. Tenant and commercial policies in the Gateway are mocked; there is no actual RBAC/customer entitlement.
- This stage has no genuine Owner epoch fencing of external effects. Phase27 same-UID owner-lock unlink remains a known blocker. Phase28 supervised FD trust boundary must be applied to any later real runtime integration.

## ACTUALLY executed verification

Test environment: disposable local Linux/amd64 container, Go **1.23.2**; no GitHub Actions, internet dependency, live Licence Authority, Docker, production database or real customer effects. Execution uses `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`.

- `go test -race -count=1 -timeout=120s -v ./...`: **23 named Go test functions, 23 PASS, 0 FAIL**, including actual cross-process coordination, crash-after-fsync (child exits with code 77), unknown result + post-restart denial, fake evidence, different provider key, replay, symlink/hardlink, tampered partial/world-readable journal and rejected cross-workspace claim.
- `go test -race -count=3 -timeout=75s ./...`: **3 complete test suites PASS**, **69 named test-function executions**, no reported races.
- `go vet ./...`: **PASS**.
- Another `go test -race -count=1 ./...` PASS after GitHub upload; all three Git blob IDs were fetched through the connected GitHub account and exactly match local `git hash-object` for the executed Go source and module.
- An attempted `-race -count=15` loop was **terminated by local test-command timeout** and has **NO PASS evidence**; it is **not** counted. A smaller bounded 3-run confirmation passed.

Local SHA256 evidence (test-only output files, not uploaded execution attestations):
- Verbose 23-test race run: `3298a512bc02580162b9267c5330033a442595ce987435d424c37251980f60bf`.
- Complete 3-run race: `10ae4fde7eec58a841a58aebee04ba52df66ae2529aaaf113f183786a80013c0`.

**Important evidence distinction:** these are actual runtime Go tests for the separately isolated nested test module, not tests of Product Go1.27, a real Authority V2 signed lease, PostgreSQL16, real systemd/FD security or external provider E2E.

## Acceptance / mandatory blocked gates

**ACCEPT:**
- Synthetic crash-safe *fail-closed design*, append-only intent journal and signed synthetic outcome receipt under cooperative, isolated directory assumptions.
- Test-only Phase28 Gateway interface wiring: pending ambiguous external effect cannot trigger a second synthetic effect even after reopening.
- Exact-source provenance of all 3 Go files and real test logs.
- Product/Authority `main` and Production deliberately untouched.

**BLOCK / NO-GO:**
- Real Product `Manager.Effect` source integration, signed lease V2 refresh/failover, service-manager multi-UID Owner/Worker IPC are **NOT_RUN**.
- Real business `RBAC`, plugin plan/quota enforcement, Provider-specific secure reconciliation, exchange/payment/bot actual effects, database transaction journal are **NOT_IMPLEMENTED/NOT_RUN**.
- Hostile same-UID pathname substitution and distributed former-owner fencing **NOT RESOLVED**.
- Real Product Go1.27 / Authority complete `-race`, 8+ Product×Authority PostgreSQL16 E2E cases, independent two-cluster DR, signed easy installer and owner release approval remain **NOT_RUN / NO-GO**.
- Phase25 Product `main` owner-key custody divergence remains separate; no merge authorization.

**Phase29 test-only deliverables complete. Production NO-GO. STOP HERE; Phase30 needs separate authorization.**
