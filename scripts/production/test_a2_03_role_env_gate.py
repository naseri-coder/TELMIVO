"""Offline regression for A2-03 API/Worker/Migration credential isolation."""
import tempfile
import unittest
from pathlib import Path

from a2_03_role_env_gate import inspect, RoleSeparationError

def url(login, secret):
    return "postgresql://" + login + ":" + secret + "@127.0.0.1:5432/telmivo?sslmode=verify-full"

class RoleEnvironmentGate(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        d = Path(self.temp.name)
        self.paths = [d / "tenant.env", d / "worker.env", d / "migration.env"]
        self.baselines = [
            {"TELMIVO_ENV": "production", "SYNTHETIC_MODE": "false",
             "TELMIVO_DB_ROLE_SPLIT": "required",
             "DATABASE_URL": url("telmivo_api", "synthetic-api-only")},
            {"TELMIVO_ENV": "production", "SYNTHETIC_MODE": "false",
             "TELMIVO_DB_ROLE_SPLIT": "required",
             "DATABASE_URL": url("telmivo_worker", "synthetic-worker-only")},
            {"TELMIVO_ENV": "production", "SYNTHETIC_MODE": "false",
             "MIGRATION_DATABASE_URL": url("telmivo_migration", "synthetic-migration-only")},
        ]
        self.write()

    def write(self):
        for p, env in zip(self.paths, self.baselines):
            p.write_text("\n".join(k + "=" + v for k, v in env.items()) + "\n")
            p.chmod(0o600)

    def check(self):
        return inspect(*self.paths)

    def test_distinct_role_files_accepted_as_source_only(self):
        self.assertEqual(self.check()["status"], "SOURCE_ROLE_ENV_FILES_DISTINCT_ONLY")

    def test_credential_and_environment_negative_matrix(self):
        scenarios = [
            (0, "DATABASE_URL", url("telmivo_worker", "synthetic-api-only")),
            (1, "DATABASE_URL", url("telmivo_api", "synthetic-worker-only")),
            (0, "DATABASE_URL", url("telmivo_runtime", "legacy")),
            (2, "MIGRATION_DATABASE_URL", url("telmivo_api", "syn")),
            (0, "TELMIVO_DB_ROLE_SPLIT", "off"),
            (1, "SYNTHETIC_MODE", "true"),
            (1, "TELMIVO_ENV", "test"),
            (0, "MIGRATION_DATABASE_URL", url("telmivo_migration", "leak")),
            (1, "TENANT_DATABASE_URL", url("telmivo_api", "leak")),
            (2, "DATABASE_URL", url("telmivo_worker", "leak")),
            (0, "PGOPTIONS", "-c role=postgres"),
            (0, "PGHOST", "database.production"),
            (0, "DATABASE_URL", url("telmivo_api", "syn")+"&host=prod"),
            (1, "DATABASE_URL", url("telmivo_worker", "syn")+"&service=prod"),
            (0, "DATABASE_URL", url("telmivo_api", "syn")+"&sslmode=disable"),
        ]
        for i, k, value in scenarios:
            with self.subTest(case=(i, k, value[:17])):
                self.baselines[i][k] = value
                self.write()
                with self.assertRaises(RoleSeparationError):
                    self.check()
                self.baselines = [
                    {"TELMIVO_ENV": "production", "SYNTHETIC_MODE": "false",
                     "TELMIVO_DB_ROLE_SPLIT": "required",
                     "DATABASE_URL": url("telmivo_api", "synthetic-api-only")},
                    {"TELMIVO_ENV": "production", "SYNTHETIC_MODE": "false",
                     "TELMIVO_DB_ROLE_SPLIT": "required",
                     "DATABASE_URL": url("telmivo_worker", "synthetic-worker-only")},
                    {"TELMIVO_ENV": "production", "SYNTHETIC_MODE": "false",
                     "MIGRATION_DATABASE_URL": url("telmivo_migration", "synthetic-migration-only")},
                ]

    def test_same_role_env_file_rejected(self):
        with self.assertRaises(RoleSeparationError):
            inspect(self.paths[0], self.paths[0], self.paths[2])

    def test_file_permission_denied(self):
        self.paths[0].chmod(0o644)
        with self.assertRaises(ValueError):
            self.check()

    def test_identical_secrets_denied(self):
        self.baselines[1]["DATABASE_URL"] = url("telmivo_worker", "synthetic-api-only")
        self.write()
        with self.assertRaises(RoleSeparationError):
            self.check()

if __name__ == "__main__":
    unittest.main()
