# LIC-A8 installation client

Parent: `7fc6d5a4cd7f9f66fe259b402d34cb75bd2ab75d`.
Authority protocol: `392fc89087d8dac001d1709d0f7973c2f28a8515`.
The installation license supplements customer entitlements, quotas and RBAC.
It never replaces them. No Authority source or private signing key is shipped.

Every application role enters `runtime.Run`, installs the central manager before
business resources, and starts restricted when trust/configuration/state is
missing or invalid. Read-only diagnosis and health remain available. Restricted
GET requests use read-only tenant transactions; any attempted write fails.
Commercial mutations are forbidden. Authenticated, parsed Telegram webhooks ACK
without business execution, including customer-bot updates. Invalid secrets and
malformed updates still fail. Workers pause admission; queue delivery is not
fetched while restricted. Existing durable attempt/outcome fences remain intact.

External Telegram, media HTTP, AI and extractor/FFmpeg dispatches perform a fresh
central check at the effect boundary. Checks/state updates serialize across
processes through the installation lock. An already dispatched bounded operation
can finish and persist its outcome; settlement context cannot grant another
external effect. Migrations and license administration remain administrative
commands. The existing disabled payment provider remains disabled.

## Explicit configuration

- `TELMIVO_LICENSE_STATE_DIR`: absolute dedicated writable directory, outside Git.
- `TELMIVO_LICENSE_TRUST_FILE`: externally provisioned public root set, regular
  non-symlink file, root/current-user owned and not group/world writable.
- `TELMIVO_LICENSE_AUTHORITY_ID`: pinned Authority UUID.
- `TELMIVO_LICENSE_ENVIRONMENT`: `PRODUCTION`, `DEVELOPMENT`, or `TEST`.
- `TELMIVO_LICENSE_AUTHORITY_URL`: HTTPS origin without path/query/credentials.

Production requires a `PRODUCTION` trust purpose and environment. No root fetching,
license bypass, insecure HTTP or production test-clock option exists. Trust JSON
schema is RootSet/RootRecord in roots.go: version 1, purpose, roots array with id,
version, standard-base64 public_key, RFC3339 not_before/not_after and status.
The operator provisions public trust independently; do not create signer material.
All roles on one installation must share its directory and OS user. Directories
are 0700, private files 0600. Back up identity/state/high-water together securely.
Corruption, partial loss, unsafe permissions and wrong identity/key fail closed.
Do not delete identity to resolve a licensing error or copy it to another install.

`licensectl identity` returns only the installation ID/public key; `status` returns
local decision. Activate with `licensectl activate LICENSE_UUID AUTHORITY_EPOCH`,
passing the activation code through stdin. Never put the code in argv or logs.
`refresh`, `authority-status`, and `deactivate` use signed installation proofs.
Pending activation secrets live only in the protected state until completion;
retry preserves the semantic body/idempotency key with a fresh nonce/timestamp.

Refresh follows signed refresh_after, independently of commercial product plans.
Temporary outages preserve the signed online/offline deadlines. Reconnection can
recover only through a newly verified lease. Status can restrict/raise floors,
never grant or clear restriction. Epoch/revision/generation/issuance/time floors
persist; observed expiry cannot be revived by rolling the local clock backwards.
There is no claim of hardware-bound clone protection or resistance to a privileged
attacker restoring the entire installation directory plus clock. Authority and
customer checks remain the final defense against such full-state rollback.

## Non-production acceptance

`tests/installationlicense/prepare_authority_fixture.py` archives exact LIC-A7 into
a NEW directory outside both repositories and copies the E2E template. Its only
store adaptation injects a TEST business/lease clock (`lic_a8_clock()`); transport
replay timestamps remain real. Set LIC_A8_TEST_DATABASE_URL to an exclusively
NON-PRODUCTION disposable database. Each TestE2E1..6 creates a fresh schema,
TEST-only root/signer/identity and a TLS loopback Authority; teardown removes state.
Run phases individually with separate logs/exit files and bounded timeouts. Never
use production DBs, credentials, customer data or signing material. Injected clock
advances cover offline/expiry without waiting real 24h/72h periods.
