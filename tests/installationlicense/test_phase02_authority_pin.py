"""Offline/nonproduction exact-source fence tests for Phase02 joint E2E.

Never connects to Authority, Docker, PostgreSQL or production. All Git worktrees
are synthetic and created under a temporary directory.
"""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

import run_joint_postgres_e2e as runner


EXPECTED_FILES = frozenset({
    "docs/development/LIC_A9_PHASE01_CANONICAL_CONSOLIDATION_20261015.md",
    "docs/development/LIC_A9_PHASE2B_AUTHORITY_PG16_CI_BLOCKER_20261014.md",
    "docs/release/lic-a9-offline-preinstall-bundle-check.md",
    "internal/domain/phase2b_safety_test.go",
    "tests/online_install_bundle_test.py",
    "tests/release_test.py",
    "tools/build-release",
    "tools/verify-online-install-bundle",
})


def cmd(directory, *argv):
    return subprocess.run(
        ["git", "-C", str(directory), *argv], check=True,
        text=True, capture_output=True,
    ).stdout.strip()


class Phase02AuthorityPinTests(unittest.TestCase):
    def test_frozen_canonical_authority_pins_and_exact_allowlist(self):
        self.assertEqual(
            runner.AUTHORITY_MERGED_SHA,
            "d4cff712d39e21f934afb0f43ee36a376f1e5345",
        )
        self.assertEqual(
            runner.AUTHORITY_ARCHIVE_SHA,
            "68cdb6996f85981c36dd20ecb7435ec010351e9b",
        )
        self.assertEqual(runner.EXPECTED_AUTHORITY_NONRUNTIME_DELTA, EXPECTED_FILES)
        self.assertTrue(all(path.startswith(("docs/", "tests/", "tools/"))
                            or path == "internal/domain/phase2b_safety_test.go"
                            for path in EXPECTED_FILES))
        self.assertFalse(any(path.startswith((
            "cmd/", "db/", "deploy/", "internal/m2m/", "internal/store/",
            "pkg/", "internal/operator/",
        )) for path in EXPECTED_FILES))

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="lic-a9-phase02-pin-test-only-")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        cmd(self.root, "init", "-q")
        cmd(self.root, "config", "user.name", "Synthetic Test")
        cmd(self.root, "config", "user.email", "synthetic@example.invalid")
        self.write("README.md", "TEST_ONLY baseline\n")
        self.commit()
        self.archive = cmd(self.root, "rev-parse", "HEAD")

    def write(self, relative, content="TEST_ONLY\n"):
        path = self.root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")

    def commit(self):
        cmd(self.root, "add", "-A")
        cmd(self.root, "commit", "-qm", "TEST_ONLY synthetic provenance")

    def test_accepts_exact_eight_path_synthetic_delta(self):
        for path in sorted(EXPECTED_FILES):
            self.write(path)
        self.commit()
        merged = cmd(self.root, "rev-parse", "HEAD")
        runner.validate_authority_archive_provenance(
            self.root, self.archive, merged, EXPECTED_FILES,
        )

    def test_rejects_subset_even_if_only_safe_paths(self):
        for path in sorted(EXPECTED_FILES - {"tools/build-release"}):
            self.write(path)
        self.commit()
        with self.assertRaises(ValueError):
            runner.validate_authority_archive_provenance(
                self.root, self.archive, cmd(self.root, "rev-parse", "HEAD"),
                EXPECTED_FILES,
            )

    def test_rejects_added_runtime_go_file(self):
        for path in sorted(EXPECTED_FILES):
            self.write(path)
        self.write("internal/m2m/server.go", "package m2m\n")
        self.commit()
        with self.assertRaises(ValueError):
            runner.validate_authority_archive_provenance(
                self.root, self.archive, cmd(self.root, "rev-parse", "HEAD"),
                EXPECTED_FILES,
            )

    def test_rejects_added_migration_and_deploy_paths(self):
        for extra in ("db/migrations/0009_unsafe.sql", "deploy/api.service"):
            with self.subTest(extra=extra):
                branch_root = self.root / extra
                for path in sorted(EXPECTED_FILES):
                    self.write(path)
                self.write(extra)
                self.commit()
                with self.assertRaises(ValueError):
                    runner.validate_authority_archive_provenance(
                        self.root, self.archive, cmd(self.root, "rev-parse", "HEAD"),
                        EXPECTED_FILES,
                    )
                branch_root.unlink()

    def test_disallows_unreviewed_test_or_tool_even_if_nonruntime(self):
        for path in EXPECTED_FILES:
            self.write(path)
        self.write("tools/optional-bypass", "TEST_ONLY malicious extra\n")
        self.commit()
        with self.assertRaises(ValueError):
            runner.validate_authority_archive_provenance(
                self.root, self.archive, cmd(self.root, "rev-parse", "HEAD"),
                EXPECTED_FILES,
            )

    def test_git_checkout_requires_exact_clean_commit(self):
        self.commit()
        merged = cmd(self.root, "rev-parse", "HEAD")
        runner.check_checkout(self.root, merged)
        with self.assertRaises(ValueError):
            runner.check_checkout(self.root, self.archive)
        self.write("untracked", "TEST_ONLY")
        with self.assertRaises(ValueError):
            runner.check_checkout(self.root, merged)


if __name__ == "__main__":
    unittest.main()
