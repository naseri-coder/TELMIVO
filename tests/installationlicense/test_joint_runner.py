import contextlib
import io
import os
from pathlib import Path
import subprocess
import unittest
from unittest import mock

import run_joint_postgres_e2e as runner

class RunnerTests(unittest.TestCase):
    def test_version_equal(self):
        runner.check_go_version('go version go1.27.0 linux/amd64\n')

    def test_version_newer(self):
        runner.check_go_version('go version go1.28.1 linux/amd64\n')

    def test_old_version_rejected(self):
        with self.assertRaises(ValueError):
            runner.check_go_version('go version go1.23.2 linux/amd64')

    def test_garbled_go_version_rejected(self):
        with self.assertRaises(ValueError):
            runner.check_go_version('installed Go version unknown')

    def test_port_must_be_exactly_one_loopback(self):
        self.assertEqual(runner.loopback_port('127.0.0.1:59123\n'), 59123)
        for output in ('0.0.0.0:59123', ':::59123', '127.0.0.1:59123\n127.0.0.1:59124', '127.0.0.1:12'):
            with self.subTest(port=output), self.assertRaises(ValueError):
                runner.loopback_port(output)

    def test_go_environment_strips_sensitive_tokens(self):
        with mock.patch.dict(os.environ, {'TELEGRAM_BOT_TOKEN': 'NEVER_PASS', 'TELMIVO_ENV': '',
                                          'PATH': '/bin', 'GOTOOLCHAIN': 'auto'}, clear=True):
            env = runner.go_test_env()
        self.assertNotIn('TELEGRAM_BOT_TOKEN', env)
        self.assertEqual(env['GOTOOLCHAIN'], 'local')
        self.assertEqual(env['GOWORK'], 'off')

    def test_missing_authority_fail_closed_no_docker(self):
        with (mock.patch.object(runner, 'run') as commands,
              contextlib.redirect_stderr(io.StringIO())):
            rc = runner.main(['--authority', '/tmp/not-a-checkout-x',
                              '--product', '/tmp/not-a-checkout-y',
                              '--acknowledge-disposable-host'])
        self.assertEqual(rc, 3)
        commands.assert_not_called()

    def test_same_checkout_refuses_without_git_or_docker(self):
        with (mock.patch.object(runner, 'run') as commands,
              contextlib.redirect_stderr(io.StringIO())):
            rc = runner.main(['--authority', '/tmp/same',
                              '--product', '/tmp/same',
                              '--acknowledge-disposable-host'])
        self.assertEqual(rc, 3)
        commands.assert_not_called()

    def test_pinned_clean_checks(self):
        with mock.patch.object(runner, 'git', side_effect=['a'*40, '']) as g:
            runner.check_checkout(Path('/tmp'), 'a'*40)
        self.assertEqual(g.call_count, 2)
        with mock.patch.object(runner, 'git', side_effect=['b'*40]):
            with self.assertRaises(ValueError):
                runner.check_checkout(Path('/tmp'), 'a'*40)
        with mock.patch.object(runner, 'git', side_effect=['a'*40, ' M file.go']):
            with self.assertRaises(ValueError):
                runner.check_checkout(Path('/tmp'), 'a'*40)

    def test_reject_inherited_external_docker_target(self):
        with (mock.patch.dict(os.environ, {'DOCKER_HOST': 'ssh://production'}, clear=True),
              mock.patch.object(runner, 'check_checkout') as check,
              mock.patch.object(runner, 'run') as commands,
              contextlib.redirect_stderr(io.StringIO())):
            rc = runner.main(['--authority','/tmp/repo-1','--product','/tmp/repo-2',
                              '--acknowledge-disposable-host'])
        self.assertEqual(rc, 3)
        check.assert_not_called()
        commands.assert_not_called()

    def test_mocked_joint_path_compiles_before_docker_and_always_stops(self):
        calls = []
        def fake_run(argv, **kwargs):
            calls.append(argv)
            if argv[:2] == ['go', 'version']:
                return subprocess.CompletedProcess(argv, 0, stdout='go version go1.27.0 linux/amd64\n')
            if argv[:3] == ['docker', 'context', 'inspect']:
                return subprocess.CompletedProcess(argv, 0, stdout='unix:///var/run/docker.sock\n')
            if argv[:2] == ['docker', 'port']:
                return subprocess.CompletedProcess(argv, 0, stdout='127.0.0.1:59123\n')
            return subprocess.CompletedProcess(argv, 0, stdout='')
        def fake_subprocess(argv, **kwargs):
            calls.append(argv)
            return subprocess.CompletedProcess(argv, 0)
        with (mock.patch.object(runner, 'check_checkout'),
              mock.patch.object(runner, 'run', side_effect=fake_run),
              mock.patch.object(runner.subprocess, 'run', side_effect=fake_subprocess),
              contextlib.redirect_stdout(io.StringIO())):
            rc = runner.main(['--authority','/tmp/repo-1','--product','/tmp/repo-2',
                              '--acknowledge-disposable-host'])
        self.assertEqual(rc, 0)
        compile_idx = next(i for i, c in enumerate(calls) if c[:3] == ['go','test','-count=1'] and '-run' in c and '^
 in c)
        docker_run_idx = next(i for i, c in enumerate(calls) if c[:2] == ['docker','run'])
        joint_idx = next(i for i, c in enumerate(calls) if c[:3] == ['go','test','-count=1'] and '-timeout' in c)
        stop_idx = next(i for i, c in enumerate(calls) if c[:2] == ['docker','stop'])
        self.assertLess(compile_idx, docker_run_idx)
        self.assertLess(docker_run_idx, joint_idx)
        self.assertLess(joint_idx, stop_idx)

    def test_mocked_joint_failure_stops_test_container(self):
        calls = []
        def fake_run(argv, **kwargs):
            calls.append(argv)
            if argv[:2] == ['go','version']:
                return subprocess.CompletedProcess(argv, 0, stdout='go version go1.27.0 linux/amd64\n')
            if argv[:3] == ['docker','context','inspect']:
                return subprocess.CompletedProcess(argv, 0, stdout='unix:///var/run/docker.sock\n')
            if argv[:2] == ['docker','port']:
                return subprocess.CompletedProcess(argv, 0, stdout='127.0.0.1:59123\n')
            if argv[:2] == ['go','test'] and '-timeout' in argv:
                raise subprocess.CalledProcessError(1, argv)
            return subprocess.CompletedProcess(argv, 0, stdout='')
        def fake_subprocess(argv, **kwargs):
            calls.append(argv)
            return subprocess.CompletedProcess(argv, 0)
        with (mock.patch.object(runner, 'check_checkout'),
              mock.patch.object(runner, 'run', side_effect=fake_run),
              mock.patch.object(runner.subprocess, 'run', side_effect=fake_subprocess),
              contextlib.redirect_stderr(io.StringIO()),
              contextlib.redirect_stdout(io.StringIO())):
            rc = runner.main(['--authority','/tmp/repo-1','--product','/tmp/repo-2',
                              '--acknowledge-disposable-host'])
        self.assertEqual(rc, 3)
        self.assertEqual(sum(x[:2] == ['docker', 'stop'] for x in calls), 1)

if __name__ == '__main__': unittest.main()
