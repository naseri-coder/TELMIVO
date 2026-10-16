# LIC-A9 Phase43 — global unresolved-outcome safety gate

**Date:** 2026-10-16. **Status:** local Linux Go1.23 STUB `-race` PASS; genuine Product Go1.27 Android NOT_RUN; Linux root Go1.27 and Production NO-GO.

**Parent:** Product Phase42 Draft PR #96, immutable HEAD `f0da21a1cb2a3d5c6381473dbbfd05c5e1eb49b4`. The only proposed code changes are three `*_test.go` files in the real installationlicense package, with NO production/runtime modifications.

## Precisely addressed gap
Phase42's test-only journal blocks reuse of one RequestID, but not a new and different RequestID while a prior effect is **RECONCILE_REQUIRED**. Phase43 adds an **opt-in TEST-ONLY strictGlobalPending policy**:
- Check for any unresolved journal reservation before accepting a new signed boot grant.
- Atomically, under the verified shared journal flock, enumerate the anchored journal directory FD and reserve a distinct ID only if no unresolved effects remain. Avoid a check/reserve race across two gate instances.
- Validate records, IDs, SHA filenames, matching synthetic signed provider receipts and absence of unexpected/orphan entries. Corruption and forged receipts are fail-closed.
- Only an authentic **synthetic** provider confirmation may allow the next distinct ID; the old ID can never be replayed.
- Honor genuine Product V2 Manager revocation and existing Ed25519 signed scoped requests; stricter behavior is opt-in to avoid altering Phase42's eight existing tests.

## Source integrity / pending GitHub source transfer

| File | Git blob SHA | SHA256 |
|---|---|---|
| modified `internal/platform/installationlicense/phase42_real_manager_portable_challenge_test.go` | `3e285800441262bd60844977bcd1165556b6e42d` | `ad438ba1f8281d8a0b9f43df55d5eae3e349d5fca334c839d8247e52708c1ebe` |
| new `internal/platform/installationlicense/phase43_unresolved_global_journal_test.go` | `ac1a9df34e8fb3600805054dded81d186bb8e58e` | `fc5ed52d79548608182d306b7ce36b6892447f6eee67d6fd763726576ec95850` |
| new `internal/platform/installationlicense/phase43_real_manager_global_fence_test.go` | `aed0defee2491e390b1a7c2b612591c8316ffeb0` | `96af11d36ce8028a3c06a6178d2af89502ad4b675d66758e3dfcc1632dbecdc2` |

**These three sources have not yet been uploaded to GitHub.** The user will run a guarded, hash-verified Termux transfer script which **first** runs 16 real Product Go1.27 Android tests and `go vet`; commits and pushes only after PASS.

## ACTUALLY executed independent isolated Linux Go1.23.2 STUB checks
In the local scratch module with exactly the three source files and a **compatible fake Manager/journal implementation (not genuine Product)**:
- `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -race -count=30 -timeout=180s -run '^TestLICA9Phase4[23]' ./...` **30/30 complete repeated suites PASS**, each suite including eight Phase42 and eight new Phase43 named tests.
- Eight Phase43 `-v` individual cases **8/8 PASS**, log SHA256 `445b6d070825536d1ac45b5076d3c6be2a0b66af4a0c005f8109ad7d55107430`.
- `go vet ./...` PASS and `gofmt` clean.

These results establish **local test contract and race stability against a STUB**, not actual cryptographically validated TELMIVO Product Manager on Linux.

**Outstanding:** Android Go1.27 true Manager.Effect Phase43 tests, Linux Go1.27 root multi-UID + Race, actual Product five Linux hardlink cases, real Authority-signed fresh-boot challenge, PostgreSQL16 distributed fencing and verified external provider reconciliation. This remains test-only, not an operational fix.

**PRODUCTION NO-GO. All PRs Draft; no main merge, Actions, servers, deployed license authority or real effects. Stop after Phase43.**