# LIC-A9 Phase30 — Fail-Closed Owner Handoff, Crash Recovery and Synthetic External Fencing

Date: 2026-10-16
Review state: **OFFLINE_PHASE30_PROTOTYPE_TESTS_PASS** / **REAL_AUTHORITY_AND_PROVIDER_FENCING_NOT_VERIFIED** / **PRODUCTION_NO_GO**

## Exact source lineage

- TELMIVO Product Phase29 Draft PR #82: exact parent HEAD `35fc79c5871e90480ae8ef10eff10b3489511570`.
- TELMIVO License Authority Phase21 Draft PR #57: review-only HEAD `d6960982e322d4458c7d963bb153f205a1f8fb59`. The Authority repo was not changed.
- Phase28 `phase28_effect_gate_proof` and Phase29 `phase29_outcome_ledger` are used only via local nested-Go `replace` directives. No root Product module, business runtime, database schema, Authority issuer, signed session or Production service was touched.

## Security problem

Phase27 showed that `flock` plus a Unix-socket pathname is vulnerable to an **untrusted same-UID directory writer** capable of unlinking/replacing both names. Phase28 demonstrated a distinct-UID supervisor-owned FD boundary on Linux, but did not implement production socket activation or genuine `Manager.Effect`. Phase29 added a crash-aware *synthetic* intent ledger, but not external fencing.

This phase models owner transition, monotonic owner epoch, and pending-outcome handoff denial **without relaxing Product's signed V2 runtime-session anti-clone check**.

## Exact implemented test-only contract

New nested module `tests/installationlicense/phase30_owner_failover/`:

| File | Git blob SHA (matches independently executed local source byte for byte) |
| --- | --- |
| `go.mod` | `c557fe3eaa5aaca4edea03634a1e20de0029601f` |
| `owner.go` | `20d1a552d2bb0067e34929b4885d7caa46514ae6` |
| `owner_test.go` | `cdc4b54744ee0ebd834fd3bdfc215e4789191d96` |
| `process_test.go` | `ba9679bb1f083cd95fec36f7da4fbdf9264cacfd` |

- A **test-generated** Ed25519 signer issues canonical owner-grant claims restricted to a specific installation, owner ID, process session, previous epoch, next epoch and bounded validity. Unknown/forged/expired/mismatched grants fail closed. **This is not a real Authority concurrency V2 lease or its signing key.**
- An exclusive, fsynced supervisor state records one owner/session/monotonic epoch. Files are canonical JSON in an owned mode-0700 test directory; locked with cooperative `flock`, `O_NOFOLLOW` on named files, atomic rename and directory fsync. A missing/corrupt state is rejected; no automatic zero-epoch reset when owner marker exists.
- On `Acquire`, the synthetic external `Fencer.Fence(ctx, nextEpoch)` **must succeed before** the local epoch commit. Failure/unavailability denies takeover. A successful external fence followed by a failed local commit can cause **safe unavailability**, not an automatic retry or unsafe downgrade. The modeled provider enforces current epoch on every mock `Apply`.
- Each `Begin` writes a pending owner-epoch fence **before** calling the independently fsynced Phase29 synthetic ledger. A crash between the writes still blocks takeover. A lost/ambiguous result remains `RECONCILE_REQUIRED`; neither removal nor second execution is implied.
- `Settle` releases the pending owner marker only after the Phase29 ledger confirms a receipt with independently verified synthetic provider signature. No unsigned clear, false confirmed status or manual permissive reset exists in this prototype.
- Fresh takeover must carry a new signed mock session and strict epoch increment. A stale former owner is denied by both local supervisor and synthetic provider epoch. A provider without real fencing is a **hard stop**, not a permissive fallback.

## What was actually executed

Disposable local Linux/amd64 environment, **Go1.23.2**, stdlib only, `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`. No server, Docker, PostgreSQL, real external effect, Production secrets, or GitHub Actions.

