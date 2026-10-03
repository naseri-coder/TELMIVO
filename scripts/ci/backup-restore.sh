#!/bin/sh
set -eu
# Disposable CI-only procedure v1. No arbitrary remote databases are accepted.
[ "${GITHUB_ACTIONS:-}" = true ] || { echo 'GitHub disposable drill required'; exit 1; }
[ "${PGDATABASE:-}" = telmivo_test ] || exit 1
[ "${PGHOST:-}" = 127.0.0.1 ] || exit 1
[ "${PGUSER:-}" = telmivo_migration ] || exit 1
[ "${PGPORT:-}" = 5432 ] || exit 1
pg_dump --format=custom --no-owner --no-acl --file=synthetic-backup.dump
psql -X -v ON_ERROR_STOP=1 -At -f scripts/ci/restore-manifest.sql > before-restore.txt
# Restore target is a separate, fresh PostgreSQL service.
PGPORT=5433 psql -X -v ON_ERROR_STOP=1 -At -c "SELECT count(*) FROM pg_tables WHERE schemaname='public'" | awk '$0!=0{exit 1}'
PGPORT=5433 pg_restore --exit-on-error --no-owner --no-acl --dbname=telmivo_test synthetic-backup.dump
PGPORT=5433 psql -X -v ON_ERROR_STOP=1 -At -f scripts/ci/restore-manifest.sql > after-restore.txt
diff -u before-restore.txt after-restore.txt
PGPORT=5433 psql -X -v ON_ERROR_STOP=1 -f infrastructure/postgres/grants.sql
MIGRATION_DATABASE_URL="$TEST_RESTORE_MIGRATION_DATABASE_URL" go run ./cmd/migrate
# Dumps contain synthetic credentials/session data and are deliberately not uploaded.
rm synthetic-backup.dump
echo 'Fresh restore: canonical table identities/counts and balanced ledger PASS'
