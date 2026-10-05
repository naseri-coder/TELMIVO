# TELMIVO R3A validation — recovered product vertical

Foundation parent: `ab640fbdbafc76354d0f56fc71e4033fff9db711`; tree `11149347c4ea99021809d51741188ed4781d2142`. Validated source: `7ff94a95842d5ddfbfe47208a4c140c608c1d26c`. Product branch: `feature/r3a-main-miniapp-v0.1`.

## RECOVERED CHECKPOINT

The original TELMIVO checkout remained clean on the Foundation branch. Its existing linked worktree `/workspace/scratch/f0ab416e623c/TELMIVO-R3A` contained 19 modified and 13 untracked R3A files, no staged paths, and parent HEAD `ab640fbdbafc76354d0f56fc71e4033fff9db711`. This exact candidate was continued. No reset, deletion, overwrite, branch recreation or architecture restart occurred.

## IMPLEMENTED

Main Bot accepts validated private `/start` and menu commands through a thin secret-checked Telegram adapter, shares identity/workspace/trial initialization, writes responses to the existing outbox, and dispatches supported menu responses through the existing Telegram transport boundary. Menu items open appropriate Mini App routes. Admin visibility uses existing granular platform authority.

The fa-IR RTL dark customer control center has real Dashboard, Plans & Access, read-only Wallet and Settings. Workspace selection survives reload and each API request is authorized server-side. Dashboard uses PostgreSQL counts and catalog labels, policy output and ledger balances. Wallet amounts remain atomic strings and history uses bounded stable keyset pagination. Workspace-name changes require current owner membership, expected prior name and audit actor/old-new/correlation. Deferred navigation is factual.

## VALIDATED

All 23 R3A gates below are PASS with evidence. Restricted runtime native behavior uses NOSUPERUSER / NOBYPASSRLS / NOINHERIT; no migration-capable role performs customer acceptance.

- 21 Go unit/contract tests, race / vet / build / gofmt.
- R3A: 4 native top-level tests with 18 subtests. Entire targeted native selection: 19 top-level and 44 subtests, plus restored `TestR2BRecovered` PASS (3.17s).
- 6 web unit tests; 8 lightweight desktop/mobile browser regressions; 2 authoritative native browser tests, including the full customer journey at 390x844 without API interception.
- Real browser: signed entry, Dashboard, two workspaces, resolver equivalence, Plans, ledger balance, 20+1 wallet history entries, owner Settings update, reload persistence, ordinary admin denial, fa-IR/RTL and no page overflow.
- PostgreSQL 17.11 source and fresh second restore instance: all 84 business-table fingerprints equal; restored behavioral recovery regression PASS.
- govulncheck reports zero reachable/imported vulnerabilities, with inherited module-only GO-2026-5932. npm audit reports zero vulnerabilities. Supplementary secret scan: 192 source files, zero matches.
- Images and synthetic Compose startup/health PASS. No deployment or live service operation.

### Final source CI

