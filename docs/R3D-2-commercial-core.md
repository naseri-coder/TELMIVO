# R3D-2 commercial core

Account (existing workspace) → assigned plan → central entitlement evaluator →
plugin admission → independent bot installation/activation.

**Pricing ≠ Plan ≠ Entitlement ≠ Subscription ≠ Activation ≠ Usage.**
`commerce.Service` implements R3D-1 `plugincontract.AdmissionPolicy`. Compose a
`pluginplatform.Service{DB: db, Registry: registry, Policy: commercialService}`
and its existing Runtime. This opt-in foundation does not convert legacy bot
modules or introduce business plugins. Plugin manifests contain references only;
Plugin Core has no prices, payment providers or commercial state machine.

## Catalog and administrative control

Migration 0013 adds four tables: immutable typed `commercial_catalog` versions,
`commercial_agreements`, append-only trial consumption, and tenant quota usage.
Existing `plans` and `subscriptions` remain plan identities/assignments. FREE,
PRO and BUSINESS have initial empty commercial policies; no plugin prices or
inclusions are seeded. Additional plans work identically. A plan definition owns
name, ordering, metadata, plugin UNAVAILABLE / PAID_ADDON / INCLUDED policies and
quota defaults. Missing plugin policy means unavailable for new purchases/trials;
an explicit historical agreement or admin grant is independently honored.

Catalog definitions have exactly one typed payload: PLAN, PLUGIN, QUOTA, BUNDLE
or ADDON. Plugin definitions own scope, permitted capabilities, sale switch,
daily/monthly prices, trial policy and grace duration. Products own membership,
quota increments, scope and prices. Quota definitions own reset and default limit.
Bundles/add-ons must have the same scope as every member plugin/quota; mixed-scope
packs are intentionally rejected. Member references are checked on save.

Authenticated routes use the existing session, origin protection, tenant and
platform permission checks, without a parallel RBAC system:

| Route under `/api/v1/commercial` | Capability |
| --- | --- |
| GET `/catalog` | Current effective typed catalog; `platform.commercial.read` |
| POST `/catalog` | Append version; expected_version + reason; `platform.commercial.manage` |
| GET `/agreements` | Tenant commercial history and computed state; commercial.read |
| POST `/agreements` | Privileged fulfillment/admin grant; commercial.manage |
| PATCH `/agreements/{id}` | Suspend, restore, cancel or expire with expected_revision + reason; commercial.manage |
| POST `/decision` | Member's structured plugin access decision (`bot`, `plugin`) |
| POST `/trials` | Member with bots.manage; catalog-defined trial (`key`, `plugin`, `bot`) |
| POST `/quota` | Read-only member quota evaluation; `amount` is an integer string |

Plan catalog writes also require `platform.plans.manage`, update the established
plan catalog and retain the existing FREE safeguard. Plan assignment continues
through the existing subscription API. Plan policy edits are immediate; other
catalog versions may be scheduled. Versions have monotonically nondecreasing
effective timestamps and optimistic concurrency. Disabled latest versions never
fall back to older enabled versions. Catalog mutations, issuance and state changes
write append-only existing audit logs in the same transaction. No new admin UI is
needed to validate this backend contract.

Example plugin catalog mutation (first version):

```json
{
  "expected_version": 0,
  "reason": "Initial commercial policy",
  "catalog": {
    "kind": "PLUGIN", "key": "example", "enabled": true,
    "definition": {
      "plugin": {
        "capabilities": ["telegram.send"], "scope": "PER_BOT",
        "sale_enabled": true,
        "prices": {
          "daily": {"unit": "USD", "scale": 2, "amount": "100"},
          "monthly": {"unit": "USD", "scale": 2, "amount": "2000"}
        },
        "trial": {"enabled": true, "duration_seconds": 86400},
        "grace_seconds": 3600
      }
    }
  }
}
```

## Prices and agreements

Money uses int64 atomic units, a denomination identifier and explicit decimal
scale. JSON amounts are strings to preserve precision in JavaScript. Negative,
fractional, overflowing, unknown-field and mismatched-denomination prices fail.
Zero prices are supported. Free pricing still requires plan inclusion, a trial or
an explicit agreement; zero price is not an implicit permission bypass.

DAILY means 24 UTC hours; MONTHLY means the same UTC time next calendar month,
clamped to the last day when needed. Each agreement snapshots the entire catalog
version, exact price/unit/scale, billing period, effects and grace duration. SQL
forbids changing historical terms and deleting agreements/catalog/trial history.
Catalog changes apply to **new agreements only**. There is no automatic renewal:
a future provider issues a new agreement with an explicit operation key and the
then-effective catalog. Grandfathered terms remain traceable and valid for the
original term. A future grandfathered renewal must be an explicit separate
fulfillment policy; never update a historical agreement silently.

