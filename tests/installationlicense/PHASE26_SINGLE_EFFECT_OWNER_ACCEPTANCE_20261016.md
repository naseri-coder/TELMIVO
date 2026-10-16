# LIC-A9 Phase26 — Architecture Freeze Acceptance, 2026-10-16

## Final scope and decision
`SINGLE_EFFECT_OWNER_ARCHITECTURE_FROZEN_FOR_DESIGN_REVIEW` / `NO_RUNTIME_INTEGRATION` / `PRODUCTION_NO_GO`.

**Exact Product parent:** Phase25 Draft PR #78 HEAD `153021a4ecf19686075633b10fddf6d3ff0f3299`.
**Exact Authority reference:** Phase21 Draft PR #57 HEAD `d6960982e322d4458c7d963bb153f205a1f8fb59`.

Security guard reviewed: `internal/platform/installationlicense/state.go` blob `c5c81548a1b063825eccc9c3d69cc6006160d4cd`, including `x.RuntimeSessionID != m.runtimeSession` rejection and genuine `Manager.Effect` final recheck. Exclusive refresh-leader policy reviewed from Product `concurrency.go` blob `9b27feb2fae155ddd7c3000272c58f1e7ba84ae3`. Phase24 synthetic broker source `6b1f920f0017f2f39cc0b11bbb40c1c262a37685` is nonproduction and must not be used as a real drop-in authorization broker.

## Chosen architecture
Single genuinely licensed effect owner per installation; own runtime session and Authority-signed V2 lease; signed lease and installation private key NEVER shared with workers; typed bounded local Unix broker, per-worker proof-of-possession, dedicated OS identities, independently derived tenant/RBAC/entitlement/quota/domain checks inside real `Manager.Effect` at last effect point. Durable intent/outcome ledger and monotonic owner fencing; unknown outcomes require reconciliation and never blind retries. No claim of exactly-once execution. Hostile same-UID attacker can unlink pathnames if directory ownership is not truly isolated, so old `flock` prototype is explicitly insufficient.

The decision is documented in `docs/lic-a9/LIC_A9_PHASE26_SINGLE_EFFECT_OWNER_ARCHITECTURE_20261016.md` and mirrored in machine-readable `tests/installationlicense/phase26_single_effect_owner_contract.json`.

## Actually executed Phase26 checks
1. Source review via connected GitHub: exact pinned Product Phase25 PR #78 and Authority PR #57 OPEN/DRAFT/UNMERGED. Source blobs of Product `state.go`, `concurrency.go` and synthetic broker fetched and reviewed.
2. **69/69 PASS** synchronous JavaScript contract-policy assertions against the **exact newly committed GitHub contract JSON**: fixed source pins, single effect owner, disallow shared leases, OS isolation requirements, IPC/authentication constraints, 12 required final-effect gates, lifecycle restrictions, no blind retry, and absence of false GO/operational permissions. **These are policy assertions, not external implementation tests.**
3. **11/11 PASS** locally executed Python3.13.5 isolated *independently authored design-policy test model*, including adversarial mutations and synthetic denials. This local exploratory model was not the same byte-for-byte source as the newly committed Python test file. Thus the new committed `tests/installationlicense/test_phase26_single_effect_owner_contract.py` is **NOT_EXECUTED_AGAINST_EXACT_COMMITTED_SOURCE**, and a later independent rerun is mandatory. Do not confuse this with executing committed Go/IPC runtime.
4. GitHub files fetched after push:
   - contract JSON blob `578ea0956a8e5c74cb0e137ec38f8e9e8dda76a8`;
   - Python test blob `7ffa7ce848b4530a81ca4874d7f835b49a4e93db`;
   - architecture ADR blob `39fb2d15fd8c0f757a875e047f62b44de77249fc`.

**NOT_RUN**: new committed Python test file byte-exact local run, full Product Go1.27 and Authority Go suites, actual Linux kernel/Unix IPC threat model test, real independent Worker/Owner signed lease takeover and leader loss, PostgreSQL16 8+ cross-repo E2E, real provider idempotency, DR and installer.

## Mandatory independent gates
- Phase27 security hardening of the local Broker and directory ownership using kernel-enforced/fd-relative safe path resolution and worker identities.
- Complete committed Python test rerun and independent security review before accepting implementation.
- Phase28 real `Manager.Effect` integration, workload RBAC/tenant isolation, no generic callbacks.
- Durable outcome reconciliation, provider-specific fencing, leader-crash and replay matrix.
- Exact Go1.27 + PostgreSQL16 tests on owner-approved **independent** disposable host (not TELMIVO live VPS).
- Main-only Termux age recovery key helper remains segregated from Product security branch pending a separate custody review.
- Independent Authority highwater witness/DR, signed Ubuntu package and owner signoff remain blocked.

## Repository boundary
Design-doc + JSON-policy + offline Python-test + this report only. No `internal/` runtime paths, `go.mod`, SQL, API, Docker, services, credentials, TLS, live keys, GitHub Actions, `main` or production host modified.

**Phase26 architecture deliverables complete. Release NO-GO; do not promote to Phase27 without new instruction.**
