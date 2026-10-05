# TELMIVO production readiness — v0.1.1

This runbook hardens deployment packaging after the validated v0.1.0 source release.
It does not authorize production access or deployment. The v0.1.0 tag is immutable.

## Deployment model

compose.production.yaml is application-only. PostgreSQL, Redis and NATS are separately
provisioned production dependencies and must use encrypted transport. Normal application
startup never runs migrations or grants automatically. The maintenance profile exposes
explicit one-shot migrate and grants jobs for an authorized change window.

The standalone customer-bot-runtime shell is intentionally absent. Customer bot Telegram
updates enter through:

POST /api/v1/customer-bot-updates/{account}/{bot}

The public reverse proxy exposes three distinct official-bot webhook paths:

- /telegram/main/webhook
- /telegram/group/webhook
- /telegram/downloader/webhook

Each is proxied internally to that bot's /webhook endpoint. The ambiguous public /webhook
path returns 404.

## Mandatory preflight

Prepare a root/operator-readable environment file outside Git from .env.production.example,
mode 0600, then run:

~~~sh
python3 scripts/production/preflight.py \
  --env-file /secure/telmivo.env \
  --tls-cert /secure/fullchain.pem \
  --tls-key /secure/privkey.pem

docker compose --env-file /secure/telmivo.env \
  -f compose.production.yaml config --quiet
~~~

The preflight rejects mutable image tags, plaintext DB/Redis/NATS transports, synthetic mode,
missing official bot credentials, weak webhook configuration, invalid keyrings, temporary
media roots and missing TLS material. It never prints secret values.

All deployment images, including the PostgreSQL client used by the maintenance profile,
must be supplied as immutable @sha256 references. Build/publish provenance and registry
retention are deployment-environment responsibilities.

## Credential rotation

Production uses CREDENTIAL_KEYRING plus CREDENTIAL_KEY_ID. Existing encrypted bot tokens
remain decryptable while their historical key IDs remain in the ring; new tokens are sealed
with the active key. WEBHOOK_DIGEST_KEY is separate and stable, so rotating
credential-encryption keys does not change derived customer-bot webhook secrets.

Do not remove an old key while any database row still references its key ID. TELMIVO v0.1.1
does not provide a bulk re-encryption/retirement command; old-key retirement therefore
requires a separately validated procedure.

## Persistent media

Live media processing uses MEDIA_STORAGE_ROOT and MEDIA_WORK_ROOT. The production Compose
profile mounts named persistent volumes at /var/lib/telmivo/media and /var/lib/telmivo/work.
This is a single-host persistence model, not high availability. Production S3 remains a
future adapter and must not be implied by the reserved configuration fields.

## Database change window

Before any migration:

1. Record database identity, PostgreSQL version, runtime/migration role attributes and current
   schema_migrations names/checksums.
2. Verify the runtime role is neither superuser nor BYPASSRLS.
3. Take a fresh backup and restore it into a disposable PostgreSQL 17 target.
4. Compare canonical table counts/content identities and ledger balance using the same
   principles as scripts/ci/backup-restore.sh.
5. Decide the incident fallback before changing schema.

Only after those checks pass may an authorized operator run:

~~~sh
docker compose --env-file /secure/telmivo.env -f compose.production.yaml \
  --profile maintenance run --rm migrate

docker compose --env-file /secure/telmivo.env -f compose.production.yaml \
  --profile maintenance run --rm grants
~~~

The migration engine verifies released SHA256 checksums and serializes migration execution
with an advisory lock. Application startup does not invoke either maintenance job.

## Rollback policy

Default to forward-fix after a schema migration. Never automatically restore an older
database over a live system. Database restore is an incident operation only when the exact
backup, restore rehearsal, compatibility decision and traffic-stop plan are approved.

Application rollback may use the previous immutable image set only when its schema
compatibility with the current database has been explicitly established.

## Start and smoke

After the authorized change window:

~~~sh
docker compose --env-file /secure/telmivo.env -f compose.production.yaml up -d
~~~

Read-only smoke checks must cover HTTPS, API readiness, Mini App load, official bot readiness,
Telegram webhook configuration, customer-bot ingress authentication, queue connectivity and
database/Redis/NATS health. Do not create customer data merely to prove deployment.

## Operational baseline

Before public traffic, define alerts for sustained CPU/memory pressure, disk free space and
inode pressure, container restart loops, API 5xx/rate-limit growth, PostgreSQL connection
saturation, Redis/NATS connectivity, JetStream storage/consumer backlog, outbox backlog,
failed/ambiguous external deliveries and media-worker failure rate.

Capacity numbers are environment-specific. No production capacity claim is made by source CI.

## Release rule

Any source or deployment-packaging change after v0.1.0 must ship under a new immutable
version. Do not move or recreate the v0.1.0 tag.
