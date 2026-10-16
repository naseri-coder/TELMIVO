#!/usr/bin/env python3
"""Offline checks for test-only V2 inputs that must not enter Production."""
import importlib.util
import tempfile
import unittest
from pathlib import Path

SOURCE = Path(__file__).resolve().with_name("preflight.py")
spec = importlib.util.spec_from_file_location("telmivo_production_preflight", SOURCE)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class Phase51KPreflight(unittest.TestCase):
    def test_regular_production_keys_not_rejected_by_fixture_gate(self):
        module.reject_test_fixture_configuration({
            "TELMIVO_ENV": "production",
            "SYNTHETIC_MODE": "false",
            "TELMIVO_LICENSE_ENVIRONMENT": "PRODUCTION",
        })

    def test_explicit_test_environment_refused(self):
        with self.assertRaises(module.PreflightError):
            module.reject_test_fixture_configuration({"TELMIVO_LICENSE_ENVIRONMENT": "TEST"})

    def test_forbidden_keys_refused_even_when_empty(self):
        for key in (
            "TELMIVO_LIC_A9_BROWSER_TEST",
            "TELMIVO_LIC_A9_NATIVE_FIXTURE",
            "TEST_DATABASE_URL",
            "TEST_MIGRATION_DATABASE_URL",
            "TEST_RESTORE_DATABASE_URL",
            "TEST_RESTORE_MIGRATION_DATABASE_URL",
            "TEST_NATS_URL",
            "TEST_REDIS_URL",
            "GOFLAGS",
            "GOWORK",
        ):
            with self.subTest(key=key):
                with self.assertRaises(module.PreflightError):
                    module.reject_test_fixture_configuration({key: ""})

    def test_source_does_not_mutate_production_config(self):
        with tempfile.TemporaryDirectory() as td:
            p = Path(td) / "env"
            p.write_text("TELMIVO_ENV=production\nSYNTHETIC_MODE=false\n")
            p.chmod(0o600)
            before = p.read_bytes()
            module.reject_test_fixture_configuration(module.load_env(p))
            self.assertEqual(p.read_bytes(), before)


if __name__ == "__main__":
    unittest.main()
