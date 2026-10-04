#!/bin/sh
set -eu
# Reject accidental live configuration; do not source arbitrary env files.
test -f .env || { echo 'Run make bootstrap first'; exit 1; }
grep -qx 'TELMIVO_ENV=development' .env
grep -qx 'SYNTHETIC_MODE=true' .env
grep -qx 'DATABASE_URL=postgres://telmivo_runtime:dev-runtime-only@postgres:5432/telmivo_dev?sslmode=disable' .env
grep -qx 'MIGRATION_DATABASE_URL=postgres://telmivo_migration:dev-migration-only@postgres:5432/telmivo_dev?sslmode=disable' .env
