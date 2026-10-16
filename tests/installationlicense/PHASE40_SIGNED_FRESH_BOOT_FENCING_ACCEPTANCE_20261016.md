# LIC-A9-PHASE-40 — Signed Fresh-Boot Supervisor Fencing (2026-10-16)

**Status:** `ISOLATED_LINUX_SIGNED_BOOT_CHALLENGE_CONTRACT_PASS` / `REAL_PRODUCT_GO127_LINUX_NOT_RUN` / **PRODUCTION NO-GO**.

## Exact ancestry and file scope

- Parent: Product Phase39 Draft PR #93 at `dff77f9c45d7cc0d5dc6752b8c9aae0b95c7e180`.
- Product and Authority `main` unchanged; Authority Draft PR #57 remains read-only.
- Scope **only** `tests/installationlicense/phase38_supervisor_durable_bridge`, a standalone, **nonproduction** Linux test module.
- Modified `bridge.go`: exact locally race-tested Git blob **`3fdd151390329bf4766d61f20f2f1b0018362f55`**.
- Added `phase40_fresh_boot_test.go`: exact locally race-tested Git blob **`742c9fe86a9aeb480ce62a5ab54664648df6da8d`**.
- No Product runtime, production Supervisor daemon, root Product `go.mod`, Authority data, production credentials or provider effect was modified.

## Security gap specifically tested

Previous test-only Supervisor persisted a monotonic epoch and reset volatile `readyEpoch` on restart. **An authenticated future FENCE signed before a crash could still be presented after restart** if its epoch and time window happened to match. An increasing epoch is necessary but insufficient to prove *fresh authorization for the new process*.

This phase adds:
1. A fresh `crypto/rand` **256-bit process-ephemeral** `bootChallenge` on every `Bridge.Open`. Not persisted to disk; fail closed on generation failure.
2. A canonical signed claim field `boot_challenge`, included in the existing domain-separated Ed25519 input.
3. **Test-only opt-in** `EnableTestFreshBootFence`, callable only by UID0 before `Start`, immutable afterward.
4. Root-only test helper `TestBootChallenge` allowing the disposable in-memory test issuer to sign the current boot challenge. **No real Authority challenge-issuance, transport or certificate flow has been built.**
5. A strict FENCE gate `DENIED_FRESH_BOOT_CHALLENGE` **before** any epoch write; a signed old-boot future FENCE or even a signed claim missing the current challenge cannot activate the restarted Supervisor. This supplements (not replaces) the existing fsync journal, `readyEpoch`, UID checks and no-blind-retry contract.
6. The Phase39 optional effect callback remains blocked for stale FENCE and old APPLY when this test-only strict mode is explicitly enabled.

## Actual executed evidence, isolated Linux/amd64 Go1.23.2 as root

- **All six new `TestP40*` named tests:** `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -race -count=1 -timeout=100s -v -run '^TestP40' ./...` **6/6 PASS**, total Go reported 18.233s. Full log SHA256 **`036e4eeea845d2a4e041a89fa606816264c1cd294764913aade154c40b133bf7`**.
- Five selected Phase38 root UID, durable journal, signed denial and epoch/restart cases: `-race -count=1` **5/5 PASS**, log SHA256 **`7142629db984c7c67dfdf52b98e5c85afd1e71a5583466ef0e4e91fc0f873fea`**.
- Five Phase39 synthetic effect gate regression tests: `-race -count=1` **5/5 PASS**, log SHA256 **`b670b27172ec0504c69ab1dab7990395f9b4e78330c86d87bb1c416e6a77414a`**.
- `go vet ./...`: **PASS**, `gofmt -l` for new/modified files: **no output**.
- Six new tests cover missing/wrong challenge; legitimate signed challenge; signed future FENCE pre-recorded before crash and rejected after restart; durable unresolved record blocking future epoch even with fresh signature; real kernel-unprivileged outsider cannot advance epoch; boot mode immutability; and stale requests never reaching the optional synthetic Phase39 callback after restart.
- A two-pass repetition of five initial Phase40 tests passed, but the later **six-test two-pass attempt** hit the container's execution time cap and produced **no verdict**. A full all-phase suite similarly hit a time limit at a Phase38 crash test; **NO FULL-SUITE PASS** is claimed. Only the above isolated completed suites count.

## Critical limitations — NOT RELEASE ACCEPTANCE

- **Strict mode remains opt-in to preserve Phase38/39 backward compatibility** and is not automatically enforced by the earlier Phase39 test-only bridge. Therefore this is a tested mitigation contract **NOT a universal or deployed fix**. Default Phase39 test behavior does not require the boot challenge.
- The root-only `TestBootChallenge` accessor and in-memory test issuer are **not** an authenticated online Authority challenge/grant protocol. Real independent issuer verification, nonce delivery, multi-issuer policy, trusted clock, hostname/namespace hardening and replay lifecycle remain to design and validate.
- The phase40 tests use the standalone Linux synthetic bridge, not real Product `Manager.Effect`; **Go1.27 Linux root Product -race Phase39 runner remains NOT_RUN**. No externally observable provider action, PostgreSQL16/outbox or distributed exactly-once guarantee is certified.
- The five real Product hardlink-defense security cases on full Linux Go1.27 are still NOT_RUN. Phase37 Android Go1.27 Product functional PASS is independent prior evidence.
- Only root-owned ephemeral test directories and disposable synthetic Ed25519 signers used. No Production server, deleted Authority VPS, main merge, actions, release signing or deployment.

**Disposition:** isolated Linux root multi-UID *strict-mode* security contract accepted; real Product/Authority and deployment gates remain **NO-GO**. Stop after Phase40.
