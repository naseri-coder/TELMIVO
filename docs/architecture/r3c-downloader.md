> Validated source: `0d6d601c99e464c53bd91495738dd15bbbd6f1d1`. Final source CI and independent restore/recovery PASS; live provider/Telegram networks remain untested. See r3c-validation.md.

# R3C Downloader

R3C extends the frozen R3B checkpoint `14b27792936c6d004e7e127f3d5de8f1be91ae79`. Downloader Bot is an independent official entry. Its `/start` uses the existing identity/workspace/global-trial path; Main Bot is not a prerequisite. Bot updates use the existing Telegram inbox and durable notification outbox. The existing Mini App adds Persian, RTL Downloader overview, format selection, details, cancellation and keyset history.

## Durable execution

Handlers detect a supported canonical public URL, authorize the existing access resolver, reserve quota through `jobs.Create`, and create a download row bound to an existing media job. Analysis is durable worker work, not handler network extraction. Reserving before analysis bounds expensive work and means pending analysis consumes allowance. The stable operation key is shared across job, reservation and existing lease allocation. Unknown commercial BYTES policies fail closed; count-based policy remains dynamic data.

The worker uses the existing lease owner/generation, renews during work, and fences every result. Lifecycle: QUEUED → ANALYZING → AWAITING_FORMAT → DOWNLOADING → optional PROCESSING → UPLOADING → COMPLETED. FAILED and CANCELLED are terminal. Selection must belong to the owned analyzed snapshot, be fresh, and occur in AWAITING_FORMAT. User callbacks contain only job and format identifiers.

## Minimal migration 0011

Existing tables had no constrained download-to-leased-media relationship, independent durable cancellation intent while work is active, analyzed format expiry, or acknowledged/ambiguous Telegram result. Format JSON is not a suitable relational execution or cancellation invariant; the notification outbox does not acknowledge media uploads.

`0011_downloader_lifecycle.sql` adds six columns to `download_jobs`: nullable legacy-compatible `media_job_id`, safe analyzed `title` (default empty), `format_expires_at`, monotonic `cancel_requested` (default false), constrained `delivery_outcome` (default DELIVERY_PENDING), and nullable `delivery_message_id`. A composite FK to `(media_jobs.id, workspace_id)` prevents foreign workspace binding; media_job_id is unique. A guard trigger enforces initial state, immutable ownership/binding, state transitions, snapshot selection, terminal immutability and existing lease fencing. Acknowledgment requires a positive Telegram message ID and COMPLETED; UNKNOWN cannot equal COMPLETED. Existing rows without media_job_id retain their prior contract. The downloads feature flag is enabled. No new table, role, privilege grant, or retry scheduler is introduced; 0001–0010 remain byte-identical.

Rejected duplicate fields: `attempts` and `available_at` reuse existing scheduling/retry contracts; `completed_at` reuses immutable terminal updated_at; `actual_bytes` belongs to download_assets; `upload_started_at` reuses UPLOADING; unused duration storage was removed. Cancellation intent cannot use the terminal CANCELLED state while an active fenced worker must still release resources, so the single boolean is retained.

The draft privileged dispatch enumerator was removed entirely. There is no new SECURITY DEFINER function. `Product.Dispatch` validates existing authenticated membership through `DB.Tenant`, then uses tenant RLS plus creator scoping, deterministic order and a bounded result. Internal workers derive trusted user/workspace coordinates from the existing transactional `downloader.queued` outbox event before invoking that same authorization. Arbitrary caller workspace IDs do not confer membership. This follows the inherited backend/GUC trust model, not protection against theft of database credentials.

`jobs.AcquireExpiredSettlement` is an R3C-only FOUNDATION_COMPATIBLE_EXTENSION to the existing lease contract: it permits fenced failure settlement of expired quota, not new quota or successful work. Frozen Foundation history is untouched.

## Provider and network boundary

Production paths use the pinned `yt-dlp 2026.8.19` (upstream release spelling 2026.08.19) behind normalized TELMIVO models for YouTube, Instagram and Facebook public routes. Only HTTPS allowlisted provider routes enter extraction. There is no generic URL fallback, customer cookie/session/password import, DRM/private/paywall/auth/CAPTCHA/geo bypass. Protected content returns ACCESS_RESTRICTED. Live availability is UNTESTED_LIVE_PROVIDER_NETWORK.

The extractor is installed with exact version and wheel hash, no dependencies or runtime update. Its wheel LICENSE is Unlicense. Execution uses structured argv, bounded context, constrained environment and no customer flags. Raw provider diagnostics are suppressed in favor of stable categories. The Python transport validates each destination/redirect and public DNS addresses, and pins the actual TLS dial address. Go CDN fetch applies the same host/public-address boundary with bounded redirects, no proxy and streaming byte counting. Neither trusts Content-Length or a prior DNS resolution alone.

Only actual analyzed formats are offered, bounded to eight in production. Compatible progressive MP4/M4A/MP3 passes through. Actual ADTS AAC may use the single deterministic AAC→M4A stream-copy remux plan. DASH merging, HLS and arbitrary transcoding are not offered. ffprobe validates real container/streams/codecs and actual bytes before delivery. FFmpeg uses file-only structured args, timeout/cancel, bounded output and no shell; output is probed again. No user title enters an internal path or process argument.

## Resources, delivery and recovery

Configuration bounds media bytes (default/hard ceiling 49 MiB), worker concurrency (default 2, range 1–32), job context, provider requests, download, ffprobe, FFmpeg and Telegram upload. Internal temporary directories use job UUID/generation with mode 0700; files use 0600. Existing private scoped storage and durable download_assets are reused. Success/failure/cancel removes owned partials. Startup/periodic cleanup checks database row lock and lease generation before deleting orphan generations; a valid current generation is retained. Unknown paths are retained for operator review; each pass considers at most 1024 job directories.

Telegram chooses video/audio/document from validated metadata and uses the requesting customer's trusted server-side Telegram account. UPLOADING is committed before external send, after which customer cancellation is denied: the operation may already have reached Telegram. Known temporary rejections retry at most three times with bounded delay. A network ambiguity is durable DELIVERY_OUTCOME_UNKNOWN/FAILED and is not blindly resent. Reclaiming UPLOADING follows the same conservative rule. No external exactly-once claim is made.

PostgreSQL is authoritative for relationship, cancellation, analyzed snapshot, quota and delivery outcome. Recovery validates restored completed/unknown jobs are not replayed, abandoned leases are reclaimed, stale generations cannot settle, cancellation survives, and workspace isolation remains. Inherited lease horizon is 24 hours; beyond-horizon recovery needs operator handling. Redis is not the business-state authority.

## Acceptance boundary

Native tests use restricted PostgreSQL runtime roles and deterministic provider/Telegram adapters. The authoritative browser connects the real Mini App to real API/PostgreSQL without API interception. Only external dependencies are injected; the explicit synthetic worker is limited to the browser fixture customer so abandoned restore fixtures remain unchanged until the recovery drill. Live Telegram/provider credentials and production operations are excluded. Exact test/CI evidence and limitations are in `docs/operations/r3c-evidence.json` and `r3c-validation.md`.
