#!/usr/bin/env python3
import argparse
import base64
import re
import stat
import sys
from pathlib import Path
from urllib.parse import parse_qs, urlparse
from phase51k_fixture_gate import TestFixtureInProduction, reject_test_fixture_configuration

IMAGE = re.compile(r"^[^\s@]+@sha256:[0-9a-f]{64}$")
DOMAIN = re.compile(r"^(?=.{1,253}$)(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$")
TOKEN = re.compile(r"^[1-9][0-9]{4,19}:[A-Za-z0-9_-]{30,80}$")
KEY_ID = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$")

class PreflightError(Exception):
    pass

def load_env(path: Path):
    if not path.is_file():
        raise PreflightError("environment file missing")
    mode = stat.S_IMODE(path.stat().st_mode)
    if mode & 0o077:
        raise PreflightError("environment file permissions must be 0600 or stricter")
    out = {}
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("export ") or "=" not in line:
            raise PreflightError("environment file contains unsupported syntax")
        key, value = line.split("=", 1)
        key = key.strip()
        value = value.strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in {"'", '"'}:
            value = value[1:-1]
        if not re.fullmatch(r"[A-Z][A-Z0-9_]*", key) or key in out:
            raise PreflightError("environment file contains invalid or duplicate keys")
        out[key] = value
    return out

def required(env, name):
    value = env.get(name, "")
    if not value or value == "REPLACE" or "REPLACE_" in value:
        raise PreflightError(f"{name} is not configured")
    return value

def key32(raw, name):
    try:
        value = base64.b64decode(raw, validate=True)
    except Exception as exc:
        raise PreflightError(f"{name} is not a valid base64 key") from exc
    if len(value) != 32:
        raise PreflightError(f"{name} must decode to 32 bytes")
    return value

def secure_db(raw, name):
    u = urlparse(raw)
    if u.scheme not in {"postgres", "postgresql"} or not u.hostname:
        raise PreflightError(f"{name} is invalid")
    sslmode = parse_qs(u.query).get("sslmode", [""])[0]
    if sslmode not in {"require", "verify-ca", "verify-full"}:
        raise PreflightError(f"{name} must require TLS")

def main():
    p = argparse.ArgumentParser()
    p.add_argument("--env-file", required=True)
    p.add_argument("--tls-cert")
    p.add_argument("--tls-key")
    args = p.parse_args()

    env = load_env(Path(args.env_file))
    reject_test_fixture_configuration(env)
    if required(env, "TELMIVO_ENV") != "production" or env.get("SYNTHETIC_MODE") != "false":
        raise PreflightError("production environment mode is not fail-closed")

    # Source-stage A2-03: production Compose now REQUIRES exclusive role
    # env files. Never approve a legacy shared-credential release by omission.
    if env.get("TELMIVO_DB_ROLE_SPLIT") != "required":
        raise PreflightError("separate PostgreSQL API/worker credentials required")
    from a2_03_role_env_gate import inspect, RoleSeparationError
    try:
        inspect(
            Path(required(env, "TELMIVO_TENANT_ENV_FILE")),
            Path(required(env, "TELMIVO_WORKER_ENV_FILE")),
            Path(required(env, "TELMIVO_MIGRATION_ENV_FILE")),
        )
    except (RoleSeparationError, OSError) as exc:
        raise PreflightError("database role environment admission denied") from exc

    secure_db(required(env, "DATABASE_URL"), "DATABASE_URL")
    secure_db(required(env, "MIGRATION_DATABASE_URL"), "MIGRATION_DATABASE_URL")
    if urlparse(required(env, "REDIS_URL")).scheme != "rediss":
        raise PreflightError("REDIS_URL must use rediss")
    if urlparse(required(env, "NATS_URL")).scheme != "tls":
        raise PreflightError("NATS_URL must use tls")

    domain = required(env, "TELMIVO_DOMAIN").lower()
    if not DOMAIN.fullmatch(domain):
        raise PreflightError("TELMIVO_DOMAIN is invalid")
    mini = urlparse(required(env, "MINIAPP_URL"))
    if mini.scheme != "https" or mini.hostname != domain or mini.username or mini.password or mini.query or mini.fragment:
        raise PreflightError("MINIAPP_URL must be the HTTPS production domain")

    for name in ("BOT_TOKEN", "GROUP_BOT_TOKEN", "DOWNLOADER_BOT_TOKEN"):
        if not TOKEN.fullmatch(required(env, name)):
            raise PreflightError(f"{name} has invalid syntax")
    if len(required(env, "WEBHOOK_SECRET")) < 32:
        raise PreflightError("WEBHOOK_SECRET is too short")
    key32(required(env, "WEBHOOK_DIGEST_KEY"), "WEBHOOK_DIGEST_KEY")

    active = required(env, "CREDENTIAL_KEY_ID")
    if not KEY_ID.fullmatch(active):
        raise PreflightError("CREDENTIAL_KEY_ID is invalid")
    keys = {}
    for item in required(env, "CREDENTIAL_KEYRING").split(";"):
        parts = item.split("=", 1)
        if len(parts) != 2 or not KEY_ID.fullmatch(parts[0]) or parts[0] in keys:
            raise PreflightError("CREDENTIAL_KEYRING is invalid")
        keys[parts[0]] = key32(parts[1], "CREDENTIAL_KEYRING")
    if active not in keys:
        raise PreflightError("active credential key is absent from keyring")

    ids = [x.strip() for x in required(env, "ADMIN_TELEGRAM_IDS").split(",")]
    if not ids or any(not x.isdigit() or int(x) <= 0 for x in ids):
        raise PreflightError("ADMIN_TELEGRAM_IDS is invalid")

    for name in ("MEDIA_STORAGE_ROOT", "MEDIA_WORK_ROOT"):
        value = Path(required(env, name))
        if not value.is_absolute() or str(value) == "/" or str(value).startswith("/tmp"):
            raise PreflightError(f"{name} must be a persistent absolute path")

    for name in ("TELMIVO_BACKEND_IMAGE", "TELMIVO_MINIAPP_IMAGE", "TELMIVO_PROXY_IMAGE", "TELMIVO_POSTGRES_IMAGE"):
        if not IMAGE.fullmatch(required(env, name)):
            raise PreflightError(f"{name} must be pinned by sha256 digest")

    cert = Path(args.tls_cert or required(env, "TELMIVO_TLS_CERT"))
    key = Path(args.tls_key or required(env, "TELMIVO_TLS_KEY"))
    if not cert.is_file() or cert.stat().st_size == 0 or not key.is_file() or key.stat().st_size == 0:
        raise PreflightError("TLS certificate or key file missing")

    print("TELMIVO production configuration preflight PASS")

if __name__ == "__main__":
    try:
        main()
    except (PreflightError, TestFixtureInProduction) as exc:
        print(f"TELMIVO production configuration preflight FAIL: {exc}", file=sys.stderr)
        sys.exit(1)
