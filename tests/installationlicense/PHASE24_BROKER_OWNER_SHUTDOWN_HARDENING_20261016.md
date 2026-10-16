# LIC-A9 Phase24 — Broker lifetime ownership, safe shutdown and path admission

**Date:** 2026-10-16. **Scope:** `tests/installationlicense/phase23_broker_proof/` only, plus tests/docs source freeze.  
**Status:** `PHASE24_SYNTHETIC_BROKER_HARDENED` / `GO123_RACE_16_PASS` / `STRESS_50_PASS` / `PYTHON_191_PASS` / `PRODUCTION_NO_GO`.

## Authoritative baseline

- Parent Product Phase23 **Draft PR #76**, exact HEAD `34a1459c3f44a7395f3044040d707c53644443fc`; source broker Git Blob `827339007bbda5163a7fb268a69511489b7a9a85`.
- Phase22 multiworker safety Draft PR #75 `24ef50684148a2a434f0dbde8c81017c6c0e244c`, Phase19 signed runtime fence PR #74 `a9e09c8e0029fa0d35e6e06ed6da0fe640c672b8`, Authority Phase21 Draft PR #57 `d6960982e322d4458c7d963bb153f205a1f8fb59`; all untouched, unmerged.
- Hardened Product ancestor PR61: `d88fdb2e4d34f18e8f9c8313f73db5b14a077d5a`.
- Original exact verified offline module `go.mod` and `broker_test.go` remain unchanged.

## Three **actually reproduced** pre-fix failures

The exact original GitHub broker code was recovered into a model-owned isolated local container; Git Blob SHA matched all 3 original source files. Genuine local **Go 1.23.2 `-race`** tests initially failed:

1. **DOUBLE_OWNER_AFTER_SOCKET_UNLINK**: remove the pathname of an active Unix socket while its listener is still alive, then invoke `Open(root)` again. The original broker accepted a second owner; the socket filename alone did not fence the process lifetime.
2. **SYMLINKED_PARENT_REDIRECTION**: a symlink in an ancestor of a final `0700` owned broker root redirected `Open` into an unintended directory because only the leaf `root` was `Lstat`-checked.
3. **QUEUED_EFFECT_AFTER_SHUTDOWN**: a request that passed the pre-lock cancellation test was queued on `b.mu`. After `Close` signaled cancellation and the lock was released, the original handler ran the effect without rechecking cancellation. The test observed exactly **one unexpected synthetic effect**.

Original source passes its nine prior unit tests, but those nine did not cover these cases. This Phase24 is a genuine source-level defect finding, **not evidence of any compromised Production system**.

## Narrow corrective patch (nonproduction demo module only)

GitHub broker `tests/installationlicense/phase23_broker_proof/broker.go` must match locally executed Git Blob **`6b1f920f0017f2f39cc0b11bbb40c1c262a37685`**. The code:

- Rejects explicit noncanonical paths and *any currently symlinked ancestor* before opening an owned `0700` root. This is a **best-effort path check**, **NOT** a race-free `openat2` containment proof against a hostile shared-UID actor.
- Adds `broker-owner` regular file with `O_NOFOLLOW|O_CLOEXEC`, owned UID, one hardlink, private permissions, and an **exclusive nonblocking kernel `flock` held for the full broker lifecycle**. A missing/unlinked socket path alone cannot allow a second honest owner while this lock remains held. The lock is released on failed `Open` and only **after all admitted handlers finish** on `Close`.
- Adds an accept-handler registration barrier, concurrent idempotent `Close`, and a post-mutex cancel recheck. A request queued when the broker is shutting down is refused with `DENIED_CLOSED` rather than reaching a synthetic effect.

New Go regression `tests/installationlicense/phase23_broker_proof/phase24_hardening_test.go` Git Blob **`9f0840e34e0cee822753a64f2b00330e0c1bf2e5`** contains **seven** test functions. Added negative cases: competing owner after socket unlink, symlink ancestor rejection, queued cancellation, symlink/hardlink/world-readable owner marker refusal, in-flight effect drain before releasing leadership, ambiguous effect denial of blind retry, and repeated concurrent `Close` calls.