`Issue` is the privileged future payment fulfillment boundary. It derives prices,
duration, scopes and effects from the catalog, never from client-supplied prices.
Today only authorized commercial administration may invoke it; it does not assert
that payment occurred. Unique operation key + request fingerprint makes exact
replays stable across catalog changes and conflicting replays fail. No charging,
checkout, provider webhook, transaction ledger or revenue system is implemented.

## Entitlements, scopes and lifecycle

One evaluator resolves INCLUDED plan access, standalone subscription, trial,
bundle, add-on and administrative grant effects. Results include structured
ENTITLED / NO_ENTITLEMENT / PLAN_UNAVAILABLE / PLUGIN_UNAVAILABLE /
ENTITLEMENT_EXPIRED reasons, scope, sources and policy revision. Independent
sources are additive; revoking a bundle removes only that agreement's access.
Sale disabled stops future paid issuance and preserves current terms. Disabled
plugin catalog prevents all new admission. Administrative grants bypass plan/sale
restrictions and may optionally expire; revocation is a retained CANCELLED row.

PER_ACCOUNT agreements cover any bot in that workspace. PER_BOT agreements match
one owned bot exactly. Bot ownership and tenant membership are checked before
lookup; one bot purchase never grants another bot access. Scope changes affect
new agreements; historical agreement scope is preserved.

Commercial admission checks current entitlement and catalog-authorized manifest
capabilities at installation, activation and every runtime invocation. The
existing registry still enforces dependency/conflict/version safety. The
conservative R3D-1 runtime checks all active installations: an expired dependency
must be disabled with its dependents before healthy execution resumes.

Entitlement expiry does **not** update/delete plugin_installations, configuration
or data. An ACTIVE retained record is blocked by runtime admission. It may be
explicitly disabled and later reactivated after restored access. Commercial
ACTIVE / GRACE / SUSPENDED / CANCELLED / EXPIRED remain separate states. There is
no scheduler falsely equating commercial expiry with uninstall.

## Time, trials and grace

Clock is injected; every evaluation uses an explicit UTC instant. Expiry and grace
ends are exclusive. Trials use configured duration and no grace. Append-only
consumption is unique by workspace/plugin/license scope/subject, regardless of
request key or catalog version. Only an audited admin `repeat_trial` request may
repeat a consumed trial. Customer routes accept neither overrides nor durations.

ACTIVE agreements become computed GRACE at end and EXPIRED at grace end. The
minimal policy is **full existing access during a bounded grace interval**.
Cancellation/suspension revoke immediately; restoring suspended state cannot
extend original dates. Cancelled/expired persisted records cannot be resurrected;
restoration creates a new agreement. No payment retry daemon is implemented.

## Quotas and products

Generic quota limits support zero, integer finite values and explicit unlimited.
Plan limits replace quota defaults; active/grace add-on and bundle increments add
to that limit. Negative usage and arithmetic overflow fail closed. DAILY/MONTHLY
buckets are UTC calendar boundaries; NONE is lifetime. PER_BOT meters are isolated
by bot, PER_ACCOUNT meters share one account counter. `Check` is read-only;
trusted Core `Consume` serializes check + increment inside a tenant transaction.
No public self-reported metering endpoint or fake business-plugin integration is
provided. A future handler must consume/reserve its real metric through this
service; plugin activation itself does not spend quota.

Products snapshot member/effect lists at issuance; later bundle membership edits
affect future agreements only. Quota usage history is retained by bucket. Generic
metering is not a reservation/idempotent settlement system yet; operation-specific
metering belongs to the later plugin/provider integration. Existing R3A/R3B/R3C
quota/reservation flows remain unchanged.

## Limits and next stage

No checkout/UI, business plugin, deployment or production DB change. Administrative
catalog management is backend-first. Catalog snapshots are immutable database
history; agreement history embeds price versions. Catalog reads currently return
only effective versions. Admission uses the existing read-only R3D-1 boundary;
it is evaluated per operation, not a distributed revocation barrier for already
in-flight effects. A real external-effect handler still needs the existing outbox
pattern. Mixed-scope bundles, advanced restricted/read-only grace, automatic
renewal, grandfathered renewal orchestration and provider idempotent quota
settlement are explicit future extensions, not implicit commercial promises.
