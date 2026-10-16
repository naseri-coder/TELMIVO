#!/usr/bin/env python3
"""Adversarial native test-target admission: no network or database I/O."""
import unittest
from phase51l_test_target import check, UnsafeNativeTestTarget

def safe():
    return {
        "TEST_DATABASE_URL": "postgres://telmivo_runtime:synthetic@127.0.0.1:5432/telmivo_test?sslmode=disable",
        "TEST_MIGRATION_DATABASE_URL": "postgres://telmivo_migration:synthetic@127.0.0.1:5432/telmivo_test?sslmode=disable",
        "TEST_NATS_URL": "nats://127.0.0.1:4222",
        "TELMIVO_ENV": "test",
        "TELMIVO_LICENSE_ENVIRONMENT": "TEST",
    }

class NativeTargets(unittest.TestCase):
    def test_canonical_allowed(self):
        self.assertTrue(check(safe()))

    def test_restore_pair_loopback_allowed(self):
        env = safe()
        env["TEST_RESTORE_DATABASE_URL"] = "postgres://telmivo_runtime:synthetic@127.0.0.1:5433/telmivo_test?sslmode=disable"
        env["TEST_RESTORE_MIGRATION_DATABASE_URL"] = "postgres://telmivo_migration:synthetic@127.0.0.1:5433/telmivo_test?sslmode=disable"
        self.assertTrue(check(env))

    def test_production_or_remote_denied(self):
        edits = {
            "wrong_db": ("TEST_DATABASE_URL", "postgres://telmivo_runtime:synthetic@127.0.0.1:5432/telmivo_prod?sslmode=disable"),
            "host_override": ("TEST_DATABASE_URL", safe()["TEST_DATABASE_URL"] + "&host=db.production.example"),
            "service_override": ("TEST_DATABASE_URL", safe()["TEST_DATABASE_URL"] + "&service=production"),
            "connection_target": ("TEST_DATABASE_URL", "postgres://telmivo_runtime:synthetic@db.production.example:5432/telmivo_test?sslmode=disable"),
            "ssh_tunnel_port": ("TEST_DATABASE_URL", "postgres://telmivo_runtime:synthetic@127.0.0.1:5434/telmivo_test?sslmode=disable"),
            "wrong_role": ("TEST_DATABASE_URL", "postgres://postgres:synthetic@127.0.0.1:5432/telmivo_test?sslmode=disable"),
            "missing_password": ("TEST_DATABASE_URL", "postgres://telmivo_runtime@127.0.0.1:5432/telmivo_test?sslmode=disable"),
            "query_duplicate": ("TEST_DATABASE_URL", safe()["TEST_DATABASE_URL"] + "&sslmode=require"),
            "postgres_alias": ("TEST_DATABASE_URL", "postgres://telmivo_runtime:synthetic@postgres:5432/telmivo_test?sslmode=disable"),
            "local_hostname": ("TEST_DATABASE_URL", "postgres://telmivo_runtime:synthetic@localhost:5432/telmivo_test?sslmode=disable"),
            "prod_env": ("TELMIVO_ENV", "production"),
            "prod_license_env": ("TELMIVO_LICENSE_ENVIRONMENT", "production"),
            "remote_nats": ("TEST_NATS_URL", "nats://nats.production.example:4222"),
            "nats_userinfo": ("TEST_NATS_URL", "nats://admin:secret@127.0.0.1:4222"),
            "inherited_prod_db": ("DATABASE_URL", "postgres://runtime:prod@127.0.0.1:5432/telmivo_prod?sslmode=disable"),
            "inherited_prod_nats": ("NATS_URL", "nats://remote:4222"),
            "inherited_signing_flag": ("TELMIVO_LIC_A9_NATIVE_FIXTURE", "YES"),
        }
        for name, (key, value) in edits.items():
            with self.subTest(name=name):
                env = safe()
                env[key] = value
                with self.assertRaises(UnsafeNativeTestTarget):
                    check(env)

    def test_restore_incomplete_or_wrong_port_denied(self):
        for mutate in ("missing_partner", "wrong_port"):
            with self.subTest(case=mutate):
                env = safe()
                env["TEST_RESTORE_DATABASE_URL"] = "postgres://telmivo_runtime:synthetic@127.0.0.1:5433/telmivo_test?sslmode=disable"
                if mutate == "wrong_port":
                    env["TEST_RESTORE_MIGRATION_DATABASE_URL"] = "postgres://telmivo_migration:synthetic@127.0.0.1:5432/telmivo_test?sslmode=disable"
                with self.assertRaises(UnsafeNativeTestTarget):
                    check(env)

if __name__ == "__main__":
    unittest.main()
