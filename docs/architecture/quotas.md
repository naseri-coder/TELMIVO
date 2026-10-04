# Limit semantics

Feature metadata declares `limit_kind`, `limit_unit` and `reset_window`. Plan, grant and override values use that same dimension. No plan-name branching exists.

| Kind | Measurement | R1 behavior |
| --- | --- | --- |
| USAGE | Operations in declared period | Monthly counters for tickets/downloads; reservations serialize per tenant/feature |
| RESOURCE_COUNT | Current persisted resources | Channels/groups/bots/team count actual tenant rows; preserve existing excess and deny new over-limit creation |
| CAPACITY | Current storage bytes | Model reserved; resolver explicitly returns LIMIT_UNIMPLEMENTED |
| CREDIT | Consumed internal AI credits | Counter foundation only; AI product disabled |
| ITEM_SIZE | Bytes of one item | Model reserved; resolver explicitly returns LIMIT_UNIMPLEMENTED |
| NONE | No quota | Wallet access |

Monthly usage is never substituted for storage or resource count. Daily windows and commercial capacity/credit consumption are deferred; the current configuration APIs cannot select those policies. New resource writers must use Reserve inside the same transaction as the insert; resource-count Reserve takes the same feature lock but does not increment monthly counters. A downgrade changes access, not resources; reads remain possible above creation limits. Disabling a feature globally still denies it regardless of grants.
