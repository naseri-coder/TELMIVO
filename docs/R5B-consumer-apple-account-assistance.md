# R5B — Consumer Apple Account Assistance

R5B builds the consumer-facing Apple Account assistance workflow on top of the R5A foundation.
It does not create Apple Accounts on behalf of users and it never receives Apple passwords,
one-time codes, verification codes or Apple credentials.

## Scope

R5B supports three guided intents:

- `CREATE_ACCOUNT`
- `RECOVER_ACCESS`
- `CHECK_EXISTING`

The assistance operation is always bound to the authenticated TELMIVO user. The API does not
accept an arbitrary subject/user identifier from the client.

The guided lifecycle is:

`REQUESTED → PRECHECK → APPLE_ACTION_REQUIRED → APPLE_COMPLETED → VERIFICATION_PENDING → READY`

Every step is explicit and revision-fenced. A consumer operation created through R5B cannot be
advanced through the generic R5A transition endpoint.

## Apple-controlled actions

When an Apple action is required, TELMIVO only provides an official Apple destination:

- account creation/account management: `https://account.apple.com/`
- access/password recovery: `https://iforgot.apple.com/`
- support context: `https://support.apple.com/apple-account`

Apple's current support guidance directs users to create an account through Apple devices/apps or
the Apple Account website and to perform password/account recovery through Apple's own recovery
flow. TELMIVO does not proxy those pages, scrape them, solve challenges, receive OTPs or submit
credentials.

## Completion semantics

R5B `READY` means only that the authenticated user explicitly attested that the official Apple
flow was completed. The durable record stores `completion_source=USER_ATTESTED`.

R5B never reports `identity_verified=true`. Cryptographic/account identity linking belongs to
R5C Sign in with Apple.

## Persistence and privacy

Migration 0025 adds `apple_consumer_assistance`, keyed one-to-one to an R5A
`CONSUMER_ACCOUNT` operation. It stores only non-secret workflow metadata: goal, device/channel,
locale, optional two-letter region, guidance version and completion source.

The table uses FORCE RLS. Consumer identity metadata is immutable, completion attestation is
irreversible, and cross-workspace reads are blocked.

The HTTP JSON decoder rejects unknown fields. Requests containing fields such as `password`,
`otp` or `verification_code` therefore fail before reaching the service layer.

## Authorization and commercial boundary

R5B reuses the R5A `apple.consumer.setup` capability, Apple Services entitlement and operation
quota. No new plan is enabled and no consumer pricing is introduced in R5B.

R5A currently grants Apple capabilities only to OWNER/ADMIN workspace roles; R5B does not invent a
new general-member role. Broader end-user packaging and Mini App UX remain R5G work.

## Validation

Native validation covers idempotent start, conflicting replay, lifecycle sequencing, prevention
of generic-transition bypass, official-link selection, quota accounting, FORCE RLS, secret-field
absence, irreversible user attestation and PostgreSQL backup/restore.

No outbound Apple request is used in tests or runtime R5B code.
