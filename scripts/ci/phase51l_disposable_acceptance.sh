#!/bin/sh
# LIC-A9 Phase51-L: independent, disposable TEST-ONLY native acceptance.
# This runner does NOT provision a server, use Actions or contact Authority.
# Do not use on Production machines or tunnels into actual databases.
set -eu
[ "${LIC_A9_DISPOSABLE:-}" = YES ] || { echo "LIC_A9_DISPOSABLE=YES required" >&2; exit 2; }
[ "${TELMIVO_ENV:-}" = test ] || { echo "TEST environment required" >&2; exit 2; }
[ "${TELMIVO_LICENSE_ENVIRONMENT:-}" = TEST ] || { echo "TEST-only license required" >&2; exit 2; }
[ "${SYNTHETIC_MODE:-}" = true ] || { echo "SYNTHETIC_MODE=true required" >&2; exit 2; }
[ "${TEST_NATS_URL:-}" = nats://127.0.0.1:4222 ] || { echo "Exact loopback TEST_NATS_URL required" >&2; exit 2; }
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd -P)
cd "$ROOT"
sh scripts/ci/check-test-target.sh
# Before any DB or broker connection, refuse the wrong toolchain.
command -v go >/dev/null 2>&1 || { echo "Go1.27.2 unavailable" >&2; exit 3; }
VERSION=$(GOTOOLCHAIN=local go version 2>/dev/null) || { echo "Cannot inspect local Go toolchain" >&2; exit 3; }
case "$VERSION" in
  "go version go1.27.2 linux/"*) ;;
  *) echo "REAL_PRODUCT_GO1.27.2_NOT_AVAILABLE: $VERSION" >&2; exit 3 ;;
esac
# This script does not start PostgreSQL or JetStream; use a completely
# disposable namespace and synthetic fixtures before invoking it.
python3 - <<'PY'
import socket
for name, port in (("TEST_POSTGRES", 5432), ("TEST_NATS", 4222)):
    with socket.socket() as s:
        s.settimeout(1)
        if s.connect_ex(("127.0.0.1", port)):
            raise SystemExit(name + "_LOOPBACK_SERVICE_NOT_AVAILABLE")
PY
# Full genuine package compilation with authentic Ed25519 V2 Manager.
# The real TestMain deliberately runs controlled PG migrations.
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off go test -tags=integration -race -count=1 -timeout=6m \
  ./internal/platform/installationlicense ./tests/integration
