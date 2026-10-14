# Frozen LIC-A9 concurrency protocol V1

Protocol ID: TELMIVO_CONCURRENT_INSTALLATION_LEASE_V1.
Wire and policy freeze for LIC-A9-B; validation/publication are separate gates.
Lock architecture remains exactly the proven lock-order.md.

## Request and PoP

POST /v1/licenses/concurrency, application/json, bounded 16KiB body, HTTPS.
The canonical JSON body uses exactly the field order and names in
protocol-vectors.json/canonical_request. Counters are JSON integers, UUIDs are
lowercase UUIDv4 strings, key hash is canonical unpadded base64url SHA-256 of the
installation Ed25519 public key. Unknown/missing/duplicate fields, alternative
ordering, trailing data and noncanonical encodings fail closed.

Request fields: protocol_id, license_id, installation_id,
installation_public_key_hash, activation_id, authority_epoch, license_revision,
activation_generation, requested_concurrency_generation, recovery_generation,
previous_lease_id, runtime_session_id, request_id. Idempotency-Key must equal request_id.

Reuse exact LIC-A5 transport proof: domain TELMIVO_INSTALLATION_REQUEST_V1 plus
NUL, then uint32 big-endian length-prefixed UTF-8 method, path, raw query,
millisecond timestamp, base64url nonce, idempotency key, installation ID, raw
SHA-256(public key), raw SHA-256(body). Installation Ed25519 signs those bytes.
Headers X-Telmivo-Installation-ID/Timestamp/Nonce/Signature carry the proof.
Timestamp tolerance five minutes. A retry retains identical body/request_id,
but uses a new authenticated transport nonce/timestamp. Nonce transaction
finishes before the ratchet transaction; it holds no locks across that boundary.
Fingerprint is SHA-256(canonical body), excluding retry transport headers.

## Response and signed bytes

HTTP 200 returns the existing response framing with request_id, state SIGNED and
license_envelope (canonical unpadded base64url TLE1). No unsigned field grants
authority. TLE1 envelope/certificate framing and Ed25519 LicenseDomain remain
unchanged; payload version 2 retains every LIC-A8 field in exact codec order and
appends: protocol ID string, runtime session UUID string, concurrency lease UUID
string, int64 result generation, int64 recovery generation, conflict-state string,
int64 concurrency issued-at and int64 concurrency expires-at (UTC microseconds).
Canonical binary lengths/counters use the existing big-endian codec.

The payload binds license/installation/key hash, activation ID, epoch/revision/
activation generation, signer kid/certificate hash, full license time bounds,
lease identity/session/generation and concurrency time bounds. Signature covers
all bytes. Conflict state may be NORMAL or CLONE_SUSPECTED for the current winner;
no signed claim authorizes a losing contender or retired/recovery-required state.

Concurrency expiry never extends the existing signed license OfflineUntil.
Commercial license refresh retains LIC-A8's issuance/highest-deadline/capacity
accounting. It obtains a fresh base license first, then advances concurrency.
The concurrency hot transaction does not update activation/license rows or
extend their independently accounted lease deadline. No permanent connectivity:
the signed concurrency lease permits operation only within both signed windows.

## Ownership and retry

Successful fresh authenticated request with the current generation advances
exactly once. A new random runtime session at a legitimate restart may take over
using the current generation. Session changes alone are not clone evidence.
Different installations have independent streams even under one license, and
session-string collisions cannot cross identity binding.

Exact installation/request/fingerprint replay recovers the original committed
material without generation/session change, additional request row or additional
audit intent. Changed fingerprint returns 409 IDEMPOTENCY_CONFLICT.
Different stale request returns 409 STALE_CONCURRENT_INSTALLATION_STATE and
records CLONE_SUSPECTED while preserving the winner. Future generation returns
409 FUTURE_CONCURRENCY_GENERATION without granting or claiming a clone.
Recovery mismatch returns 409 RECOVERY_GENERATION_MISMATCH.
Epoch/mode mismatch returns 503 AUTHORITY_RECOVERY_HOLD.
Commercial/identity denials preserve existing 403/409 mappings.
Unknown protocol returns 400 UNSUPPORTED_CONCURRENCY_POLICY; malformed request
returns 400 INVALID_REQUEST; invalid proof returns 401 INVALID_SIGNATURE.
Audit backlog denies new preparations with 503 AUDIT_BACKLOG_REFRESH_REQUIRED.
Signing unavailability returns retryable 503; retain the same request.