- Security Baseline: [37128112206](https://github.com/naseri-coder/TELMIVO/actions/runs/37128112206) — SUCCESS
- CI Web: [37128112170](https://github.com/naseri-coder/TELMIVO/actions/runs/37128112170) — SUCCESS
- Build Images: [37128112158](https://github.com/naseri-coder/TELMIVO/actions/runs/37128112158) — SUCCESS
- CI Go: [37128112512](https://github.com/naseri-coder/TELMIVO/actions/runs/37128112512) — SUCCESS

Actual logs for jobs 111217510093 / 111217510337 / 111217508962 / 111217508849 / 111217508778 were reviewed. No failing test or panic was hidden. Both native `go test | tee` commands keep explicit `set -o pipefail`. Expected synthetic job states named FAILED are not test failures.

Native artifact: 11275776683, SHA256 `8355647efdd30ce52643942c66ed6bcaf9951ef289d5d16311d0071642c37df9`; contains native/recovery logs, before/after restore fingerprints and real mobile Dashboard/Wallet screenshots.

### Correction checkpoint

Run 37127752756 at `daf7a08` correctly FAILED browser acceptance: the test's unscoped Plans link selector matched two legitimate links. Native business assertions passed, but that run was not complete browser closure evidence. Commit `7ff94a95842d5ddfbfe47208a4c140c608c1d26c` scopes the selector to the customer navigation landmark without removing assertions. Replacement run 37128112512 is authoritative.

## Gate table

| Gate | Verdict | Direct evidence |
| --- | --- | --- |
| SOURCE_CONTINUITY_GATE | PASS | Exact PRIVATE repository, frozen parent SHA/tree, remote refs and PR #1 checked before work and after final source CI. |
| FOUNDATION_FREEZE_GATE | PASS | Migrations 0001–0009 byte-identical to parent; Foundation branch remains ab640fb; resolver, ledger, trial and durable infrastructure contracts reused. |
| BRANCH_ISOLATION_GATE | PASS | Separate preserved product worktree; four source commits descend only from frozen Foundation; no delta on Foundation/main/develop. |
| LOCAL_RECOVERY_GATE | PASS | Recovered 19 modified + 13 untracked paths on TELMIVO-R3A, zero staged paths, HEAD ab640fb; no reset, deletion, overwrite or reconstruction. |
| MAIN_BOT_ENTRY_GATE | PASS | TestR3ASharedEntry/MAIN_BOT_first and menu_and_thin_transport PASS with actual restricted PostgreSQL. |
| MAIN_BOT_IDEMPOTENCY_GATE | PASS | Repeated starts, duplicate webhook delivery and one durable outbox response per update asserted in TestR3ASharedEntry. |
| MINIAPP_DIRECT_ENTRY_GATE | PASS | TestR3ASharedEntry/MINIAPP_first and TestR3AAuth/direct_signed_login_and_replay PASS without Main Bot prerequisite. |
| SHARED_IDENTITY_GATE | PASS | Both entry orders preserve Platform User, default Workspace, two distinct entry-source records and immutable trial end/version; no duplicate enrollment. |
| WORKSPACE_SWITCH_GATE | PASS | Native authorized_workspace_list and real_dashboard_switch_and_isolation; browser switches two real workspaces and retains validated selection after reload. |
| DASHBOARD_GATE | PASS | Native real counts and workspace ID; browser observes real group count 1 versus 0, real balances and catalog label; no invented analytics. |
| ACCESS_SUMMARY_GATE | PASS | Native resolver_equivalence_and_quota and browser compare customer read model to /access/decision; committed/reserved quota = 1/1, remaining/reset authoritative. |
| PLAN_CATALOG_GATE | PASS | Native plan_catalog and real browser Plans screen read active PostgreSQL catalog; purchase controls truthfully unavailable. |
| WALLET_READ_GATE | PASS | Native ledger balance 300 atomic USD, keyset pagination and mutation denial; browser reads 2100 atomic USD and loads 20 + 1 real entries. |
| SETTINGS_GATE | PASS | Native owner-only mutation, optimistic stale-name conflict, audit actor/correlation and nonowner denial; browser saves and reloads persisted name. |
| ADMIN_AUTHORIZATION_GATE | PASS | Native ordinary owner denied policy API; existing configured administrator recognized; real browser hides menu and denies route/API. Existing R2A policy browser also PASS. |
| MINIAPP_AUTH_SECURITY_GATE | PASS | Native valid signed login, signature tamper/malformed denial, valid but expired signature denial, replay rejection and restricted runtime role assertions PASS. |
| WORKSPACE_ISOLATION_GATE | PASS | Native foreign dashboard/wallet/settings denial and nonowner update denial; browser switching shows distinct real state; restored restricted-runtime isolation regression PASS. |
| NATIVE_BROWSER_E2E_GATE | PASS | customer.native.spec.ts PASS through actual frontend -> API -> PostgreSQL, no interception; includes wallet pagination, settings, admin denial and reload. |
| PERSIAN_RTL_GATE | PASS | Real browser asserts fa-IR, RTL, 390x844 mobile and no page overflow; native screenshots reviewed; retained desktop/mobile focus tests PASS. |
| REGRESSION_GATE | PASS | 21 Go unit/contract tests; 19 native top-level + 44 subtests and restored TestR2BRecovered PASS; existing R2A native browser and eight lightweight browser regressions PASS. |
| SECURITY_GATE | PASS | gofmt/vet/build/race, govulncheck, npm audit and 192-file secret scan PASS. GO-2026-5932 remains module-only, unimported/uncalled; not an advisory-free graph. |
| CI_GATE | PASS | All four final-source workflows SUCCESS; actual job logs inspected with no failed test/panic; both piped native commands retain explicit pipefail. |
| DOCUMENTATION_GATE | PASS | Architecture, validation, machine evidence and Git-generated exact changed-file inventory finalized and checked in evidence-only delivery commit. |

## R3A COMMITS AND DELIVERY

- `7171c6bb61dd6d5abb53064e6dc9471264c21372` — feat(r3a): connect Main Bot and scoped customer read models
- `f63a849ce8a63e1d1818a92b46f482304e3e6ff9` — feat(r3a): deliver Persian customer control center journey
- `daf7a08ae135b9b0802d5611233520280c930805` — test(r3a): verify shared identity and native customer acceptance
- `7ff94a95842d5ddfbfe47208a4c140c608c1d26c` — fix(r3a): scope native journey navigation to customer menu
- Final evidence/documentation-only commit — the Git HEAD containing this report, directly parented by `7ff94a95842d5ddfbfe47208a4c140c608c1d26c`.

The exact evidence commit SHA is recorded in the stacked Draft PR description and final response after publication. A Git commit cannot contain its own hash literally; machine evidence records the delivery revision, required parent and external exact-SHA location. `r3a-files.json` is generated from Git's complete staged delta against the frozen parent, then checked against final delivery.

Migrations: **zero new**. Existing 0001–0009 are byte-identical to Foundation; checksums are in machine evidence. Foundation contracts were reused, without a new generic durable contract or ADR.

## DEFERRED

- Full Channel Management / Publishing / Scheduler / Content Studio / Advertising / Growth / full Analytics
- R3B Group Bot / moderation / Anti-Spam / Anti-Flood / Captcha / Raid / warnings-mute-ban / live group authority
- R3C Downloader / real YouTube-Instagram-Facebook media / DRM-private-paywall bypass / FFmpeg product pipeline
- R3D Uploader / Bot Factory / full customer bot runtime
- Full Ticketing / Broadcast Center
- Real purchasing / payment rails / Telegram Stars / crypto / refunds / wallet mutation
- AI Copilot
- Network S3 / external KMS
- Production deployment / secrets / DB operations / scheduled production backups / load testing

## BLOCKED_EXTERNAL

- CodeQL: inherited private repository/account blocker; not retried and not claimed as passed.
- Branch Protection: inherited unsupported administration blocker; no administration attempted.

## UNTESTED

- Live Telegram webhook registration, actual bot delivery and live Telegram client rendering
- Production credentials / deployment / production data / production load and restore scheduling
- Payment and deferred product workflows
- Network S3 and external KMS
- Manual device matrix beyond automated 390x844 mobile / 1100x760 desktop; comprehensive accessibility certification

## KNOWN_LIMITATIONS

- Customer workspace list and active plan catalog return at most 100 records; workspace enumeration pagination is not implemented. Wallet history is keyset-paginated, default 20 and maximum 50.
- Customer quota presentation currently selects the existing downloads policy; no Downloader operation is implemented. Catalog names are data, not authorization branches.
- Durable Settings only changes workspace name and only for its current owner/member, with expected-name concurrency and audit. Locale is fa-IR and timestamps use browser timezone; neither is claimed as persisted server preference.
- Active workspace selection persists locally per Platform User, validated against the authorized list on every bootstrap; it is presentation state, never server authorization truth.
- Telegram menu response delivery uses the existing PostgreSQL outbox / JetStream inbox. Ambiguous external send success before worker crash can cause duplicate messages; durable identity/trial/customer state remains idempotent. Exactly-once Telegram delivery is not claimed.
- Existing Foundation ticket component and synthetic authorized-admin wallet-adjustment regression surface are preserved for compatibility; neither is advertised as an R3A customer product. Customer Wallet has no write controls or new write endpoint.
- No new durable architectural decision or schema contract was introduced; no ceremonial ADR.

Local PostgreSQL/Docker were unavailable and the local Chromium download was truncated; authoritative native/browser evidence comes from successful disposable GitHub services. Source CI was one coherent checkpoint plus one justified selector correction cycle. Historic suites were not manually rerun wholesale; affected regressions and the existing CI selection executed naturally.

## FINAL SAFETY CHECKPOINT

Foundation still ends at `ab640fbdbafc76354d0f56fc71e4033fff9db711`; PR #1 OPEN / DRAFT / NOT MERGED. main and develop remain `024b3a3465285f9758ce109551a6574ced087dc7`. The R3A PR is stacked on Foundation and remains Draft. No production operations, live Telegram credentials, payment, deployment or R3B work occurred.

TELMIVO_R3A_MAIN_MINIAPP_VERTICAL_PASS
