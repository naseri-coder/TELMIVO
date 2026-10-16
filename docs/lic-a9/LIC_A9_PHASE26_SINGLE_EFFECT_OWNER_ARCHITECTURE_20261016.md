# LIC-A9-PHASE-26 — Single Effect Owner Architecture Freeze (2026-10-16)

**Decision:** `ONE_LICENSED_EFFECT_OWNER_PER_INSTALLATION_WITH_AUTHENTICATED_LOCAL_COMMAND_DISPATCH`.

**Disposition:** **DESIGN CONTRACT ACCEPTED FOR DEVELOPMENT ONLY** / **REAL MULTIWORKER NOT IMPLEMENTED** / **PRODUCTION NO-GO**. This ADR does not authorize runtime changes, `main` merge, service activation, a signing key, Go/PostgreSQL E2E PASS, or deployment.

## Exact review ancestry

- TELMIVO parent Phase25 Draft PR #78 HEAD: `153021a4ecf19686075633b10fddf6d3ff0f3299`.
- TELMIVO Phase24 test-only Broker PR #77 HEAD: `255a55123015a0ad3d5efe1ab9a233e678e5db03`.
- Product Phase19 live source guard: `internal/platform/installationlicense/state.go` Git blob `c5c81548a1b063825eccc9c3d69cc6006160d4cd`.
- Phase24 synthetic Broker implementation Git blob `6b1f920f0017f2f39cc0b11bbb40c1c262a37685`.
- Independent Authority Phase21 Draft PR #57 HEAD: `d6960982e322d4458c7d963bb153f205a1f8fb59`.
- Main divergence review: Phase25; Product main-only Termux age-custody helper remains held for independent review.
- This ADR supersedes **no** cryptographic field, signed Authority lease wire format, anti-clone guard, or lock-order contract.

## Problem and architecture selection

`Manager.Open` gives **every process a new runtime session**. In the real Product `decisionLocked`, a V2 signed lease with `x.RuntimeSessionID != m.runtimeSession` correctly rejects stale/restarted/cloned-process effects. The Product `acquireRefreshLeader` process-lifetime flock permits only one refresh coordinator per shared installation. Therefore simply letting multiple processes independently call `Manager.Effect` with the same local state cannot provide both safety and liveness.

**Chosen**: one trusted *Effect Owner* process is the only owner of the installation private key, signed V2 lease, Authority concurrency refresh and real `Manager.Effect(ctx, fn)`. It has one current signed session, and serializes final external-effect admission under the existing license/state fences. Other workers (e.g., group/download/support) send narrowly typed intent messages over authenticated **local Unix sockets**, never run protected effects and never read or copy license/installation keys. Workers may prepare or enqueue non-effect computation without local license bypass.

Rejected alternatives for this release:
- **Shared lease / shared runtimeSession / disabled session check**: defeats the Phase19 anti-clone fence.
- **Independent per-worker signed leases**: would require an entirely new Authority protocol and accounting/security review, not an implicit Phase26 change.
- **One installation per Worker**: increases license/accounting/admin complexity and changes customer entitlements; not chosen.

**Boundary:** the existing nested `phase23_broker_proof` is a Linux Go1.23 **synthetic test module**, not production code. It uses a mock EffectGate and same-UID credential check, and never establishes business identity. Reuse its ideas only after redesigned strong isolation and independent review. None of its hardcoded `SYNTHETIC_RECORD_EFFECT` requests are production commands.

## Trust boundaries and principals

