"""Security tests for the mobile preflight, NOT for Product Manager.Effect."""
from pathlib import Path
import os
import subprocess
import tempfile
import unittest

RUNNER = Path(__file__).with_name('run_termux_manager_tests.sh')
MANIFEST = [
    ('internal/platform/installationlicense/state.go', 'c5c81548a1b063825eccc9c3d69cc6006160d4cd'),
    ('internal/platform/installationlicense/phase19_session_fencing_test.go', '3fe0316182760ccf6504b7f36e7be8cd19dacd77'),
    ('internal/platform/installationlicense/phase34_real_manager_effect_test.go', 'd32c4dac201fcb78216b633830a6d70e37b7f0b5'),
]

def run(*argv, cwd=None, env=None):
    return subprocess.run(argv, cwd=cwd, env=env, capture_output=True, text=True, check=False)


class SecurityEnvelopeTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='phase35-termux-')
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.repo = self.root / 'checkout'
        self.repo.mkdir()
        self.binaries = self.root / 'tools'
        self.binaries.mkdir()
        self.log_dir = self.root / 'audit'
        self.mock_calls = self.root / 'mock_go_calls'
        for args in (('init', '-q'), ('config', 'user.name', 'Phase35 Test'), ('config', 'user.email', 'phase35@example.invalid'), ('checkout', '-qb', 'development/lic-a9-phase35-offline-go127-execution-gate-20261016')):
            assert run('git', *args, cwd=self.repo).returncode == 0
        (self.repo / 'go.mod').write_text('module github.com/naseri-coder/TELMIVO\n\ngo 1.27.0\n')
        (self.repo / 'go.sum').write_text('mock fixture\n')
        for path, _ in MANIFEST:
            dest = self.repo / path
            dest.parent.mkdir(parents=True, exist_ok=True)
            dest.write_text('fixture')
        # The manifest is deliberately strict. Mock files are not Git blobs
        # matching Product; the successful mock branch uses a MOCK git binary
        # only for hash-object, and never proves genuine Product execution.
        assert run('git', 'add', '-A', cwd=self.repo).returncode == 0
        assert run('git', 'commit', '-qm', 'isolated fixture', cwd=self.repo).returncode == 0
        self.sha = run('git', 'rev-parse', 'HEAD', cwd=self.repo).stdout.strip()
        git_path = self.binaries / 'git'
        git_path.write_text('#!/usr/bin/env bash\nif [[ "$1" == hash-object ]]; then case "$2" in\n'
            + ''.join(f'{path}) echo {sha} ;;\n' for path, sha in MANIFEST)
            + 'esac; exit; fi\nexec /usr/bin/git "$@"\n')
        git_path.chmod(0o755)
        go_path = self.binaries / 'go'
        go_path.write_text('''#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == version ]]; then echo "go version ${PHASE35_MOCK_GO_VERSION:-go1.27.1} android/arm64"; exit; fi
if [[ "$1" == env && "$2" == GOOS ]]; then echo android; exit; fi
if [[ "$1" == test || "$1" == vet ]]; then
  printf '%s\\n' "$*" >> "$PHASE35_MOCK_CALLS"
  if [[ "${PHASE35_MOCK_FAIL:-}" == YES ]]; then exit 13; fi
  echo 'MOCK_ONLY_DOES_NOT_EXECUTE_GO_TESTS'; exit
fi
exit 12
''')
        go_path.chmod(0o755)
        self.env = dict(os.environ, PATH=f'{self.binaries}:{os.environ["PATH"]}', PREFIX='/data/data/com.termux/files/usr', LIC_A9_DISPOSABLE='YES', LIC_A9_EXPECT_HEAD=self.sha, LIC_A9_LOG_DIR=str(self.log_dir), PHASE35_MOCK_CALLS=str(self.mock_calls), PHASE35_MOCK_GO_VERSION='go1.27.1', APP_ENV='test', GITHUB_ACTIONS='false')

    def checked(self, **updates):
        return run('bash', str(RUNNER), cwd=self.repo, env=dict(self.env, **updates))

    def deny(self, **updates):
        p = self.checked(**updates)
        self.assertEqual(p.returncode, 78, (p.stdout, p.stderr))
        self.assertFalse(self.mock_calls.exists(), 'no Go test should run')

    def test_no_disposable_consent(self): self.deny(LIC_A9_DISPOSABLE='')
    def test_no_termux(self): self.deny(PREFIX='/usr')
    def test_no_github_actions(self): self.deny(GITHUB_ACTIONS='true')
    def test_no_production(self): self.deny(APP_ENV='production')
    def test_no_external_effects(self): self.deny(LIC_A9_ALLOW_SIDE_EFFECTS='YES')
    def test_no_unreviewed_commit(self): self.deny(LIC_A9_EXPECT_HEAD='0'*40)
    def test_go123_rejected(self): self.deny(PHASE35_MOCK_GO_VERSION='go1.23.2')

    def test_no_main(self):
        self.assertEqual(run('git', 'checkout', '-qb', 'main', cwd=self.repo).returncode, 0)
        self.deny()

    def test_dirty_checkout(self):
        (self.repo/'unexpected.txt').write_text('x')
        self.deny()

    def test_wrong_module(self):
        (self.repo/'go.mod').write_text('module wrong.example/x\n\ngo 1.27.0\n')
        self.deny()

    def test_exact_sha_manifest_mismatch(self):
        # A valid clean repo HEAD cannot match the reviewed object contents in
        # this fake fixture if the hash-object command returns a different blob.
        fake_git = self.binaries/'git'
        text = fake_git.read_text().replace('c5c81548a1b063825eccc9c3d69cc6006160d4cd', '0'*40)
        fake_git.write_text(text)
        self.deny()

    def test_mock_success_only_validates_runner_dispatch(self):
        result = self.checked()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        calls = self.mock_calls.read_text().splitlines()
        self.assertEqual(len(calls), 3)
        self.assertTrue(calls[0].startswith('test -count=1 -timeout=300s -run ^TestLICA9Phase34'))
        self.assertTrue(calls[1].startswith('test -count=1 -timeout=420s'))
        self.assertTrue(calls[2].startswith('vet'))
        self.assertIn('FUNCTIONAL_PASS_NOT_RACE_NOT_PRODUCTION', result.stdout)
        self.assertNotIn('-race', ' '.join(calls))

    def test_mock_failure_no_success_marker(self):
        result = self.checked(PHASE35_MOCK_FAIL='YES')
        self.assertEqual(result.returncode, 13, result.stdout + result.stderr)
        self.assertNotIn('FUNCTIONAL_PASS_NOT_RACE_NOT_PRODUCTION', result.stdout)


if __name__ == '__main__': unittest.main(verbosity=2)
