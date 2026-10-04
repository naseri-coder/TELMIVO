# Access Gate and File Manager (R3D-4)

Two real trusted plugins register through the R3D-1 registry and appear in the
R3D-3 factory. R3D-2 alone owns price, inclusion, license scope, trials and quotas.
`access-gate` requests only telegram.read_member_status; `file-manager` requests
files.read/files.write/telegram.send and declares Access Gate **optional**.
No R3C URL extraction/download pipeline is duplicated or modified.

## Access Gate

User → named per-bot policy → ALL/ANY channel requirements → Telegram membership
check → structured GRANT/DENY. File Manager consumes an authorization contract,
never membership implementation. ALL requires every member; ANY requires at least
one member **and no operational/unknown result**. Disabled/missing policies,
disabled/uninstalled Access Gate, missing entitlement, quota denial and unavailable
checks fail closed. Owner bypass is deliberately deferred; managers also satisfy
membership when downloading protected content.

Requirements use stable numeric Telegram channel IDs (-100…); usernames/URLs are
not identities. Optional join buttons accept only HTTPS t.me public usernames.
Owners must supply a matching link; a link is not evidence of channel identity or
membership and can become stale. Private invitation URLs and username resolution
are deferred. The existing Telegram Client checks the bot's channel administrator
status before inspecting a member. creator/administrator/member and restricted
with is_member are members; left/kicked are not. Unrecognized status and API/network
failure never grant. BOT_NOT_AUTHORIZED/CHANNEL_NOT_ACCESSIBLE/CHECK_UNAVAILABLE
remain separate from NOT_MEMBER. Management UI offers explicit live re-check;
Telegram denial shows satisfied/missing requirements and Check Again.

## File Manager

Download request → enabled file reference → optional policy → Access Gate →
Telegram sendDocument/sendVideo/sendAudio/sendPhoto by file_id. No binary download,
filesystem upload or duplicate binary storage. Title/description/category are a
flat organization model. Incoming authorized owner's private media is registered
**disabled**, then edited/assigned/enabled in the web UI. File IDs may alternatively
be registered explicitly through the same manager API. References are 192-bit
opaque values, usable as `/start file_<reference>`; links never bypass authorization.
Every new protected request rechecks membership, lifecycle and entitlement.

Policy/file records live in three small plugin-owned tables, with existing
workspace/bot foreign keys and FORCE RLS. Core alone accesses them via scoped
services; plugin code receives only SDK Invoker/narrow interfaces, never pgx or
credentials. Read/write/assignment/statistics operations reuse bots.manage and
active plugin admission. Lifecycle, entitlement and content enable flags remain
separate. Disable/uninstall/expiry preserve records and references. Destructive
purge, nested folders, automatic reinstall and full analytics are deferred.

## Runtime, quotas and delivery safety

Trusted transactional handlers compose the unchanged R3D-1 execution checks; pgx
never enters the SDK. Core lock/revision/version/dependency/capability checks remain
server-authoritative. Commercial shared-catalog/account locks precede the bot lock,
preventing policy/revocation/lifecycle races. A protected file with unavailable
Access Gate never falls back to PUBLIC. Existing conservative Core admission also
blocks execution when another ACTIVE plugin loses entitlement; safe lifecycle
management remains available in the factory.

`files.count` consumes retained registration count (NONE reset),
`downloads.monthly` counts authorized delivery attempts (including denied/failed
attempts), and `membership_checks.monthly` counts channel requirement checks.
Generic R3D-2 QuotaTx provides atomic checks/usage. Initial quota definitions are
unlimited; admins can configure limits/add-ons through the existing commercial API.
Undefined quota fails closed. No hard-coded product limits or local metering engine.

Creation retries use fingerprints/operation keys; metadata updates require the
current revision. A unique (bot, file_unique_id) prevents duplicate registration.
Durable SENDING is committed before external delivery. Same request key never
resends or increments counters twice; changed content under that key rejects.
Acknowledged sends become SENT; explicit failure becomes FAILED; unacknowledged
network/crash outcomes remain UNKNOWN/SENDING and never auto-retry. This deliberately
favors avoiding duplicate delivery over automatic recovery. Telegram is not
transactional: a crash after its acknowledgment may leave an unknown outcome.
Manual reconciliation is a later stage. New Check Again updates have new request
IDs and re-evaluate access. Denied/successful/unique requester/last success statistics
are derived from immutable terminal events. Requesters use bot-scoped keyed HMAC;
no requester profile, raw Telegram ID or message content is stored in analytics.
Owner registration confirmations can repeat on update retry; file rows cannot.

## Connection and APIs

Authenticated per-bot customer APIs under `/api/v1/customer-bots/{bot}`:
- GET/POST access-policies; POST access-policies/{policy} (expected revision)
- POST access-policies/{policy}/check (explicit Telegram user ID)
- GET/POST managed-files; POST managed-files/{reference} (expected revision)
- POST connect (explicit customer intent)

Connect reuses existing encrypted credentials and Telegram Client: verifies getMe,
then sets the webhook to the configured MiniAppURL HTTPS origin's customer update
route. It changes that bot to RUNNING only after acknowledgment. The HTTPS origin
must route `/api/v1/customer-bot-updates/{workspace}/{bot}` to TELMIVO API. No endpoint
or secret is deployed/provisioned by this change; owners must understand that
connection replaces the bot's existing webhook. A per-bot HMAC secret authenticates
Telegram updates before tenant access. Only private chat/from identity matches are
accepted; end users never receive management privileges. Arbitrary Telegram update
metadata is ignored after bounded decoding; manager APIs retain strict decoding.
Tokens never appear in responses/audits/logs, and transport errors omit raw URLs.
Requests and raw Telegram messages are not logged.

Commercial plugin seed rows are sale-disabled zero-price **placeholders**, using
the existing USD minor-unit example denomination; no launch pricing, plan inclusion,
trial or grant is invented. Super Admin must explicitly configure commercialization
before customers can install. PER_BOT is initial catalog data, administratively
changeable; current/historical pricing still follows R3D-2. No payment integration.

## Validation and next stage

All tests use disposable restricted PostgreSQL and synthetic Telegram adapters.
TEST_CONTENT_PLUGIN_FIXTURE is rejected outside test+synthetic mode; no real files,
channels, accounts or tokens are used. Unit tests, real PostgreSQL/RBAC/runtime,
factory regressions, mobile/desktop browser journeys and restored data exercise
both plugins. Production behavior always uses the existing Telegram client;
synthetic behavior is never an entitlement bypass.

R3D-5 can extend requirement kinds and handlers or add another independent plugin.
Live production Telegram/channel permission validation, sustained load and manual
unknown-delivery reconciliation remain operational validation beyond this stage.
