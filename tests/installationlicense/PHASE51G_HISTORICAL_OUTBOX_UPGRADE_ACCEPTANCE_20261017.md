# LIC-A9 Phase51-G — Historical outbox attempt quarantine and old-Relay mutation gate

Date: 2026-10-17. Status: **REAL SOURCE PATCHED / EXACT-SOURCE OFFLINE RACE + SQLITE SUBSET PASS / REAL PRODUCT GO1.27.2 + POSTGRESQL16/NATS + SIGNED V2 NOT_RUN / PRODUCTION NO-GO**.

## Verified parent and safety constraints

- Base: TELMIVO Phase51-F DRAFT PR #110, immutable HEAD `d355018fe503cf5e0f6c2e539f328d3d2590a5a0`. Branch: `development/lic-a9-phase51g-legacy-outbox-quarantine-20261017`.
- Only Product review branch source was changed; NO merge, Actions, Termux, actual TELMIVO/Authority servers, production DB, live NATS, private signing keys, provider-side effect or mainline mutation.
- Model's disposable Linux/amd64 Go toolchain is **Go1.23.2**. `GOTOOLCHAIN=go1.27.2 GOPROXY=off GOSUMDB=off go version` returned `toolchain not available`. `postgres`, `psql`, `nats-server`, Docker/Podman absent and public DNS unavailable from shell. No genuine Product Go1.27.2 or PG16/NATS integration execution.

## Security finding from the real Phase51-F source

Old `queue.Relay` increments the durable `outbox_events.attempts` before attempting to publish. A timeout or lost broker ACK could leave `attempts>0` and `published_at IS NULL`, possibly after the legacy 60-second lease expired. Phase51-F migration `0019` initially assigned **all** unacknowledged rows the default `READY`, including historically attempted rows. This could silently replay old UNKNOWN outcomes after upgrade, defeating the otherwise sound Phase51-F at-most-one-automatic-publish policy.

Phase51-G uses the **same unmerged draft migration**, not a modification to any released schema:
- `db/migrations/0019_outbox_unknown_delivery.sql` Git blob `e5d8f66d0326c29ca1b31ef8f4fb626e192e7098`: first UPDATE old `published_at IS NULL AND attempts>0` rows to `UNKNOWN` and sets `delivery_attempted_at=clock_timestamp()` (observation time of quarantine **not** proof of the historic network-send time). Only rows with `published_at IS NOT NULL` become `PUBLISHED`, with original ACK timestamp. Never-attempted rows (`attempts=0,published_at=NULL`) remain READY. Both backfills precede constraints/triggers.
- Adds `outbox_delivery_ready_pristine` CHECK `delivery_state<>'READY' OR attempts=0`. A legacy worker updating a READY row to increase attempts without transitioning to UNKNOWN cannot persist its claim.
- BEFORE UPDATE/DELETE trigger additionally forbids changing `attempts` in-place on READY or UNKNOWN; old Relay code cannot increment a quarantined row and then publish. Preserve prior guards blocking UNKNOWN->READY, PUBLISHED->READY, direct READY->PUBLISHED, UNKNOWN deletion and missing durable claim evidence.
- Old Relay workers **MUST be stopped and fenced before any authorized migration**; concurrent in-flight old transactions or direct `Client.Publish` can still bypass the new protection. DO NOT infer that DB checks remove all mixed-version dangers.

## Test source changes

- `db/migrations/phase51f_outbox_migration_test.go`, exact Git blob `7f2ac84b201aab450e0cc188265fdea1b7d74f8e`: verify old attempted-and-unacknowledged UPDATE text, quarantine order before acknowledged backfill and constraints, READY pristine constraint and legacy attempt-increment trigger. Runs against the actual embedded 0019 SQL.
- `tests/integration/phase51f_outbox_integration_test.go`, exact Git blob `7c73ba569d29d13725a4efbe4033351a34ab58bb`: add native PostgreSQL acceptance tests that fail if an old Relay increases attempts while leaving READY/UNKNOWN unchanged; assert SQLSTATE `P0001`, no persistent mutation. Uses restricted isolated `TEST_DATABASE_URL` TestMain. **Actual PG integration was NOT_RUN**.
- The previous Phase51-F `internal/platform/queue/nats.go` and owner-CAS SQL were unchanged. Existing `TestOutboxLease`, `TestJetStreamRecovery` and `TestDownloadQueue` still require signed-V2/native verification.

## Actually executed independently; preserve scope

