# R3C validation

Validated source: `0d6d601c99e464c53bd91495738dd15bbbd6f1d1`. Final delivery is the evidence-only child commit; exact SHA is recorded in the Draft PR and delivery response. Source CI is authoritative; docs-only push does not repeat it.

## Recovery and schema

The original local R3C implementation was recovered at 14b27792936c6d004e7e127f3d5de8f1be91ae79 (16 modified, 21 untracked paths, zero staged/local commits). Generated Python cache and node_modules symlink are excluded. The source was resumed. Attempts/available_at stayed removed; the unsafe draft SECURITY DEFINER dispatcher was removed entirely. 0011 extends existing download_jobs with six fields and ownership/fencing constraints, no tables or grants. See architecture and machine evidence for exact rationale and SHA256. 0001–0010 are byte-identical.

LOCAL_POSTGRES_ENVIRONMENT_UNAVAILABLE: local root/non-root process limitations were respected. No host users/services/security were altered. Fresh migration and runtime behavior used existing isolated GitHub Actions PostgreSQL 17 services with dedicated migration role and NOSUPERUSER/NOBYPASSRLS runtime. TestR3CSchema proves authorized dispatch, denied foreign enumeration/lease/binding, durable cancellation and expiry. Restore validates durable relationship/reservation/cancel/outcomes and stale fencing.

## Final source CI

