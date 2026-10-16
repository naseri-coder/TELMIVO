# LIC-A9 Phase33 — Session-bound synthetic Owner IPC + fail-closed final Effect gate

**Date:** 2026-10-16  
**Disposition:** `PHASE33_SOURCE_MATCHED_TEST_BOUNDARY_PASS` / `REAL_PRODUCT_MANAGER_EFFECT_E2E_NOT_RUN` / **PRODUCTION NO-GO**.

## Verified source ancestry

- Product parent: Phase32 Draft PR #85 HEAD `90daad8e991e107080da7ebd4fcc3736c3f87ce2`; Phase33 branch is stacked **only** on Phase32.
- Authority Draft PR #57 HEAD `d6960982e322d4458c7d963bb153f205a1f8fb59`, read-only reference.
- Real Product `internal/platform/installationlicense/state.go` blob `c5c81548a1b063825eccc9c3d69cc6006160d4cd`; GitHub live refetched on the Phase33 branch.
- No Product `internal/` files, Authority, `main`, Production, servers, databases, business effects, keys or GitHub Actions changed.

## Scope: real changes to isolated Phase32 Go IPC proof (NOT Product runtime)

Three exact locally tested source files:

| File in `tests/installationlicense/phase32_supervisor_boundary/` | Git blob SHA matched to local `git hash-object` |
| --- | --- |
| `supervisor.go` | `41863221a1388ef80eb673014fe6642ef858483e` |
| `supervisor_test.go` | `e3a6867f44703e5de8b8fab20a55069aff280137` |
| New `phase33_security_test.go` | `e9892379e19dec0d211386d5daf7d8d1a329dc6f` |

The Go module remains the Phase32 standalone Go1.23 test module. It relies on the Phase31 local witness and synthetic file-backed effects, NOT an actual Provider, the TELMIVO runtime, or a signed Authority lease.

### Security hardening actually implemented

1. **Fail closed by default:** `Open` with no final license effect gate cannot perform synthetic `APPLY` at all (`DENIED_FINAL_EFFECT_NOT_WIRED`). `OpenSecured` requires explicit nonnil `EffectGate` and `DomainPolicy`. Both are test-only interfaces with the same `Effect(ctx, fn)` method signature as the real Product `Manager.Effect`.
2. **Signed request binding:** `APPLY` must carry canonical, Ed25519-signed `installation, owner, session, epoch, request_id, workspace_id, actor_id, resource_id`. No generic operation or callback from the worker is accepted. `FENCE` must not include domain effect fields.
3. **Active owner/session fence:** after a successful strictly-monotonic signed synthetic `FENCE`, only that Owner and runtime session may request `APPLY` for the epoch. Different Owner or different session is rejected EVEN when the test-issuer request has a valid signature for that other identity.
4. **No authorization-to-effect race:** the local supervisor lock spans active Owner/session check, mock final `Effect` gate and synthetic file-backed `Witness.Apply`. Every mock `DomainPolicy.Authorize(ctx,claim)` is evaluated INSIDE `Effect(ctx,fn)`, immediately before the synthetic effect. The actual Product license manager also locks and revalidates signed session, but is **NOT instantiated in this module**.
5. **Domain policy/tenancy:** test policy enforces `tenantA/actorA/recordA` and denies tenant/actor tampering, missing identifiers and mutated signatures. This is a mock policy, not real TELMIVO subscription/RBAC/quota evaluation.
6. The Phase32 OS-level root-owner witness namespace, Linux `SO_PEERCRED` UID, restart quarantine, request replay prevention and subprocess tests remain in the module, with a new mock Gate wired explicitly for synthetic effects.

## Actual test execution — isolated Go1.23.2, Linux/amd64

Environment uses `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`, root for disposable multi-UID fixture only, no user-controlled host or remote services.

- **New Phase33 tests:** `go test -race -count=1 -run '^TestP33' -v ./...`: **15/15 named tests emitted PASS**, with no race after synchronizing mock-policy writes. The wrapper execution ran out of time **after** Go emitted `PASS; ok ... 34.615s`; the Go-reported success is accepted only as that test process's result, not as a successful enclosing wrapper command. Actual log SHA256 `7b5f462406a6fed55ae8ff6b953d045fc8388dbb28586b3e8031b08cab7bf6a4`.
- **Phase32 regression:** `go test -race -count=1 -run '^TestP32' ./...`: **PASS**, exit code `0`, elapsed 25.515s. Log SHA256 `d424e1ee63bcdb438c12aaa6e4d0701b294ca6d092fc024d19215a99e452c886`. Contains 14 pre-existing Phase32 test functions, root-owned witness/real multi-UID policy, no customer effects.
- **Independent focused gate/run:** `go test -race -count=1 -run '^(TestP33NilGateFailsClosed|TestP33PolicyRunsInsideFinalEffectCallback|TestP33SessionBindingRejectsSignedOtherSession)$' ./...`: **3/3 PASS**, successful tool exit 0, Go process 10.923s.
- `go vet ./...`: **PASS**; `gofmt -l supervisor.go supervisor_test.go phase33_security_test.go`: empty, **PASS**.
- **3/3** GitHub source blobs for the exact test fixture correspond byte-for-byte to the actual locally Go-tested files.

**Negative evidence retained:** the initial new-test pass triggered Race Detector on unsafe test mutation of the mock Effect/Policy fields. This was fixed with `s.mu` in test setup; the final focused run **did not report races**. One attempted combined full suite exceeded the shell/tool time limit after partial output; **that incomplete invocation is NOT counted as a full-suite PASS**. The Phase33 and Phase32 subsets were individually exercised as above.

### Real Product source contract (GitHub-refetched, read-only)

**10/10 source-presence/order checks passed** against blob `c5c81548a1b063825eccc9c3d69cc6006160d4cd`, confirming:

- Current real `func (m *Manager) Effect(ctx context.Context, fn func() error) error` uses `m.lock`, checks context, calls `m.decisionLocked()` and rejects invalid lease before invoking `fn()`.
- `decisionLocked()` operates on verified claims and rejects `x.Version == 2 && x.RuntimeSessionID != m.runtimeSession` before testing product capability.
- Production forces `RequireConcurrency`.

**These are source-contract checks, NOT compilation or runtime invocation of the actual Product package.** A real signed V2 concurrency-refresh test and authentic Product Go1.27 build are still mandatory.

## Remains blocked / NO-GO

1. **Real `Manager.Effect` wiring:** mock interface matches the method signature, but no Product manager object or real V2 Authority-signed lease was imported or executed. This is an isolated test fixture, not Production integration.
2. **Trusted issuer and authorization:** test-generated Ed25519 keys and mock tenant policy are NOT deployable. The test signer must never be treated as an Authority license signer; authn/authz, key rotation, worker/installation identity and real Product RBAC need formal integration.
3. **Remote provider fencing:** no exchange, payment, Telegram provider, outbox reconciliation or external epoch enforcement. Phase32/31 local witness does not prove distributed exactly-once delivery. Prior same-UID attack against writable witness root remains valid outside supervisor trust boundary.
4. Real Go1.27 Product unit/race tests, Authority PostgreSQL16 cross-repo 8+ E2E, live installed multi-UID process lifecycle, DR, signed installer, root-key custody reconciliation and independent security approval remain NOT_RUN.

**Phase33 isolated contract hardening and tests: ACCEPT. Real license/operations integration: BLOCKED. Release: NO-GO. Stop after Phase33; do not merge or deploy.**
