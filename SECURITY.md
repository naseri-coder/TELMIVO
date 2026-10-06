# Security

TELMIVO is private, proprietary software. Report vulnerabilities privately to the repository owner through GitHub private vulnerability reporting if enabled, or an agreed private owner channel. Do not post secrets or customer information in issues.

Runtime database credentials use NOSUPERUSER/NOBYPASSRLS and cannot inherit the migration role. API membership checks select tenant context before FORCE RLS queries. These credentials are trusted server credentials, not credentials given to customers: possession of a runtime connection can set tenant context and is a server compromise. RLS supplements application authorization; it does not authenticate end users.

The migration role has BYPASSRLS for migrations and the narrowly scoped SECURITY DEFINER `platform_plan_in_use` function. The function checks `platform.plans.manage`, returns only a boolean and cannot return subscription rows. Public execution is revoked. Runtime role must never own it or receive migration role membership. No shared privileged connection is used for tenant queries.

Opaque session tokens are hashed in PostgreSQL. Telegram initData is verified server-side and replay rejected. Mutation Origin must match the configured Mini App scheme and host. Missing Origin with a session cookie is denied. Use HTTPS and production Secure cookies for live sessions.

Customer credential encryption uses AES-256-GCM with tenant/bot context as authenticated data. Production encryption keys must come from a secret manager/KMS with key IDs and rotation; the local AES adapter is an interface foundation, not a managed credential service. Never log authorization headers, bot tokens, infrastructure URLs, payment secrets or credential ciphertext/plaintext. Errors log fixed classifications instead of raw SQL/transport errors.

R4 payment effects use durable intent/event/refund state and treat uncertain provider outcomes as UNKNOWN rather than authorizing blind replay. Telegram Stars verification rechecks payment identity, amount, currency and payer context before marking a payment paid. Production payment-provider settlement remains outside this checkpoint.

Automation 2.0 accepts bounded declarative schedules only. Cron/SLA scheduling does not execute user code, shell, arbitrary SQL or arbitrary webhook URLs. SLA effects re-read authoritative ticket state before execution. AI Assistant 2.0 imports only same-tenant TELMIVO File Manager metadata or bounded private Uploader text; it does not fetch arbitrary URLs or run arbitrary parsers. Retrieved/model content cannot grant write authority: write tools still require configured permission plus explicit request authorization.

R4G freezes the v0.2.0 source migration chain at 0001–0023 and adds final FORCE-RLS, cross-feature recovery and release-matrix checks. A GitHub Actions failure before the first step, a cancelled/missing job or an unapproved skip is not security validation evidence and must not be reported as a release PASS.

CI scans source secret patterns, Go vulnerabilities and npm high/critical advisories; CodeQL depends on private-repository capabilities. The R1 run reported code scanning disabled; the prepared job requires the CODEQL_ENABLED repository variable after the owner enables that capability. Pattern scanning is supplementary, not exhaustive security assurance. Synthetic mode disables live bot webhooks, notifications, media and payments. No production deployment exists.

## R5A Apple Services foundation

Apple Services foundation state is workspace-scoped and protected by FORCE RLS. R5A stores no
Apple password, one-time code, access token, client secret or provider credential. The foundation
API can only create and transition internal TELMIVO operations; it performs no outbound Apple,
Google, Microsoft or generic IdP request.

Apple operation creation is entitlement- and quota-gated, idempotent, revision-fenced and audited.
Only OWNER/ADMIN receive Apple permissions in R5A. Existing plans and plugin entitlements remain
disabled by default; controlled grants are required until later commercialization work.

## R5B consumer Apple Account assistance

R5B binds consumer assistance to the authenticated TELMIVO user and never accepts an arbitrary
consumer subject from the client. The HTTP decoder rejects unknown fields, so request payloads
containing password, OTP or verification-code fields fail before reaching the service layer.

TELMIVO never proxies Apple sign-in/account pages, never submits Apple credentials, never handles
Apple two-factor challenges and never claims that R5B READY state is verified Apple identity.
R5B READY means only USER_ATTESTED completion of an Apple-controlled flow. R5C remains responsible
for future Sign in with Apple identity verification and account linking.

The consumer assistance table stores only goal, device/channel, locale, optional region, guidance
version and completion source. It uses FORCE RLS; identity metadata is immutable and a recorded
USER_ATTESTED completion cannot be reverted.
