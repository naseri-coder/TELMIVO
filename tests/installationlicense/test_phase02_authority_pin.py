"""Phase08 exact-current-Authority source pin and no-stale-archive regression tests.

All Git trees are disposable synthetic fixtures. No network or PostgreSQL.
"""
from pathlib import Path
import subprocess
import tempfile
import unittest

import run_joint_postgres_e2e as runner


def git(root, *args):
    return subprocess.run(['git', '-C', str(root), *args], check=True,
                          text=True, capture_output=True).stdout.strip()


class FinalAuthoritySourcePinTests(unittest.TestCase):
    def test_fixed_source_and_fixture_represent_same_real_phase07_head(self):
        selected = 'bfceaef06d41aa185677a796652d17b07979058a'
        self.assertEqual(runner.AUTHORITY_MERGED_SHA, selected)
        self.assertEqual(runner.AUTHORITY_ARCHIVE_SHA, selected)
        self.assertEqual(runner.EXPECTED_AUTHORITY_NONRUNTIME_DELTA, frozenset())
        self.assertEqual(runner.PRODUCT_MERGED_SHA,
                         'd88fdb2e4d34f18e8f9c8313f73db5b14a077d5a')
        self.assertEqual(runner.FIXTURE_PREPARER_BLOB,
                         '816456d387c8e231f1c469c668e01c722e8b5a32')

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='lic-a9-phase08-source-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        git(self.root, 'init', '-q')
        git(self.root, 'config', 'user.name', 'Synthetic Fixture')
        git(self.root, 'config', 'user.email', 'fixture@example.invalid')
        (self.root / 'README.txt').write_text('TEST_ONLY\n')
        git(self.root, 'add', '-A')
        git(self.root, 'commit', '-qm', 'TEST_ONLY baseline')
        self.base = git(self.root, 'rev-parse', 'HEAD')

    def change(self, name, payload='TEST_ONLY\n'):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(payload)
        git(self.root, 'add', '-A')
        git(self.root, 'commit', '-qm', 'TEST_ONLY modification')
        return git(self.root, 'rev-parse', 'HEAD')

    def test_identical_commit_is_admissible_fixture_source(self):
        runner.validate_authority_archive_provenance(
            self.root, self.base, self.base, frozenset())

    def test_stale_archive_rejected_even_for_docs_only(self):
        fresh = self.change('docs/other.md')
        with self.assertRaises(ValueError):
            runner.validate_authority_archive_provenance(
                self.root, self.base, fresh, frozenset())

    def test_added_runtime_go_source_rejected(self):
        fresh = self.change('internal/m2m/server.go', 'package m2m\n')
        with self.assertRaises(ValueError):
            runner.validate_authority_archive_provenance(
                self.root, self.base, fresh, frozenset())

    def test_new_sql_migration_rejected(self):
        fresh = self.change('db/migrations/0009_unreviewed.sql', 'SELECT 1;\n')
        with self.assertRaises(ValueError):
            runner.validate_authority_archive_provenance(
                self.root, self.base, fresh, frozenset())

    def test_added_deploy_script_rejected(self):
        fresh = self.change('deploy/service.sh')
        with self.assertRaises(ValueError):
            runner.validate_authority_archive_provenance(
                self.root, self.base, fresh, frozenset())

    def test_added_test_tool_rejected(self):
        fresh = self.change('tests/skip.py')
        with self.assertRaises(ValueError):
            runner.validate_authority_archive_provenance(
                self.root, self.base, fresh, frozenset())

    def test_checkout_rejects_old_head_and_untracked_material(self):
        fresh = self.change('new.txt')
        runner.check_checkout(self.root, fresh)
        with self.assertRaises(ValueError):
            runner.check_checkout(self.root, self.base)
        (self.root / 'untracked').write_text('TEST_ONLY\n')
        with self.assertRaises(ValueError):
            runner.check_checkout(self.root, fresh)


if __name__ == '__main__':
    unittest.main()