Exact source files from GitHub HEAD were reconstructed locally from the byte-exact Phase51-F evidence and source changes. Git blob identity verified:
- 0019 SQL: `e5d8f66d0326c29ca1b31ef8f4fb626e192e7098`.
- embedded migration source-contract test: `7f2ac84b201aab450e0cc188265fdea1b7d74f8e`.
- queue SQL constants: `86936c5db80c0d718367e61e3e1bf491e512fd72`.
- queue model tests: `8677362c8488b7dbf4318143ffe3b4e8663ed521`.

**Go source-contract + in-memory model execution**: `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off go test -race -count=15 -timeout=120s -v ./...` on 2 isolated source packages, **60 top-level test function executions + 75 named scenario subtests, PASS**; log SHA256 `c3159020f47a1397ab6af9c271d3b39195062cbc82dd97934885b0f099ed0eec`. `go vet ./...` PASS, no output; gofmt on exact local Go sources clean.

**Executable migration UPDATE subset evidence**: standalone Python 3 standard-library `sqlite3.connect(':memory:')` with in-memory synthetic rows and custom `clock_timestamp()`; extracts the **exact two SQL UPDATE statements from the actual new migration** and executes them against a minimal local subset table. Five tests PASS, including never-attempted, attempted unacknowledged with expired/active/no lease, historically ACKed with and without attempt count, and no `READY` with `attempts>0`. Log SHA256 `8585ef840e7a6038246b9e5f61e47e2f207d2414bafe993261d25960f5f37476`. This is **SQLite execution of two compatible SQL UPDATEs, NOT PostgreSQL execution of 0019 DDL or triggers**.
- Negative baseline, exact Phase51-F old migration, predictably **FAILS** the five-test script because it contains only the acknowledged-PUBLISHED UPDATE. Deliberate expected regression failure, log SHA256 `46402d2fc133d24f5068e3fe31ee1ea6780f6b5334d9c23146ceaf3e7047ecd2`.
- Actual branch GitHub source-only invariants **19/19 PASS** across the changed files and existing queue source. All 19 are structural checks, not live-runtime certification.

Archive `LIC_A9_PHASE51G_TEST_EVIDENCE.zip` (sha256 `91cbc126afc3d85c4d1d19674b2468d041dc971054c1c85a2c0242cf5dcbf4a5`) includes logs, exact source bytes, runnable `REPRODUCE.sh`, the offline SQL UPDATE subset smoke, its negative-control log and SHA256 manifest. No network, secrets, private DB or production content.

## Other discovered integration blocker

Real `tests/integration/native_test.go` `TestMain` initializes PostgreSQL pools but does not call `installationlicense.InstallRuntime`. Since Phase50's global Runtime is default-deny until explicitly installed, existing native queue tests invoking `q.Relay`/`q.Publish` need a genuine **ephemeral TEST-only signed V2 Manager** and explicit process-level installation in a disposable test harness. Without that prerequisite, old suite results cannot be claimed PASS. Existing `TestInstallationLicenseRestrictedDatabaseAndQueue` must continue denying `RequireContext` lacking the correctly bound context, without bypassing runtime security. Constructing, compiling and executing such an isolated test fixture is a **separate release blocker**; do not add a production bypass.

## Mandatory admission next

1. On authorized **disposable** Linux with verified Go1.27.2 and genuine TEST-only V2 Manager, apply migrations 0001–0018 to a NEW PG16 sandbox, seed synthetic historical outbox cases before 0019: `attempts=0`, attempted-but-unacknowledged with expired and active lease, and acknowledged. Stop old Relay. Apply 0019 once and assert exact persisted historical classification and idempotent refusal of automatic reopening.
2. Execute PG16 actual trigger rejection of legacy READY and UNKNOWN same-state attempt increments and malicious READY->PUBLISHED, including parallel transactions, crashes and rollback; preserve RLS and restricted runtime roles. Execute the staged integration package test and original migration/queue tests.
3. Exercise actual JetStream broker ACK loss, timeout, lost SQL receipt, restart/recovery, finite broker dedupe window, and consumer inbox uniqueness, under signed V2 and RuntimeEffect. Run `go test -race` full Product and Authority PG16 plus two-process/multi-UID tests.
4. Prove no old Relay version can remain active before any later authorized upgrade. Define owner-reviewed manual `UNKNOWN` reconciliation with independent broker/consumer evidence; **no blind retry**, no auto-clear UNKNOWN. Secure installer/rollback and owner approval are still pending.

**Decision: PHASE51G_HISTORICAL_REPLAY_BUG_FIXED_IN_DRAFT / EXACT_SOURCE_GO_RACE_AND_SQLITE_BACKFILL_PASS / REAL_PRODUCT_SIGNED_V2_GO127_POSTGRES_NATS_NOT_RUN / PRODUCTION_NO_GO.**
