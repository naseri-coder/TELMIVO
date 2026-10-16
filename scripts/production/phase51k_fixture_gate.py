"""LIC-A9 Phase51-K: pure fail-closed production/test V2 boundary.

Import-only helper for scripts/production/preflight.py. No I/O or secrets.
"""

class TestFixtureInProduction(ValueError):
    pass


FORBIDDEN = frozenset({
    "TELMIVO_LIC_A9_BROWSER_TEST",
    "TELMIVO_LIC_A9_NATIVE_FIXTURE",
    "TEST_GROUP_AUTHORITY_FIXTURE",
    "TEST_DOWNLOADER_FIXTURE",
    "TEST_BOT_FACTORY_FIXTURE",
    "TEST_CONTENT_PLUGIN_FIXTURE",
    "TEST_DATABASE_URL",
    "TEST_MIGRATION_DATABASE_URL",
    "TEST_RESTORE_DATABASE_URL",
    "TEST_RESTORE_MIGRATION_DATABASE_URL",
    "TEST_NATS_URL",
    "TEST_REDIS_URL",
    "GOFLAGS",
    "GOWORK",
})


def reject_test_fixture_configuration(env):
    present = sorted(FORBIDDEN.intersection(env))
    if present:
        raise TestFixtureInProduction(
            "test-only fixture or Go build controls in production environment: "
            + ", ".join(present)
        )
    if env.get("TELMIVO_LICENSE_ENVIRONMENT", "").upper() == "TEST":
        raise TestFixtureInProduction("TEST-only V2 license environment forbidden in production")
