# LIC-A9 Phase39 — genuine Product Manager.Effect through Linux root-owned UID-secured supervisor

2026-10-16. **Status: TEST IMPLEMENTATION AND LINUX CONTRACT PASS; REAL PRODUCT GO1.27 LINUX EXECUTION PENDING; PRODUCTION NO-GO.**

## Exact ancestry and objective

- Parent: Product Phase38 Draft PR #91, `012f02f61b7b96808624ec15a2158527e2011cf2`.
- New draft Phase39: `development/lic-a9-phase39-linux-real-manager-uid-supervisor-20261016`.
- Objective: connect an **optional TEST-ONLY effect gate** from Phase38 actual root-owned durable Linux bridge to **real Product `Manager.Effect`** inside the unchanged Product's `internal/platform/installationlicense` package; test with actual isolated Linux UID 25111 worker and 25112 outsider, durable journal and signed synthetic V2 lease.
- The default bridge behavior when no integration gate is registered remains Phase38 **NO external effect**, only synthetic fsynced reservation. Production manager source, Product `go.mod`, Authority and `main` remain unchanged.

## Three reviewed Go changes, test-only

1. Modified `tests/installationlicense/phase38_supervisor_durable_bridge/bridge.go`: optional single-use `SetIntegrationTestEffectGate` before `Start`. Only after **peer UID, signed claim, tenant/actor/resource policy, epoch/session checks, and fsynced persistent reservation** does it call the gate. Both gate failure and success return `RECONCILE_REQUIRED`; neither proves a provider confirmation or authorizes automatic retries. Cross-process caller cannot change the gate.
2. New `tests/installationlicense/phase38_supervisor_durable_bridge/phase39_gate_test.go`: 5 tests: one callback after durable reserve; unknown outcome no blind retry; outsider denied before callback; immutable gate after Start; wrong signed session denied before callback.
3. New `internal/platform/installationlicense/phase39_real_product_supervisor_test.go`: six substantive tests + no-op child helper; compiled only with `linux && lic39`. Uses **actual Phase19 disposable Ed25519 V2 fixture** and binds gate to real `manager.Effect(ctx, syntheticCallback)` in the Product package. Real non-root OS child sends a signed FENCE then APPLY over root-owned Unix socket. Tests include genuine Product admission, revoked V2 session, outsider peer UID, crash/reopen quarantine, forged issuer signature and cross-tenant prohibition. No actual provider, customer data or commercial effect.

## Reviewable execution and exact limitations

**Actually run** on isolated Linux/amd64 Go1.23.2, root-owned test-only filesystem:

- Original Phase38 bridge plus optional gate: `go test -race -count=1 -timeout=180s ./...` **PASS**, 47.608s, including previous multi-UID kernel and crash checks and five new Phase39 bridge contract checks. `go vet ./...` **PASS**.
- Five new Phase39 bridge tests alone: `go test -race -count=2 -run '^TestP39' ./...` **PASS** twice (all five scenarios), no races.
- Root-package Phase39 integration test source with a **compatible Go1.23 stub Manager** (not actual Product Manager): `go test -tags=lic39 -race -count=1 ./...` **PASS**, all six substantive cases and child helper, `go vet -tags=lic39 ./...` **PASS**. Stub fixture was hardened with a mutex after a Race Detector finding in the *stub* only; repeat PASS. Local log SHA256 `30a8ec4ae99bcc617e30aa39186ff1629d1384375e4ba8444123265f1c1e843f`.
- Bridge full suite PASS log: `phase39_bridge_full_race.log` in model's isolated workspace.
- `bash -n run_real_manager_linux39.sh` **PASS**.
- Go source files and script formatted/checked locally; source SHA256 verified via transfer manifest.

**NOT executed:** Product's actual Manager.Effect with Phase39 root Linux runner under Go1.27. The container has only Go1.23.2 with no network, and the user's Termux `GOOS=android` cannot run a Linux root multi-UID test or official Race Detector. The Phase37 Product Go1.27.2 PASS on Android and separate Phase38 real Linux root kernel proofs are **independent evidence**; neither is the missing integration PASS. For genuine acceptance, run the included `run_real_manager_linux39.sh` on a disposable, full and clean Product Git checkout on isolated **Linux/amd64 with Go>=1.27, root, CGO=1**, explicit consent and exact reviewed SHA; the runner uses a temporary alternative modfile and offline `replace` to the existing nested test module. It does not modify root `go.mod`.

## Non-waivable blockers

- **This is test-only wiring, not a deployed runtime Owner/Supervisor service.** No operational systemd/root service, verified Authority boot-nonce protocol, per-worker independent Authority signatures, PostgreSQL16 distributed fencing, external provider transactional/idempotency contract, payment effects or exactly-once end-to-end delivery.
- Five actual Product hardlink-denial cases on Linux Go1.27, complete Product Go1.27 Linux race suite, genuine provider/Authority E2E and independent security audit remain pending.
- The synthetic callback only increments an in-memory counter. Even an allowed callback returns `RECONCILE_REQUIRED` to the client since there is no independently attested provider receipt. A rejected or unknown effect remains durably reserved and needs authoritative reconciliation, never blind retry.
- The Phase38 durable root-owned bridge retains its own documented operational limitations (root provisioning, hostile mounts, fresh-boot challenge and worker lifespan). Phase22 simultaneous real multi-worker availability remains unproven.

**Development test gate accepted for Linux proof. Genuine Product-integrated release gate PENDING. PRODUCTION NO-GO. Do not use production host, deleted Authority VPS or GitHub Actions; no Merge. Stop after Phase39.**
