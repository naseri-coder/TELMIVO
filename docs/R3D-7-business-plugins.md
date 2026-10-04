# R3D-7 Broadcast, Forms and Commerce

Base: `bf3b3918432b7ad64b74e6b0fb126397d6f9d8f7` (R3D-6).

Three independent first-party SDK manifests enter the existing registry/factory/catalog.
Each requires CRM 1.x; Forms optionally uses Access Gate. Neither Automation nor AI
is a hard dependency. Catalog placeholders are sale-disabled, with separate daily /
monthly price policies and no automatic entitlement. OWNER/ADMIN receive separate
broadcast.manage, forms.manage and commerce.manage permissions; support agents do not.
All API reads/mutations validate membership, actual RBAC, bot scope, installation,
commercial entitlement and dependencies through Core. Customer requests additionally
require the bot runtime to be enabled. No owner-facing endpoint accepts a customer ID.

## Broadcast

Declarative audiences require ACTIVE contacts with scoped canonical Telegram identities.
Filters: at most eight ALL tags, locale and optional first-seen time; no expressions,
SQL, regex or user webhook. Preview is read-only and counts at most 10,000 recipients.
START creates one deterministic UUID-ordered immutable recipient snapshot transactionally;
subsequent contact/tag additions do not change it. Blocked/archived identities are skipped
at delivery. Campaign definitions are editable only in DRAFT. Lifecycle: DRAFT → SCHEDULED
→ RUNNING → COMPLETED/FAILED; PAUSED/resume and CANCELLED are explicit revision-checked,
idempotent operations. Scheduling uses available_at in the existing worker process.
Definitions cap at 100 per bot. Generic catalog quotas govern definitions, per-snapshot
recipient ceilings and monthly accepted send attempts, including failed/retry attempts.

One sequential worker visits bounded workspace/campaign batches. An account lock
serializes policy changes, campaign actions and claims; a durable per-bot rate record
limits reservation to one message per second across campaigns. The SENDING reservation
and quota commit before Telegram. No database transaction spans a send. ACK with a
positive message ID becomes SENT. Explicit 400/403 rejection becomes FAILED; 429 becomes
QUEUED only with proven rejection, bounded retry_after (1–86400 seconds), and at most
three attempts. Ambiguous HTTP/network/provider outcomes and abandoned reservations
become UNKNOWN and are never automatically replayed. Global per-bot backoff applies
even across campaigns. An already reserved send may complete after pause/cancel;
queued recipients are not newly reserved. Restored terminal rows cannot be requeued.
Completion with failed/unknown recipients is FAILED, not fabricated success. Unknown
outcomes require later operator reconciliation; exactly-once external delivery is not
claimed. Campaign lifecycle and recipient reports are paginated.

## Forms

Schemas contain 1–20 unique bounded fields: TEXT, LONG_TEXT, NUMBER, BOOLEAN,
SINGLE_CHOICE, MULTI_CHOICE. NUMBER uses canonical signed integer strings (no floating
point). Text is byte-bounded at 2,000, choice options at 20 × 100 bytes, and submissions
at 8 KiB. Unknown keys, nulls, duplicate selections, out-of-range integers and unknown
choices fail validation. Explicit mappings only write bounded CRM custom fields (500
bytes each) or allowlisted selection tags, validating the final CRM aggregate limits.

The authenticated private Telegram webhook fixes user identity outside plugin payloads.
Command foundation: `/form <form UUID> <JSON answers>`. Public mode means public to a
verified user of this bot, not an anonymous owner API. Protected mode uses the existing
Access Gate's scoped policy, entitlement, membership and quota service, failing closed
on missing/disabled gate or failed membership. Definition revisions are rechecked after
gate evaluation. Form snapshot, answers, CRM resolve/mapping, quota, immutable operation
receipt and minimal form.submitted outbox event commit together. Same-key/same-content
replays without new CRM/submission/quota/events; changed content conflicts. Historical
submission schema versions survive edits. Managers can edit/disable forms and page answers.

## Commerce

Customer Commerce is separate from platform subscriptions/wallets. Products contain
name, bounded description, enabled flag and existing ledger Money (asset + int64 atomic
amount serialized as a string). Orders accept only product IDs/quantities: 1–20 distinct
items, quantity 1–1000. Server reads current scoped enabled products and snapshots their
name, revision, unit price and exact checked line/total amounts in one transaction.
Multiplication/sum overflow, mixed currencies, negative prices and disabled products fail.
No float conversion, price supplied by the customer or historical repricing exists.

Command foundation: `/order <JSON product/quantity array>`. CRM identity, order,
monthly quota, receipt, history and commerce.order.created event commit atomically.
Customer orders begin PLACED; centralized legal transitions support DRAFT → PLACED →
CONFIRMED → FULFILLED, with cancellation before fulfillment. This command foundation
does not expose a customer draft-cart flow. Revision-checked owner/admin status changes
preserve immutable line/price snapshots, ordered history and emit status-change events.
No gateway, checkout payment, refunds, inventory reservation or fulfillment transport
is implemented. The UI shows exact amounts without JS Number rounding.

## Persistence / retention / events

Only additive `0018_business_plugins.sql`; migrations 0001–0017 remain byte-identical.
New tables use FORCE RLS, composite workspace/bot references and retention triggers.
The recipient table is `plugin_broadcast_recipients` to preserve the existing platform
broadcast recipient table. Definitions, submissions, snapshots, operation receipts and
history survive disable/uninstall/entitlement loss. Purge, export and operator uncertain-
outcome reconciliation require a later explicit policy. Prefix search and stable bounded
keyset pagination never accept SQL/wildcards. Monthly quotas use existing commercial
policies and account locks; no hardcoded Free/Pro/Business limits are added.

Transactional minimal business events expose allowlisted campaign/form/order identifiers
and statuses plus scoped CRM identity to existing Automation. Answers, text and prices
are not copied into generic logs/events. The foundation customer commands register data;
a conversational form wizard, cart UI and durable customer acknowledgment messaging are
not included. Persian manager pages cover campaign preview/control/history, form schema /
CRM mappings/submissions, product management and order lifecycle/history.

## Validation / safety

Focused models/contracts, synthetic Telegram response classification, PostgreSQL
migration/transaction/RLS/RBAC/quota/replay/recovery tests and concurrent claims/orders
run in an isolated disposable localhost PostgreSQL instance with a non-bypass-RLS role.
Focused race gate, gofmt/vet/build and frontend lint/typecheck/build/tests are the local
FAST GATE. Existing GitHub migration CI includes R3D-7 tests; the expensive full release
matrix and CI results are reviewed separately. No production database, real customers,
real broadcasts, payments, deploy, migration on production or PR merge was used.
