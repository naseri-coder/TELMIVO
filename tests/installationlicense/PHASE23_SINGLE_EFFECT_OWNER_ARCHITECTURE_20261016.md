# LIC-A9 Phase23 — Single Effect Owner (Nonproduction Go Proof)

**Date:** 2026-10-16. **Review mode:** Nonproduction architecture experiment only.  
**Release:** `MULTIWORKER_NOT_CERTIFIED / PRODUCTION_NO_GO`.

## An explicit architecture choice, not a license bypass

Phase22 proved a conflict between independent per-worker `runtimeSession` tokens and a single leader owning refresh of a shared signed lease. The Phase19 clone fence `x.RuntimeSessionID != m.runtimeSession` must **remain enforced**.

Phase23 prototypes the *single effect-owning leader* approach: only the leader process holds its **own legitimate signed installation session** and executes external business effects; followers may submit narrowly typed requests to an authenticated **local** dispatcher, but may not receive the signed lease, installation secret or a generic capability to execute commands. A real Product implementation must call the **real `Manager.Effect(ctx, fn)` and domain/RBAC authorization at the actual effect point**, not merely at ingress.

This PR includes an **offline test-only** Unix-socket proof in `phase23_broker_proof/`. It has no Production connector, real License Authority transport, trading/order/bot function, real signer/key, cloud access or deployment interface. It is a standalone nested Go module ignored by normal Product `go test ./...`; its tests must be run explicitly.

## Exact source and real local execution

Go module source at this branch is byte-identical to code executed locally in an isolated Linux container on **Go 1.23.2**, with `GOPROXY=off`, `GOSUMDB=off`, `GOTOOLCHAIN=local`:

| Test-only source | Git Blob SHA |
| --- | --- |
| `tests/installationlicense/phase23_broker_proof/go.mod` | `62ef83cf2aa0ed6f7b412034fee534a9491a8207` |
| `tests/installationlicense/phase23_broker_proof/broker.go` | `827339007bbda5163a7fb268a69511489b7a9a85` |
| `tests/installationlicense/phase23_broker_proof/broker_test.go` | `b24147fff7687ad87f5e58fa6821bab7924fd6cc` |

Actual executed commands:

```sh
cd tests/installationlicense/phase23_broker_proof
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -race -count=1 -v ./...
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -race -count=50 ./...
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go vet ./...
```

**9/9 actual Go race tests passed**, followed by **50 repeated green race-test runs = 450 successful case executions** and a green `go vet`, *for this synthetic nested module only*. Local full-run SHA-256: `19613581b47f046200284131fedc3e7056bbb27dc51957b023b14796fce02f4b`; local `-count=20` stress-log SHA256: `fd57cbf8e91f18b52c28ae71c392ffa3ef6514ae56a2d1211d444a8dbe220d91`. All three committed Go source blobs are verified against those exact locally run source bytes.

The first test attempts found real defects **in the newly written prototype** (an EOF-blocking frame reader and a race in an injected peer-UID hook), then a rejected peer response reset. These were repaired and full race tests rerun before committing; no failure was masked by relaxing assertions.

## Precisely what is and is not tested

All **nine real Go test cases** are synthetic, with a hardcoded, non-operational effect (`SYNTHETIC_RECORD_EFFECT`) and a **mock authorization gate**. Tests exercise:
- Unix socket under a fresh owned 0700 root; socket chmod 0600 and Linux `SO_PEERCRED` uid check.
- Restricting request shape to canonical JSON, a single test-only operation and 32-lowercase-hex identifier; rejecting repeated JSON fields, arbitrary commands, invalid or oversized frames.
- Immediate **injected** authorization recheck at effect time, rejection on revoked mock state and on forged peer UID.
- At-most-once intent reservation via `O_CREAT|O_EXCL|O_NOFOLLOW`, file+directory fsync before effect. Duplicate concurrent requests execute the callback at most once.
- Restart with retained temporary journal denies replay of already used request ID.
- Stopped leader and unsafe directory fail closed; no network, real license, signer or PostgreSQL.

**Critical: none of these checks proves Production-ready IPC security.** UID equality alone is not business identity, RBAC or tenant authorization; a malicious process with the same OS UID could submit a test-only request. Production must have authenticated role/caller/tenant permissions and a complete **finite** domain-command registry. No generic arbitrary code, shell, remote trading orders or user-chosen callback may be accepted.

**At-most-once is NOT exactly-once**: if a process crashes after durable reservation but before or during the external effect, the outcome can be unknown. A retry with that ID is intentionally refused, not automatically executed. A Production design requires independently reconciled domain result/outbox/idempotency, authoritative fencing tokens on leader takeover, audit semantics and explicit crash-state recovery. A test fixture could not authorize these features.

## Frozen Product source contract and release blockers

This phase adds only the three `tests/installationlicense/phase23_broker_proof/` Go proof files plus this report, and updates the exact source-fence list and corresponding Python refreeze test. The full changed-file inventory relative to Product hardened PR #61 expands from **37 to 41** *literal* paths; **the only allowed `internal/` paths remain exactly the Phase19 runtime guard, its signed Go regression fixture and the Phase22 Go fixture**. No Go runtime, configs, migrations, actual business adapter, Go root manifest, deployment or `main` file is touched.

Release is still blocked pending:
1. Real **Go1.27.x** Product runtime integration of the chosen architecture under signed V2 leases with multiple separate Worker processes, trustworthy caller credentials, RBAC and per-effect recheck.
2. Multi-process leader crash/handoff, kill-switch/revocation, at-most-once failure injection with real side-effect idempotency, persistent broker journal and audit.
3. Eight real **Product×Authority Go1.27.x/PostgreSQL16** cross-repository E2E scenarios, full `-race` suites, image and module cache attestation.
4. Reconciliation of two Product `main` commits and Authority release custody/recovery/Ubuntu signed easy-install certification.
5. Explicit independent owner release decision; no arbitrary automated Production GO.

**Phase23 decision:** `OFFLINE_SINGLE_EFFECT_OWNER_GO123_RACE_9_PASS` / `STRESS_50_PASS` / `REAL_PRODUCT_BROKER_NOT_IMPLEMENTED` / `MULTIWORKER_PRODUCTION_LIVENESS_UNPROVEN` / `PRODUCTION_NO_GO`.

**STOP. Draft only, no merge, no Production host, no live keys or databases.**
