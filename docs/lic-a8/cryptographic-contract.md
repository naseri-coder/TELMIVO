# LIC-A4 cryptographic contract v1

PRIVATE — PROPRIETARY — NOT FOR PUBLIC DISTRIBUTION.

## Trust and custody

An offline Ed25519 root certifies an online delegated Ed25519 signer. The
signer signs short-lived installation licenses only. Root private material is
outside the authority process. `RootCertifier` is an offline interface;
`signing.Signer` is the online boundary. Neither business code nor PostgreSQL
receives private keys. LIC-A4 has no production signer implementation or custody.
Only `internal/testfixture` creates TEST ONLY synthetic ephemeral signing keys.
No production key, seed, pepper, credential, or infrastructure is supplied.

Trusted root public metadata is caller-supplied: opaque UUIDv4 ID, version,
32-byte public key, UTC validity and PENDING/ACTIVE/RETIRING/REVOKED/EXPIRED
status. Multiple roots are accepted during controlled migration. Retiring roots
verify historical certificates; they cannot register new signer certificates.
Root rotation is a future operator/custody procedure, without cross-signing.
The verifier trusts only the explicit root set, never roots from an envelope.

## Canonical positional binary encoding

This is an explicitly specified deterministic binary format, not JSON, TLV, JWT,
or arbitrary Go marshaling. All integers are big-endian. `u16` is unsigned;
`i64` is signed two's complement. Times are UTC Unix microseconds, durations are
microseconds. `blob` is uint32 byte length followed by bytes; `str` is a blob of
valid UTF-8, at most 256 bytes. There is no terminator or alignment padding.
Entire objects are at most 65,536 bytes. UUIDs are lowercase canonical UUIDv4.
No trailing bytes, alternate ordering, unknown fields, or duplicate fields are
accepted: every field has exactly one position. Capabilities are at most 128
nonempty strings, strictly ascending by UTF-8 bytes with no duplicates.
Future format changes require a new version; do not extend v1 silently.

The following field sequences are exhaustive and ordered:

| Object | Exact sequence |
|---|---|
| Certificate payload | ASCII `TSC1`; version u16; algorithm str; product str; certificate_id str; key_id str; root_id str; root_version i64; public_key blob; not_before i64; not_after i64; token_type str; audience str; authority_id str; environment str; maximum_lifetime i64; epoch_min i64; epoch_max i64 |
| Packed certificate | ASCII `TCB1`; certificate payload blob; root signature blob |
| License payload | ASCII `TLP1`; version u16; algorithm str; product str; type str; issuer str; audience str; license_id str; activation_id str; installation_id str; installation_key_hash blob; edition str; capability count u16; each capability str; license_revision i64; activation_generation i64; authority_epoch i64; issued_at i64; not_before i64; refresh_after i64; online_until i64; offline_until i64; commercial_expires_at i64; token_id str; kid str; environment str; certificate_id str; certificate_hash blob |
| Signed envelope | ASCII `TLE1`; version u16; algorithm str; certificate payload blob; root signature blob; license payload blob; delegated signature blob |

Version is 1; algorithm is exactly `Ed25519`; public keys have 32 bytes and
signatures 64 bytes. Product is `TELMIVO`, audience `TELMIVO_INSTALLATION`, token
type `TELMIVO_INSTALLATION_LICENSE`. Environments are TEST, DEVELOPMENT or
PRODUCTION, independently scoped to an authority UUID. Epoch bounds zero/zero
mean unrestricted by the certificate; otherwise positive ordered bounds.
The certificate maximum lifetime cannot exceed 72 hours.

Exact signing inputs, including a terminating zero byte after each domain:

- Certificate: ASCII `TELMIVO_LICENSE_SIGNER_CERT_V1` + byte 0 + certificate payload.
- License: ASCII `TELMIVO_INSTALLATION_LICENSE_V1` + byte 0 + license payload.

Ed25519 signs these exact bytes, without an extra prehash. Certificate hash is
SHA-256 of the complete `TCB1` packed certificate including root signature.
The license signature therefore binds the certificate ID and hash as well as
all claims. Neither certificate substitution nor envelope re-framing grants
additional authority. Installation key hash is SHA-256 of the installation's
raw public key, as established by LIC-A3. License payloads contain no customer PII.

## Verification and time

`pkg/licensecrypto` is standalone and standard-library-only. Frame parsing and
certificate parsing precede signature verification solely for key selection;
certificate authorization follows root-signature verification. License claims
are parsed only after delegated-signature verification. Errors return no claims.
Result is typed: VALID_ONLINE or VALID_OFFLINE; the latter carries the
ONLINE_LEASE_EXPIRED warning. `RequireOnline` instead rejects an expired online
lease. Malformed, algorithm/version, trust/key, scope, identity, signature,
revision, epoch, commercial/time and clock errors are explicit typed codes.

The caller supplies expected product, audience, authority, environment,
installation ID/hash and explicit verification time. Optional signer status,
minimum epoch/revision and last trusted time constrain cached trust. A nil
signer-status map permits any correctly root-certified signer; a non-nil map
requires known eligible key metadata. Ordinary verification accepts ACTIVE and
RETIRING signers. Registration permits staging a future-valid certificate;
that path never authorizes a license.

Defaults: refresh after 6 hours plus deterministic 0–5 minute jitter, normal
online lease 24 hours, offline lifetime at most 72 hours, clock tolerance 5
minutes. Issuance truncates leases at commercial/key validity limits and rejects
an insufficient grant window. Enforce issued_at <= not_before + tolerance,
issued_at < refresh_after <= online_until <= offline_until <= commercial expiry.
Offline lifetime measured from issued_at must also fit certificate maximum
lifetime, global policy and signer expiry; certificate validity fits root
validity. Future not_before and issued_at allow only five minutes skew. Optional
local time high-water detects rollback beyond that tolerance. Expiration at the
exact deadline is denied; expiration is never extended by skew tolerance.

## Rotation, revocation and cloning

Database STAGED is cryptographic PENDING; RETIRED is cryptographic RETIRING.
New signer registration is public certificate metadata only. Atomic rotation
activates the staged new signer and retires the old signer. Retired keys stop new
issuance but their certificates/public keys remain for historical verification.
REVOKED and EXPIRED are terminal. Metadata and certificate evidence are retained.
Expiry transitions require elapsed validity. Root retirement/revocation and
public trust-set distribution need an authorized operations procedure later.

A supplied revoked signer status denies verification; authority-side revocation
denies new grants and finalization immediately. Disconnected clients cannot
learn key/license revocations instantly and may retain a valid cached lease up
to its signed offline deadline. Epoch/revision high-waters must be distributed
and durably retained by future clients. Cryptography cannot distinguish a
perfect clone containing the same installation ID and private key. Binding
rejects different identities or public-key fingerprints, not perfect clones.
