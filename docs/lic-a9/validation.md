# LIC-A9-B non-production validation

Protocol: `TELMIVO_CONCURRENT_INSTALLATION_LEASE_V1`.
Authority parent: `392fc89087d8dac001d1709d0f7973c2f28a8515`.
TELMIVO parent: `e87e85d9cd44470ac46fb73732ca2fed59bf229b`.

The authoritative LIC-A7/LIC-A8 validations and the LOCK-1..LOCK-10 checkpoint are retained. The lock checkpoint evidence was neither replaced nor rerun. The hot transaction remains Authority SHARE, installation lease UPDATE, immutable request lookup, capacity reservation, intent/request insertion, lease advancement. Audit finalization retains its separate Authority UPDATE, intent UPDATE, canonical chain append, finalization and capacity release transaction.

The shared request, claims, verification, protocol document and vectors are byte-identical. A fresh request also binds the unpredictable previous signed lease ID: a copied private key cannot guess the next public generation and obtain another lease without the current ownership evidence. Public M2M status does not reveal that ownership evidence.

AC-1 through AC-30 each PASS in the final race-enabled cross-repository run. All 15 protocol vectors PASS against both implementations, including real authenticated local TLS requests. Tests use disposable PostgreSQL, isolated local state directories and explicitly synthetic/test-only signer identities. Backup/restore is an actual schema dump and isolated restore followed by recovery epoch fencing. No production backup, signer custody or deployment is claimed.

## Acceptance closure

| Case | Result | Scenario |
|---|---|---|
| AC-1 | PASS | Fresh activation |
| AC-2 | PASS | Sequential refresh |
| AC-3 | PASS | Exact retry, no second mutation |
| AC-4 | PASS | Same generation race |
| AC-5 | PASS | Different clone request |
| AC-6 | PASS | Complete state-directory copy |
| AC-7 | PASS | Signed lease replay |
| AC-8 | PASS | Old generation replay |
| AC-9 | PASS | Snapshot restore |
| AC-10 | PASS | Fresh runtime session on restart |
| AC-11 | PASS | Response loss after committed preparation |
| AC-12 | PASS | Out-of-order response |
| AC-13 | PASS | Authority store restart |
| AC-14 | PASS | Client restart/high-water persistence |
| AC-15 | PASS | Bounded offline outage |
| AC-16 | PASS | Legitimate reconnect |
| AC-17 | PASS | Stale clone reconnect |
| AC-18 | PASS | Copied private key cannot coexist indefinitely online |
| AC-19 | PASS | Confirmed retirement permanently fences old key |
| AC-20 | PASS | One-time replacement/new identity |
| AC-21 | PASS | Independent authorized installations |
| AC-22 | PASS | Shared session string does not cross-bind |
| AC-23 | PASS | Epoch fence |
| AC-24 | PASS | Revision fence |
| AC-25 | PASS | Suspend/resume |
| AC-26 | PASS | Revocation |
| AC-27 | PASS | Commercial expiry |
| AC-28 | PASS | Malformed/unsigned/wrong-installation evidence |
| AC-29 | PASS | Wrong Root/delegated signature |
| AC-30 | PASS | Backup/restore and forward recovery fencing |

Mandatory acceptance skips: 0. Final source/evidence scan: REAL_SECRET_FINDINGS=0. Evidence remains outside both repositories. The frozen audit backlog limit is 256 pending intents and five minutes oldest age; exact committed retries remain recoverable. Production readiness exposes both metrics.

The affected startup test uses a controlled test-only time probe and asserts the unchanged production disk fence fails closed on a small staging disk. It is credential/registry validation, not evidence of production host readiness. Its epoch mismatch setup now follows the required committed HOLD, drain, epoch advance sequence.

## Production boundary

No production host or FQDN has been explicitly designated. No real Root key, online signer, certificate, DB credentials, operator token, admin allowlist, encrypted off-host backup destination/recipient or monitoring destination has been supplied and verified for this stage. All remain production-gate inputs. No production mutation, merge, DNS/TLS/firewall change or deployment is authorized here.
