"""No-Docker execution required: strict mock tests for the isolated Docker runner."""
import contextlib
import io
import os
from pathlib import Path
import subprocess
import unittest
from unittest import mock

import run_joint_docker_only_e2e as runner


ARGS = ['--authority', '/tmp/authority-fixture', '--product', '/tmp/product-fixture',
        '--acknowledge-disposable-host']


class DockerOnlyTests(unittest.TestCase):
    def test_go_image_is_exact_and_image_exists(self):
        self.assertEqual(runner.GO_IMAGE, 'golang:1.27.0-bookworm')
        self.assertEqual(runner.PG_IMAGE, 'postgres:16')

    def test_prevents_remote_network_and_relative_mounts(self):
        for name in ['host', 'none', 'container:production', 'container:lic-a9-joint-bad']:
            with self.subTest(name=name), self.assertRaises(ValueError):
                runner.go_docker_command(Path('/fixture'), Path('/product'), Path('/cache'), name, ['go', 'version'])
        with self.assertRaises(ValueError):
            runner.go_docker_command(Path('fixture'), Path('/product'), Path('/cache'), 'bridge', ['go', 'version'])

    def test_mounts_product_read_only_without_host_secrets(self):
        cmd = runner.go_docker_command(Path('/fixture'), Path('/product'), Path('/cache'), 'bridge', ['go', 'version'])
        self.assertIn('type=bind,src=/product,dst=/product,readonly', cmd)
        self.assertIn('--cap-drop', cmd)
        self.assertIn('ALL', cmd)
        self.assertIn('no-new-privileges', cmd)
        self.assertNotIn('-p', cmd)
        self.assertNotIn('/var/run/docker.sock', ' '.join(cmd))
        self.assertEqual(cmd[-3:], [runner.GO_IMAGE, 'go', 'version'])

    def test_rejects_inherited_remote_docker_target_before_any_calls(self):
        with (mock.patch.dict(os.environ, {'DOCKER_HOST': 'ssh://server'}, clear=True),
              mock.patch.object(runner, 'run') as commands,
              mock.patch.object(runner, 'check_checkout') as check,
              contextlib.redirect_stderr(io.StringIO())):
            result = runner.main(ARGS)
        self.assertEqual(result, 3)
        check.assert_not_called()
        commands.assert_not_called()

    def test_invalid_source_refuses_docker(self):
        with (mock.patch.object(runner, 'check_checkout', side_effect=ValueError('wrong commit')),
              mock.patch.object(runner, 'run') as commands,
              contextlib.redirect_stderr(io.StringIO())):
            result = runner.main(ARGS)
        self.assertEqual(result, 3)
        commands.assert_not_called()

    def simulate(self, *, go_version='go version go1.27.0 linux/amd64\n', fail_test=False):
        calls = []
        def fake_run(argv, **kwargs):
            calls.append(list(argv))
            if argv[:3] == ['docker', 'context', 'inspect']:
                return subprocess.CompletedProcess(argv, 0, stdout='unix:///var/run/docker.sock\n')
            if argv[:2] == ['docker', 'run'] and 'go' in argv and 'version' in argv:
                return subprocess.CompletedProcess(argv, 0, stdout=go_version)
            if fail_test and '^TestE2E[1-6]$' in argv:
                raise subprocess.CalledProcessError(1, argv)
            return subprocess.CompletedProcess(argv, 0, stdout='')
        def fake_subprocess(argv, **kwargs):
            calls.append(list(argv))
            return subprocess.CompletedProcess(argv, 0, stdout='')
        with (mock.patch.object(runner, 'check_checkout'),
              mock.patch.object(runner, 'validate_authority_archive_provenance'),
              mock.patch.object(runner, 'check_product_checkout'),
              mock.patch.object(runner, 'run', side_effect=fake_run),
              mock.patch.object(runner.subprocess, 'run', side_effect=fake_subprocess),
              contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO())):
            result = runner.main(ARGS)
        return result, calls

    def test_compiler_runs_before_unpublished_database(self):
        code, calls = self.simulate()
        self.assertEqual(code, 0)
        compiler = next(i for i, c in enumerate(calls) if '^$' in c)
        pg_start = next(i for i, c in enumerate(calls) if c[:2] == ['docker', 'run'] and runner.PG_IMAGE in c)
        e2e = next(i for i, c in enumerate(calls) if '^TestE2E[1-6]$' in c)
        stop = next(i for i, c in enumerate(calls) if c[:2] == ['docker', 'stop'])
        self.assertLess(compiler, pg_start)
        self.assertLess(pg_start, e2e)
        self.assertLess(e2e, stop)
        pg_argv = calls[pg_start]
        self.assertEqual(pg_argv[pg_argv.index('--network') + 1], 'none')
        self.assertNotIn('-p', pg_argv)
        e2e_argv = calls[e2e]
        net = e2e_argv[e2e_argv.index('--network') + 1]
        self.assertRegex(net, r'^container:lic-a9-joint-[0-9a-f]{12}$')
        self.assertIn('GOPROXY=off', e2e_argv)
        self.assertIn('GOSUMDB=off', e2e_argv)
        self.assertTrue(all('go' != c[0] for c in calls))

    def test_failed_e2e_still_stops_postgres(self):
        status, calls = self.simulate(fail_test=True)
        self.assertEqual(status, 3)
        self.assertEqual(sum(c[:2] == ['docker', 'stop'] for c in calls), 1)

    def test_cleanup_error_never_allows_success_verdict(self):
        for mode in ('nonzero', 'timeout', 'missing'):
            calls = []
            capture = io.StringIO()
            def fake_run(argv, **kwargs):
                calls.append(list(argv))
                if argv[:3] == ['docker', 'context', 'inspect']:
                    return subprocess.CompletedProcess(argv, 0, stdout='unix:///var/run/docker.sock\n')
                if argv[:2] == ['docker', 'run'] and 'go' in argv and 'version' in argv:
                    return subprocess.CompletedProcess(argv, 0, stdout='go version go1.27.0 linux/amd64\n')
                return subprocess.CompletedProcess(argv, 0, stdout='')
            def fake_subprocess(argv, **kwargs):
                calls.append(list(argv))
                if argv[:2] == ['docker', 'stop']:
                    if mode == 'timeout':
                        raise subprocess.TimeoutExpired(argv, 60)
                    if mode == 'missing':
                        raise FileNotFoundError('Docker unavailable on cleanup')
                    return subprocess.CompletedProcess(argv, 1, stdout='', stderr='stop failed')
                return subprocess.CompletedProcess(argv, 0, stdout='')
            with self.subTest(mode=mode):
                with (mock.patch.object(runner, 'check_checkout'),
                      mock.patch.object(runner, 'validate_authority_archive_provenance'),
                      mock.patch.object(runner, 'check_product_checkout'),
                      mock.patch.object(runner, 'run', side_effect=fake_run),
                      mock.patch.object(runner.subprocess, 'run', side_effect=fake_subprocess),
                      contextlib.redirect_stdout(capture),
                      contextlib.redirect_stderr(io.StringIO())):
                    self.assertEqual(runner.main(ARGS), 3)
                self.assertNotIn('LIC_A9_JOINT_DOCKER_GO127_POSTGRES16_E2E_PASS', capture.getvalue())
                self.assertEqual(sum(c[:2] == ['docker', 'stop'] for c in calls), 1)

    def test_wrong_container_go_never_starts_postgres(self):
        status, calls = self.simulate(go_version='go version go1.26.8 linux/amd64\n')
        self.assertEqual(status, 3)
        self.assertFalse(any(c[:2] == ['docker', 'run'] and runner.PG_IMAGE in c for c in calls))

    def test_early_argv_check_rejects_same_source(self):
        with (mock.patch.object(runner, 'run') as commands,
              contextlib.redirect_stderr(io.StringIO())):
            code = runner.main(['--authority', '/tmp/same', '--product', '/tmp/same',
                                '--acknowledge-disposable-host'])
        self.assertEqual(code, 3)
        commands.assert_not_called()


if __name__ == '__main__':
    unittest.main()
