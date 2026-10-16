# LIC-A9 Phase28 — Supervisor-owned FD boundary and synthetic final-effect gate (2026-10-16)

**Overall verdict:** `PHASE28_NONPRODUCTION_KERNEL_BOUNDARY_AND_GO_GATE_TESTS_PASS` / `REAL_MANAGER_EFFECT_INTEGRATION_BLOCKED` / `PRODUCTION_NO_GO`.

**Scope is deliberately limited.** The Phase27 defect showed that same-UID actors able to unlink `owner.lock` and `broker.sock` can admit a competing owner. Phase28 probes a stronger **supervisor-owned namespace and inherited listening FD**, plus an isolated Go proof of final-effect ordering. It does **not** connect the real Product Broker to `Manager.Effect`, does **not** certify actual worker failover and does **not** close the Product release gate.

## Exact GitHub ancestry and immutable review references

- TELMIVO parent Phase27 Draft PR #80: `79275b650722bffdce1d9561c0ec5a07a22fb326`.
- License Authority Phase21 Draft PR #57: `d6960982e322d4458c7d963bb153f205a1f8fb59` (untouched).
- Phase27 nested test-only Broker remains unchanged; the same-UID pathname replacement vulnerability is **not silently marked fixed in that module**.
- Product signed-session check in actual `internal/platform/installationlicense/state.go` remains unchanged (`x.RuntimeSessionID != m.runtimeSession`), as does the real `Manager.Effect` implementation.
- No Production server, deleted Authority host, live database, credentials, signer, app or exchange effect, GitHub Actions, or `main` merge.

## Real Linux kernel multi-UID experiment

`tests/installationlicense/phase28_fd_boundary/test_supervisor_fd_boundary.py`

- In a disposable local Linux container with root privileges **only inside the test environment**, root supervisor creates a mode-`0750` **root-owned** directory whose group matches the permitted Worker UID, and a mode-`0660` root-owned Unix listener socket.
- Listening socket FD is inherited by a forked, privilege-dropped **Owner UID 24101**, who **cannot traverse** the parent directory or unlink its Socket pathname.
- Distinct **Worker UID 24102** can connect, and the Owner checks real kernel `SO_PEERCRED` UID.
- An outsider **UID 24103** cannot connect to the pathname.
- A second actor with UID 24101 cannot unlink the supervisor-owned Socket or bind a replacement inside the directory.
- This is a **fork-inherited FD test**; systemd socket activation, true service-manager FD handoff after exec, process isolation/namespace policy, production directory rotation, and malicious privileged supervisor are **NOT TESTED**.

Run: `python3 -B test_supervisor_fd_boundary.py`

**Executed result: 4/4 Python unittest methods PASS**. Test byte-identical Git blob: `bac81a9eb8ebc47eb33b783c14e5ca01e0f94fd1`. Local log SHA256: `48a7c2db1dc1b650364e4aa68df1ccf798d1d690dc686da64aac0f37ea2eef7e`. A Python `os.fork` deprecation warning about a multithreaded parent was emitted; it is not a proof that arbitrary fork-from-multithreaded Production code is safe. No persistent host UID/service/database was created, only disposable child processes with dropped privileges.

**Interim security decision:** Original same-UID attack is preventable **within a newly enforced supervisor-owned filesystem/FD trust boundary**. The old shared-writable-owner-dir design is still **REJECTED**. This experiment is NOT an independent security certification; in particular, privilege separation must remain enforced against all potential Worker/Owner paths, and real multi-process acceptance remains pending.

## Isolated Go final-effect ordering proof

`tests/installationlicense/phase28_effect_gate_proof/` (standalone Go1.23 stdlib module; does not import Product):

- `EffectExecutor` exposes the same method signature as the real `Manager.Effect(context.Context, func() error) error`, but **all tests use a mock implementation**, not real signed V2 leases, installations, Authority, or Product runtime.
- A finite allowlist with **only** `SYNTHETIC_RECORD_EFFECT` forbids arbitrary callbacks from an IPC request. Required fields are checked; final actor/tenant policy runs INSIDE the mocked `Effect` callback. No callback runs if the mock license gate or domain policy rejects.
- In-memory `MemoryLedger` reserves a request ID before a synthetic effect and rejects replay. **It is NOT durable** and proves neither Provider idempotency nor exactly-once execution; outcome failure is reported `ErrUncertain` and never automatically retried.
- Real Product domain authorization, worker enrollment, quotas, actor policy lookup and the actual `Manager.Effect` call are **NOT RUN**, and no Worker is actually routed to the Product effect owner.

**Executed in isolated Go1.23.2 Linux amd64 with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`:**

1. `go test -race -count=1 -v ./...`: **11/11 Go test functions PASS**.
2. `go test -race -count=30 ./...`: **30/30 full runs PASS (330 test-function executions)**.
3. `go vet ./...`: **PASS**.
4. All four committed executable/source blobs including the Python test match the locally tested bytes.

| Exact GitHub path | Verified Git blob |
| --- | --- |
| `tests/installationlicense/phase28_fd_boundary/test_supervisor_fd_boundary.py` | `bac81a9eb8ebc47eb33b783c14e5ca01e0f94fd1` |
| `tests/installationlicense/phase28_effect_gate_proof/go.mod` | `edae8e01c54d276ef5aacc2372bf31d6442109af` |
| `tests/installationlicense/phase28_effect_gate_proof/gate.go` | `907a7a4a8a57c144e97ba19000704e6f56d731e4` |
| `tests/installationlicense/phase28_effect_gate_proof/gate_test.go` | `921d6114a463e2cb8cac1640dafa4d4f55ae1d3a` |

SHA256 of local executed logs:
- Go race complete first run: `c9ab95ebb98a527f446c5d3b9a136026f99cd4de2f8235c7ff017e7ee417d1a5`.
- Go race 30-run: `fb352a9487bf6c3420ee338affbb5cee47c8d0611700b23dd3a099f8dc81a636`.
- Go vet (empty success log): `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

## Non-waivable release and next-phase blocks

1. Real Product **Go1.27** checkout/build/race suites, genuine signed V2 lease/anti-clone session and actual `Manager.Effect` integration are **NOT_RUN**. Go1.23 mock cannot certify those.
2. Real root-owned service-manager `systemd` FD passing, multi-UID worker account provisioning, shared group/ACL policy, UID spoof/credential rotation, supervisor compromise and kernel denial under hostile restart are **NOT_RUN**.
3. Real Customer RBAC/tenant isolation and entitlement/quotas, durable PostgreSQL-backed intent/outcome ledger, external-provider idempotency, reconciliation of unknown outcomes and previous-owner fencing are **NOT_IMPLEMENTED / NOT_RUN**.
4. Full Authority×Product real PostgreSQL16 eight-scenario E2E, encrypted two-cluster recovery, independent witness custody, signed Ubuntu installer/rollback, Product `main` divergence resolution and independent security approval remain **NO-GO**.
5. Phase27 old broker still permits pathname replacement in an untrusted same-UID directory; the architecture must reject such deployment settings. Do not relax anti-clone or enable a fallback direct effect.

## Phase28 acceptance

**PASS (limited):** Real Linux UID/kernel permissions and inherited listening FD test; standalone synthetic Go final-effect-ordering test; exact Git blob integrity; no mutation of operational code.

**BLOCKED (mandatory for full Phase28):** Production-source integration of Broker and actual `Manager.Effect` with real Worker authorization and signed multi-process liveness. This cannot be considered complete merely because separate test fixtures passed.

**NO MERGE. KEEP DRAFT. RELEASE NO-GO. STOP at Phase28** until a separate instruction.
