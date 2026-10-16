#!/usr/bin/env python3
"""No-network regression: native shell gate must reject implicit TEST identity."""
import unittest

from phase51l_test_target import UnsafeNativeTestTarget, check
from test_phase51l_test_target import safe


class ExplicitTestIdentity(unittest.TestCase):
    def test_missing_test_environment_identity_denied(self):
        for key in ("TELMIVO_ENV", "TELMIVO_LICENSE_ENVIRONMENT", "SYNTHETIC_MODE"):
            with self.subTest(key=key):
                env = safe()
                del env[key]
                with self.assertRaises(UnsafeNativeTestTarget):
                    check(env)

    def test_empty_test_environment_identity_denied(self):
        for key in ("TELMIVO_ENV", "TELMIVO_LICENSE_ENVIRONMENT", "SYNTHETIC_MODE"):
            with self.subTest(key=key):
                env = safe()
                env[key] = ""
                with self.assertRaises(UnsafeNativeTestTarget):
                    check(env)

    def test_no_case_folding_of_sensitive_test_flag(self):
        for key, value in (
            ("TELMIVO_ENV", "TEST"),
            ("TELMIVO_LICENSE_ENVIRONMENT", "test"),
            ("SYNTHETIC_MODE", "TRUE"),
        ):
            with self.subTest(key=key):
                env = safe()
                env[key] = value
                with self.assertRaises(UnsafeNativeTestTarget):
                    check(env)


if __name__ == "__main__":
    unittest.main()
