# LIC-A9 Phase 2K — cross-repository test coverage/certification boundary

**Date:** 2026-10-15 · **Nonproduction only** · **No Go 1.27.2 / PostgreSQL 16 real E2E executed**

## Verified coverage gap

The pinned six-test cross-repo fixture is the old `tests/installationlicense/authority_e2e_test.go.template`, Git blob `31babdd5f389133e1947bb5620c4336ebe3c46f3`. Its fixture preparer explicitly identifies it as historic LIC-A8 research. Test function names are exactly `TestE2E1` to `TestE2E6` and cover:

| Test | Existing LIC-A8 cross-repo validation |
|---|---|
| E2E1 | Installation identity, activation, reply loss and idempotent retry |
| E2E2 | Restart persistence, refresh/status, deactivation |
| E2E3 | Temporary outage, signed offline deadline, recovery |
| E2E4 | License suspension, resumption, revocation, expiry |
| E2E5 | Authority recovery hold, epoch/revision fencing |
| E2E6 | Restricted effects, settlement and bot webhook ACK fences |

None of these six implements a **cross-repo anti-cloning/concurrency test**: the template does not contain a concurrency endpoint/client invocation, clone or supersession scenario, or explicit same-generation conflict. Therefore `LIC_A9_JOINT_..._E2E_PASS` incorrectly suggested overall A9 verification.

Authority has *separate* LIC-A9 tests, for example `internal/m2m/concurrency_test.go` (`TestLICA9BIssuingAndExactReplay`, `TestLICA9BSameGenerationRace`), `internal/store/postgres/concurrency_lock_test.go` and `concurrency_grants_test.go`. These are not imported into or executed by the six cross-repo smoke-test function filter. Their existence is not evidence that the **real PostgreSQL16 plus end-to-end product** A9 suite ran.

## Fix implemented (no product / Authority runtime changes)

1. Docker-only cross-repo runner success now emits `LIC_A8_LEGACY_CROSSREPO_GO1272_PG16_SMOKE_PASS` followed by `LIC_A9_ANTI_CLONE_CONCURRENCY_E2E_NOT_TESTED`. Existing six-test discovery, JSON run/pass proof, nonroot PostgreSQL, cleanup, Git SHA gates and Go1.27.2 pin remain intact.
2. Older host-Go runner is explicitly labeled **historical, deprecated for A9 certification**. On command completion it emits `LIC_A8_LEGACY_HOST_GO127_PG16_SMOKE_COMMAND_COMPLETED_TEST_COUNT_UNVERIFIED`, plus the A9 coverage gap; it no longer publishes an A9 E2E PASS. The old path's lack of six independent JSON proofs is explicitly reflected.
3. Two new local regression tests confirm those boundaries, including previous false-certification strings being absent from outputs/success code paths.

## Local evidence

Executed using the pre-PR57 exact source-executed Python/Git/synthetic-Go test environment and the three edited files, each compared by Git blob SHA to branch:

- `run_joint_docker_only_e2e.py`: `14216c65f46ecb6be5afa1d7da6a2c0c335a1563`
- `run_joint_postgres_e2e.py`: `a0e1beef57fdbff2f83d9d7739d9097018dc1c8e`
- `test_joint_docker_only_e2e.py`: `ee377a6bb83dcbdacce5c4f5fa46bfc9df56cbcb`

```text
PYTHONDONTWRITEBYTECODE=1 python3 -B -m unittest discover -s . -p 'test_*.py' -v
Ran 42 tests in 0.817s
OK
```

Log SHA256: `5bb1ea005041c6305edd9935524ae98fc7fbbb0717b6f61055f782ede4a11f2d`.

## Release implications / next hard gate

LIC-A8 cross-repo smoke, even when eventually run on independent Docker/PG16, **cannot close the LIC-A9 anti-cloning integration gate**. In addition to executing six legacy cross-repo tests with real database, design and execute a **separately scoped LIC-A9 product-client ⇄ Authority PostgreSQL16** integration suite with verified real Go1.27.2/PG16 evidence:
- cloned identity/public-key mismatch (must fail closed)
- replay/idempotency and simultaneous same-generation conflict
- generation/recovery epoch fencing, supersession and restart persistence
- lock contention and backlog/refusal conditions before permits
- correct restrict/block behavior across product clients and bot side effects

Freeze exact test cases and evidence criteria before execution; do not conflate Authority-only isolated tests with joined product-client tests. No thresholds or policy relaxation; no production deployment or Authority PR32 merge until authorized release criteria are achieved.

**Result:** `PHASE2K_LOCAL_REGRESSION_42_42_PASS`; `LEGACY_SMOKE_NOT_A9_CERTIFICATION`; `REAL_GO1272_POSTGRES16_E2E_NOT_RUN`.
