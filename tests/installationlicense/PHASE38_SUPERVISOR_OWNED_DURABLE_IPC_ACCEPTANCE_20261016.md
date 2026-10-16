# LIC-A9 Phase 38 — Root-owned durable bridge / multi-UID Linux proof

Date: 2026-10-16. Scope: strictly **nonproduction** standalone Linux Go test module. **Production NO-GO.**

## Provenance

- Product parent: Phase37 Draft PR #90 HEAD `0c6b39700c8e3cfa4ec31c33fcfe80b6760e14ad`.
- Phase38 branch: `development/lic-a9-phase38-root-supervisor-durable-ipc-20261016` (stacked on Phase37, no merge).
- Root Product Go module and license runtime untouched. Authority PR #57 and both `main` branches unchanged.
- New nested module: `tests/installationlicense/phase38_supervisor_durable_bridge` requiring only Go standard library; built under Linux Go 1.23.2 with no network, server, Actions, customer credentials or external effect.

## Actual Linux results

**Genuine separate-process OS UID security test**, not a simulated UID:
- Root-owned supervisor with protected journal directory 0700, root:worker socket directory 0750, Unix socket 0660; actual child worker UID **25111**, disallowed outsider UID **25112**.
- Kernel `SO_PEERCRED` verifies connecting client UID; outsider rejected; unprivileged child cannot unlink or overwrite root-owned journal, lock, epoch or Unix socket.
- Synthetic signed claim, installation, owner, session, tenant, actor and resource checks; adversarial forged signature and cross-tenant denial.
- Root-owned file journal with `O_NOFOLLOW`, exclusive record creation, `fsync` file + directory, `flock`, fail-closed corruption and idempotency collision handling; synthetic signed provider confirmation only, never live effects.
- `readyEpoch` intentionally volatile: a restarted supervisor rejects a prior signed session and will not accept a fresh synthetic APPLY until a **higher** signed epoch is fenced. Pending unknown outcome prevents new fencing until synthetic verified receipt is present.
- Multiple independent worker OS subprocesses concurrently submitting one request produce **one durable reservation**, with all other attempts denied.
- Abrupt root supervisor subprocess exit **after fsync** (exit 77), trusted stale socket cleanup, reopen journal, rejected replay and blocked new epoch pending reconciliation.
- Ancestor hardening: writable nonsticky intermediate directory and symlink intermediate directory both rejected before opening a supposedly root-owned endpoint.

Executed on isolated Linux/amd64 root test fixture:

```
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -race -count=1 -timeout=180s -v ./...
# PASS, 15 named functions: 13 substantive cases + 2 subprocess harness helpers
# ok telmivo.local/lic-a9-phase38-supervisor-durable-bridge 38.504s
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go vet ./...
# PASS
```

- Complete Go test log SHA256: `76548662b8ef75250b538cf2a9c89d4a9cd4b924d55ce94fb3a211dba28173fb`.
- Go source Git blob SHAs (exact locally tested source):
  - `bridge.go`: `5c275f985f6ede2411fa812769661d0393881da8`.
  - `bridge_test.go`: `215abd500e6f10b019365d8ea3463e9fc87af797`.
  - `go.mod`: `d8888e69d5ba4adb281745494b043be553533ae5`.
- `gofmt -l bridge.go bridge_test.go` produced no output.

## Mandatory non-waivable remaining gates

**No Product `Manager.Effect` invocation in the Phase38 Linux proof.** The local file reservation is the sole synthetic effect; this is NOT an operational Supervisor/Owner service and NOT actual concurrent Product Workers. No real Authority V2 credentials, PostgreSQL16 E2E, provider-side epoch fencing, signed installer or release certification. The 5 genuine Product hardlink-defense tests and Go1.27 Linux full Product Race suite remain NOT_RUN. Android/Termux Go1.27 proved earlier Phase37 behavior, but Android cannot run this root Linux multi-UID test.

A signed future FENCE request is not bound to a server-generated fresh-boot nonce; full replay prevention across process generations needs an additional trusted Authority/boot challenge protocol. The check here enforces monotonically increasing persisted epoch and restart quarantine, NOT a complete production session grant design. Supervisor trusted startup, provisioning, root ownership under hostile mount namespaces, PID identity and worker peer lifetimes remain deployment blockers.

**Acceptance: independent Linux syscall, UID and durable synthetic journal CONTRACT PASS; REAL Product integration NOT_RUN; Production NO-GO.** Stop after Phase38. Keep all PRs Draft and no Merge, Actions or deployment.
