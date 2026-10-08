# LIC-A5 installation M2M contract v1

PRIVATE — PROPRIETARY — NOT FOR PUBLIC DISTRIBUTION.

This API is for customer-owned installations. It has no Telegram/Mini App or
operator endpoints. Software licensing remains separate from product customer
plans, workspace authorization and quotas. No production custody/deployment is
implemented. Request-contract changes require a new version.

## Routes and bodies

| Route | Method | JSON/body or query | Successful result |
|---|---|---|---|
| /v1/licenses/activate | POST | activation_code, installation_id, installation_public_key, license_id, authority_epoch | 201 new activation; 200 committed retry; 202 pending issuance |
| /v1/licenses/refresh | POST | activation_id, authority_epoch | 200 signed lease or committed retry; 202 pending issuance |
| /v1/licenses/deactivate | POST | activation_id, authority_epoch | 200 durable operation result |
| /v1/licenses/status | GET | exactly `activation_id=<lowercase UUIDv4>`; empty body | 200 installation-scoped snapshot |

All identity fields and Idempotency-Key are lowercase UUIDv4. Installation public
keys and signatures use strict unpadded base64url. A public key has 32 bytes; a
signature has 64. The installation private key never enters the authority.
Mutation JSON is strict: unknown fields, duplicate fields, malformed data and
multiple objects are rejected. Body bytes need not have canonical JSON order:
the signature binds the SHA-256 of the exact supplied bytes, while business
fingerprinting uses decoded semantic fields. Clients must re-sign after any
body-byte change. Mutations require application/json, limited to 16 KiB by
default; configuration cannot exceed 64 KiB. GET has no body.

## Authentication headers

Exactly one value each:

- X-Telmivo-Installation-ID
- X-Telmivo-Timestamp: canonical decimal UTC Unix milliseconds
- X-Telmivo-Nonce: 32 random bytes, unpadded base64url (43 characters)
- X-Telmivo-Signature: Ed25519 signature, unpadded base64url
- Idempotency-Key: required for mutations, absent for status
- X-Request-ID: optional UUIDv4 correlation ID; server replaces absent/invalid IDs

Duplicate security-sensitive headers, comma-merged values, unknown X-Telmivo-*
headers and ambiguous forwarding headers are rejected. Signature header ordering
and HTTP framing are never signed. Correlation ID is excluded from authentication
and business identity; it cannot grant authority.

Exact signing input:

1. ASCII `TELMIVO_INSTALLATION_REQUEST_V1`, followed by one zero byte.
2. Nine length-prefixed byte fields in this exact order: HTTP method, path,
   query, timestamp, nonce, idempotency key, installation ID, public-key hash,
   body hash.

Each length is unsigned uint32 big-endian, followed immediately by field bytes.
Text uses UTF-8; hashes are raw 32-byte SHA-256 values. Method is exactly POST or
GET. Path is exactly the route above, without percent encoding, aliases, trailing
slashes or normalization redirects. Mutation query is empty. Status query is the
exact single UUID parameter shown above. Empty body hashes as SHA-256(empty).
The public-key hash is SHA-256(raw installation Ed25519 public key).

Activation selects the submitted public key, checks header/body identity and
verifies possession before any code redemption. Registered routes select the
immutable stored public key, verify possession, then check activation ownership.
Unknown registered identities fail authentication without returning records.
Server UTC time must be within +/-5 minutes of the signed request timestamp.
Transport timestamps are independent of license validity clocks.

## Durable replay and retry

Accepted nonce hashes are unique in PostgreSQL. Activation scope is submitted
public-key fingerprint; registered scope is installation UUID. A nonce is retained
for 11 minutes after acceptance, covering the full timestamp acceptance window.
Timestamp checks also occur at insertion. Nonces are consumed before business
mutation; business denial does not make an authenticated packet replayable.

An identical packet is REQUEST_REPLAYED, including after handler/process reset
or database reconnect. A legitimate network-loss retry signs a NEW nonce and
NEW timestamp while retaining the SAME Idempotency-Key and semantic request.
Nonce, timestamp, signature and correlation ID never alter operation identity.
Exact SIGNED artifacts are retrieved, not regenerated. Signer rotation resolves
an existing activation operation's intended signer rather than changing its
fingerprint; refresh semantic identity excludes server-selected signer metadata.
Idempotency tombstones are not removed by transport cleanup.

CleanupTransport deletes at most 1,000 expired nonce rows and 1,000 old expired
rate buckets per call. Invoke it through a later authorized bounded maintenance
schedule. No daemon or operational schedule is deployed in LIC-A5. Omitted
cleanup increases storage usage but never relaxes replay protection.

## Rate limits

All counters are PostgreSQL-backed, using atomic UPSERT and fixed-length
windows starting at the first request after expiration. Defaults per minute: source IP 120,
activation/source IP 20, authenticated installation 120, global safety 2,000.
Values are validated and injectable configuration, not production tuning claims.
No Redis or process-local authoritative limiter exists.

Source budgets are checked before the global budget. Once a source is blocked,
its continued attempts do not drain the global budget. Installation limits are
applied only after valid authentication. Distributed sources may intentionally
reach the global safety ceiling: that limit is an availability tradeoff, not a
promise of immunity to distributed denial of service. IP bucket identities are
hashed; a plain hash is pseudonymization, not confidentiality against guessing.
429 returns RATE_LIMITED and integer Retry-After seconds. Abuse cannot discover
whether an activation code was almost valid.

## Operations and status

