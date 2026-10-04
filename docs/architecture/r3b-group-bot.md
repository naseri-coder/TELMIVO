# R3B Group Bot and Group Management

R3B extends the frozen R3A delivery `05fc043c53a7a4361b1b99b092c0d253fdf2e881` on `feature/r3b-group-bot-v0.1`. Main Bot, customer identity, wallet, commercial resolver, queues and Foundation ownership contracts remain authoritative.

## Product flow

Validated Telegram webhook → normalized update → Group Bot application → existing identity/claim/workspace/access services → bounded deterministic policy decision → durable intent/outbox → authorized Telegram boundary → immutable action result and audit. The existing Mini App manages the same workspace-scoped installation, settings and history. No live Telegram validation or deployment is included.

Private `/start` is an independent shared identity entry. `my_chat_member` installation uses current installer admin authority, actual bot administrator state and `groups.moderate`. The existing globally unique immutable `group_claims` prevents silent workspace takeover. Reinstallation preserves group ID, settings and trial snapshot. Telegram group-to-supergroup migration adds an immutable chat alias referencing the same group, verifies bot authority in the new chat and preserves trial identity.

## Small compatible persistence extension

Migration `0010_group_moderation.sql` leaves migrations 0001–0009 byte-identical. Existing `group_settings.moderation` stores the typed product policy; a revision adds optimistic concurrency. Existing `security` stores the last verified bot-right snapshot. A new workspace/group-scoped FORCE-RLS `group_moderation_events` table records message digests, decisions, warning/clear events and transport results, with immutable history, scoped operation identity and a monotonic sequence for warning clear ordering. Raw message content is not stored there.

A narrow `telegram_group_route(bigint)` SECURITY DEFINER function projects only workspace/group coordinates from the existing immutable claims for validated server webhook routing. It does not expose settings or grant tenant membership. Its migration-role owner may read claims; PUBLIC execution is revoked, runtime execution explicitly granted, and restore grants repeat this restriction. All group content, customer behavior, policy resolution, mutations and assertions execute through the restricted runtime role and tenant scope. No second ownership or Telegram dedup table exists.

## Authority and decisions

TELMIVO application permissions and Telegram authority are separate. Sensitive configuration/commands revalidate current Telegram membership and bot rights every time; no cached installer authority grants sensitive access. Membership changes invalidate the presentation snapshot. Delivery revalidates actor authority, bot rights, protected target and effective product access immediately before transport. Creator, administrator and bot accounts are protected from moderation; insufficient/unknown authority fails closed.

Configuration defaults disable automatic moderation. Operators explicitly choose thresholds, action and warning escalation. Flood and duplicate counters inspect at most 1001 recent digest events for a bounded 1–3600 second window; thresholds max 1000. Rules max 20 × 1000 Unicode characters; domains max 50; message max 4096 characters/16384 bytes; hidden links max 50. Pure policy evaluation makes no network request and uses no regex/ML/AI. Parsed visible, bare and Telegram `text_link` URLs are compared to normalized domains without fetching URLs. Detection does not imply ban. Default action/escalation is LOG_ONLY.

Warnings are append-only. Clearing records a new event; active count uses sequence after the latest clear. Explicit policy threshold may request further action. A changed intent under a reused operation key conflicts. Existing `telegram_updates` deduplicates official updates; existing outbox/inbox transports intents and replay.

## Telegram delivery semantics

Decision and transport result are separate records: ACTION_REQUESTED, ACTION_ACKNOWLEDGED, ACTION_FAILED, ACTION_OUTCOME_UNKNOWN. Boolean Telegram acknowledgement is required. Unknown external outcomes are recorded and not blindly retried by the consumer. Ambiguous send/commit failure may still duplicate an external action: exactly-once Telegram execution is not claimed. Durable internal operation identity remains idempotent.

The production adapter supports getMe/getChatMember, deleteMessage, restrictChatMember, banChatMember and unbanChatMember. The Group Bot webhook checks the configured secret, bounded JSON, update identity and request correlation. Runtime synthetic mode disables live transport. Native browser authority fixtures require BOTH explicit test environment and synthetic mode plus the opt-in fixture flag; normal production cannot enable them.

## Customer API and Mini App

GET `/groups`, POST `/groups/claims`, GET `/groups/{group}`, PATCH `/groups/{group}/policy`, POST `/groups/{group}/actions`, GET `/groups/{group}/history`, all beneath `/api/v1`. The existing session, tenant authorization, safe origins, machine errors and secure headers apply. List is capped at 100; history has descending timestamp/UUID keyset pages of 50. An arbitrary group or workspace ID never grants authority.

The fa-IR RTL mobile Groups list/detail displays real installation, resolver trial/access, last verified bot-right snapshot, policy/rules, warning/action history and protected action requests. The existing workspace picker persists and re-authorizes selection. Settings remain available for inspection after expiry; protected behavior is denied rather than deleting configuration. Main Bot, Dashboard, Plans, Wallet and Settings remain the R3A product surface.

## Operational limits

No live Telegram credential, registration, deployment, payment or media provider was used. Bot-right presentation is a last verified snapshot, not a promise of future action rights; every sensitive request and delivery rechecks authority. Flood state is presently PostgreSQL digest history rather than Redis counters; production throughput/load/retention tuning is untested. Workspace selection for automatic installation uses the existing default workspace; an authorized customer can explicitly claim through the workspace-scoped API, and existing claims cannot be reassigned. History/list caps, conservative heuristics and external ambiguity are disclosed in validation evidence.