1. **Authority** (separate trusted system) issues signed Ed25519 V2 lease bound to installation ID/key, generation, epoch, lease ID, current runtimeSession and expiry. No new Authority issuance endpoint is proposed.
2. **Effect Owner** verifies the Authority-signed lease with existing pinned roots, holds exclusive ownership through all admitted handler completions, uses the genuine `Manager.Effect` for each protected final effect, and owns business authorization lookup and audited outcome ledger.
3. **Workers** each have a dedicated OS security identity and distinct locally enrolled **worker authentication key**; they hold no installation secret. An authenticated Unix peer UID is necessary but **not sufficient**. Server-side immutable workerID↔credential↔allowed role/tenant mapping is authoritative; every request has bounded lifetime/nonce and proves private-key possession. Enrollment/revocation/rotation is an independent audited administrative operation.
4. **Customer/actor** claims provided by workers are untrusted selectors; Owner re-fetches workspace, actor, rights, plan/plugin entitlements, quotas, resource ownership and revocation at the **effect point**. Ownership decisions are never delegated to an IPC field.
5. **External providers** can be called only via finite registered domain adapters. No arbitrary callback, shell, SQL, URL, Telegram method, payment instruction or trading order can arrive as a generic IPC command. Additional sensitive vertical-specific approvals are separate.

Broker socket and owner-control directory must be owner-controlled and inaccessible to an adversarial same-UID writer. Use a platform-verified kernel-enforced path walk (fd-relative `openat2` RESOLVE flags where supported or an independently reviewed equivalent), inode/ownership/mode checks, and dedicated UIDs or namespaces. A `flock` inode alone is insufficient when a malicious same-UID process may unlink and replace the lock pathname. If such isolation cannot be proved, **NO-GO**, not a fallback to leaf-only `Lstat`.

## Request / IPC v1 design (not yet implemented)

Contract data: `tests/installationlicense/phase26_single_effect_owner_contract.json` with protocol `TELMIVO_LIC_A9_EFFECT_DISPATCH_V1`.

- Local Unix domain socket only, no TCP; maximum full frame 16 KiB; length/format must be bounded before decoding; strict canonical JSON, no duplicate, missing, extra or trailing data; exact protocol and finite versioned operation registry.
- Typed intent includes `protocol,request_id,worker_id,installation_id,workspace_id,actor_id,operation,resource_id,payload_sha256,nonce,expires_at_unix_ms,authentication`. The authenticated request covers the **entire canonical request context**, including protocol/version, destination owner/install, tenant/resource, operation and payload digest; canonical message/signature format and replay-window clock tolerance must be finalized before any implementation. An ID/header alone authorizes nothing.
- Perform OS peer admission, worker proof, server-side enrollment/role, tenant scope, actor RBAC, commercial quotas/entitlement, domain resource policy, fresh signed owner lease, owner fencing and durable intent/outcome checks. At final execution call **real** `Manager.Effect` and recheck the authorization inside the effect gate.
- Failure to authenticate, expired nonce/lease, cross-tenant request, revoked identity, stale owner, unsupported effect, journal unavailability, disconnected broker or malformed JSON → **DENY**. Never fall back to a worker's direct effects. Failures leave customer data intact.
- A local authenticated worker command is an *intent*, not a grant. The Owner can deny without invoking any external provider. All rejections should be audited without leaking request private keys or payload secrets.
- Do not hold the license file lock across arbitrary unbounded network calls in an eventual implementation without independent lock-contention/timeouts review. For effects that cannot safely release a fence before delivery, enforce bounded operation timeouts and lifecycle drain; track potentially completed network calls as **unknown outcomes**, not successful cancellations.

## Owner lifecycle / leader handoff

Explicit states: `STARTING -> ACTIVE -> DRAINING -> FENCED -> RECOVERING -> ACTIVE` with fatal branches to `FENCED`, as enumerated in the JSON contract. No `FENCED -> ACTIVE` shortcut.

