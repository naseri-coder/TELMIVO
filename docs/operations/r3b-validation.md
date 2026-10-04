# R3B Group Bot validation

Validated source: `6c561a8dc9ae43c2d0ed7bdbbb6545402b099bf1`. R3A parent: `05fc043c53a7a4361b1b99b092c0d253fdf2e881`. Final delivery is the single documentation-only child of validated source; exact SHA is recorded in the Draft PR. Existing local work was preserved. No implementation or CI was repeated.

## Delivery

Independent Group Bot entry/shared identity, immutable workspace group claims, current Telegram authority, scoped rules and warnings, deterministic flood/spam/link policies, protected moderation intents/results, and real Persian RTL Group Mini App. All evidence is synthetic Telegram plus real isolated PostgreSQL; no production or live Telegram operation.

Six R3B commits: five existing source/test commits and one final evidence commit. 28 source paths + 5 documentation paths = 33 final paths. Migration 0010_group_moderation.sql is the only addition; 0001–0009 byte-identical to R3A. The R3A and Foundation branch refs remain frozen. Shared wiring/grants/harness extensions occur only on R3B.

## Existing commits

- `6a5dadfb77bc2aaebfc6be9bd8db5182700b1c91` — feat(r3b): add scoped group moderation and Telegram authority contracts
- `c9bec0152e98171c77f3f116c639750d1b1b16ea` — feat(r3b): extend Persian Mini App with group management
- `2c11aff40440e8e57195974e9df2c51d2f2a860e` — test(r3b): cover native moderation concurrency and real group journey
- `9d9eaab8d1b7054ddd0f459711845d8cd3f26f99` — fix(r3b): separate native browser harness and align acceptance fixtures
- `6c561a8dc9ae43c2d0ed7bdbbb6545402b099bf1` — fix(r3b): protect bot targets and normalize hidden Telegram links
- Final documentation-only child: `HEAD` of this delivery; exact SHA outside its own content.

## Final-source CI

