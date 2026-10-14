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

if __name__ == '__main__': unittest.main()