1. `go test -race -count=1 -timeout=120s -v ./...`: **25/25 named test functions PASS**, zero reported races. Independently rerun on 2026-10-16 against source subsequently verified byte-identical to GitHub. Includes real subprocess races, a child `os.Exit(77)` after durable `Begin` and persistence on restart, forged/expired grants, missing external fencer, exact signature, stale owner refusal, no premature transition with unknown outcomes, signed synthetic reconciliation, corrupt/missing state rejection and concurrency.
2. `go test -race -count=20 -timeout=180s ./...`: **20/20 complete suite iterations PASS**, equivalent to 500 named test-function executions, no reported race. The repeated suite produced `ok telmivo.local/lic-a9-phase30-owner-failover 34.213s`; process wrapper later hit its own outer time limit after printing `ok`, so the evidence is the completed `go test` output, **not** a claim that every command in the enclosing shell succeeded.
3. `go vet ./...`: **PASS**.
4. `gofmt -l owner.go owner_test.go process_test.go`: no output (formatted source).
5. GitHub connector re-fetched **4/4** uploaded source files and the Git blob SHA matched `git hash-object` calculated in the local container for each actually tested file.

Local execution evidence hashes:
- Full independent `-race -count=1 -v` log SHA256 `8017f487aa7df7937282afd1c65f21e6a086a43e508394a2986778a1f49a0886`.
- Completed `-race -count=20` Go result log SHA256 `05c0eb530a3ef570dc9ae7d476dd2d7d997d5768f85c7aa98ce33bc6fba4b471`.
- The prior session also recorded a successful independent `-race -count=20` output, SHA256 `ffa1ebb10ddfdf0f54a45be471b3ddb7079e353ad63fc6e85895c27c8928a002`.
- This evidence is a local model-owned container log, not a signed trusted build provenance or workflow artifact.

## Critical non-waivable blockers

1. **The public `SignTestGrant` mock and ephemeral test Ed25519 keys are not the real Authority grant and may not be used as a Production signer.** Actual Product cryptographic signed V2 concurrency lease, anti-clone runtime session, and real `Manager.Effect` were **NOT integrated or run**.
2. **The `TestProvider` is in-memory**, not a genuine provider/remote monotonic witness. The two real subprocess contenders each use a separate mock Fencer; their single-winner test proves only the **cooperative local state lock and epoch**. It does **not** prove cross-process or cross-host external epoch fencing.
3. Phase27's same-UID pathname substitution risk is **not resolved by this new nested module**. Direct-path `Open` and temp-file `os.Rename` require a trustworthy supervisor-controlled root and ancestor path; attacker-writable directory, hostile same UID, adversarial mount namespace, or filesystem-level TOCTOU are **NOT certified**. Phase28 supervisor-owned FD approach must be implemented/reviewed end-to-end.
4. The model has no genuine atomic transaction spanning Authority, PostgreSQL, journal and external provider. A crash between mock external Fence and durable state update may cause indefinite hold until independently authorized recovery; never auto-heal or guess the external epoch.
5. Real tenant/actor RBAC, entitlement/quotas, domain operation registration, order/payment/Telegram effects, real provider signed receipt and exactly-once settlement are **NOT implemented/tested**. This model proves at-most-once *intent reservation* in its limited scope, **not exactly-once external execution**.
6. Real Product Go1.27, multi-UID systemd FD broker, **eight real Product × Authority PostgreSQL16 E2E tests**, root key/epoch witness, two-cluster recovery, signed easy installer and owner release approval remain **NOT_RUN / NO_GO**.
7. Product `main` two-commit Termux owner-key custody divergence remains separate, unresolved for release; no main merge.

## Boundaries and final acceptance

Product Phase30 branch adds **four isolated nested module files and this report only**. No `internal/` runtime paths, `go.mod` Product root, migration, Authority repo, `main`, GitHub Actions, live keys, servers or deployment changed.

**Phase30 OFFLINE TEST PROTOTYPE: PASS.** **REAL FAILOVER / EFFECT OWNER SECURITY CERTIFICATION: BLOCKED.** **PRODUCTION: NO-GO.** Stop after Phase30. Do not begin Phase31 or promote this proof until explicitly requested and independently reviewed.