These changes **do not** modify the original Phase19 signed session check, Authority, Product live runtime, business operations, identity schema, migrations, `main`, Docker images, install scripts, or actual licence signer.

## Actually executed acceptance (model-owned disposable Linux container)

- **Pre-fix Go1.23.2 `-race` focused regression:** `0/3 PASS`, **three genuine FAIL**, one per defect above.
- **After-fix Go1.23.2 `-race -count=1 -v ./...`: 16/16 PASS**, covering nine original and seven new Go test functions; all three previous failures changed to PASS. Three subtests of file safety also PASS.
- **Go1.23.2 `-race -count=50 ./...`: 50/50 complete passes, corresponding to 800 executions of the sixteen test functions**, with no test failure or reported race. Not a probabilistic proof of absence of races.
- **`go vet ./...`: PASS**, clean output.
- **`gofmt -l`: empty**, all modified and existing Go proof source formatted.
- **Python3.13.5 `unittest discover`: 191/191 PASS**, including a new deterministic regression rejecting a missing Phase24 source path. The suite is only the `tests/installationlicense` tooling subtree, not the full Product.
- **Python source gate:** `SYNTAX_AND_RUNNER_ENTRYPOINT_PASS_ONLY`, **20/20 inspected Python source files**; deliberately avoids any Docker/PG16 work.
- Source-fence exact allowlist increased from **41 to 43** changed paths relative to the hardened Product ancestor; the only non-test/docs paths stay the three preexisting reviewed `internal/platform/installationlicense/` exceptions from Phases19/22. Phase24 introduces no new `internal/` changes.

Local evidence SHA256:
- Pre-fix three-error log `2b37f657055f735d858cf72ee5177653fd8421bb7bec8e2382622c31037e1327`.
- Post-fix complete 16-test verbose log `622df41a9bc6a680411b264ca28e58413e2c3cdd51a45da7a5d785b202f410fc`.
- Go `-race -count=50` summary `a34e83adf9b596aa910cd495c4fe15b1f3adf73ee2101e2f020d7e6c9e82ba6c`.
- Python 191-test log `b200948fb4ddbc399dc3721ac0549f906d65c89780c97a220a207b3419a13bcf`.
- Python AST log `85065f3fb326334615e041ac6d088c169aab5e898534403b58778d28074396bb`.
- Test-only broker source SHA256 `7a65bd32381cf869573229ced324998df1c435de7c06662bc7078b9b47dd2f32`.

## Important security limitations / remaining blocker

The owner-file `flock` assumes cooperative, owner-controlled processes and a stable pathname: **a hostile process with the same effective UID and write permission to the protected directory could unlink and replace both owner-lock and socket paths**. The path check has a time-of-check/time-of-use race and does not replace a kernel-enforced secure directory capability or mount namespace. The test-only `SO_PEERCRED` same-UID check is **not** a tenant, service role, or commercial authorization mechanism; no public IPC endpoint is exposed.

The broker continues to use durable **at-most-once reservation**, **NOT exactly-once settlement**. A crash or timeout after reservation and before external outcome can leave an unknown outcome; the system must **never blindly retry** the same payment/trade/message. A production design requires audited cross-system outcome reconciliation and domain idempotency.

**The demo `EffectGate` callback remains synthetic. The broker is NOT connected to the real `Manager.Effect`, licence Authority, PostgreSQL16, background workers, Telegram, payments, exchange orders, or real business RBAC.** Full Go1.27 Product `-race`, independent multiworker failover, eight real Go/PG16 cross-repo E2E cases, external high-water witness, encrypted restore, signed Ubuntu installer, and Product `main` two-commit reconciliation all remain **NOT_RUN / NOT_VERIFIED**.

**Decision: `PHASE24_TEST_ONLY_BROKER_SECURITY_FIX_ACCEPTED_LOCALLY` / `REAL_MULTIWORKER_LIVENESS_NOT_CERTIFIED` / `REAL_GO127_PG16_E2E_NOT_RUN` / `PRODUCTION_NO_GO`. KEEP DRAFT. NO MERGE, PRODUCTION, REAL VPS, DB OR SIGNER. STOP AFTER PHASE24.**
