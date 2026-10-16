"""No-Docker unit tests for offline LIC-A9 Phase09 prerequisites."""
import importlib.util
from pathlib import Path
import json
import subprocess
import sys
import unittest
from unittest import mock


SCRIPT = Path(__file__).with_name('phase09_host_requirements.py')
SPEC = importlib.util.spec_from_file_location('phase09_host_requirements', SCRIPT)
preflight = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(preflight)


class Phase09HostRequirementTests(unittest.TestCase):
    def test_no_docker_client_blocks(self):
        s = preflight.scan(which=lambda x: '/usr/bin/git' if x == 'git' else None, env={})
        self.assertEqual(s['verdict'], 'BLOCKED_NO_DOCKER_CLIENT')
        self.assertFalse(s['docker_client_present'])
        self.assertFalse(s['release_certified'])

    def test_docker_available_is_not_engine_certification(self):
        s = preflight.scan(which=lambda _: '/some/executable', env={})
        self.assertEqual(s['verdict'], 'STATIC_PREREQUISITES_SEEN_ENGINE_NOT_VERIFIED')
        self.assertFalse(s['docker_daemon_contacted'])
        self.assertFalse(s['docker_images_checked'])
        self.assertFalse(s['postgresql_tests_run'])

    def test_no_host_go_required_by_docker_only_runner(self):
        s = preflight.scan(which=lambda _: '/some/executable', env={})
        self.assertFalse(s['local_go127_required'])
        self.assertFalse(s['local_postgresql16_required'])

    def test_forbidden_docker_host_overrides_fake_docker(self):
        s = preflight.scan(which=lambda _: '/some/executable',
                           env={'DOCKER_HOST': 'ssh://production'})
        self.assertEqual(s['verdict'], 'BLOCKED_POSSIBLE_PRODUCTION_ENV')
        self.assertEqual(s['forbidden_environment_variable_names'], ['DOCKER_HOST'])

    def test_all_sensitive_variable_names_reported_without_values(self):
        context = {key: 'HIDDEN_SECRET_VALUE' for key in preflight.BLOCKING_ENV}
        s = preflight.scan(which=lambda _: '/some/executable', env=context)
        self.assertEqual(set(s['forbidden_environment_variable_names']), set(context))
        self.assertNotIn('HIDDEN_SECRET_VALUE', json.dumps(s))

    def test_empty_untrusted_environment_does_not_trigger_false_positive(self):
        s = preflight.scan(which=lambda _: '/some/executable', env={'PGHOST': ''})
        self.assertEqual(s['verdict'], 'STATIC_PREREQUISITES_SEEN_ENGINE_NOT_VERIFIED')

    def test_missing_git_blocks(self):
        s = preflight.scan(which=lambda x: '/docker' if x == 'docker' else None, env={})
        self.assertEqual(s['verdict'], 'BLOCKED_MISSING_LOCAL_PREREQUISITE')

    def test_result_always_no_go(self):
        for which, env in (
            (lambda _: '/fake', {}),
            (lambda _: None, {}),
            (lambda _: '/fake', {'DOCKER_HOST': 'ssh://prod'}),
        ):
            s = preflight.scan(which=which, env=env)
            for key in ('go_tests_run', 'postgresql_tests_run', 'production_changed',
                        'release_certified', 'docker_daemon_contacted'):
                self.assertIs(s[key], False)

    def test_real_scanner_does_not_invoke_docker(self):
        with mock.patch.object(preflight.shutil, 'which',
                               side_effect=lambda x: None if x == 'docker' else '/usr/bin/git'):
            with mock.patch.object(subprocess, 'run', side_effect=AssertionError('unexpected process')):
                state = preflight.scan(env={})
        self.assertEqual(state['verdict'], 'BLOCKED_NO_DOCKER_CLIENT')


if __name__ == '__main__':
    unittest.main()
