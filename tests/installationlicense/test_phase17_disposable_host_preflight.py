"""Isolated, read-only tests for Phase17 nonproduction host admission."""
import contextlib
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock

import phase17_disposable_host_preflight as preflight


class Phase17HostAdmissionTests(unittest.TestCase):
    def setUp(self):
        self.facts = {
            'os_id': 'ubuntu', 'os_version': '24.04',
            'vcpu': 4, 'memory_bytes': 8 * 1024**3,
            'free_bytes': 40 * 1024**3,
            'python_310': True, 'git_cli': True, 'docker_cli': True,
            'socket_path_safe': True, 'docker_socket': True,
            'socket_world_writable': False, 'scratch_safe': True,
            'sensitive_env_names': [],
        }

    def result(self, **changes):
        return preflight.evaluate({**self.facts, **changes})

    def test_full_static_resources_never_mean_approved_host(self):
        r = self.result()
        self.assertEqual(r['blocking_codes'], [])
        self.assertEqual(r['verdict'], 'STATIC_PREREQUISITES_PRESENT_EXTERNAL_APPROVAL_REQUIRED')
        self.assertFalse(r['independent_host_approval_verified'])
        self.assertFalse(r['production_authorized'])

    def test_ubuntu_2404_is_supported(self):
        self.assertNotIn('UNSUPPORTED_OS', self.result()['blocking_codes'])

    def test_debian_13_is_supported(self):
        self.assertNotIn('UNSUPPORTED_OS', self.result(os_id='debian', os_version='13')['blocking_codes'])

    def test_other_or_malformed_os_refuses(self):
        for platform in [('debian', '12'), ('ubuntu', '22.04'), ('alpine', '3'), ('', '')]:
            with self.subTest(platform=platform):
                self.assertIn('UNSUPPORTED_OS', self.result(os_id=platform[0], os_version=platform[1])['blocking_codes'])

    def test_cpu_requirement_at_least_two(self):
        self.assertIn('INSUFFICIENT_VCPU', self.result(vcpu=1)['blocking_codes'])
        self.assertNotIn('INSUFFICIENT_VCPU', self.result(vcpu=2)['blocking_codes'])

    def test_ram_requirement_four_gib(self):
        self.assertIn('INSUFFICIENT_RAM', self.result(memory_bytes=4*1024**3-1)['blocking_codes'])
        self.assertNotIn('INSUFFICIENT_RAM', self.result(memory_bytes=4*1024**3)['blocking_codes'])

    def test_free_disk_requirement_ten_gib(self):
        self.assertIn('INSUFFICIENT_FREE_DISK', self.result(free_bytes=10*1024**3-1)['blocking_codes'])
        self.assertNotIn('INSUFFICIENT_FREE_DISK', self.result(free_bytes=10*1024**3)['blocking_codes'])

    def test_missing_git_blocks(self):
        self.assertIn('GIT_NOT_INSTALLED', self.result(git_cli=False)['blocking_codes'])

    def test_missing_docker_binary_blocks(self):
        self.assertIn('DOCKER_CLI_NOT_INSTALLED', self.result(docker_cli=False)['blocking_codes'])

    def test_missing_local_docker_socket_blocks(self):
        self.assertIn('LOCAL_DOCKER_SOCKET_MISSING', self.result(docker_socket=False)['blocking_codes'])

    def test_world_writable_socket_blocks(self):
        self.assertIn('DOCKER_SOCKET_WORLD_WRITABLE', self.result(socket_world_writable=True)['blocking_codes'])

    def test_unreviewed_socket_path_blocks(self):
        self.assertIn('UNSAFE_DOCKER_SOCKET_PATH', self.result(socket_path_safe=False)['blocking_codes'])

    def test_unreviewed_scratch_blocks(self):
        self.assertIn('SCRATCH_DIRECTORY_UNSAFE', self.result(scratch_safe=False)['blocking_codes'])

    def test_missing_python310_blocks(self):
        self.assertIn('PYTHON_310_REQUIRED', self.result(python_310=False)['blocking_codes'])

    def test_production_env_names_block_without_exposing_values(self):
        x = self.result(sensitive_env_names=['DATABASE_URL', 'TELEGRAM_BOT_TOKEN'])
        self.assertIn('SENSITIVE_ENVIRONMENT_PRESENT', x['blocking_codes'])
        self.assertEqual(x['sensitive_environment_variable_names'], ['DATABASE_URL','TELEGRAM_BOT_TOKEN'])
        self.assertNotIn('password123', json.dumps(x))

    def test_reviewed_socket_paths(self):
        for path in ('/var/run/docker.sock', '/run/user/1000/docker.sock', '/run/user/0/docker.sock'):
            self.assertTrue(preflight.admissible_socket_path(path), path)

    def test_unsafe_socket_paths(self):
        for path in ('/tmp/docker.sock', 'tcp://127.0.0.1:2375', 'ssh://prod',
                     '/run/other/docker.sock', '/run/user/1/../0/docker.sock',
                     '/var/run/docker.sock/../docker.sock', '/run/user/bot/docker.sock',
                     'docker.sock', None):
            with self.subTest(path=path):
                self.assertFalse(preflight.admissible_socket_path(path))

    def test_os_release_parser_not_shell(self):
        self.assertEqual(preflight.parse_os_release('ID=ubuntu\nVERSION_ID="24.04"\n'),('ubuntu', '24.04'))
        with self.assertRaises(ValueError):
            preflight.parse_os_release('ID=ubuntu\nVERSION_ID=$(rm -rf /)')

    def test_memtotal_parser_converts_kib(self):
        self.assertEqual(preflight.parse_memtotal('MemTotal:       4096000 kB\n'),4096000*1024)
        with self.assertRaises(ValueError):
            preflight.parse_memtotal('SwapFree: 1234 kB\n')

    def test_real_gather_is_read_only_without_subprocess(self):
        with (mock.patch.object(subprocess, 'run', side_effect=AssertionError('no process allowed')),
              mock.patch('socket.socket.connect', side_effect=AssertionError('no network'))):
            result=preflight.gather(scratch_dir=Path('/tmp'), docker_socket='/var/run/docker.sock',
                                    environ={}, which=lambda name: None)
        self.assertEqual(result['docker_cli'], False)
        self.assertEqual(result['sensitive_env_names'], [])

    def test_real_gather_uses_injected_environment_without_values(self):
        result=preflight.gather(scratch_dir=Path('/tmp'), docker_socket='/var/run/docker.sock',
                                environ={'DOCKER_HOST': 'ssh://prod','DATABASE_URL':'super-secret'},
                                which=lambda x: '/usr/bin/'+x)
        self.assertEqual(result['sensitive_env_names'],['DATABASE_URL','DOCKER_HOST'])
        self.assertNotIn('super-secret', json.dumps(result))
        self.assertNotIn('ssh://prod', json.dumps(result))

    def test_relative_scratch_denied(self):
        r=preflight.gather(scratch_dir=Path('relative'), docker_socket='/var/run/docker.sock',
                           environ={}, which=lambda x: None)
        self.assertFalse(r['scratch_safe'])

    def test_structural_results_never_assert_execution(self):
        for changes in ({}, {'docker_cli':False}, {'sensitive_env_names':['DOCKER_HOST']}):
            s=self.result(**changes)
            for key in ('docker_daemon_contacted','containers_created','postgresql_started',
                        'production_host_identity_verified','independent_host_approval_verified',
                        'private_repositories_cloned','real_joint_e2e_executed','production_authorized'):
                self.assertFalse(s[key], key)

    def test_cli_does_not_print_host_identity_or_credentials(self):
        sink=io.StringIO()
        with contextlib.redirect_stdout(sink):
            rc=preflight.main(['--scratch-dir','/tmp'])
        result=json.loads(sink.getvalue())
        self.assertIn(rc, (2,3))
        self.assertNotIn('host_name',result)
        self.assertFalse(result['production_authorized'])

    def test_missing_docker_is_real_observed_blocker_in_this_container(self):
        observed=preflight.gather(scratch_dir=Path('/mnt/data'),docker_socket='/var/run/docker.sock',
                                  environ={},which=lambda x: '/usr/bin/git' if x=='git' else None)
        result=preflight.evaluate(observed)
        self.assertIn('DOCKER_CLI_NOT_INSTALLED', result['blocking_codes'])
        self.assertIn('LOCAL_DOCKER_SOCKET_MISSING',result['blocking_codes'])


if __name__ == '__main__':
    unittest.main()