- [Security Baseline 37150904191](https://github.com/naseri-coder/TELMIVO/actions/runs/37150904191): SUCCESS; actual logs reviewed.
- [CI Web 37150904207](https://github.com/naseri-coder/TELMIVO/actions/runs/37150904207): SUCCESS; actual logs reviewed.
- [Build Images 37150904309](https://github.com/naseri-coder/TELMIVO/actions/runs/37150904309): SUCCESS; actual logs reviewed.
- [CI Go 37150904505](https://github.com/naseri-coder/TELMIVO/actions/runs/37150904505): SUCCESS; actual logs reviewed.

Test counts (verbose native counts are actual emitted PASS lines; non-verbose Go count is static enumeration):

```json
{
  "go_unit_contract_top_level_static": 33,
  "go_count_method": "Static Test function enumeration in successfully executed non-verbose Go race packages, not a verbose individual count",
  "native_before_restore_top_level": 40,
  "native_before_restore_subtests_including_nested": 74,
  "r3c_native_top_level_before_restore": 14,
  "r3c_native_subtests": 9,
  "native_counts_include_fixture_preparation": true,
  "restored_recovery_top_level": 2,
  "r3c_restored_recovery_top_level": 1,
  "python": 4,
  "frontend_unit": 6,
  "lightweight_browser": 8,
  "native_browser": 4,
  "r3c_native_browser": 1,
  "native_browser_selection": "Four separate executions, exactly one test each"
}
```

## Gates

| Gate | Verdict | Evidence |
|---|---|---|
| SOURCE_CONTINUITY_GATE | PASS | Recovered local delta at exact R3B parent, six source/test commits; exact source checkout SHA in all five reviewed job logs. |
| R3B_FREEZE_GATE | PASS | Remote R3B SHA unchanged; 0001–0010 hashes byte-identical; R3B native and browser regressions pass. |
| BRANCH_ISOLATION_GATE | PASS | Frozen R3A/Foundation/main/develop refs verified; only R3C advanced; PR1–3 open/draft/unmerged. |
| LOCAL_RECOVERY_GATE | PASS | Preserved original 16 modified + 21 untracked-path checkpoint, tar/patch/inventory; implementation resumed, not reconstructed. Generated cache/node_modules excluded. |
| SCHEMA_MINIMALITY_GATE | PASS | Six additive lifecycle columns, composite FK, uniqueness, ack check and invoker guard; no duplicate attempts/available_at, no new privilege grants/tables; TestR3CSchema behavioral assertions pass. |
| MIGRATION_CHAIN_GATE | PASS | Fresh 0001→0011 via dedicated migration role in disposable postgres:17-bookworm; restore/reapply succeeds, frozen hashes unchanged. |
| RLS_GATE | PASS | TestR3CSchema restricted NOSUPERUSER/NOBYPASSRLS runtime, hidden foreign rows, denied foreign lease/binding; recovered tenant isolation pass. |
| DISPATCH_ISOLATION_GATE | PASS | TestR3CSchema own dispatch succeeds, foreign coordinates absent, spoofed workspace denied and foreign leasing fails; no SECURITY DEFINER in 0011. |
| DOWNLOADER_ENTRY_GATE | PASS | TestR3CIdentityBot and TestR3CBotCallbacks direct Downloader start/help/status/history/link/select/cancel; shared durable inbox/outbox. |
| SHARED_IDENTITY_GATE | PASS | TestR3CIdentityBot Downloader-first, Mini App identity match, Main-first shared identity, repeated start and one global trial; existing R3A entry tests pass. |
| PROVIDER_DETECTION_GATE | PASS | TestProviderBoundary supported YouTube/Instagram/Facebook public routes; malformed/unsupported/IP/login routes denied. |
| PROVIDER_BOUNDARY_GATE | PASS | TestProviderBoundary and metadata DRM denial; native ACCESS_RESTRICTED fixture fails without delivery; pinned production adapters with no generic fallback. |
| SSRF_SAFETY_GATE | PASS | Go TestSSRFGuard; four Python GuardTests check private DNS, pre-DNS host rejection, rebinding and private redirect denial using injected transport. |
| ANALYSIS_GATE | PASS | TestR3CJourney and Bot duplicate update: one durable job, normalized real fixture formats and AWAITING_FORMAT; native provider temporary failure bounded. |
| FORMAT_SELECTION_GATE | PASS | TestR3CSchema expiry; TestR3CJourney and BotCallbacks valid owned selection, tampered/foreign selection denied; current-state/snapshot guards. |
| DOWNLOAD_STATE_MACHINE_GATE | PASS | TestR3CJourney, failures, schema, processing and restored terminal replay; invalid/stale state mutation denied by database guard. |
| JOB_LEASE_GATE | PASS | TestR3CLeaseRenewReclaimFence and concurrency: existing acquire/renew/reclaim, only authoritative leased worker completes. |
| LEASE_FENCING_GATE | PASS | TestR3CStaleFencing, LeaseRenewReclaimFence and restored stale Finish denied; no stale terminal/quota settlement. |
| CANCELLATION_GATE | PASS | TestR3CCancelCompletionRace cancel wins before send; owned cancel, foreign denial, reconnect and restored durable intent prevent completion. |
| ACCESS_GATE | PASS | Existing resolver in actual Submit/Select; R3CIdentity/Concurrency plus existing access/trial contracts under restricted runtime; browser backend access shown. |
| DYNAMIC_QUOTA_GATE | PASS | TestR3CConcurrency uses stored policy 5→3 with real Downloader admission; admits two additional concurrent operations after one pending, without source change. |
| QUOTA_RESERVATION_GATE | PASS | TestR3CJourney/Concurrency pending counts, stable key retry, concurrent ceiling, one settlement; restored reservation snapshot pass. |
| QUOTA_JOB_COORDINATION_GATE | PASS | Stable existing operation key/media job/lease allocation; stale fencing denied and TestR3CFailurePolicy verifies RELEASED vs COMMITTED policy outcomes. |
| DOWNLOAD_BYTE_LIMIT_GATE | PASS | TestActualByteCap counts real bytes independent of headers; native oversize stream fails, no upload, partials removed; RemuxSafety output cap denial. |
| TEMP_STORAGE_GATE | PASS | Native ProcessingCleanup active generation retained, expired/older generation removed; terminal cleanup; trusted UUID/generation paths and scoped private storage. |
| MEDIA_VALIDATION_GATE | PASS | TestMediaValidation valid fixture and malformed rejection; native INVALID_MEDIA no delivery; real ffprobe for direct and remux output. |
| FFMPEG_GATE | PASS | TestRemuxSafety actual AAC→M4A stream copy, output probe, canceled context/error/output ownership/cap cleanup; native ProcessingCleanup real PROCESSING then acknowledged completion. Direct journey skips processing. |
| CLEANUP_GATE | PASS | Native terminal success/failure/cancel cleanup and explicit generation-aware crash/expiry cleanup; no valid generation deleted. |
| TELEGRAM_DELIVERY_GATE | PASS | TestMediaTransportOutcomes fake actual multipart adapter and trusted chat; native acknowledged success, permanent rejection; production media methods and bounded retries. |
| DELIVERY_AMBIGUITY_GATE | PASS | Native unknown fixture and restored unknown terminal; fake transport malformed/network outcomes; no false COMPLETED or blind replay. |
| UPDATE_IDEMPOTENCY_GATE | PASS | BotCallbacks duplicate same update produces one job/reservation; repeated worker terminal execution denied, native same-job concurrency has one outcome; inherited inbox/outbox replay tests pass. |
| HISTORY_GATE | PASS | TestR3CHistoryPagination stable 20+1 keyset pages; journey detail/history and browser history/reload pass. |
| WORKSPACE_ISOLATION_GATE | PASS | Native foreign callback/cancel/dispatch/lease/binding/history isolation; real browser workspace switch hides jobs and foreign cancel denied; restored isolation pass. |
| RECOVERY_GATE | PASS | Fresh dump→independent restore canonical manifest identical; TestR3CRecovered reservation, binding, cancel, reclaim, stale settlement, completed/unknown no replay and isolation; TestR2BRecovered pass. |
| DOWNLOADER_MINIAPP_GATE | PASS | Native downloader browser real frontend/API/PostgreSQL submission, formats, selection, worker completion, history, cancellation/reload and workspace isolation; no API interception. |
| NATIVE_BROWSER_E2E_GATE | PASS | Four native browser scenarios pass: existing infrastructure, customer, groups and downloader; separate eight lightweight browser tests are not the authoritative product proof. |
| PERSIAN_RTL_GATE | PASS | Native Downloader browser mobile 390×844 html RTL/no overflow, Persian states; existing customer/group mobile regression. |
| REGRESSION_GATE | PASS | Native existing R3A/R3B/R2B and restored R2B assertions plus successful Go race, Web unit and browser regressions; prior assertions retained. |
| SECURITY_GATE | PASS | Actual Security job zero secret matches/npm advisories, zero reachable Go vulnerabilities with inherited module-only advisory disclosure; native tenant/callback/fencing negatives and SSRF/command/path tests. |
| CI_GATE | PASS | All four final source workflows and five job logs reviewed with exact source SHA, no FAIL/panic/runner error; native/restore pipelines pipefail. |
| DOCUMENTATION_GATE | PASS | Four final documents include exact inventory, migration hashes, test count method, source CI and disclosed superseded failures/limits; evidence-only final commit. |

## Superseded failures

Run 37146410429 failed because the Go job lacked ffprobe and the browser selected a stale first-job link before the second POST completed. Commit ed6665e installed tooling and waited for the created job response. Native schema assertions had already passed. Run 37147598168 passed native/browser but failed the restore reservation snapshot: the injected browser worker consumed the intentionally abandoned native recovery job before backup. Commit a84ab58 restricts that test worker to its synthetic browser customer, keeps the reservation assertion unchanged, and strengthens restored stale-generation/cancellation checks. Run 37148337895 passed native/R3C browser but the old Preview assertion received HTTP429: added detail quota polling exhausted the shared per-IP test budget. Commit d137f03 bounds detail polling and refreshes quota at entry/settlement without changing the rate limit or assertion. Run 37148911492 still failed Preview and skipped restore. Run 37149290088 introduced rate windows but the first native.spec.ts positional regex accidentally selected four files; the old ordinary login assertion failed and restore was skipped. Commit 0d6d601 changes only the selector to /$spec$, with exactly one scenario per iteration and all security assertions preserved. It was explicitly published as a candidate before behavioral validation because local PostgreSQL was unavailable. All five failed source runs are disclosed; final source logs provide the superseding browser and restore proof.

## Tooling and scope

yt-dlp 2026.08.19 is exact-version/hash pinned, Unlicense; no runtime self-update. Image ffmpeg/ffprobe 5.1.9-0+deb12u1 and UID10001 are verified in image logs. Local media tests used 6.1.1-3ubuntu5. Real provider adapters target public YouTube/Instagram/Facebook; CI uses deterministic injected transports without production URL exemptions. Live status: UNTESTED_LIVE_PROVIDER_NETWORK. No live Telegram/customer credentials.

## Known limits and untested work

- Live provider network availability/extraction is untested; supported routes are narrower than every possible platform URL.
- Formats limited to compatible progressive MP4/M4A/MP3 and actual AAC→M4A remux; no DASH merge, HLS or arbitrary transcode choices.
- Commercial BYTES policies with unknown pre-analysis size fail closed; count-based dynamic quota is exercised.
- Cancellation is denied after durable UPLOADING boundary; ambiguous delivery is FAILED with UNKNOWN outcome and requires operator/customer review, not automatic resend.
- Inherited 24-hour lease horizon requires operator recovery beyond its deadline.
- Cleanup considers at most 1024 job directories per pass and retains unknown paths for operator review; production capacity/retention tuning unvalidated.
- Operational outcomes are durable audit/state/failure categories and bounded category logging; no new dedicated metrics backend.
- No external exactly-once delivery claim. Inherited GO-2026-5932 remains disclosed: final govulncheck reports one module-only advisory, zero imported-package/called vulnerabilities; no advisory-free graph claim.
- UNTESTED_LIVE_PROVIDER_NETWORK
- UNTESTED_LIVE_TELEGRAM: API/WebView and webhook registration
- Production credentials/data/deployment/load, scheduled backup/RPO/RTO
- Network S3/external KMS
- A deliberately stalled FFmpeg process killed at the full one-minute timeout (canceled-context/error cleanup is tested)
- Real CDN misleading Content-Length response over network (independent actual byte counter is tested)
- Browser processing journey (processing exercised natively; browser journey uses direct MP4)

CodeQL remains BLOCKED_EXTERNAL and Branch Protection EXTERNAL_BLOCKER, without retries. R3D, Uploader, Bot Factory, payments, AI, bypass/session import, generic URL fetching and production operations remain deferred. Frozen branch histories were not changed; the narrow compatible expired-settlement helper exists on R3C only.
