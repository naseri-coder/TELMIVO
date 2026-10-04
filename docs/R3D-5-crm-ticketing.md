# CRM and Ticketing (R3D-5)

Two first-party SDK plugins: `crm` requests crm.read/crm.write; `ticketing` requests
tickets.read/tickets.write/telegram.send and **requires CRM 1.x**. Both are registered
in the existing factory/catalog; licensing, prices, trials, grants and quotas belong
to R3D-2. Initial PER_BOT/sale-disabled USD zero-price placeholders grant no access.
The previous TELMIVO platform-support tickets are unchanged: they concern authenticated
workspace customers contacting TELMIVO. These bot-owned customer tickets use separate
plugin tables because their customer principal is a CRM contact, not a workspace user.

## Contact identity and authorization

CRM contact UUID ↔ provider/external-key identity, unique per workspace/bot. Telegram
IDs are positive canonical integer strings; no identity or ticket spans bots. New
providers can use the same identity table/contract. The CRM plugin has no ticket
knowledge. Fields: display name, username, locale, ACTIVE/BLOCKED/ARCHIVED, first/last
seen, unique data-driven tags (20), internal contact notes (20 × 1000 bytes) and custom
fields (20 keys, 500-byte values, 8 KiB aggregate). No binary files or customer profiles
are collected beyond submitted support identity/content. Disabled/expired plugins
retain all records. No purge or identity merge is offered.

Managers/agents reuse workspace membership and role_permissions: crm.read/manage,
tickets.read/reply/assign/close plus tickets.manage/notes. SUPPORT_AGENT gets support
permissions without bots.manage; SALES_AGENT has read/reply, not internal ticket notes.
Core broker independently validates the calling agent, then uses the scoped workspace
owner as execution/commerce principal. This does not grant lifecycle privileges to
agents. Assignment requires tickets.reply on a member of that same workspace; composite
foreign keys and FORCE RLS enforce scope. CRM details are reduced for readers without
crm.read; internal messages are excluded unless tickets.notes is granted. End users
never enter agent APIs and never acquire workspace membership.

## Customer flow and tickets

Authenticated per-bot Telegram webhook → trusted SDK Ticketing handler → narrow CRM
contact resolver → enabled department → one active ticket per contact/department/bot
→ immutable CUSTOMER message. The default department is the enabled row with smallest
ordering/UUID; owners configure it first (no invented department). Ordinary private
text or `/support <message>` enters support. Private attachments from an end user enter
support as Telegram references. Existing owner File Manager capture and file_<reference>
links retain their R3D-4 behavior. Private sender/chat identity must match. Incoming
updates carry immutable operation fingerprints; same update replays without quota
consumption/contact creation/message duplication, changed payload conflicts.

Central lifecycle: OPEN, PENDING_CUSTOMER, PENDING_AGENT, RESOLVED, CLOSED. Reopening
resolved/closed requires the explicit reopen action; it is not an ordinary transition.
Priority LOW/NORMAL/HIGH/URGENT is independent. Queue is derived: UNASSIGNED, ASSIGNED,
or WAITING for PENDING_CUSTOMER. Department, assignment, priority, tags and status
updates require revision and audit actor/time/previous state. Closed tickets remain.
Flat department definitions and canned replies are enabled/configurable, revisioned,
idempotent and bounded to 100/bot. Agent UI supports list filters, detail, CRM history,
assignment, priority/status, private notes and replies. CRM/ticket lists use stable
immutable identifier cursors, max 100/page. Search is prefix-based with SP-GiST indexes;
input max 100 bytes and no client-controlled wildcard scans. Conversation is paginated
chronologically (100/page). CRM profile history is a bounded first page; full inbox can
page/filter by contact via API.

## Messages and delivery

CUSTOMER/AGENT/SYSTEM/INTERNAL are separate persisted kinds. An INTERNAL note is always
NOT_APPLICABLE delivery, never reaches a Telegram sender and is visible only to permitted
agents. Message content is retained in plugin storage, not generic audit/log payloads.
Attachments are at most 4 validated Telegram document/video/audio/photo references,
using File Manager validation primitives, without creating a second binary store.
Canned reply selection resolves enabled body server-side. No AI or automation.

Agent reply first commits quota reservation + immutable SENDING message + ticket revision,
then rechecks runtime/RBAC/entitlement and sends through the existing Telegram Client.
SENT is recorded only on acknowledgment; explicit fake/provider failure is FAILED;
network/crash/partial attachment outcomes remain UNKNOWN/SENDING. Same operation key
never re-sends, even after a lost ACK/DB commit failure. Manual reconciliation is deferred.
This avoids duplicate delivery but is not an exactly-once external guarantee. Internal
notes bypass that entire send phase. Concurrent close/reply serialize; accepted replies
may finish after a subsequently serialized close, without reopening the ticket.

## SLA and quotas

Clock is injected; all timestamps are UTC. Department response/resolution targets are
snapshotted as due timestamps on creation (0 = no target). First acknowledged agent
response sets first_response_at; resolve/close timestamps retain history. Breaches are
deterministically derived on read, including late responses. Reopen clears current
closed/resolved timestamps but retains first response/original deadlines; audit keeps
history. Department changes do not silently rewrite existing SLA targets. No escalations.

Generic R3D-2 quota evaluation is reused: crm.contacts.count counts retained contacts;
ticketing.active_tickets.count counts current active tickets; ticketing.agents.count
counts distinct assigned agents; ticketing.messages.monthly consumes accepted messages,
including internal notes and failed outgoing attempts. Occupancy uses current persisted
state, not a cumulative creation meter, so closing frees active-ticket capacity. Catalog
scope controls bot/account occupancy. Defaults unlimited; no plan-specific constants.
Shared catalog/account lock precedes bot lock, serializing quota checks and policy changes.
Trusted runtime admission reuses the same transaction when the commercial policy supports
AuthorizeTx; concurrent dashboard reads cannot exhaust the pool through nested admission.
Inbound principal lookup releases its connection before entering the runtime. A one-connection
PostgreSQL regression proves both inbound processing and concurrent dashboard reads.
Quota denial rolls back contact/ticket/message creation together; agent reply denial
leaves no SENDING row or external delivery. Plugin dependency/entitlement failures block
execution while safe lifecycle disable/uninstall retains content.

## Persistence and validation

Only additive 0016: six scoped plugin tables (contacts, external identities, departments,
tickets, messages, templates); tags/notes/fields are bounded JSON rather than one table
per noun. Tickets link CRM contact, department and authorized workspace agents. Retention
triggers reject deletes, scope/identity changes and stale revisions; messages and terminal
delivery outcomes are immutable. Existing 0001–0015 checksums remain unchanged.

Customer/private Telegram adapters, isolated PostgreSQL/RLS/RBAC/quotas, restore/recovery
and mobile/desktop browser journeys validate the contract. Test adapters are injected
only under the existing test+synthetic guard; production never auto-grants entitlement.
HTTPS customer webhook routing still requires explicit operator connection from R3D-4;
no production deployment, real data, tokens, channels or conversations are used here.
R3D-6 can consume structured statuses, audit and deterministic SLA deadlines for future
automation. Non-Telegram delivery, CRM merging, cross-bot identity consolidation, routing,
AI, payment checkout, full-text multilingual search and destructive retention are deferred.
