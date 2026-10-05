# R4B — Channel Manager

Base checkpoint: R4A customer-bot runtime convergence.

R4B turns the top-level Channels surface into a bounded Telegram channel product. It reuses
the existing `channels` and `channel_settings` ownership tables and adds migration 0019 for
draft/scheduled publication state and immutable history.

## Connection and authority

A channel connection is accepted only when:

- the caller currently has `channels.manage` and effective Channels entitlement,
- the caller's verified Telegram identity is an administrator of the target chat,
- `getChat` confirms the target is a Telegram channel,
- the TELMIVO Main Bot is an administrator and can post messages.

Connection stores only the channel identity/title and an authority snapshot. Tokens are never
stored in channel records. Revalidation is explicit. Disconnecting TELMIVO from a channel
changes product state only; it does not delete drafts or publication history.

## Drafts and scheduling

Channel posts are tenant/channel scoped. A create request has a bounded idempotency key and
fingerprint. Draft text may be edited only while the post is DRAFT. Scheduling freezes the
message body. Supported management transitions are:

- DRAFT -> SCHEDULED
- DRAFT -> CANCELLED
- SCHEDULED -> SCHEDULED (explicit reschedule)
- SCHEDULED -> CANCELLED
- DRAFT/SCHEDULED -> publish-now, represented as a due SCHEDULED post

The worker is database-driven. No browser or API request performs the Telegram send directly.

## Delivery safety

Before an external send the worker atomically moves one due post from SCHEDULED to SENDING.
It rechecks the original actor's current product access and Telegram administrator authority,
and rechecks the Main Bot's channel posting right.

A successful Telegram acknowledgement becomes SENT. A deterministic rejection becomes
FAILED. A transport result that may have reached Telegram becomes UNKNOWN. UNKNOWN is
terminal and is never blindly retried. If a worker disappears after claiming SENDING, the
claim is conservatively sealed as UNKNOWN after the uncertainty window rather than resent.

This preserves the existing TELMIVO rule that uncertainty must not create duplicate external
effects.

## Persistence and recovery

Migration 0019 adds `channel_posts` and `channel_post_history`, forced RLS, revision guards,
terminal immutability and a due-work index. The existing Channels feature flag is enabled;
plan/role policy remains authoritative.

Native CI covers connection authority, acknowledged delivery, UNKNOWN/no-retry behavior,
browser RTL acceptance and backup/restore recovery. No production Telegram resource or
Production deployment is authorized by R4B.
