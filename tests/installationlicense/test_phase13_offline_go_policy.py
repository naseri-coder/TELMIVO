"""No Docker, Go, GitHub, database or Internet access required for these tests."""
import unittest
from phase13_offline_go_policy import (
    audit_go_docker_argv, offline_go_env_flags, validate_go_network,
)


def valid_args(network='none'):
    return ['docker', 'run', '--pull=never', '--rm', '--network', network,
            '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges',
            *offline_go_env_flags(),
            'sha256:' + 'a' * 64, 'go', 'test', './authority']


class OfflineGoPolicyTests(unittest.TestCase):
    def test_network_none_allowed(self):
        self.assertEqual(validate_go_network('none'), 'none')

    def test_only_exact_reviewed_pg_namespace_allowed(self):
        self.assertEqual(validate_go_network('container:lic-a9-joint-aabbccddeeff'),
                         'container:lic-a9-joint-aabbccddeeff')

    def test_no_host_or_bridge_network(self):
        for value in ('bridge', 'host', 'default', 'slirp4netns', 'container:postgres',
                      'container:lic-a9-joint-aa', 'container:lic-a9-joint-AABBCCDDEEFF',
                      'container:lic-a9-joint-aabbccddeeff_more', '', 'none ', None, True):
            with self.subTest(value=value), self.assertRaises(ValueError):
                validate_go_network(value)

    def test_offline_flags_have_exact_five_settings(self):
        result = offline_go_env_flags()
        self.assertEqual(result.count('--env'), 5)
        self.assertIn('GOPROXY=off', result)
        self.assertIn('GOSUMDB=off', result)
        self.assertIn('GOTOOLCHAIN=local', result)

    def test_precompile_disconnected_network_is_valid(self):
        audit_go_docker_argv(valid_args())

    def test_postgres_namespace_is_allowed_when_exact(self):
        audit_go_docker_argv(valid_args('container:lic-a9-joint-aabbccddeeff'))

    def test_network_host_rejected_even_with_offline_flags(self):
        with self.assertRaises(ValueError):
            audit_go_docker_argv(valid_args('host'))

    def test_network_bridge_rejected_even_with_offline_flags(self):
        with self.assertRaises(ValueError):
            audit_go_docker_argv(valid_args('bridge'))

    def test_second_network_argument_cannot_override_isolation(self):
        argv = valid_args() + ['--network', 'bridge']
        with self.assertRaises(ValueError):
            audit_go_docker_argv(argv)

    def test_missing_offline_proxy_fails_closed(self):
        argv = valid_args()
        argv.remove('GOPROXY=off')
        with self.assertRaises(ValueError):
            audit_go_docker_argv(argv)

    def test_proxy_override_fails_closed(self):
        argv = valid_args() + ['--env', 'GOPROXY=https://proxy.golang.org']
        with self.assertRaises(ValueError):
            audit_go_docker_argv(argv)

    def test_toolchain_auto_download_fails_closed(self):
        argv = valid_args()
        argv[argv.index('GOTOOLCHAIN=local')] = 'GOTOOLCHAIN=auto'
        with self.assertRaises(ValueError):
            audit_go_docker_argv(argv)

    def test_modules_sumdb_network_fails_closed(self):
        argv = valid_args()
        argv[argv.index('GOSUMDB=off')] = 'GOSUMDB=sum.golang.org'
        with self.assertRaises(ValueError):
            audit_go_docker_argv(argv)

    def test_port_publish_flag_fails_closed(self):
        with self.assertRaises(ValueError):
            audit_go_docker_argv(valid_args() + ['-p', '5432:5432'])

    def test_missing_immutable_pull_policy_fails_closed(self):
        argv = valid_args()
        argv.remove('--pull=never')
        with self.assertRaises(ValueError):
            audit_go_docker_argv(argv)

    def test_privileged_test_image_fails_closed(self):
        with self.assertRaises(ValueError):
            audit_go_docker_argv(valid_args() + ['--privileged'])

    def test_untyped_command_argument_fails_closed(self):
        with self.assertRaises(ValueError):
            audit_go_docker_argv(valid_args() + [None])

    def test_network_equals_alias_cannot_override_isolation(self):
        with self.assertRaises(ValueError):
            audit_go_docker_argv(valid_args() + ['--network=bridge'])

    def test_docker_net_alias_cannot_override_isolation(self):
        with self.assertRaises(ValueError):
            audit_go_docker_argv(valid_args() + ['--net=host'])

    def test_env_equals_proxy_override_fails_closed(self):
        with self.assertRaises(ValueError):
            audit_go_docker_argv(valid_args() + ['--env=GOPROXY=https://proxy.invalid'])

    def test_cap_add_alias_fails_closed(self):
        with self.assertRaises(ValueError):
            audit_go_docker_argv(valid_args() + ['--cap-add=SYS_ADMIN'])

    def test_privileged_true_fails_closed(self):
        with self.assertRaises(ValueError):
            audit_go_docker_argv(valid_args() + ['--privileged=true'])

    def test_publish_equals_fails_closed(self):
        with self.assertRaises(ValueError):
            audit_go_docker_argv(valid_args() + ['--publish=5432:5432'])

    def test_missing_network_fails_closed(self):
        argv = valid_args()
        i = argv.index('--network')
        del argv[i:i+2]
        with self.assertRaises(ValueError):
            audit_go_docker_argv(argv)

    def test_unreviewed_environment_duplicate_fails_closed(self):
        with self.assertRaises(ValueError):
            audit_go_docker_argv(valid_args() + ['--env', 'GOPROXY=off'])

    def test_absent_drop_caps_fails_closed(self):
        argv = valid_args()
        i = argv.index('--cap-drop')
        del argv[i:i+2]
        with self.assertRaises(ValueError):
            audit_go_docker_argv(argv)


if __name__ == '__main__':
    unittest.main()
