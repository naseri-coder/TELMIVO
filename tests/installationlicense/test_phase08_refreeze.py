"""Phase08 disposable-Git regressions for exact reviewed cross-repository pins.

No Docker/DB/GitHub API call; synthetic files only. A PASS here does not
assert that eight real Go+PG16 integration tests were executed.
"""
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

import run_joint_postgres_e2e as runner


def git(root, *args):
    return subprocess.run(['git', '-C', str(root), *args], text=True,
                          capture_output=True, check=True).stdout.strip()


class Phase08RefreezeTests(unittest.TestCase):
    def test_selected_sources_are_exact_and_distinct(self):
        self.assertEqual(runner.AUTHORITY_MERGED_SHA,
                         'bfceaef06d41aa185677a796652d17b07979058a')
        self.assertEqual(runner.PRODUCT_MERGED_SHA,
                         'd88fdb2e4d34f18e8f9c8313f73db5b14a077d5a')
        self.assertEqual(runner.AUTHORITY_ARCHIVE_SHA, runner.AUTHORITY_MERGED_SHA)
        self.assertNotEqual(runner.PRODUCT_MERGED_SHA,
                            '3b871473955e6803636bba0e45a843536afb226a')

    def test_exact_fixture_helper_blob_is_frozen(self):
        preparer = Path(__file__).with_name('prepare_authority_fixture.py')
        self.assertEqual(
            subprocess.run(['git', 'hash-object', str(preparer)], check=True,
                           text=True, capture_output=True).stdout.strip(),
            runner.FIXTURE_PREPARER_BLOB)
        src = preparer.read_text()
        self.assertIn('sha = "' + runner.AUTHORITY_MERGED_SHA + '"', src)
        self.assertNotIn('sha = "68cdb6996f85981c36dd20ecb7435ec010351e9b"', src)

    def test_allowlist_only_reviewed_tests_docs_not_runtime(self):
        values = runner.EXPECTED_PRODUCT_TEST_DELTA
        self.assertEqual(len(values), 11)
        self.assertTrue(all(x.startswith('tests/installationlicense/') for x in values))
        self.assertFalse(any(x.startswith(('internal/', 'cmd/', 'deploy/', 'pkg/'))
                             or x in ('go.mod', 'go.sum') for x in values))

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='phase08-git-only-')
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        git(self.root, 'init', '-q')
        git(self.root, 'config', 'user.name', 'TEST ONLY')
        git(self.root, 'config', 'user.email', 'test@example.invalid')
        (self.root / 'go.mod').write_text('module synthetic\ngo 1.27\n')
        (self.root / 'internal').mkdir()
        (self.root / 'internal' / 'core.go').write_text('package internal\n')
        self.commit()
        self.base = git(self.root, 'rev-parse', 'HEAD')

    def commit(self):
        git(self.root, 'add', '-A')
        git(self.root, 'commit', '-qm', 'TEST ONLY fixture')
        return git(self.root, 'rev-parse', 'HEAD')

    def make_test_delta(self, omitted=None):
        for rel in sorted(runner.EXPECTED_PRODUCT_TEST_DELTA):
            if rel == omitted:
                continue
            path = self.root / rel
            path.parent.mkdir(parents=True, exist_ok=True)
            if rel.endswith('/prepare_authority_fixture.py'):
                shutil.copyfile(Path(__file__).with_name('prepare_authority_fixture.py'), path)
            else:
                path.write_text('TEST ONLY fixture\n')
        return self.commit()

    def test_exact_full_test_delta_passes_without_touching_runtime(self):
        self.make_test_delta()
        runner.check_product_checkout(self.root, self.base)

    def test_missing_single_expected_tool_blocks(self):
        self.make_test_delta('tests/installationlicense/test_phase08_refreeze.py')
        with self.assertRaises(ValueError):
            runner.check_product_checkout(self.root, self.base)

    def test_unapproved_runtime_mutation_blocks(self):
        self.make_test_delta()
        (self.root / 'internal/core.go').write_text('package changed\n')
        self.commit()
        with self.assertRaises(ValueError):
            runner.check_product_checkout(self.root, self.base)

    def test_unreviewed_deployment_file_blocks(self):
        self.make_test_delta()
        (self.root / 'deploy').mkdir()
        (self.root / 'deploy' / 'service.sh').write_text('TEST ONLY\n')
        self.commit()
        with self.assertRaises(ValueError):
            runner.check_product_checkout(self.root, self.base)

    def test_unreviewed_python_tool_blocks(self):
        self.make_test_delta()
        (self.root / 'tests/installationlicense/extra.py').write_text('TEST ONLY\n')
        self.commit()
        with self.assertRaises(ValueError):
            runner.check_product_checkout(self.root, self.base)

    def test_dirty_checkout_blocks(self):
        self.make_test_delta()
        (self.root / 'untracked').write_text('TEST ONLY\n')
        with self.assertRaises(ValueError):
            runner.check_product_checkout(self.root, self.base)

    def test_changed_fixture_helper_blob_blocks(self):
        self.make_test_delta()
        (self.root / 'tests/installationlicense/prepare_authority_fixture.py').write_text(
            '# fake fixture helper\n')
        self.commit()
        with self.assertRaises(ValueError):
            runner.check_product_checkout(self.root, self.base)

    def test_module_file_mutation_blocks(self):
        self.make_test_delta()
        (self.root / 'go.mod').write_text('module different\ngo 1.27\n')
        self.commit()
        with self.assertRaises(ValueError):
            runner.check_product_checkout(self.root, self.base)


if __name__ == '__main__':
    unittest.main()
