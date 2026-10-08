# LIC-A9 lock architecture

Protocol: TELMIVO_CONCURRENT_INSTALLATION_LEASE_V1.
Authority parent: 392fc89087d8dac001d1709d0f7973c2f28a8515.
Client parent: e87e85d9cd44470ac46fb73732ca2fed59bf229b.

This checkpoint implements transaction primitives, schema invariants and audit
finalization. It does not expose an issuing endpoint, complete the client,
execute AC-1..30, or authorize production. The existing crypto drafts remain
drafts until the subsequent complete protocol integration. No unsigned prepared
material is a concurrency authority.

## Existing graph

Existing business, activation, operation and recovery paths acquire
authority_state UPDATE through authority() before business rows. Their audit()
inserts invoke append_audit(), which reacquires the already-held singleton lock.
License/slot/activation triggers can lock license rows; those paths remain under
the initial UPDATE fence. Their detailed relative business-row order is unchanged.
The new paths never call an existing mutation helper after acquiring a lease row.

## New acquisition order

| Path | Exact order |
| --- | --- |
| Ratchet preparation | authority_state SHARE; target concurrency_leases UPDATE; immutable concurrency_requests lookup; one free audit_intent_capacity row UPDATE SKIP LOCKED; new audit_intents INSERT; new concurrency_requests unique key INSERT; target lease UPDATE |
| Exact replay | authority_state SHARE; target lease UPDATE; immutable request lookup; return committed bytes without any insert, generation change or ownership change |
| Audit finalizer | authority_state UPDATE; one pending audit_intents UPDATE; audit_events INSERT via unchanged append_audit; intent finalization UPDATE; its capacity row UPDATE/release |
| Enter drain hold | authority_state UPDATE; authority mode UPDATE; existing synchronous audit INSERT; commit |
| Epoch recovery | authority_state UPDATE; assert previously committed RECOVERY_HOLD and zero pending intents; existing license/slot/issuance reconciliation and audit path |
| Future signer response release | authority_state SHARE; target lease SHARE; immutable request lookup; existing license/activation/installation reads; signing_keys SHARE; final live-fence validation; commit; response |

No ratchet touches existing operations rows, license UPDATE rows, activation
UPDATE rows or signing state. Request lookups need no row lock: committed
requests cannot change. The per-installation lease lock serializes their unique
key insert. Initial installation/lease enrollment will remain an existing
exclusive administration transaction, not a repeated refresh transaction.

Foreign-key checks take compatible key-share locks on identities; they do not
upgrade authority_state. New row inserts and deferred integrity checks never
call append_audit or any function acquiring authority_state UPDATE.

Finalization acquires authority UPDATE before selecting an intent, and never
locks a lease. Capacity allocation is SKIP LOCKED, not a shared singleton counter.
Every shared-fence holder takes its per-installation row before capacity; every
exclusive-fence holder takes authority before intents/capacity. Exclusive
authority ownership excludes all ratchets before the finalizer touches any row
they could hold. Consequently no row-to-authority upgrade edge exists and no
cycle exists among these paths. Never invoke finalizeAuditIntent inside a ratchet
transaction. Never write directly to these tables from a differently ordered
service path.

## Durable evidence and backlog

A deferred constraint requires each nonzero committed generation to reference
its immutable request and matching audit intent, including epoch/entity/revision.
Requests distinguish requested_generation from result_generation and preserve
canonical_result bytes with their SHA-256 hash. Intent identity is the eventual
canonical event_id. Payload is immutable; finalization columns may transition
once, and must match the actual canonical event content, sequence and hash.
No DELETE/TRUNCATE; no request UPDATE.

The hard pending limit is 256 capacity tokens. Each new mutation atomically
claims a distinct token; exhausted tokens fail closed. An oldest pending age
above five minutes also denies new preparations with
AUDIT_BACKLOG_REFRESH_REQUIRED. Exact committed request replay remains available.
Already-valid signed offline allowances are unaffected. AuditIntentBacklog
exposes pending count and oldest age in seconds; production readiness/monitoring
must consume both before launch. This checkpoint does not claim monitoring
deployment.

## Epoch transition

EnterAuditDrainHold commits RECOVERY_HOLD under authority UPDATE, waiting for
existing shared-fence transactions. New granting preparations fail closed.
Finalizers remain allowed in the old epoch while mode is held. Drain all pending
intents and verify zero. A database epoch trigger rejects any epoch advance
unless the OLD mode is RECOVERY_HOLD and no pending intent remains. Then use the
existing root-authorized high-water recovery/reconciliation path. Never bypass
its authorization or return to NORMAL before its existing invariants pass.