Refresh serializes under the existing authority lock, establishes a durable
operation, locks the activation/license/installation/slot, validates ownership,
ACTIVE state, fences, term, epoch/high-water, capacity and signer eligibility,
then creates exactly one immutable PREPARED issuance per operation. It keeps the
same activation generation and ASSIGNED slot, and increases the maximum possible
lease deadline monotonically. LIC-A4 finalization accepts ACTIVE/ASSIGNED only
for REFRESH; initial ACTIVATE still requires RESERVED/RESERVED.

Deactivation requires authenticated ownership, fences refresh, cancels unfinished
issuances, moves the activation to DEACTIVATED and quarantines capacity through
highest possible lease deadline plus five minutes. Mutation, operation result
and audit commit together. Neither heartbeat silence nor successful deactivation
immediately frees capacity.

Status uses one REPEATABLE READ read-only transaction. Raw authoritative states
are reported separately from refresh eligibility/restricted reason. Restriction
precedence: authority/recovery hold, license revocation, retired/disabled
installation, supersession, suspension, commercial expiry, other inactive license,
inactive/fenced activation or unassigned slot. Independent high-water failure also
disables refresh eligibility. Raw activation ACTIVE is never a claim of effective
access when license state or restricted_reason denies it. Latest lease metadata
comes only from SIGNED issuances. No customer PII, code, other installations,
operator data or private signing details are returned.

## Error/status mapping

Responses contain request_id and, on denial, error.code/error.retryable. Pending
issuance also includes durable operation/activation/issuance references. SQL
messages, table names and provider errors never leave the transport.

| HTTP | Codes/meaning |
|---|---|
| 200 | Read, refresh, deactivation, committed retry |
| 201 | Newly signed activation |
| 202 | TOKEN_ISSUANCE_PENDING; retry same operation with fresh proof |
| 400 | INVALID_REQUEST |
| 401 | INVALID_SIGNATURE, INVALID_INSTALLATION_IDENTITY, REQUEST_TIMESTAMP_INVALID |
| 403 | ACTIVATION_DENIED, LICENSE_NOT_ACTIVE/SUSPENDED/EXPIRED/REVOKED, INSTALLATION_DISABLED/RETIRED, TRANSPORT_SECURITY_REQUIRED |
| 404 | NOT_FOUND for unknown route only; no license enumeration |
| 405 | METHOD_NOT_ALLOWED, with Allow |
| 409 | REQUEST_REPLAYED, IDEMPOTENCY_CONFLICT, INSTANCE_LIMIT_REACHED, ACTIVATION_NOT_ACTIVE, INSTALLATION_SUPERSEDED |
| 413 | BODY_TOO_LARGE |
| 415 | CONTENT_TYPE_UNSUPPORTED |
| 429 | RATE_LIMITED, with Retry-After |
| 503 | AUTHORITY_RECOVERY_HOLD, AUTHORITY_UNAVAILABLE, SIGNING_TEMPORARILY_UNAVAILABLE |
| 500 | INTERNAL_ERROR |

ACTIVATION_CODE_EXPIRED/USED are internal taxonomy concepts and collapse to
ACTIVATION_DENIED externally, together with unknown/revoked/wrong-scope codes.
The signer-unavailable case after durable preparation returns 202, not another
allocation. Worker contention is also pending. A changed finalization fence is
rejected; no in-memory signature is delivered. Temporary database failures use
503; retries retain their original idempotency key. No 410 is used in v1.

## Transport and logging

Production requires direct TLS or an immediate peer inside an explicitly trusted
CIDR with exactly X-Forwarded-Proto: https. Untrusted forwarded headers never
establish TLS or source IP. The supported proxy contract is one proxy with one
X-Forwarded-For IP; comma chains and Forwarded on trusted peers are rejected.
The proxy must replace incoming forwarding headers. No wildcard trust/default
public IP is supplied, and no proxy/TLS is deployed here.

Server defaults: header-read 5s, read 10s, write 20s, idle 60s, headers 16 KiB,
handler context deadline 15s. Signer implementations must respect context
cancellation. Responses use no-store and nosniff. Logs allow only generated or
validated request ID, fixed route label, status, elapsed duration, authenticated
installation ID, operation ID and stable code. No bodies, activation codes,
signatures, nonces, DB diagnostics or environment values are logged. Unknown
paths are logged as `unknown`, never as arbitrary user-supplied text.

GET /health/live reports process liveness; /health/ready checks database authority
state, configured public certificate trust, signer presence and independent high-water. Neither exposes infrastructure
or business counts. No mutation endpoint exists outside /v1.

## Command and production boundaries

cmd/license-api uses AUTHORITY_DATABASE_URL and injected activation pepper/version,
AUTHORITY_ENVIRONMENT, AUTHORITY_ID, AUTHORITY_SIGNER_KEY_ID,
AUTHORITY_SIGNER_PROVIDER, AUTHORITY_LISTEN_ADDRESS and optional
AUTHORITY_TRUSTED_PROXY_CIDRS. No real values or example secrets are embedded.
Listen defaults to loopback. It never runs migrations automatically.

The only command provider mode in LIC-A5 is `disabled`. Production signer/root
trust/high-water providers must be wired under a later authorization. The library
server accepts real LIC-A4 interfaces; tests inject synthetic signers. The command
fails closed for new grants and readiness without custody/high-water providers.
TEST ONLY fixtures are never dependencies of the command. Server configuration,
roots and providers are immutable during concurrent request serving; future hot
rotation requires synchronized replacement. Production database role provisioning,
maintenance cadence and TLS/provider tuning remain operations qualification.
