"""No-Docker execution required: strict mock tests for the isolated Docker runner."""
import contextlib
import io
import json
import tempfile
import os
from pathlib import Path
import subprocess
import unittest
from unittest import mock

import run_joint_docker_only_e2e as runner


ARGS = ['--authority', '/tmp/authority-fixture', '--product', '/tmp/product-fixture',
        '--acknowledge-disposable-host']

GO_DISCOVERY = "\n".join(sorted(runner.COMBINED_TESTS)) + "\nok\n"
GO_SUCCESS_EVENTS = "\n".join(
    json.dumps(event) for event in (
        [{'Action': 'run', 'Package': 'example/joint', 'Test': f'TestE2E{i}'}
         for i in range(1, 7)]
        + [{'Action': 'pass', 'Package': 'example/joint', 'Test': f'TestE2E{i}'}
           for i in range(1, 7)]
        + [{'Action': 'pass', 'Package': 'example/joint'}]
    )
) + "\n"
COMBINED_EVENTS = "\n".join(json.dumps(event) for event in (
    [{'Action': 'run', 'Package': 'example/joint', 'Test': name}
     for name in sorted(runner.COMBINED_TESTS)]
    + [{'Action': 'pass', 'Package': 'example/joint', 'Test': name}
       for name in sorted(runner.COMBINED_TESTS)]
    + [{'Action': 'pass', 'Package': 'example/joint'}]
)) + "\n"


