"""Real local Git/archive/Go regression for LIC-A9's nested-module fixture fix.

All repositories and fixture output are disposable; no Docker, PostgreSQL,
network, GitHub Actions or production credentials are used.
"""
import importlib.util
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


class NestedAuthorityModuleFixtureTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix='lic-a9-nested-fixture-')
        self.addCleanup(self.temporary.cleanup)
        self.base = Path(self.temporary.name)
        self.product = self.base / 'product'
        self.repo = self.base / 'authority-source'
        self.product.mkdir()
        self.repo.mkdir()
        scripts = self.product / 'tests' / 'installationlicense'
        scripts.mkdir(parents=True)
        shutil.copyfile(Path(__file__).with_name('prepare_authority_fixture.py'),
                        scripts / 'prepare_authority_fixture.py')
        (scripts / 'authority_e2e_test.go.template').write_text(
            'package e2e\nimport "testing"\nfunc TestSyntheticFixture(t *testing.T) {}\n')
        (self.product / 'go.mod').write_text(
            'module github.com/naseri-coder/TELMIVO\n\ngo 1.23\n')
        (self.product / 'go.sum').write_text('')
        (self.repo / 'go.mod').write_text(
            'module github.com/naseri-coder/TELMIVO-LICENSE-AUTHORITY\n\ngo 1.23\n')
        (self.repo / 'go.sum').write_text('synthetic-unused-checksum\n')
        package = self.repo / 'internal' / 'store' / 'postgres'
        package.mkdir(parents=True)
        (package / 'dummy.go').write_text(
            'package postgres\nconst Sample = "clock_timestamp()"\n')
        domain = self.repo / 'internal' / 'domain'
        domain.mkdir(parents=True)
        (domain / 'dummy.go').write_text(
            'package domain\n'
            'const Name = "github.com/naseri-coder/TELMIVO-LICENSE-AUTHORITY"\n')
        subprocess.run(['git', 'init', '-q', str(self.repo)], check=True)
        self.commit()
        spec = importlib.util.spec_from_file_location(
            'synthetic_fixture_preparer', scripts / 'prepare_authority_fixture.py')
        self.module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.module)
        self.module.sha = subprocess.run(
            ['git', '-C', str(self.repo), 'rev-parse', 'HEAD'],
            check=True, capture_output=True, text=True).stdout.strip()

    def commit(self):
        subprocess.run(['git', '-C', str(self.repo), 'add', '.'], check=True)
        subprocess.run([
            'git', '-C', str(self.repo), '-c', 'user.name=Fixture Test',
            '-c', 'user.email=test@example.invalid', 'commit', '-qm', 'synthetic input'
        ], check=True)

    def test_nested_go_module_is_absent_and_real_go_command_finds_package(self):
        dst = self.base / 'joint'
        self.module.prepare(self.repo, dst)
        self.assertTrue((dst / 'go.mod').is_file())
        self.assertFalse((dst / 'authority' / 'go.mod').exists())
        self.assertFalse((dst / 'authority' / 'go.sum').exists())
        env = dict(os.environ, GOTOOLCHAIN='local', GOPROXY='off', GOSUMDB='off',
                   GOWORK='off')
        for argv in (['go', 'list', './authority'],
                     ['go', 'test', '-count=1', '-run', '^$', './authority']):
            with self.subTest(command=argv):
                result = subprocess.run(argv, cwd=dst, env=env, capture_output=True,
                                        text=True, timeout=40)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_real_archived_go_code_only_is_rewritten(self):
        dst = self.base / 'joint'
        self.module.prepare(self.repo, dst)
        prepared = (dst / 'authority' / 'internal' / 'store' / 'postgres' / 'dummy.go').read_text()
        original = (self.repo / 'internal' / 'store' / 'postgres' / 'dummy.go').read_text()
        self.assertIn('lic_a8_clock()', prepared)
        self.assertIn('clock_timestamp()', original)
        converted = (dst / 'authority' / 'internal' / 'domain' / 'dummy.go').read_text()
        self.assertIn('github.com/naseri-coder/TELMIVO/lica8-e2e/authority', converted)
        self.assertTrue((self.repo / 'go.mod').is_file())
        self.assertTrue((self.repo / 'go.sum').is_file())
        self.assertEqual(subprocess.run(
            ['git', '-C', str(self.repo), 'status', '--porcelain'],
            check=True, capture_output=True, text=True).stdout.strip(), '')

    def test_unexpected_archived_module_refused(self):
        (self.repo / 'go.mod').write_text('module unexpected.example/authority\n\ngo 1.23\n')
        self.commit()
        self.module.sha = subprocess.run(
            ['git', '-C', str(self.repo), 'rev-parse', 'HEAD'],
            check=True, capture_output=True, text=True).stdout.strip()
        with self.assertRaises(SystemExit):
            self.module.prepare(self.repo, self.base / 'bad')

    def test_unavailable_pinned_commit_fails_before_creating_output(self):
        self.module.sha = '0' * 40
        target = self.base / 'must-not-exist'
        with self.assertRaises(subprocess.CalledProcessError):
            self.module.prepare(self.repo, target)
        self.assertFalse(target.exists())

    def test_existing_destination_refused(self):
        existing = self.base / 'existing'
        existing.mkdir()
        with self.assertRaises(SystemExit):
            self.module.prepare(self.repo, existing)


if __name__ == '__main__':
    unittest.main()