## Signing and rotation recovery

Canonical result and original certificate/root signature are committed with the
request, generation and durable intent before signing. Persist the verified
signed artifact in a separate immutable request-keyed table before returning it.
Crash before canonical commit rolls back; crash after commit retries identical
bytes using the original signer; response loss recovers the cached artifact.
Routine rotation must retain the original eligible signing provider until every
unexpired prepared result is finalized. Cached artifacts remain recoverable
without that private provider. Explicit signer revocation/security fencing takes
precedence over idempotent delivery; never substitute a different signer or
recompute time/lease identifiers.

Final response release reacquires authority SHARE, installation lease SHARE,
live binding reads and signer SHARE, revalidates commercial/recovery/signer
state and signed time bounds, persists/reloads the unique artifact, then commits.
No lease-to-exclusive-authority upgrade. Audit finalizer never locks leases.

## Client persistence and effect decisions

Persist the exact pending body/request identity before dispatch. Resolve pending
outcomes before generating a new request. Atomic high-water precedes envelope
commit; restrictive permissions and fsync/rename/directory fsync remain LIC-A8.
Reject lower epoch, revision, activation, concurrency or recovery generation;
reject prior-lease replay and out-of-order responses. Restricted/clone/recovery
states never delete data or installation identity. Existing pre-effect and
durable-outcome fences stay authoritative; Telegram ACK-safe behavior remains.

A local lifetime leader lock permits one refresh coordinator per installation
directory; followers use its durable verified state. Each runtime has a fresh
random session. Completely offline byte-identical clones remain indistinguishable
without hardware attestation; bounded offline operation is allowed, and stale
ownership is detected on Authority contact. No hardware/IP identity.

## Administrative recovery

Authenticated, RBAC-protected, confirmed operator action retires the old
installation/key permanently, fences activations and concurrency state, retains
audit, and creates one-time replacement authorization. New installation keys are
generated only at the legitimate client. Existing capacity quarantine/highest
lease deadlines govern slot reassignment. Never merely clear CLONE_SUSPECTED.
Epoch restore follows RECOVERY_HOLD, drain, zero-pending check, independently
authorized high-water advance and existing reconciliation. No epoch downgrade.

## Current ownership proof

A fresh request also binds previous_lease_id: null only at generation zero, otherwise the exact last signed unpredictable UUIDv4 lease ID. Correct predicted generation without the current lease ID is stale concurrent state and cannot grant a lease. Exact committed replay is resolved first. Only privileged operator views expose the current lease ID; installation status must not reveal it to holders of a copied key. Legitimate restarts retain it, and pending response-loss retries recover it. This is possession of signed current state plus Installation PoP, not bearer-only licensing or hardware DRM.

## Operational rotation and customer recovery

The online signer registry retains the current and up to four previous eligible
delegated providers. The optional AUTHORITY_PREVIOUS_SIGNERS_MANIFEST is a
version-1 public configuration listing key_id, credential_name and
credential_file; secret files still pass LIC-A7 custody checks. No private value
belongs in that manifest or logs. A DB guard blocks RETIRED while an unexpired
(72-hour conservative retention) committed concurrency result lacks its cached
artifact. Explicit REVOKED overrides retries for security. Preserve retained
credentials through overlap and finalize/recover pending results before removal.

Private operator commands /retire ACTIVATION_UUID GENERATION REASON and
/recover ACTIVATION_UUID GENERATION EXPIRY_RFC3339 REASON use existing RBAC and
five-minute destructive confirmation. /instances includes concurrency state,
lease/session, conflict/recovery timestamps and pending intent counts; /audit
retains the canonical history. Replacement code is shown once, never evidence.

For a legitimate replacement, use a NEW protected TELMIVO_LICENSE_STATE_DIR,
run licensectl identity to create a fresh keypair, then activate the one-time
replacement authorization through stdin. Do not delete the old state or copy
its private key. Set the normal services to the new state directory only after
successful verification; ordinary LIC-A8 business/outcome fencing remains.
Slot quarantine and commercial capacity determine when activation is allowed.
licensectl status exposes safe concurrency diagnostics; concurrency-refresh
resolves pending concurrency outcomes. This stage changes no customer host.