class DockerOnlyTests(unittest.TestCase):
    def test_go_image_is_exact_and_image_exists(self):
        self.assertEqual(runner.GO_IMAGE, 'golang:1.27.2-bookworm')
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

    def simulate(self, *, go_version='go version go1.27.2 linux/amd64\n',
                 fail_test=False, missing_discovery=False, missing_passes=False):
        calls = []
        def fake_run(argv, **kwargs):
            calls.append(list(argv))
            if argv[:3] == ['docker', 'context', 'inspect']:
                return subprocess.CompletedProcess(argv, 0, stdout='unix:///var/run/docker.sock\n')
            if argv[:2] == ['docker', 'run'] and 'go' in argv and 'version' in argv:
                return subprocess.CompletedProcess(argv, 0, stdout=go_version)
            if '-list' in argv:
                return subprocess.CompletedProcess(argv, 0, stdout='' if missing_discovery else GO_DISCOVERY)
            if '-json' in argv:
                if fail_test:
                    raise subprocess.CalledProcessError(1, argv)
                return subprocess.CompletedProcess(argv, 0, stdout='{"Action":"pass","Package":"example/joint"}\n' if missing_passes else COMBINED_EVENTS)
            return subprocess.CompletedProcess(argv, 0, stdout='')
        def fake_subprocess(argv, **kwargs):
            calls.append(list(argv))
            return subprocess.CompletedProcess(argv, 0, stdout='')
        with (mock.patch.object(runner, 'check_checkout'),
              mock.patch.object(runner, 'validate_authority_archive_provenance'),
              mock.patch.object(runner, 'check_product_checkout'),
              mock.patch.object(runner, 'install_a9_fixture'),
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
        discover = next(i for i, c in enumerate(calls) if '-list' in c)
        e2e = next(i for i, c in enumerate(calls) if '-json' in c)
        stop = next(i for i, c in enumerate(calls) if c[:2] == ['docker', 'stop'])
        self.assertLess(compiler, discover)
        self.assertLess(discover, pg_start)
        self.assertLess(pg_start, e2e)
        self.assertLess(e2e, stop)
        pg_argv = calls[pg_start]
        # The official entrypoint calls gosu when started as root; dropping
        # all Linux capabilities before that prevents the UID transition.
        # Explicitly start as the image's postgres user, retaining zero caps.
        self.assertEqual(pg_argv[pg_argv.index('--user') + 1], 'postgres')
        self.assertEqual(pg_argv[pg_argv.index('--cap-drop') + 1], 'ALL')
        self.assertNotIn('--privileged', pg_argv)
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
                    return subprocess.CompletedProcess(argv, 0, stdout='go version go1.27.2 linux/amd64\n')
                if '-list' in argv:
                    return subprocess.CompletedProcess(argv, 0, stdout=GO_DISCOVERY)
                if '-json' in argv:
                    return subprocess.CompletedProcess(argv, 0, stdout=GO_SUCCESS_EVENTS)
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
                      mock.patch.object(runner, 'install_a9_fixture'),
                      mock.patch.object(runner, 'run', side_effect=fake_run),
                      mock.patch.object(runner.subprocess, 'run', side_effect=fake_subprocess),
                      contextlib.redirect_stdout(capture),
                      contextlib.redirect_stderr(io.StringIO())):
                    self.assertEqual(runner.main(ARGS), 3)
                self.assertNotIn(runner.LEGACY_SMOKE_SUCCESS, capture.getvalue())
                self.assertEqual(sum(c[:2] == ['docker', 'stop'] for c in calls), 1)

    def test_wrong_container_go_never_starts_postgres(self):
        for unsupported in ('1.26.8', '1.27.0', '1.27.1'):
            with self.subTest(version=unsupported):
                status, calls = self.simulate(go_version=f'go version go{unsupported} linux/amd64\n')
                self.assertEqual(status, 3)
                self.assertFalse(any(c[:2] == ['docker', 'run'] and runner.PG_IMAGE in c for c in calls))

    def test_zero_discovered_tests_refuses_before_database(self):
        code, calls = self.simulate(missing_discovery=True)
        self.assertEqual(code, 3)
        self.assertFalse(any(c[:2] == ['docker', 'run'] and runner.PG_IMAGE in c for c in calls))

    def test_package_pass_without_test_passes_is_not_e2e_success(self):
        code, calls = self.simulate(missing_passes=True)
        self.assertEqual(code, 3)
        self.assertEqual(sum(c[:2] == ['docker', 'stop'] for c in calls), 1)

    def test_discovery_requires_exact_six_names_and_no_duplicates(self):
        runner.require_six_discovered_tests(GO_DISCOVERY)
        bad = ['', 'ok\n', 'TestE2E1\n', GO_DISCOVERY + 'TestE2E7\n',
               GO_DISCOVERY + 'TestE2E1\n', GO_DISCOVERY.replace('TestE2E6', 'TestNotE2E6')]
        for case in bad:
            with self.subTest(discovery=case), self.assertRaises(ValueError):
                runner.require_six_discovered_tests(case)

    def test_json_events_require_six_actual_runs_and_passes(self):
        runner.require_six_passed_tests(GO_SUCCESS_EVENTS)
        events = [json.loads(l) for l in GO_SUCCESS_EVENTS.splitlines()]
        tests = [
            [{'Action': 'pass', 'Package': 'example/joint'}],
            [e for e in events if e.get('Test') != 'TestE2E6'],
            [e for e in events if not (e.get('Action') == 'run' and e.get('Test') == 'TestE2E2')],
            events + [events[0]],
            events[:-1],
            events + [{'Action': 'skip', 'Package': 'example/joint', 'Test': 'TestE2E1'}],
            events + [{'Action': 'fail', 'Package': 'example/joint'}],
            events + [{'Action': 'run', 'Package': 'example/joint', 'Test': 'TestE2E7'}],
        ]
        for case in tests:
            text = '\n'.join(json.dumps(e) for e in case) + '\n'
            with self.subTest(events=len(case)), self.assertRaises(ValueError):
                runner.require_six_passed_tests(text)
        with self.assertRaises(ValueError):
            runner.require_six_passed_tests('not-json')

    def test_real_go_json_events_differentiate_six_runs_from_zero(self):
        with tempfile.TemporaryDirectory(prefix='lic-a9-six-test-evidence-') as path:
            root = Path(path)
            (root / 'go.mod').write_text('module example.org/evidence\n\ngo 1.23\n')
            code = ['package evidence', 'import "testing"']
            code.extend(f'func TestE2E{i}(t *testing.T) {{}}' for i in range(1, 7))
            (root / 'evidence_test.go').write_text('\n'.join(code) + '\n')
            env = dict(os.environ, GOTOOLCHAIN='local', GOPROXY='off', GOSUMDB='off')
            discovery = subprocess.run(
                ['go', 'test', '-list', '^TestE2E[1-6]$', '.'], cwd=root,
                capture_output=True, text=True, env=env, check=True, timeout=60)
            runner.require_six_discovered_tests(discovery.stdout)
            positive = subprocess.run(
                ['go', 'test', '-count=1', '-json', '-run', '^TestE2E[1-6]$', '.'],
                cwd=root, capture_output=True, text=True, env=env, check=True, timeout=60)
            runner.require_six_passed_tests(positive.stdout)
            # Real Go returns status zero even when selection matches no tests.
            no_tests = subprocess.run(
                ['go', 'test', '-count=1', '-json', '-run', '^TestNeverExists$', '.'],
                cwd=root, capture_output=True, text=True, env=env, check=True, timeout=60)
            self.assertEqual(no_tests.returncode, 0)
            with self.assertRaises(ValueError):
                runner.require_six_passed_tests(no_tests.stdout)

    def test_success_output_is_legacy_smoke_not_A9_certification(self):
        result = io.StringIO()
        with contextlib.redirect_stdout(result):
            runner.emit_legacy_smoke_result()
        lines = result.getvalue().splitlines()
        self.assertEqual(lines, [runner.LEGACY_SMOKE_SUCCESS, runner.A9_COVERAGE_GAP])
        self.assertTrue(lines[0].startswith('LIC_A8_LEGACY_'))
        self.assertIn('A9_FULL_SECURITY_MATRIX_NOT_CERTIFIED', lines[1])
        self.assertNotIn('LIC_A9_JOINT_DOCKER_GO127_POSTGRES16_E2E_PASS', result.getvalue())

    def test_old_host_runner_is_not_an_A9_certification(self):
        import inspect
        import run_joint_postgres_e2e as host_runner
        source = inspect.getsource(host_runner.main)
        self.assertNotIn('LIC_A9_JOINT_GO127_POSTGRES16_E2E_PASS', source)
        self.assertIn('LIC_A8_LEGACY_HOST_GO127_PG16_SMOKE_COMMAND_COMPLETED_TEST_COUNT_UNVERIFIED', source)
        self.assertIn('LIC_A9_ANTI_CLONE_CONCURRENCY_E2E_NOT_TESTED', source)

    def test_targeted_a9_success_requires_eight_real_events(self):
        runner.require_six_discovered_tests(GO_DISCOVERY)
        runner.require_a9_discovered_tests(GO_DISCOVERY)
        runner.require_combined_passed_tests(COMBINED_EVENTS)
        stream = [json.loads(line) for line in COMBINED_EVENTS.splitlines()]
        for target in sorted(runner.A9_TESTS):
            for event in ('run', 'pass'):
                missing = [x for x in stream if not (x.get('Test') == target and x['Action'] == event)]
                with self.subTest(target=target, event=event), self.assertRaises(ValueError):
                    runner.require_combined_passed_tests('\n'.join(json.dumps(x) for x in missing))
        with self.assertRaises(ValueError):
            runner.require_a9_discovered_tests(GO_DISCOVERY.replace('TestLICA9CrossRepoRealLeaseAndRestart\n', ''))
        with self.assertRaises(ValueError):
            runner.require_combined_passed_tests(GO_SUCCESS_EVENTS)

    def test_targeted_a9_result_does_not_claim_full_certification(self):
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            runner.emit_targeted_a9_result()
        self.assertEqual(out.getvalue().splitlines(), [
            runner.LEGACY_SMOKE_SUCCESS, runner.A9_TARGETED_SUCCESS, runner.A9_COVERAGE_GAP])
        self.assertIn('TARGETED', out.getvalue())
        self.assertIn('NOT_CERTIFIED', out.getvalue())

    def test_source_pinned_a9_fixture_and_rejects_changed_bytes(self):
        with tempfile.TemporaryDirectory(prefix='lic-a9-fixture-sha-') as path:
            root = Path(path)
            product = root / 'product'
            template = product / 'tests/installationlicense/authority_lic_a9_crossrepo_test.go.template'
            template.parent.mkdir(parents=True)
            template.write_bytes((Path(__file__).parent / 'authority_lic_a9_crossrepo_test.go.template').read_bytes())
            fixture = root / 'fixture'
            (fixture / 'authority').mkdir(parents=True)
            runner.install_a9_fixture(product, fixture)
            installed = fixture / 'authority/client_lic_a9_crossrepo_test.go'
            self.assertEqual(installed.read_bytes(), template.read_bytes())
            template.write_bytes(template.read_bytes() + b'\n// unauthorized revision\n')
            with self.assertRaises(ValueError):
                runner.install_a9_fixture(product, fixture)

    def test_anticlone_template_proves_exact_restriction_and_durable_conflict(self):
        source = (Path(__file__).parent / 'authority_lic_a9_crossrepo_test.go.template').read_text()
        expected = (
            'bootstrap.Reason != "CONCURRENCY_REFRESH_REQUIRED"',
            'decision.Reason != "CONCURRENT_INSTALLATION_CONFLICT"',
            'generation != 2 || conflictState != "CLONE_SUSPECTED"',
            "action='clone_conflict_detected'",
            'conflictIntents != 1',
            'replica.Effect(context.Background()',
            'if !errors.Is(err, l.ErrRestricted) || executed',
        )
        for guard in expected:
            with self.subTest(guard=guard):
                self.assertIn(guard, source)
        self.assertIn('TestLICA9CrossRepoClonedGenerationFenced', source)

    def test_real_go_can_list_eight_named_tests(self):
        with tempfile.TemporaryDirectory(prefix='lic-a9-go-discovery-') as path:
            root = Path(path)
            (root / 'go.mod').write_text('module example.org/eight\ngo 1.23\n')
            code = ['package evidence', 'import "testing"']
            code.extend(f'func {name}(t *testing.T) {{}}' for name in sorted(runner.COMBINED_TESTS))
            (root / 'evidence_test.go').write_text('\n'.join(code) + '\n')
            env = dict(os.environ, GOTOOLCHAIN='local', GOPROXY='off', GOSUMDB='off')
            found = subprocess.run(['go', 'test', '-list', runner.COMBINED_FILTER, '.'], cwd=root,
                                   check=True, capture_output=True, text=True, env=env, timeout=90)
            runner.require_six_discovered_tests(found.stdout)
            runner.require_a9_discovered_tests(found.stdout)
            result = subprocess.run(['go', 'test', '-count=1', '-json', '-run', runner.COMBINED_FILTER, '.'],
                                    cwd=root, check=True, capture_output=True, text=True, env=env, timeout=90)
            runner.require_combined_passed_tests(result.stdout)

    def test_early_argv_check_rejects_same_source(self):
        with (mock.patch.object(runner, 'run') as commands,
              contextlib.redirect_stderr(io.StringIO())):
            code = runner.main(['--authority', '/tmp/same', '--product', '/tmp/same',
                                '--acknowledge-disposable-host'])
        self.assertEqual(code, 3)
        commands.assert_not_called()


if __name__ == '__main__':
    unittest.main()
