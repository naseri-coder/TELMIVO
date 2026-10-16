#!/usr/bin/env bash
# LIC-A9 Phase51-I: opt-in, disposable, native browser API launcher.
# Not a production installer; NEVER run on an existing TELMIVO installation.
set -Eeuo pipefail

[[ "${TELMIVO_LIC_A9_BROWSER_TEST:-}" == "YES" ]] || { echo "Browser test opt-in missing" >&2; exit 2; }
[[ "${TELMIVO_ENV:-}" == "test" && "${TELMIVO_LICENSE_ENVIRONMENT:-}" == "TEST" ]] || { echo "Only TEST permitted" >&2; exit 2; }
[[ "${SYNTHETIC_MODE:-}" == "true" ]] || { echo "Synthetic mode required" >&2; exit 2; }
[[ "${HTTP_ADDR:-}" == "127.0.0.1:8080" ]] || { echo "Loopback HTTP required" >&2; exit 2; }
[[ "${MINIAPP_URL:-}" == "http://127.0.0.1:5173" ]] || { echo "Local MiniApp required" >&2; exit 2; }
[[ "${REDIS_URL:-}" == "redis://127.0.0.1:6379/15" ]] || { echo "Local Redis required" >&2; exit 2; }
[[ "${NATS_URL:-}" == "nats://127.0.0.1:4222" ]] || { echo "Local NATS required" >&2; exit 2; }
[[ -n "${TEST_DATABASE_URL:-}" && "${DATABASE_URL:-}" == "${TEST_DATABASE_URL:-}" && -n "${TEST_MIGRATION_DATABASE_URL:-}" ]] || { echo "Isolated TEST DB URLs required" >&2; exit 2; }
[[ "${TELMIVO_LICENSE_TRUST_FILE:-}" == "" && "${TELMIVO_LICENSE_STATE_DIR:-}" == "" && "${TELMIVO_LICENSE_AUTHORITY_URL:-}" == "" ]] || { echo "Real installation trust/state must not be used" >&2; exit 2; }

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
cd "$ROOT"
umask 077
TMPDIR="$(mktemp -d)"
cleanup() { rm -rf -- "$TMPDIR"; }
trap cleanup EXIT INT TERM

# Explicitly compiling a separate test-only API, not overwriting a Product
# release binary. Startup still revalidates every environment/URL invariant.
GOTOOLCHAIN=go1.27.2 go build -tags=lic_a9_browser_test -trimpath \
  -o "$TMPDIR/telmivo-lic-a9-browser-test-api" ./cmd/api

# The API creates a new TEST V2 signed runtime in this SAME process.
# This script neither migrates a DB nor starts any PG/NATS/Redis service.
"$TMPDIR/telmivo-lic-a9-browser-test-api"