| Workflow | Run | Result |
| --- | --- | --- |
| Security Baseline | [37133275488](https://github.com/naseri-coder/TELMIVO/actions/runs/37133275488) | SUCCESS |
| CI Web | [37133275524](https://github.com/naseri-coder/TELMIVO/actions/runs/37133275524) | SUCCESS |
| Build Images | [37133275492](https://github.com/naseri-coder/TELMIVO/actions/runs/37133275492) | SUCCESS |
| CI Go | [37133275626](https://github.com/naseri-coder/TELMIVO/actions/runs/37133275626) | SUCCESS |

Actual five job logs were downloaded and inspected. All checkouts match validated source. Native artifact 11276969320 is attached to run 37133275626 and the same SHA; SHA256 digest is recorded in machine evidence. No test FAIL/panic/runner error. Expected negative-test PostgreSQL errors and synthetic failed outcomes are not concealed. Both native/restore pipelines use pipefail.

## Test counts

Go: 26 top-level functions statically enumerated in successfully executed race packages (CI is non-verbose). Native: 26 top-level and 65 nested subtests before restore; R3B contributes 7 top-level / 17 subtests. Restored recovery: 1 top-level PASS. Frontend: 6 unit tests; 8 lightweight browser scenarios; 3 native browser scenarios including real group journey. Fresh independent restore: 88 canonical public-table fingerprints identical. Existing Foundation recovery behavior passes under restricted role; group-specific external replay is tested before restore, not claimed after restore.

## Superseded failures

Run 37132225600 failed because the lightweight browser harness included the native group scenario without a backend (connection refused; 2 failures). Run 37132225826 failed on stale migration-count expectation, a fixture conflating actor/bot delete rights, and a mismatched API Origin fixture (403). Existing commit 9d9eaab separated the harness and corrected those assertions/fixtures. Subsequent runs on 9d9eaab succeeded; final source 6c561a8 also adds bot target protection and hidden/bare URL normalization, and its four workflows are the authoritative evidence. Earlier failures are not PASS evidence.

## Gates

| Gate | Verdict | Evidence |
| --- | --- | --- |
| SOURCE_CONTINUITY_GATE | PASS | Local and remote HEAD 6c561a8dc9ae43c2d0ed7bdbbb6545402b099bf1; five existing commits descend from exact R3A parent; CI checkout logs match SHA and tree 99966c91ab18e52f4645f0e761fd0e84db4fce49. |
| R3A_FREEZE_GATE | PASS | Remote R3A stays 05fc043c53a7a4361b1b99b092c0d253fdf2e881; 0001–0009 byte-identical; R3A product regressions PASS. Shared API/runtime/navigation and test harness extended on R3B only; frozen parent is not edited. |
| BRANCH_ISOLATION_GATE | PASS | Remote Foundation ab640fbdbafc76354d0f56fc71e4033fff9db711 and main/develop 024b3a3465285f9758ce109551a6574ced087dc7 unchanged; only R3B ref advances. |
| GROUP_BOT_ENTRY_GATE | PASS | TestR3BGroupJourney shared entry and TestR3BPrivateStartAndHiddenLink PASS; Mini App identity matches independent Group Bot start. |
| GROUP_CLAIM_GATE | PASS | TestR3BGroupJourney claim idempotency/nonadmin_claim_denied and TestR3BAPIIsolationAuthority/cross_workspace_conflict_audited PASS. |
| GROUP_CLAIM_CONCURRENCY_GATE | PASS | TestR3BConcurrencyIsolation concurrent competing claims yields one owner with restricted tenant isolation PASS. |
| TELEGRAM_AUTHORITY_GATE | PASS | TestR3BAPIIsolationAuthority/platform_admin_not_group_admin and application_permission_separate; TestR3BTelegramFixtures PASS with synthetic current membership. |
| AUTHORITY_DRIFT_GATE | PASS | TestR3BGroupJourney/authority_drift_and_unavailable PASS; revoked/unavailable authority denied. |
| BOT_PERMISSION_GATE | PASS | TestR3BGroupJourney/rights_and_protected_targets PASS; separate actor and bot rights fixtures; bot/creator/admin protection. |
| GROUP_TRIAL_GATE | PASS | TestR3BGroupJourney/trial_once_and_readd and expiry_preserves_rules PASS; immutable PER_GROUP enrollment reused. |
| GROUP_ACCESS_GATE | PASS | Resolver-driven TestR3BGroupJourney/expiry_preserves_rules PASS; configuration persists while protected behavior denies expired access. |
| RULES_GATE | PASS | configuration_and_conflict PASS; actual native browser saves two Persian rules and retains them after reload and workspace switch. |
| WARNING_GATE | PASS | duplicate_update_and_warning_escalation and clear_history_and_key_conflict PASS; append-only warnings and ordered clear. |
| MODERATION_ACTION_GATE | PASS | rights_and_protected_targets and transport_results_and_replay PASS; requested/acknowledged/failed/unknown facts distinguished; no live Telegram claim. |
| ANTI_FLOOD_GATE | PASS | TestDeterministicPolicy/flood and threshold_change executed under successful race package; native flood_and_configurable_threshold PASS. |
| ANTI_SPAM_GATE | PASS | TestDeterministicPolicy duplicate/mentions and native duplicate_update_and_warning_escalation PASS; conservative deterministic heuristics only. |
| LINK_PROTECTION_GATE | PASS | TestHiddenAndBareLinks successful race package; native link_allowlist_and_denylist and TestR3BPrivateStartAndHiddenLink PASS; Telegram text_link normalization and visible/bare URLs covered. |
| UPDATE_IDEMPOTENCY_GATE | PASS | duplicate_update_and_warning_escalation, concurrent_update_dedup, transport_results_and_replay and Telegram fixtures PASS. |
| WORKSPACE_ISOLATION_GATE | PASS | TestR3BAPIIsolationAuthority/cross_workspace_reads_and_mutations and real browser foreign PATCH denial/workspace switching PASS; FORCE RLS and composite group FK. |
| AUDIT_GATE | PASS | cross_workspace_conflict_audited, clear_history_and_key_conflict and immutable history/results assertions PASS; correlation-aware mutations. |
| GROUP_MINIAPP_GATE | PASS | groups.native.spec.ts passes actual frontend/API/PostgreSQL group detail, policy/rules persistence, warning history and workspace journey; 390x844 RTL/no overflow. |
| NATIVE_BROWSER_E2E_GATE | PASS | Native job 111232514489 reports 3 passed (8.8s), including existing customer/admin and group journey; no route interception in native group test. |
| REGRESSION_GATE | PASS | 26 native top-level/65 nested subtests, restored TestR2BRecovered, 26 statically enumerated Go unit/contract tests in successful race packages, 6 frontend tests, 8 lightweight and 3 native browser scenarios. |
| SECURITY_GATE | PASS | Job 111232513761: scan 210 files zero matches; govulncheck zero called/imported-package vulnerabilities and one module-only advisory; npm audit zero. Restricted role/security denial assertions PASS. |
| CI_GATE | PASS | Four completed successful source workflows; actual five logs reviewed with exact checkout SHA, no test FAIL/panic/runner error; native and restore pipelines explicitly pipefail. |
| DOCUMENTATION_GATE | PASS | Preserved architecture/ADR plus validation, machine evidence and exact inventory; final change set restricted to these five docs; JSON/inventory/migration/log checks complete before publication. |

## Deferred

- R3C Downloader
- Bot Factory/Uploader/customer bot runtime
- Payments/Stars/crypto/refunds
- AI
- Captcha/raid/advanced engagement
- Full channels/content scheduling/analytics/ticketing/broadcasts
- Network S3/external KMS
- Production deployment/load/retention tuning

## Untested

- Live Telegram getMe/getChatMember and moderation transport, webhook registration and actual Telegram WebView/device matrix
- Production credentials/data/deployment/load and scheduled recovery/RPO/RTO
- Network S3/external KMS/payment providers
- Comprehensive accessibility certification

## Known limitations

- Synthetic Telegram authority/transport tested with real restricted PostgreSQL; live behavior is untested.
- External send succeeded before transaction commit/crash may duplicate action; exactly-once Telegram execution is not promised; unknown outcome is recorded without blind retry.
- Flood/duplicate window uses PostgreSQL digest history, bounded to 1001 records / 1–3600 seconds; production capacity/retention untested.
- Groups list capped at 100; history keyset pages of 50.
- Default policy disables automatic moderation; detection/escalation default LOG_ONLY; heuristics cannot establish spam intent.
- Bot rights UI is last verified snapshot; sensitive operations revalidate authority.
- Automatic installation chooses existing default workspace; existing immutable claims cannot be silently reassigned.
- Inherited module-only Go advisory remains; govulncheck reports one module vulnerability, zero imported-package/called vulnerabilities.

## External blockers

- CodeQL: inherited private-repository/account capability blocker; not retried.
- Branch Protection: inherited administration capability blocker; unchanged.

## Stacked delivery contract

Exactly one new Draft PR: feature/r3b-group-bot-v0.1 → feature/r3a-main-miniapp-v0.1. PR #1/#2 remain OPEN/DRAFT/NOT MERGED; new PR #3 must also remain OPEN/DRAFT/NOT MERGED. No retarget, merge, R3C or deployment.

TELMIVO_R3B_GROUP_BOT_VERTICAL_PASS
