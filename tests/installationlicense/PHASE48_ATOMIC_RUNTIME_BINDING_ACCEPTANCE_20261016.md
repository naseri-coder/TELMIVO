# LIC-A9 Phase48 — coherent atomic process-runtime binding

Date: 2026-10-16.

**Disposition:** REAL PRODUCT RUNTIME BRANCH-ONLY REPAIR CANDIDATE; ISOLATED LINUX GO1.23.2 **STUB MANAGER** RACE REGRESSION PASS; AUTHENTIC PRODUCT GO1.27 NOT_RUN; **PRODUCTION NO-GO**.

## Ancestry / scope

- Phase47 Draft PR #101 parent HEAD: `33b4f59890151e11651ca3e9c6aede1e50922c71`.
- Phase48 branch: `development/lic-a9-phase48-atomic-binding-20261016`.
- Changes: `internal/platform/installationlicense/runtime.go` **genuine Product runtime code**, one `phase48_atomic_binding_test.go` and this report. No `main` merges, no Product root `go.mod` edit, no Authority, live database, workers, Telegram/provider effects, GitHub Actions, Production server or deployment.

## Precise source issue addressed

Phase47 defended against context-bound Managers overriding an installed global Manager, but still stored lifecycle state in **two independent atomic objects**:

```go
var runtimeRequired atomic.Bool
var runtimeManager atomic.Pointer[Manager]
...
runtimeRequired.Store(true)
runtimeManager.Store(m)
...
runtimeForEffect(ctx, runtimeRequired.Load(), runtimeManager.Load())
```

Those reads and writes individually avoid a data race, but a **pair of atomics is not one linearizable installation-state snapshot**. Concurrent repeated `InstallRuntime` calls can expose mixed generations to the authorization path. This is a demonstrated structural concurrency concern, **not a proven exploitation of a deployed Product**. Earlier phases' synthetic tests did not prove a global rebind transaction. The Product startup code presently installs one Manager before business infrastructure starts, so ordinary startup is less exposed than ad hoc concurrent reinstall.

### Phase48 repair — one atomic snapshot

The actual Product `runtime.go` now uses:

```go
type runtimeBinding struct { manager *Manager }
var installedRuntime atomic.Pointer[runtimeBinding]
func InstallRuntime(m *Manager) {
    installedRuntime.Store(&runtimeBinding{manager: m})
}
```

- The **absence** of a binding (nil pointer) means installation has not occurred; no unbound effect is authorized. A **non-nil** immutable binding with `manager=nil` represents explicit deny; it cannot be confused with uninitialized state.
- `runtimeForCurrentEffect` obtains **one** atomic snapshot then delegates to the existing Phase47 policy. Foreign context-bound Managers cannot override the global binding; a same-instance Manager and authentic process Manager remain eligible for their own signed-lease checks.
- A deliberately explicit Context-bound Manager **before installation** remains permitted, subject to that Manager's independent verification. This pre-install exception is inherited from Phases45–47 and must still be reviewed across all Product callers.
- `InstallRuntime(nil)` followed by another `InstallRuntime(validManager)` is a **permitted explicit re-install** by the same process. This is NOT irreversible revocation. An effect already admitted before a concurrent replacement is not retroactively canceled.
- No guarantee of distributed consensus, root-owned multi-UID enforcement, actual provider exactly-once behavior, or transactional atomicity between a Manager's `Effect` and a concurrent re-install.

**Exact reviewed Product Git blobs:**
- Parent `runtime.go`: `9b72adb763eb1e4705d407a6711942793a465123`.
- Patched `runtime.go`: `eb80737092f6d40cd9b1acfb96ef7c4033c5e30c`.
- Added `phase48_atomic_binding_test.go`: `cddfcb190deda1d31f31a2de64b08bf44a15f81c`.

Both changed source blobs were independently fetched from the GitHub branch and **exactly matched the isolated Linux-tested bytes**.

## Tests actually run — sandbox Linux/amd64 Go1.23.2, UID0, no network

A disposable isolated module compiled **the exact Product `runtime.go` bytes** and **the exact Phase48 test file bytes** together with a **compatible explicitly fake STUB Manager and Phase19 signed-lease fixture**, not actual TELMIVO cryptographic V2 verification.

```sh
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off \
  go test -race -count=3 -timeout=180s -v ./...
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off go vet ./...
gofmt -l *.go
```

**PASS, exit 0**. Six fresh-process subprocess scenarios per repetition, **18/18 subprocess cases PASS**, 19.226s. No FAIL/SKIP or Race Detector reports. Combined log SHA256: `7fcb8145bed886e2432bfc49d3c1bf51cf89efcb642c25cf7a81d41d1b602940`. `go vet` PASS and gofmt clean.

Cases: fresh process no global, explicit signed-manager path before install (in genuine Product intended; STUB in current run), `InstallRuntime(nil)` fail-closed even with foreign Context Manager, valid installed global Manager, valid→nil→valid reinstallation, coherent snapshot reads under concurrent rebindings, and canceled Context denial.

## Reviewed toolchain limitation and mandatory gates

Environment: Linux/amd64 Go **1.23.2** only, Go1.27 absent; `curl https://go.dev/dl/` fails DNS. We did **not** run on the real Product module requiring `go 1.27.0`, did not download a toolchain or use the user's Android/Termux, and did not touch the TELMIVO Production server.

**NOT_RUN / not waived:**
- Actual Product Go1.27.2 Phase45–48 tests using real signed Phase19 V2 fixtures and complete Product startup/bootstrap regression.
- Real Linux Go1.27+ `-race` root multi-UID Supervisor, Product hardlink checks, genuine Authority boot grant, PostgreSQL16 multiworker fencing, trusted external provider receipts, release installer and independent security review.
- Audit of all potential consumers of internal globals / lifecycle expectations on the *fully compiled* Product codebase; a GitHub code-search attempt returned incomplete indices and was not accepted as proof of absence.

**Decision: PHASE48_BRANCH_SOURCE_AND_STUB_RACE_PASS / AUTHENTIC_PRODUCT_GO127_NOT_RUN / PRODUCTION NO-GO.** Keep Draft; DO NOT MERGE or deploy. Stop after Phase48.
