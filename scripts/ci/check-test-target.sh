#!/bin/sh
set -eu
# Exact native target admission without connecting to any service.
# Reject URI query-host overrides, remote DNS aliases, production DB names,
# wrong migration roles, and inherited non-test destinations.
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
exec python3 "$SCRIPT_DIR/phase51l_test_target.py"