- **STARTING**: lock and service identity checks, local ledger integrity, signing roots, valid installation state; request a **fresh Authority-signed V2 lease for this new runtime session**. No external effects while starting or pending.
- **ACTIVE**: only single elected Owner may perform protected effects. Recheck signed freshness/session and business rights at each effect. The leader lock is not license authority.
- **DRAINING**: stop admissions, let admitted bounded effects finish, record durable outcomes, hold exclusive owner fence through the end of handlers, then transition to FENCED.
- **FENCED**: any failure to demonstrate live ownership/signed freshness or crash leaves all Workers denied; no direct effect fallback.
- **RECOVERING**: new owner gets a new process session and an Authority-issued ratcheted lease before ACTIVE; check old pending intents and ambiguous provider outcomes. Preserve monotonic epoch/recovery/highwater. Cannot use stale signed session even if the old lock is free.
- If Authority is unreachable **no new owner-session lease** may be granted; optional pre-existing signed offline authorizations do not cover a new runtime session under this contract. Emergency access is separate owner-authorized recovery, never a protocol bypass.

**Safety caveat:** Unix advisory locks coordinate only cooperative participants. A crashed old Owner may already have sent an external operation when a new Owner takes over. Fencing **before** external effects requires an independently durable owner epoch and, where available, provider-side fencing/idempotency. For providers without fencing, **pause and reconcile** outstanding ambiguous intents before any successor sends new effects. Do not claim split-brain impossibility or exactly-once merely from `flock`, shutdown and `O_EXCL`.

## Intent/outcome durability

Persist a typed request and stable domain idempotency key **before** dispatch; state transitions may include `RECEIVED -> AUTHORIZED -> RESERVED -> OUTCOME_CONFIRMED` and `RESERVED -> RECONCILE_REQUIRED`. Maintain immutable actor/tenant/resource, owner generation, signed session, request hash, attempt and provider receipt, plus operator reconciliation audit.

A timeout, lost response or crash after RESERVED is **not** a safe retry signal. Return `RECONCILE_REQUIRED`, prevent blind retry, and obtain provider outcome or authorized/manual reconciliation. Provider-side idempotency/fencing capabilities must be **proven for each adapter**, not presumed. At-most-once reservation is not guaranteed exactly-once settlement.

## Implementation boundaries for later phases

**Phase27** should replace the test-only proof with *nonproduction* secure IPC and process ownership prototypes, including dedicated OS identities, robust root resolution, audited worker enrollment, bounded signed requests and negative adversarial tests. It MUST NOT turn on customer effects.

**Phase28** may wire a narrowly reviewed finite domain effect registry into genuine Product `Manager.Effect` and real RBAC/tenant entitlement resolution in an isolated environment. **Phase29–31** must finish domain ledger, external outcome reconciliation, genuine failover, and Authority lease handoff. **Phase32–34** must run exact-source full Go1.27 race suites, real Go/PostgreSQL16 8+ joint E2E scenarios and secure independent disaster restore. Release signing and easy installer remain separate later gates.

Mandatory future negative tests: cloned state/restarted session; stale lease and expired signed token; duplicate JSON/unknown field; role forging/same-UID spoof; cross-workspace/actor revocation; wrong installation; leader socket/owner-marker unlink; symlink and TOCTOU; concurrent Close and queued callback; leader power loss/partition/restart; stale former-owner external effect; pending effect response lost; duplicate delivery request; unsupported-provider fencing; Authority `RECOVERY_HOLD`; malformed replay/counter regression; ledger corrupt/unavailable; missing isolated host.

## Executed versus pending evidence

This phase performs a **source-informed DESIGN freeze**; neither the real Go Product nor Authority code or PostgreSQL was modified. The accompanying **Python offline contract policy suite**, when executed, tests the *design invariants and a synthetic idealized decision model*, not the actual Go wire, process security, cryptographic proof or production liveness. Any local result must say so explicitly.

**Non-waivable NO-GO:** genuine Go1.27 compiled Product integration, signed multi-process worker E2E, independent filesystem/IPC security review, real external-effect ledger, eight real Authority×Product PostgreSQL16 E2E cases, independent disaster recovery, signed Ubuntu install/rollback, source/main reconciliation, and owner approval.

**STOP AFTER PHASE26.** Keep this branch stacked on Phase25; no `main` merge, Production, GitHub Actions, real keys, servers, certificates, service start or installer action.
