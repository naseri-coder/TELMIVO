# Source audit remediation Wave 1

Historical checkpoint: this document describes commit `644ed9ef8e44f594d61d3f4c0019c547565f864e`. The subsequent full-source remediation uses `R5B_REMEDIATED` (0001–0026); see `post-audit-ledger.json`. Historical R4G and original R5B boundaries below remain unchanged.

Scope: TEL-R5-AUDIT-001 through 004 only, from R5B
`4599f431a97f212667be022b2ca1afa258a1a2ae`. This is not R5B source-clean approval.

## Scheduled business denial

Explicit quota, permission and plugin-state admission denials produce a durable
SKIPPED execution with the existing safe business code. Schedule materialization
then advances the occurrence/next time using the existing recurrence policy.
Database/transaction/cancellation/unexpected errors still propagate. No runtime
supervisor error is broadly ignored, and no change to scheduler lock ordering is
included in this wave.

## Reconciliation evidence matrix

Every accepted resolution requires the exact currency/amount and a bounded,
nonempty provider reference. The trusted provider adapter supplies the evidence.

| Current payment | Outcome | Status | Result |
|---|---|---|---|
| UNKNOWN | ACKNOWLEDGED | PAID / REFUNDED | Verified positive transition |
| UNKNOWN | ACKNOWLEDGED | AWAITING_PAYMENT / FAILED | Deterministic recorded provider state |
| UNKNOWN | REJECTED | FAILED | Deterministic rejected checkout |
| REFUND_UNKNOWN | ACKNOWLEDGED | REFUNDED | Payment/refund resolved refunded |
| REFUND_UNKNOWN | ACKNOWLEDGED | PAID | Refund rejected; original paid state retained |
| Either unknown state | UNKNOWN / empty / unrecognized | Any | Conflict; unknown state/fence unchanged |
| Any other pair or missing/mismatched evidence | Any | Any | Conflict; no resolution |

The matrix is checked before effects/state changes and again under the payment
row lock. Existing verified success/refund behavior remains; no blind retry or
fabricated success is added. Migration 0021 is unchanged.

## Native browser contract

The native config and workflow deliberately admit the same 14 files, including
automation2.native.spec.ts and ai2.native.spec.ts. Run:

```sh
python3 scripts/ci/check-native-selection.py
python3 scripts/ci/check-native-selection.py --enumerate
python3 -m unittest discover -s scripts/ci -p 'test_*.py'
```

The enumeration gate executes the actual Playwright native configuration for
each intended filename and requires a positive test count. Enumeration is not
browser acceptance; the existing workflow subsequently executes every suite.
No pass-with-no-tests, skipped assertion or removed spec is introduced.

## Version-scoped migrations

R4G remains exactly 0001–0023, ending 0023_ai_assistant_2.sql. R5B is exactly
0001–0025, ending 0025_apple_consumer_assistance.sql. The current checkpoint is
declared in db/migrations/checkpoint.go. Historical R4G integration checks retain
the first-23 boundary and add exact current applied-chain/checksum validation.
Fresh and recovered current chains must match all embedded names/checksums;
existing TestMigration checks runtime-role separation and checksum drift/reapply.
No old SQL migration is edited and no downgrade is introduced.

```sh
python3 scripts/ci/r4g-release-check.py --checkpoint current
python3 scripts/ci/r4g-release-check.py --checkpoint R4G
```

The second command is intended for an actual R4G source snapshot and correctly
refuses the R5B tree. It must not falsely PASS a 25-migration current tree as R4G.
README already documents twenty-five migrations and remains the current contract.

## Validation limits and remaining work

Use disposable local test services and synthetic fixtures only. Dependency/setup
or pre-runner failures are VALIDATION_ENVIRONMENT_BLOCKED, never source PASS/FAIL.
The 15 P2 findings and 8 NEEDS_VERIFICATION items remain untouched. In particular,
profile suspension policy, expiry repair, recovered business replay behavior and
other payment in-flight/order lifecycle changes belong to later authorization.