## Idempotency and response/signing crash model

The authenticated protocol service must validate proof-of-possession and every
identity/epoch/revision/activation/conflict field under the shared fence before
preparing a fresh result. The private lock primitive is not that service and
does not sign or return a usable license. It is intentionally unreachable from
HTTP at this checkpoint.

Before commit the complete canonical V2 claim payload must be fixed, including
root-certified signer kid/certificate identity, license/installation/key hash,
authority epoch, revision, activation/concurrency/recovery generation, session,
lease/token identity and all issued/not-before/expiry/offline timestamps.
The private prepared transaction stores those exact bytes, request fingerprint,
requested/result generation and intent together. Canonical fields are never
recomputed from a new clock, session, signer or UUID on retry.

After commit, signing uses those exact bytes with the original delegated key.
Ed25519 signing is deterministic; retry yields the same logical result and,
with fixed certificate/envelope bytes, the same artifact. If the original signer
is unavailable/retired/revoked, fail closed; never silently substitute a key.
No response may be released before commit. A crash before commit rolls everything
back. A crash after commit or response loss recovers the original prepared bytes
by installation/request identity without advancing generation. A crash during
signing retries those same bytes.

Before releasing a freshly signed response the future service must reacquire the
shared authority/lease/signing-key fences in the stated order and revalidate
live licensing, activation, generation, epoch, signer and time bounds. Failed
revalidation does not authorize fresh effects. Exact request replay recovers the
original logical response, never changes current ownership; it does not override
commercial revocation or create a new grant. The client high-water rules reject
older returned claims. This response-release integration and its adversarial
tests belong to the next stage, not the present lock proof.

## Focused proof evidence

Disposable PostgreSQL only: loopback port 55449, database lic_a9_lock_test,
outside both repositories. Final race-enabled targeted run:
go test -vet=off -race -p=1 -count=1 -timeout=180s ./internal/store/postgres -run '^TestLICA9Lock' -v

LOCK-1 through LOCK-10: all PASS. LOCK-1 holds A's transaction open while B
commits, proving nonexclusive global-fence coexistence. LOCK-3 uses 100
independent installation streams. LOCK-4/6 inspect actual PostgreSQL blocking
relationships rather than rely solely on sleeps. LOCK-5/9 recompute the complete
canonical hash chain and verify the singleton head. LOCK-9 closes the database
connection before commit, then verifies one final event after retry.
Runtime-grant, immutable/durable-evidence, hard backlog-bound/age and
deterministic prepared-signing recovery tests also PASS. No skipped tests or
race diagnostic. Final exit 0; duration 67.009 seconds.

Durable evidence: /home/licstage/lic-a9-lock-evidence/locks.log,
locks.exit and reconciliation.json. The earlier SQL parameter-type error is
preserved separately as locks-initial-sql-error.log/exit, not a current result.
No AC matrix, LIC-A7/A8 rerun, production operation or publication occurred.

PER_INSTALLATION_RATCHET_INDEPENDENCE = TRUE.
DEADLOCK_CYCLE_FOUND = NO within the specified application lock graph.
The lock architecture is frozen; complete endpoint, PoP-service, signer-release
and client integration remain the next implementation stage.

## LIC-A9-B implementation extension

The preceding evidence is the preserved lock checkpoint, not a claim that the
current service is absent. LIC-A9-B now connects those primitives to the
authenticated issuing service and client.

The issuing hot path adds compatible signing_keys SHARE metadata validation
after authority SHARE/installation UPDATE and before the capacity token. It
never locks license or activation rows exclusively or extends their lease
deadlines. Those live reads are fenced by authority SHARE against the existing
exclusive business-mutation paths. Final response release uses authority SHARE,
lease SHARE, binding reads, signer SHARE, immutable artifact INSERT/unique-key
replay. No request/artifact holder upgrades authority to UPDATE. Artifact and
request evidence cannot be rewritten or deleted. Enrollment/recovery remain
existing exclusive administration paths. The global audit finalizer order is
unchanged and never touches leases.

Opaque previous signed lease UUID plus PoP complements the predictable
generation; it adds no database lock class. Protocol and vectors contain the
exact request and response rules. The original locks.log/exit/reconciliation
remain untouched outside both worktrees.
