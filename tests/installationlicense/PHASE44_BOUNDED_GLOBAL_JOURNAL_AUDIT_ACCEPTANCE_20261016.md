# LIC-A9 Phase44 — bounded journal enumeration and adversarial filesystem checks

Date: 2026-10-16. Scope: **strictly TEST-ONLY Product `*_test.go` changes**. No production License service, root Go module or Authority modified.

**STATUS: SOURCE VERIFIED / Linux Go1.23.2 local stub race suite PASS / real Product Go1.27 Linux and Android Phase44 NOT_RUN / PRODUCTION NO-GO.**

## Verified ancestry

- Phase43 Draft PR #97 parent commit `32517987785996db7f780dbdab49467fb6686fff`.
- Phase44 branch `development/lic-a9-phase44-bounded-journal-audit-20261016`; no `main` merge.
- The modified global pending scanner and two new adversarial files are under `internal/platform/installationlicense`, with no runtime implementation edits.

## Specific security issue fixed in test-only journal scanner

The Phase43 scanner first called `Readdirnames(-1)` (read unlimited names and allocate memory), then rejected journal directories with more than 4096 entries. This had an unnecessary **unbounded allocation before quota enforcement**. Phase44 changes it to `Readdirnames(4097)` and refuses if more than 4096 entries are returned or read fails. The scan remains anchored to a verified directory descriptor and performed under the shared journal `flock`. This is **a bounded-resource safety improvement to the synthetic test journal**, not an operational root-owned Supervisor repair.

The redundant check `bytes.Equal(phase37Encode(receipt), phase37Encode(&receipt))` was removed because it compared a value with itself. It did not establish authenticity. Validity remains enforced by `j.read`'s canonical byte check plus matching fields and Ed25519 verification of the provider-signed receipt. The implementation retains refusal of orphaned or malformed files.

## Exact new/modified Git source provenance

| Path | Git blob SHA | SHA256 of locally race-tested bytes |
|---|---|---|
| `internal/platform/installationlicense/phase43_unresolved_global_journal_test.go` (modified) | `c7037fb4f79a2733ab83c566e6dab00edcb1d8a2` | `e5fdf788ccbd413d06e7ffb22473f0fad2d6f9d5a470cf8d888a1f4cdfd00ec7` |
| `internal/platform/installationlicense/phase44_adversarial_journal_test.go` (new, portable) | `4d5046314f32a23eecf3253c61f133919a5d91d5` | `852dfe0da558a0ee2667024604ce97772d801a715d9b258fdbaada8aad56281f` |
| `internal/platform/installationlicense/phase44_linux_hardlink_test.go` (new, Linux-only) | `f5aa72aa231c48ff8978d7cabade073b3422c221` | `8412b7529485cd13dad9ee9b34bb930b20d2d58ef7b81dffa6708fa6d101db73` |

All three blobs were separately retrieved from the GitHub development branch after direct connector commits and **matched the exact local test source**.

## Actual test results — isolated Linux/amd64 Go1.23.2

The test fixture uses an explicitly labeled **compatible STUB Manager** and disposable synthetic signed Provider and Journal records. It is **NOT the actual Product Manager.Effect**. The stub initially omitted the real Product Phase37 `read()` check `Nlink == 1` and therefore the first Linux hardlink test failed **in the mock**. We repaired the local stub to mirror the real Product implementation's `Fstat` checks (regular file, single hardlink, owner, permissions, size); did not modify Product journal code to hide a failure. The correction is local testing-fixture fidelity evidence, **not** proof the real Product passed.

Executed (exit code 0):

```sh
cd /mnt/data/lic44/stub
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off \
  go test -race -count=5 -timeout=240s -run '^TestLICA9Phase4[234]' -v ./...
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off go vet ./...
gofmt -l *.go
```

- **22 distinct named tests × 5 iterations = 110 PASS**, no SKIP or FAIL, no Race Detector errors, `ok ... 4.567s`.
- **Go Vet PASS**, **gofmt clean**.
- Full suite log SHA256 `dc1619fd963fadf111c35183686327fd59d1c394544442e2a136da24cd52ef3f`.
- Six Phase44-only cases also **6/6 PASS** after fixing the local STUB's fidelity, log SHA256 `4593c528838eb46321173b3d8a6438bebd55be8b6b6f87eba7b475703485468c`.

The six cases verify: directory overbudget rejection before effect; valid synthetic provider receipt via symlink rejected using `O_NOFOLLOW`; physically persisted fake signed receipt cannot clear global uncertainty; orphan even-correctly-signed confirmation cannot rearm; mutated canonical reservation cannot match authentic signed receipt; native Linux **hardlinked reservation** rejected by real-file descriptor `Nlink` predicate mirrored in the local test fixture.

## Explicit non-waivable blockers

- Phase43/44 test code with **genuine Product Go1.27 Manager.Effect** on Android and Linux: **NOT_RUN**. We did not ask the user to execute Termux, honoring their time constraints.
- Linux Go>=1.27 real root-owned multi-UID Supervisor integration + `-race`, Product hardlink suite, real Authority fresh boot signer/revocation, PostgreSQL16 multiworker, independent external provider receipts and outbox, signed installer and security review remain pending.
- Attacker with effective journal owner UID may delete the entire directory or mutate arbitrary files: anchored `flock` / signed synthetic receipts alone **cannot** defend against the same-UID owner. Strong isolation and an independent Authority/provider remain necessary.
- No claims of operational exactly-once delivery, genuine license release authorization or Production readiness.

**DECISION: PHASE44_SYNTHETIC_JOURNAL_BOUNDING_AND_ADVERSARIAL_STUB_RACE_PASS / REAL_PRODUCT_GO127_NOT_RUN / PRODUCTION NO-GO. Keep PR Draft; no merge, Actions, Production server or deployment. Stop after Phase44.**
