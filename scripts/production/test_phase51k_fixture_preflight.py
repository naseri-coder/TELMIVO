#!/usr/bin/env python3
"""Exact-source tests for isolated TEST-only V2 production admission guard."""
import importlib.util
import unittest
from pathlib import Path

DIR = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location(
    "telmivo_phase51k_fixture_gate", DIR / "phase51k_fixture_gate.py"
)
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)


class Phase51KPreflight(unittest.TestCase):
    def test_regular_production_keys_pass_fixture_boundary(self):
        gate.reject_test_fixture_configuration({
            "TELMIVO_ENV": "production",
            "SYNTHETIC_MODE": "false",
            "TELMIVO_LICENSE_ENVIRONMENT": "PRODUCTION",
        })

    def test_explicit_test_environment_denied_case_insensitively(self):
        for mode in ("TEST", "test", "Test"):
            with self.subTest(mode=mode):
                with self.assertRaises(gate.TestFixtureInProduction):
                    gate.reject_test_fixture_configuration({"TELMIVO_LICENSE_ENVIRONMENT": mode})

    def test_all_forbidden_keys_denied_even_if_empty(self):
        self.assertGreaterEqual(len(gate.FORBIDDEN), 14)
        for key in sorted(gate.FORBIDDEN):
            with self.subTest(key=key):
                with self.assertRaises(gate.TestFixtureInProduction):
                    gate.reject_test_fixture_configuration({key: ""})

    def test_preflight_wiring_invokes_guard_before_other_validation(self):
        text = (DIR / "preflight.py").read_text(encoding="utf-8")
        self.assertIn(
            "from phase51k_fixture_gate import reject_test_fixture_configuration",
            text,
        )
        self.assertLess(
            text.index("    reject_test_fixture_configuration(env)"),
            text.index('    if required(env, "TELMIVO_ENV")'),
        )


if __name__ == "__main__":
    unittest.main()
