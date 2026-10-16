#!/usr/bin/env python3
"""Fail-closed native integration target admission. No DB/network I/O."""
import os
import sys
from urllib.parse import urlsplit

class UnsafeNativeTestTarget(ValueError):
    pass

def _fail(reason):
    raise UnsafeNativeTestTarget(reason)

def _database(raw, role, port):
    try:
        u = urlsplit(raw)
        if (u.scheme not in ("postgres", "postgresql") or
            u.hostname != "127.0.0.1" or u.port != port or
            u.path != "/telmivo_test" or u.query != "sslmode=disable" or
            u.fragment or u.username != role or not u.password or
            u.netloc.count("@") != 1 or u.hostname not in u.netloc):
            _fail("test PostgreSQL target must be explicit loopback telmivo_test with fixed role/port/query")
    except ValueError as exc:
        raise UnsafeNativeTestTarget("invalid test PostgreSQL URL") from exc

def _nats(raw):
    if raw != "nats://127.0.0.1:4222":
        _fail("test JetStream target must be exact loopback NATS URL")

def check(env):
    if (env.get("TELMIVO_ENV") != "test"
            or env.get("TELMIVO_LICENSE_ENVIRONMENT") != "TEST"
            or env.get("SYNTHETIC_MODE") != "true"):
        _fail("explicit TEST, TEST-license and synthetic mode are mandatory")
    _database(env.get("TEST_DATABASE_URL", ""), "telmivo_runtime", 5432)
    _database(env.get("TEST_MIGRATION_DATABASE_URL", ""), "telmivo_migration", 5432)
    for name, role in (("TEST_RESTORE_DATABASE_URL", "telmivo_runtime"),
                       ("TEST_RESTORE_MIGRATION_DATABASE_URL", "telmivo_migration")):
        if name in env:
            _database(env[name], role, 5433)
    if ("TEST_RESTORE_DATABASE_URL" in env) != ("TEST_RESTORE_MIGRATION_DATABASE_URL" in env):
        _fail("restore test database URLs must be provided as a pair")
    if "TEST_NATS_URL" in env:
        _nats(env["TEST_NATS_URL"])
    for source, expected in (("DATABASE_URL", "TEST_DATABASE_URL"),
                             ("MIGRATION_DATABASE_URL", "TEST_MIGRATION_DATABASE_URL"),
                             ("NATS_URL", "TEST_NATS_URL")):
        if source in env and env[source] != env.get(expected, ""):
            _fail("conflicting inherited connection target: " + source)
    if env.get("TELMIVO_LIC_A9_BROWSER_TEST") or env.get("TELMIVO_LIC_A9_NATIVE_FIXTURE"):
        _fail("native target check must not inherit test signing activation")
    return True

if __name__ == "__main__":
    try:
        check(os.environ)
    except UnsafeNativeTestTarget as exc:
        print("LIC_A9_NATIVE_TARGET_REJECTED: " + str(exc), file=sys.stderr)
        sys.exit(2)
    print("LIC_A9_NATIVE_TARGET_LOOPBACK_ONLY")
