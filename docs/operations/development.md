# Isolated development and recovery

Prerequisites: Docker Compose v2, Go version from go.mod, Node 24/npm. No OS installation or production credentials are required by R1. If Docker is unavailable, Native Infrastructure GitHub Actions is the supported isolated validation path.

```
make bootstrap
make up
```

Compose exposes only API `127.0.0.1:8080` and Mini App `127.0.0.1:5173`. PostgreSQL, Redis and NATS have no host ports. Migration and grants complete before runtime processes start. API plus all bot/worker shells share a Go image with FFmpeg available. Synthetic bot `/webhook` returns 501, synthetic health identifies disabled mode; this does not claim live operation. Empty BOT_TOKEN means Mini App login remains unavailable until a synthetic locally signed fixture is used. No real Telegram account/token is necessary for native service tests.

The tracked dev passwords are exclusively disposable fixtures, never production credentials. `make up`/`make migrate` reject a live .env. PostgreSQL uses distinct migration (NOSUPERUSER/BYPASSRLS) and runtime (NOSUPERUSER/NOBYPASSRLS/NOINHERIT) roles. The runtime is not a member of migration. Existing initialized volumes do not rerun init.sql; do not silently erase volumes to repair grants. Review and apply grants.sql through the dev migration role instead.

`make build`, `make test`, `make lint`, `make vet`, `make security-check` check source. `make test-integration`, `make test-financial`, `make migration-check` require TEST_DATABASE_URL and TEST_MIGRATION_DATABASE_URL targeting local `telmivo_test` plus TEST_REDIS_URL/TEST_NATS_URL. The CI workflow demonstrates fresh isolated provisioning and explicit role grants. These tests intentionally reject arbitrary/production databases.

## Recovery

Stop workers gracefully before maintenance. Consumer cancellation waits for goroutines before closing the database. A failed transaction rolls back inbox and business state together; an acknowledgement lost after commit safely redelivers into the inbox guard. Durable consumers are explicitly created and bound, so subscription shutdown does not delete them. Outbox lease ownership fences SQL completion and retry releases; lost publication acknowledgements may republish the same event ID. JetStream deduplication supplements permanent PostgreSQL inbox deduplication.

Outbox failures release the owned lease and delay another attempt, capped at 60 seconds. If a process dies, its lease becomes claimable after 60 seconds. Consumers explicitly acknowledge committed effects, retry handler failures with bounded delay and store a scrubbed DLQ record before terminating delivery. Malformed events use a stable stream-sequence digest, never an invalid UUID or raw payload containing possible secrets. Inspect fixed error codes and correlation IDs; repair the cause before any deliberate requeue. DLQ requeue administration is deferred.

Compose PostgreSQL and NATS use named local volumes. `make down` preserves them. Do not use `down -v` as routine recovery. Backup/restore automation and restore drills remain outside validated R1 scope; no backup guarantee is claimed. A released migration checksum mismatch stops startup migration and must be investigated; never overwrite historical SQL or checksums to silence it.
