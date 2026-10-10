#!/usr/bin/env bash
# NON-PRODUCTION ONLY. Isolated PostgreSQL 17 synthetic backup/restore smoke drill.
# This script does NOT call the TELMIVO Production database, read env files or start timers.
set -Eeuo pipefail
umask 077
command -v docker >/dev/null || { echo 'BLOCKED_DOCKER_UNAVAILABLE'; exit 21; }
docker image inspect postgres:17-bookworm >/dev/null 2>&1 || {
  echo 'BLOCKED_POSTGRES17_LOCAL_IMAGE_ABSENT; no image pulling authorized'
  exit 22
}
root="$(mktemp -d /tmp/telmivo-pg17-synthetic.XXXXXXXX)"
cid="telmivo-backup-pg17-synthetic-$$"
started=0
cleanup() {
  if [[ "$started" == 1 ]]; then docker stop -t 1 "$cid" >/dev/null 2>&1 || true; fi
  rm -rf -- "$root"
}
trap cleanup EXIT
docker run --rm --detach --name "$cid" --network none \
  --cpus=0.50 --memory=240m --pids-limit=70 \
  --security-opt no-new-privileges \
  --tmpfs /var/lib/postgresql/data:rw,nosuid,nodev,size=160m \
  -e POSTGRES_HOST_AUTH_METHOD=trust \
  postgres:17-bookworm >/dev/null
started=1
ready=0
for _ in $(seq 1 90); do
  if docker logs "$cid" 2>&1 | grep -q 'PostgreSQL init process complete; ready for start up.' &&
     docker exec "$cid" psql -X -w -At -U postgres -d postgres -c 'SELECT 1' >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep .4
done
[[ "$ready" == 1 ]] || { echo 'BLOCKED_PG17_ISOLATED_STARTUP'; exit 23; }
docker exec "$cid" psql -X -w -U postgres -d postgres -v ON_ERROR_STOP=1 -c 'CREATE DATABASE telmivo_synthetic' >/dev/null
docker exec "$cid" psql -X -w -U postgres -d postgres -v ON_ERROR_STOP=1 -c 'CREATE DATABASE restored_synthetic' >/dev/null
docker exec -i "$cid" psql -X -w -U postgres -d telmivo_synthetic -v ON_ERROR_STOP=1 >/dev/null <<'SQL'
CREATE TABLE synthetic_users (fixture_id integer PRIMARY KEY, kind text NOT NULL, generation integer NOT NULL);
INSERT INTO synthetic_users VALUES (1,'ACTIVE',1),(2,'REVOKED',8),(3,'RETIRED',4);
CREATE TABLE synthetic_audit (fixture_id integer PRIMARY KEY, intent text NOT NULL);
INSERT INTO synthetic_audit VALUES (1,'PENDING'),(2,'FINALIZED');
CREATE TABLE synthetic_binary (fixture_id integer PRIMARY KEY, payload bytea NOT NULL);
INSERT INTO synthetic_binary VALUES (1,decode('01020300feff','hex'));
SQL
docker exec "$cid" pg_dump -U postgres -w --format=custom --no-owner --no-acl \
  -d telmivo_synthetic > "$root/synthetic.pg17.dump"
[[ "$(wc -c < "$root/synthetic.pg17.dump")" -gt 100 ]] || {
  echo 'FAIL_EMPTY_SYNTHETIC_DUMP'; exit 24
}
docker exec -i "$cid" pg_restore -U postgres -w --exit-on-error --no-owner --no-acl \
  -d restored_synthetic < "$root/synthetic.pg17.dump"
read_hash() {
  docker exec "$cid" psql -X -w -At -U postgres -d "$1" -c "$2"
}
source_users="$(read_hash telmivo_synthetic \
  "SELECT md5(string_agg(fixture_id::text||':'||kind||':'||generation,'|' ORDER BY fixture_id)) FROM synthetic_users;")"
restore_users="$(read_hash restored_synthetic \
  "SELECT md5(string_agg(fixture_id::text||':'||kind||':'||generation,'|' ORDER BY fixture_id)) FROM synthetic_users;")"
source_audit="$(read_hash telmivo_synthetic \
  "SELECT md5(string_agg(fixture_id::text||':'||intent,'|' ORDER BY fixture_id)) FROM synthetic_audit;")"
restore_audit="$(read_hash restored_synthetic \
  "SELECT md5(string_agg(fixture_id::text||':'||intent,'|' ORDER BY fixture_id)) FROM synthetic_audit;")"
restore_binary="$(read_hash restored_synthetic \
  "SELECT encode(payload,'hex') FROM synthetic_binary WHERE fixture_id=1;")"
[[ "$source_users" == "$restore_users" ]]
[[ "$source_audit" == "$restore_audit" ]]
[[ "$restore_binary" == 01020300feff ]]
echo "PG17_VERSION=$(docker exec "$cid" pg_dump --version)"
echo 'ISOLATED_PG17_BACKUP_RESTORE=PASS'
echo 'ISOLATED_FIXTURE_HASHES=PASS'
echo "SYNTHETIC_DUMP_BYTES=$(wc -c < "$root/synthetic.pg17.dump")"
echo "SYNTHETIC_SHA256=$(sha256sum "$root/synthetic.pg17.dump" | cut -d ' ' -f1)"
echo 'PRODUCTION_DATABASE_ACCESSED=FALSE'
echo 'PRODUCTION_SERVICE_CHANGED=FALSE'
echo 'AGE_PIPELINE_PG17_QUALIFIED=FALSE'
echo 'TERABOX_CONNECTED=FALSE'
