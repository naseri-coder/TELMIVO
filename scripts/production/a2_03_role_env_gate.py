#!/usr/bin/env python3
"""A2-03: verify three EXCLUSIVE production env files, without DB or network I/O.

Never prints database URLs, user secrets or file contents.
This is a pre-cutover candidate gate; cannot certify live PostgreSQL grants.
"""
import argparse
import json
from pathlib import Path
from urllib.parse import parse_qs, urlsplit

from preflight import load_env, PreflightError

class RoleSeparationError(ValueError):
    pass

def deny(reason):
    raise RoleSeparationError(reason)

def verify_uri(raw, role):
    try:
        u = urlsplit(raw)
        qs = parse_qs(u.query, keep_blank_values=True, strict_parsing=True)
        if (u.scheme not in ("postgres", "postgresql")
            or not u.hostname or not u.port or not u.path or u.path == "/"
            or not u.username or u.username != role or not u.password
            or u.fragment or len(qs.get("sslmode", [])) != 1
            or qs["sslmode"][0] not in ("require", "verify-ca", "verify-full")
            or any(k in qs for k in ("host", "hostaddr", "port", "user", "password",
                                     "dbname", "service", "servicefile", "options", "role"))):
            deny("invalid exclusive production PostgreSQL role URL")
    except (ValueError, KeyError) as exc:
        raise RoleSeparationError("invalid exclusive production PostgreSQL role URL") from exc

def inspect(tenant: Path, worker: Path, migration: Path):
    files = (tenant, worker, migration)
    if len({p.resolve(strict=True) for p in files}) != 3:
        deny("role environment files must be separate physical files")
    envs = [load_env(p) for p in files]
    secrets = []
    for i, (env, login) in enumerate(zip(envs, ("telmivo_api", "telmivo_worker", "telmivo_migration"))):
        if env.get("TELMIVO_ENV") != "production" or env.get("SYNTHETIC_MODE") != "false":
            deny("all service environments must be explicit non-synthetic production")
        if i < 2:
            if env.get("TELMIVO_DB_ROLE_SPLIT") != "required":
                deny("API/Worker must explicitly require role split")
            if "MIGRATION_DATABASE_URL" in env:
                deny("migration credential exposed to application process")
            raw = env.get("DATABASE_URL", "")
            if "WORKER_DATABASE_URL" in env or "TENANT_DATABASE_URL" in env:
                deny("multiple database credentials exposed to application process")
        else:
            if "DATABASE_URL" in env:
                deny("API/Worker login leaked to migration process")
            raw = env.get("MIGRATION_DATABASE_URL", "")
        if any(k in env for k in ("PGHOST", "PGHOSTADDR", "PGUSER", "PGPASSWORD", "PGPORT",
                                   "PGDATABASE", "PGSERVICE", "PGSERVICEFILE", "PGOPTIONS", "PGSSLMODE")):
            deny("ambient libpq database overrides forbidden")
        verify_uri(raw, login)
        secrets.append(raw)
    if len(set(secrets)) != 3 or len({urlsplit(s).password for s in secrets}) != 3:
        deny("role credentials must be distinct")
    return {"status": "SOURCE_ROLE_ENV_FILES_DISTINCT_ONLY",
            "roles": ["telmivo_api", "telmivo_worker", "telmivo_migration"],
            "not_verified": ["database_server_login", "postgres_grants", "RLS",
                             "same-host_forwarding", "production_readiness"]}

def main():
    p = argparse.ArgumentParser()
    p.add_argument("--tenant", type=Path, required=True)
    p.add_argument("--worker", type=Path, required=True)
    p.add_argument("--migration", type=Path, required=True)
    args = p.parse_args()
    try:
        status = inspect(args.tenant, args.worker, args.migration)
        print(json.dumps(status, sort_keys=True))
    except (RoleSeparationError, PreflightError, OSError) as exc:
        print("A2_03_ROLE_ENV_DENIED: " + str(exc), file=__import__("sys").stderr)
        raise SystemExit(2)

if __name__ == "__main__":
    main()
