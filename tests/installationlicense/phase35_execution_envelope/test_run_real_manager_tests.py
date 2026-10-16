"""Self-tests ONLY the Phase35 offline execution envelope, not Product Go tests.

Mock-Go positive paths must never be described as real Go1.27 functional PASS.
"""
from pathlib import Path
import os
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name('run_real_manager_tests.sh')


def run(*args, cwd=None, env=None):
    return subprocess.run(args, cwd=cwd, env=env, capture_output=True, text=True, check=False)


class EnvelopeTests(unittest.TestCase):
    def setUp(self):
        self.workspace = tempfile.TemporaryDirectory(prefix='lic-a9-phase35-selftest-')
        self.addCleanup(self.workspace.cleanup)
        self.p = Path(self.workspace.name)
        self.repo = self.p/'repo'
        self.repo.mkdir()
        self.bin = self.p/'bin'
        self.bin.mkdir()
        self.go_call_log = self.p/'go_called.txt'
        assert run('git', 'init', '-q', cwd=self.repo).returncode == 0
        assert run('git', 'config', 'user.name', 'LIC-A9 test', cwd=self.repo).returncode == 0
        assert run('git', 'config', 'user.email', 'lic-a9@example.invalid', cwd=self.repo).returncode == 0
        self.branch = 'development/lic-a9-phase35-selftest'
        assert run('git', 'checkout', '-q', '-b', self.branch, cwd=self.repo).returncode == 0
        license_dir = self.repo/'internal/platform/installationlicense'
        license_dir.mkdir(parents=True)
        for name in ['phase34_real_manager_effect_test.go', 'phase19_session_fencing_test.go']:
            (license_dir/name).write_text('package installationlicense\n')
        (self.repo/'go.mod').write_text('module github.com/naseri-coder/TELMIVO\n\ngo 1.27.0\n')
        (self.repo/'go.sum').write_text('offline-test-marker\n')
        assert run('git', 'add', '-A', cwd=self.repo).returncode == 0
        assert run('git', 'commit', '-qm', 'mock-only execution gate fixture', cwd=self.repo).returncode == 0
        self.sha = run('git', 'rev-parse', 'HEAD', cwd=self.repo).stdout.strip()
        self.go = self.bin/'go'
        self.go.write_text('''#!/usr/bin/env bash
set -eu
if [[ "$1" == version ]]; then printf 'go version %s linux/amd64\\n' "${PHASE35_FAKE_GO_VERSION:-go1.27.0}"; exit; fi
if [[ "$1" == env && "$2" == GOOS ]]; then echo linux; exit; fi
if [[ "$1" == env && "$2" == CGO_ENABLED ]]; then echo 1; exit; fi
if [[ "$1" == test || "$1" == vet ]]; then
  printf '%s\\n' "$*" >> "$PHASE35_FAKE_CALL_LOG"
  if [[ "${PHASE35_FAKE_FAIL_TEST:-}" == YES && "$1" == test ]]; then echo 'mock test fails'; exit 3; fi
  echo 'MOCK_ONLY_NOT_REAL_GO_EXECUTION'
  exit 0
fi
exit 12
''')
        self.go.chmod(0o755)
        self.env = os.environ.copy()
        self.env.update({
            'LIC_A9_DISPOSABLE': 'YES',
            'LIC_A9_EXPECT_HEAD': self.sha,
            'LIC_A9_LOG_DIR': str(self.p),
            'PHASE35_FAKE_GO_VERSION': 'go1.27.0',
            'PHASE35_FAKE_CALL_LOG': str(self.go_call_log),
            'GITHUB_ACTIONS': 'false',
            'APP_ENV': 'test',
            'PATH': f'{self.bin}:{self.env["PATH"]}',
        })

    def checked(self, env):
        return run('bash', str(SCRIPT), cwd=self.repo, env=env)

    def test_requires_explicit_disposable(self):
        env = dict(self.env, LIC_A9_DISPOSABLE='')
        r = self.checked(env)
        self.assertEqual(r.returncode, 78, r.stdout+r.stderr)
        self.assertFalse(self.go_call_log.exists())

    def test_forbids_github_actions(self):
        r = self.checked(dict(self.env, GITHUB_ACTIONS='true'))
        self.assertEqual(r.returncode, 78)
        self.assertFalse(self.go_call_log.exists())

    def test_forbids_production(self):
        r = self.checked(dict(self.env, APP_ENV='production'))
        self.assertEqual(r.returncode, 78)
        self.assertFalse(self.go_call_log.exists())

    def test_rejects_older_toolchain_without_running_tests(self):
        r = self.checked(dict(self.env, PHASE35_FAKE_GO_VERSION='go1.23.2'))
        self.assertEqual(r.returncode, 78, r.stdout+r.stderr)
        self.assertIn('Go 1.27+', r.stderr)
        self.assertFalse(self.go_call_log.exists())

    def test_rejects_sha_mismatch(self):
        r = self.checked(dict(self.env, LIC_A9_EXPECT_HEAD='0'*40))
        self.assertEqual(r.returncode, 78)
        self.assertFalse(self.go_call_log.exists())

    def test_rejects_dirty_checkout(self):
        (self.repo/'untracked.txt').write_text('UNTRUSTED\n')
        r = self.checked(self.env)
        self.assertEqual(r.returncode, 78)
        self.assertFalse(self.go_call_log.exists())

    def test_rejects_wrong_branch(self):
        assert run('git', 'checkout', '-qb', 'main', cwd=self.repo).returncode == 0
        r = self.checked(self.env)
        self.assertEqual(r.returncode, 78)
        self.assertFalse(self.go_call_log.exists())

    def test_actual_local_toolchain_blocks_go_123_without_running(self):
        actual = run('/usr/local/go/bin/go', 'version')
        if 'go1.23.2' not in actual.stdout:
            self.skipTest('diagnostic requires local Go 1.23.2 toolchain')
        env = dict(self.env)
        env['PATH'] = os.environ['PATH']
        r = self.checked(env)
        self.assertEqual(r.returncode, 78, r.stdout+r.stderr)
        self.assertIn('Go 1.27+', r.stderr)
        self.assertFalse(self.go_call_log.exists())

    def test_external_effect_opt_in_is_rejected(self):
        r = self.checked(dict(self.env, LIC_A9_ALLOW_SIDE_EFFECTS='YES'))
        self.assertEqual(r.returncode, 78)
        self.assertFalse(self.go_call_log.exists())

    def test_wrong_module_is_rejected(self):
        (self.repo/'go.mod').write_text('module different.invalid/x\n\ngo 1.27.0\n')
        r = self.checked(self.env)
        self.assertEqual(r.returncode, 78)
        self.assertFalse(self.go_call_log.exists())

    def test_mock_go_127_invokes_expected_commands_no_network(self):
        r = self.checked(self.env)
        self.assertEqual(r.returncode, 0, r.stdout+r.stderr)
        commands = self.go_call_log.read_text().splitlines()
        self.assertEqual(len(commands), 3)
        self.assertIn("-run ^TestLICA9Phase34", commands[0])
        self.assertIn("go", str(self.go))
        self.assertTrue(commands[1].startswith('test -race'))
        self.assertTrue(commands[2].startswith('vet '))
        self.assertNotIn('download', ' '.join(commands))
        self.assertIn('LIC_A9_PHASE35_EXECUTION_PASS', r.stdout)

    def test_mock_failure_does_not_claim_pass(self):
        r = self.checked(dict(self.env, PHASE35_FAKE_FAIL_TEST='YES'))
        self.assertEqual(r.returncode, 3)
        self.assertNotIn('LIC_A9_PHASE35_EXECUTION_PASS', r.stdout)
        self.assertEqual(len(self.go_call_log.read_text().splitlines()), 1)


if __name__ == '__main__':
    unittest.main(verbosity=2)
