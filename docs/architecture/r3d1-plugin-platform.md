# R3D-1: Plugin platform foundation

Core domain entry points (`internal/domains/pluginplatform`) → immutable registry and
scoped runtime → infrastructure-free SDK (`internal/platform/plugincontract`) →
future trusted business plugins. No business plugin, public endpoint, UI or loader is added.
Existing `botmodules`, catalog, entitlement resolver, R3A/R3B/R3C transports remain unchanged.

## Manifest and registry

Schema version 1 declares identity/name/description, deployed stable `x.y.z` version,
Core compatibility interval `[minimum, maximum_exclusive)`, required/optional versioned
dependencies, conflicts, requested capabilities, settings, lifecycle validation hooks,
data-schema version and migration identifiers. `DecodeManifest` rejects unknown fields,
duplicate object keys and trailing documents. Registry construction validates required
closure, present optional versions and cycles. One deployed version per ID; duplicate IDs
fail. Immutable copies protect metadata. `Resolve` gives deterministic dependency-first
required closure; it never silently installs/activates dependencies. Optional dependencies
are never installed implicitly. Conflicting alternatives may coexist in the catalog and
as inactive installations, but cannot coexist in an active bot graph.

Settings schema v1 is a bounded flat object with explicit string/boolean/integer properties,
required keys, string length/enum and integer bounds. Unknown settings fail closed. This is
a documented SDK dialect, not general JSON Schema; nested structures need a future dialect.
Executable upgrades/migrations are deferred; installed executable/data versions must match
the registry exactly to activate or invoke. Migration identifiers are metadata only.

## Lifecycle and retained data

| Operation | From | To |
| --- | --- | --- |
| Install | AVAILABLE | INSTALLED |
| Activate | INSTALLED, DISABLED | ACTIVE |
| Disable | INSTALLED, ACTIVE | DISABLED |
| Request uninstall | INSTALLED, DISABLED | UNINSTALL_PENDING |
| Complete retained uninstall | UNINSTALL_PENDING | UNINSTALLED |

AVAILABLE is a registry projection without a database row. Every other state is durable.
Disabling preserves configuration and persistent data, and immediately bars runtime calls.
Uninstall requires disabling active plugins first; pending is a nonexecuting retained
installation, completion is a retained tombstone. No deletion, purge, reinstall or automatic
cleanup is implemented. Future retention/cleanup policy may extend completion explicitly.
Declared hooks are side-effect-free lifecycle validators; a missing validator or veto rolls
back the complete transaction. They receive copied snapshots and no database/Core handle.
External cleanup hooks must later use durable coordinated effects, not these validators.

The service checks the proposed whole active graph before transition. Active dependents must
be disabled before their dependency. Version mismatches and conflicts fail before writes.
Each operation owns an authorized tenant transaction and locks the parent bot row; revision
checks reject stale requests. Runtime calls take that same lock. SQL independently enforces
identity, transition, revision and data retention, with FORCE RLS and a composite bot/workspace
foreign key. Registry graph/capability policy is enforced at the Core service boundary;
direct SQL is trusted Core access, not a plugin API. Same-state revision-fenced Core writes
are reserved for future configuration/data services; no plugin database handle is exposed.

## Bot scope and permissions

`Scope{AccountID, BotID, PluginID}` maps AccountID to the existing workspace billing subject.
It does not introduce a second account system or equate an account with a Telegram user.
Installation identity is `(bot_id, plugin_id)` bound to its workspace. Two bots in the same
workspace have independent state, settings and grants. No account-wide toggle is inferred.
Management uses existing workspace membership and `bots.manage`; platform-admin status does
not bypass ownership. New customer bot creation/credential validation belongs to Bot Factory.

Plugins receive only `sdk.Invoker` from `Runtime.Bind`; actor/scope are privately bound.
Every call checks membership, bot ownership, RUNNING bot status, ACTIVE plugin state, exact versions, the active graph,
manifest declaration, persisted grants and current AdmissionPolicy before a Core handler.
Representative capabilities are `telegram.send`, `telegram.read_member_status`, `files.read`,
`files.write`, `tickets.read`, `tickets.write`, `ai.invoke`, `workflow.execute`. Unknown or
undeclared/excess grants fail closed. Missing policy/handler fails closed. A cached session
cannot bypass disable or policy revocation. Trusted handlers must independently scope resource
IDs and reject payload scope overrides. Payloads are bounded JSON, not executable code.

This is an enforceable SDK/service contract for trusted compiled modules, **not an OS sandbox
for hostile Go code**. No dynamic third-party loading exists. Third-party execution isolation
must be designed before Marketplace executes untrusted packages. No real Telegram/file/AI
handler is installed. Current handlers must be side-effect-free; future external effects
need existing outbox/lease/unknown-delivery contracts. Exactly-once network effects are not
claimed. Synchronous calls hold the bot lock; long work needs future fenced job coordination.

## Billing and quotas boundary

`AdmissionPolicy.Authorize` is mandatory for install, activate and every invocation. It is
read-only and returns allowed capabilities, PER_BOT/PER_ACCOUNT license scope and an external
policy revision. Disable/uninstall remain possible after commercial denial. For now current
admission is checked conservatively for all active plugins on the bot; revocation may require
disabling the affected module before another invocation/activation can proceed.

Manifest extensions are references for pricing, plans, trials, quotas, billing-scope policy,
bundles and marketplace/developer identities. Core interprets none of their business rules.
R3D-2 can implement policy adapters to the existing resolver and immutable policy revisions:
Free/Pro/Business inclusion, daily/monthly prices, grace, quota, add-ons, bundles and grandfathered
pricing remain outside the plugin implementation. Actual quota reservation/settlement and
payment processing are deferred and must be composed through existing transaction/outbox
services. An extension reference never grants execution by itself. No default allow policy
or hard-coded plan/price constants are shipped.

## Validation and adoption

Migration `0012_plugin_platform.sql` adds one opt-in installation table and guard; 0001–0011
remain byte-identical. Existing grants script covers the new table while FORCE RLS remains.
Native tests cover lifecycle, retained nonempty data, independent bots, dependency order,
permission/policy denial, hook veto, conflict races, version rejection, cross-tenant RLS/FK,
SQL retention guards and fresh restore/restricted completion. SDK/registry tests cover
manifest validation, deterministic resolution, duplicate/missing/cyclic graphs, optional
dependencies, conflicts, compatibility, settings, defensive copies and transition semantics.
Existing CI adds the new branch/base and selectors without removing earlier tests/assertions.
No production migration, deployment, service restart or live external effect is authorized.
