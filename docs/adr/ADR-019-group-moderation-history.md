# ADR-019: Immutable group moderation intent and result history

Status: Accepted for R3B.

The existing group settings JSON can represent bounded moderation configuration, but cannot safely represent concurrent warnings, clear history, distinct policy decisions and uncertain external results. R3B therefore adds one FORCE-RLS workspace/group event history and an optimistic revision to existing settings in migration 0010. Warning clear is an appended event ordered by a database sequence; history is not erased.

Identity, globally unique immutable group claims, product trial snapshots, existing Telegram update dedup, commercial resolver, outbox/inbox and audit stay authoritative. A narrow privileged routing projection returns only existing claim coordinates for server dispatch; runtime tenant authorization and RLS remain mandatory for all subsequent behavior.

Each external moderation request originates in a durable scoped intent. Current actor/bot/target/access checks occur again before transport. Acknowledged, failed and unknown delivery outcomes are separate immutable facts; transport results never rewrite the original decision. External exactly-once execution is explicitly not promised, and unknown results are not automatically retried as success or failure.

This avoids overwriting warning history, treating installation authority as permanent, adding a second group ownership model or introducing a parallel queue. Production load tuning, live Telegram validation and advanced group engagement remain deferred.
