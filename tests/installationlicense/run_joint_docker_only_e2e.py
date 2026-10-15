#!/usr/bin/env python3
"""LEGACY LIC-A8 cross-repo smoke: only local Docker for Go + PG16.

No GitHub Actions, on-host Go installation, live database, or production keys.
All source checkouts must pass pinned Git provenance.
Passing six legacy LIC-A8 tests does NOT certify LIC-A9 anti-cloning/concurrency.
"""
import argparse
import hashlib
from collections import Counter
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import time
import uuid

from run_joint_postgres_e2e import (
    AUTHORITY_MERGED_SHA, check_checkout, check_product_checkout,
    validate_authority_archive_provenance, run,
)
from phase10_image_identity import (
    GO_TAG, POSTGRES_TAG, reviewed_image_id, reviewed_product_head,
    inspect_pinned_image,
)

GO_IMAGE = 'golang:1.27.2-bookworm'
PG_IMAGE = 'postgres:16'
PG_DATABASE = 'lic_a9_joint_test'
PGDATA_TEST_ONLY = '/tmp/lic-a9-postgres-data'
PG_TMPFS = '/tmp:rw,nosuid,nodev,size=256m,mode=1777'


def disposable_pg_command(container: str, *, image: str = PG_IMAGE) -> list[str]:
    """Unprivileged, fully ephemeral PG16 on an isolated network namespace.

    The official PG16 image declares a default data VOLUME. Running as the
    postgres user may not be able to chown that freshly created mount. Force
    an empty private PGDATA subdirectory on writable *container-only* tmpfs.
    This function never starts Docker; it only constructs reviewable argv.
    """
    if not re.fullmatch(r'lic-a9-joint-[0-9a-f]{12}', container):
        raise ValueError('unreviewed disposable PostgreSQL container name')
    if image != PG_IMAGE:
        reviewed_image_id(image)
    return [
        'docker', 'run', '--pull=never', '-d', '--rm', '--name', container,
        '--user', 'postgres', '--cap-drop', 'ALL',
        '--network', 'none', '--security-opt', 'no-new-privileges',
        '--pids-limit', '128', '--memory', '512m', '--cpus', '1',
        '--tmpfs', PG_TMPFS,
        '--label', 'telmivo.purpose=lic-a9-disposable-joint-e2e',
        '-e', 'POSTGRES_HOST_AUTH_METHOD=trust',
        '-e', f'POSTGRES_DB={PG_DATABASE}',
        '-e', f'PGDATA={PGDATA_TEST_ONLY}',
        image,
        '-c', 'shared_buffers=32MB', '-c', 'max_connections=35',
    ]


E2E_TESTS = frozenset(f'TestE2E{i}' for i in range(1, 7))
A9_TESTS = frozenset({'TestLICA9CrossRepoRealLeaseAndRestart', 'TestLICA9CrossRepoClonedGenerationFenced'})
COMBINED_TESTS = E2E_TESTS | A9_TESTS
COMBINED_FILTER = '^(' + '|'.join(sorted(COMBINED_TESTS)) + ')$'
A9_TEMPLATE_BLOB = 'e9846d589990210074c8980321c8cb168cc0fb6f'

LEGACY_SMOKE_SUCCESS = 'LIC_A8_LEGACY_CROSSREPO_GO1272_PG16_SMOKE_PASS'
A9_COVERAGE_GAP = 'LIC_A9_FULL_SECURITY_MATRIX_NOT_CERTIFIED'
A9_TARGETED_SUCCESS = 'LIC_A9_CROSSREPO_TARGETED_ANTICLONE_2_TESTS_PASS'


def emit_legacy_smoke_result() -> None:
    """Report only the proven six-test LIC-A8 interoperability scope."""
    print(LEGACY_SMOKE_SUCCESS)
    print(A9_COVERAGE_GAP)


def emit_targeted_a9_result() -> None:
    """Clearly scoped, not a claim that the entire LIC-A9 matrix passed."""
    print(LEGACY_SMOKE_SUCCESS)
    print(A9_TARGETED_SUCCESS)
    print(A9_COVERAGE_GAP)


def install_a9_fixture(product: Path, fixture: Path) -> None:
    """Import reviewed test-only Go bytes, never modify pinned preparer."""
    source = product / 'tests/installationlicense/authority_lic_a9_crossrepo_test.go.template'
    payload = source.read_bytes()
    sha = hashlib.sha1(b'blob ' + str(len(payload)).encode() + b'\x00' + payload).hexdigest()
    if sha != A9_TEMPLATE_BLOB:
        raise ValueError('unapproved LIC-A9 integration Go test template')
    destination = fixture / 'authority' / 'client_lic_a9_crossrepo_test.go'
    if not destination.parent.is_dir() or destination.exists():
        raise ValueError('unexpected LIC-A9 fixture destination')
    destination.write_bytes(payload)



def require_six_discovered_tests(stdout: str) -> None:
    """A zero-test `go test` returns 0. Enforce complete discovery before DB startup."""
    found = [line.strip() for line in stdout.splitlines()
             if re.fullmatch(r'TestE2E[0-9]+', line.strip())]
    if len(found) != 6 or set(found) != E2E_TESTS:
        raise ValueError(f'expected exactly six E2E test names, found {found!r}')


def require_six_passed_tests(stdout: str) -> None:
    """Require actual Go -json run+pass events for all six, not package PASS alone."""
    started = Counter()
    passed = Counter()
    package_pass = 0
    for line in stdout.splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError as exc:
            raise ValueError('Go JSON test event stream malformed') from exc
        if not isinstance(event, dict):
            raise ValueError('Go JSON event was not an object')
        action = event.get('Action')
        name = event.get('Test')
        if action in ('fail', 'skip'):
            raise ValueError(f'Go reported a skipped or failed test: {name or "package"}')
        if name is None:
            if action == 'pass':
                package_pass += 1
        elif name in E2E_TESTS:
            if action == 'run':
                started[name] += 1
            elif action == 'pass':
                passed[name] += 1
        elif re.fullmatch(r'TestE2E[0-9]+', name):
            raise ValueError(f'unexpected joint E2E test: {name}')
    if package_pass != 1 or set(started) != E2E_TESTS or set(passed) != E2E_TESTS:
        raise ValueError('six E2E test executions and successes were not proven')
    if any(started[test] != 1 or passed[test] != 1 for test in E2E_TESTS):
        raise ValueError('duplicate or missing E2E execution evidence')


def require_a9_discovered_tests(stdout: str) -> None:
    found = [line.strip() for line in stdout.splitlines()
             if line.strip().startswith('TestLICA9CrossRepo')]
    if len(found) != len(A9_TESTS) or set(found) != A9_TESTS:
        raise ValueError(f'missing or duplicate LIC-A9 crossrepo test: {found!r}')


def require_combined_passed_tests(stdout: str) -> None:
    started, passed = Counter(), Counter()
    package_pass = 0
    for line in stdout.splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError as exc:
            raise ValueError('malformed Go JSON E2E event') from exc
        if not isinstance(event, dict):
            raise ValueError('malformed Go event object')
        action, name = event.get('Action'), event.get('Test')
        # Go's JSON schema uses strings for Action and optional Test.
        # Non-string / unhashable input must fail closed as an ordinary
        # validation error, never crash with TypeError/AttributeError.
        if not isinstance(action, str) or (name is not None and not isinstance(name, str)):
            raise ValueError('malformed Go E2E action/test identity')
        if action in ('fail', 'skip'):
            raise ValueError('Go test skipped or failed')
        if name is None and action == 'pass':
            package_pass += 1
        if name in COMBINED_TESTS:
            if action == 'run':
                started[name] += 1
            elif action == 'pass':
                passed[name] += 1
        elif name and (name.startswith('TestE2E') or name.startswith('TestLICA9CrossRepo')):
            raise ValueError(f'unexpected selected E2E test: {name}')
    if package_pass != 1 or set(started) != COMBINED_TESTS or set(passed) != COMBINED_TESTS:
        raise ValueError('all eight individual test run/pass events are required')
    if any(started[name] != 1 or passed[name] != 1 for name in COMBINED_TESTS):
        raise ValueError('duplicate or missing Go E2E evidence')


def safe_failed_e2e_summary(exc: subprocess.CalledProcessError) -> str:
    """Preserve failure evidence without echoing Go output or fixture secrets.

    Real Go -json output may contain arbitrary test logs and credentials. Never
    dump it to the console. Report only *approved* test identities, event types
    and digests so the first failed case is identifiable without a false PASS.
    """
    stdout = exc.stdout if isinstance(exc.stdout, str) else ''
    stderr = exc.stderr if isinstance(exc.stderr, str) else ''
    fail, skip = set(), set()
    package_failed = False
    for line in stdout.splitlines():
        try:
            event = json.loads(line)
        except (ValueError, TypeError):
            continue
        if not isinstance(event, dict):
            continue
        action = event.get('Action')
        if action not in ('fail', 'skip'):
            continue
        name = event.get('Test')
        if name is None:
            if action == 'fail':
                package_failed = True
        elif isinstance(name, str) and name in COMBINED_TESTS:
            (fail if action == 'fail' else skip).add(name)
    digest = lambda data: hashlib.sha256(data.encode('utf-8', errors='replace')).hexdigest()
    return (f'exit={exc.returncode}; failed={",".join(sorted(fail)) or "none"}; '
            f'skipped={",".join(sorted(skip)) or "none"}; '
            f'package_fail={package_failed}; '
            f'go_json_sha256={digest(stdout)}; stderr_sha256={digest(stderr)}')


def go_docker_command(fixture: Path, product: Path, cache: Path,
                      network: str, command: list[str], *,
                      image: str = GO_IMAGE) -> list[str]:
    """Never mount host credentials; source product read-only and caches disposable."""
    if network != 'bridge' and not re.fullmatch(r'container:lic-a9-joint-[a-f0-9]{12}', network):
        raise ValueError('only disposable network namespaces are supported')
    if any(not p.is_absolute() for p in (fixture, product, cache)):
        raise ValueError('Docker source mounts must be absolute')
    if image != GO_IMAGE:
        reviewed_image_id(image)
    return [
        'docker', 'run', '--pull=never', '--rm', '--network', network,
        '--security-opt', 'no-new-privileges', '--cap-drop', 'ALL',
        '--pids-limit', '256', '--memory', '2g', '--cpus', '2',
        '--user', f'{os.getuid()}:{os.getgid()}',
        '--mount', f'type=bind,src={fixture},dst={fixture}',
        '--mount', f'type=bind,src={product},dst={product},readonly',
        '--mount', f'type=bind,src={cache},dst={cache}',
        '--workdir', str(fixture),
        '--env', 'HOME=/tmp', '--env', f'GOCACHE={cache}/build',
        '--env', f'GOMODCACHE={cache}/mod',
        '--env', 'GOTOOLCHAIN=local', '--env', 'GOWORK=off',
        '--env', 'GOTELEMETRY=off', image,
        *command,
    ]


def safe_host() -> None:
    forbidden = ('TELMIVO_ENV', 'TELMIVO_LICENSE_TRUST_FILE', 'TELMIVO_LICENSE_STATE_DIR',
                 'AUTHORITY_DATABASE_URL', 'AUTHORITY_TEST_DATABASE_URL',
                 'DATABASE_URL', 'PGHOST', 'DOCKER_HOST', 'DOCKER_CONTEXT',
                 'DOCKER_TLS_VERIFY', 'LIC_A8_TEST_DATABASE_URL')
    present = [k for k in forbidden if os.environ.get(k)]
    if present:
        raise ValueError('inherited environment may address production: ' + ', '.join(present))


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--authority', type=Path, required=True)
    parser.add_argument('--product', type=Path, required=True)
    parser.add_argument('--acknowledge-disposable-host', action='store_true', required=True)
    parser.add_argument('--expected-product-head', help='independently reviewed Product commit SHA')
    parser.add_argument('--expected-go-image-id', help='out-of-band approved local Go Docker image ID')
    parser.add_argument('--expected-pg-image-id', help='out-of-band approved local PG Docker image ID')
    args = parser.parse_args(argv)
    authority, product = args.authority.resolve(), args.product.resolve()
    pg_container = None
    outcome = 3
    try:
        if authority == product:
            raise ValueError('Authority and Product need separate checkouts')
        safe_host()
        check_checkout(authority, AUTHORITY_MERGED_SHA)
        validate_authority_archive_provenance(authority)
        check_product_checkout(product)
        # Exact HEAD protects reviewed test-script bytes as well as the
        # existing changed-path fence. The pin must come from an independent
        # reviewer, not from the checkout being tested.
        expected_head = reviewed_product_head(args.expected_product_head)
        check_checkout(product, expected_head)
        approved_go = reviewed_image_id(args.expected_go_image_id)
        approved_pg = reviewed_image_id(args.expected_pg_image_id)
        endpoint = run(['docker', 'context', 'inspect', '--format',
                        '{{ .Endpoints.docker.Host }}'], output=True).stdout.strip()
        if not re.fullmatch(r'unix:///[\w./-]+\.sock', endpoint):
            raise ValueError('only local Unix Docker socket is permitted')
        run(['docker', 'info'], output=True)
        # Inspect both locally preloaded tags before any container begins.
        # Use their externally approved immutable IDs, never the mutable tags.
        go_image = inspect_pinned_image(GO_TAG, approved_go, run)
        pg_image = inspect_pinned_image(POSTGRES_TAG, approved_pg, run)
        with tempfile.TemporaryDirectory(prefix='lic-a9-docker-only-') as temp:
            fixture = Path(temp) / 'fixture'
            cache = Path(temp) / 'go_cache'
            cache.mkdir()
            prepare = product / 'tests/installationlicense/prepare_authority_fixture.py'
            run([sys.executable, '-B', str(prepare), str(authority), str(fixture)], timeout=90)
            install_a9_fixture(product, fixture)
            version = run(go_docker_command(fixture, product, cache, 'bridge', ['go', 'version'],
                                               image=go_image),
                          output=True, timeout=180).stdout
            if not re.search(r'\bgo version go1\.27\.2\b', version):
                raise ValueError('container Go version not exactly 1.27.2')
            # Compile all imported real sources and populate only an isolated Go cache,
            # BEFORE any PostgreSQL service exists. Failures are not called E2E PASS.
            run(go_docker_command(fixture, product, cache, 'bridge',
                                  ['go', 'test', '-count=1', '-run', '^$', './authority'],
                                  image=go_image),
                timeout=420)
            discovered = run(go_docker_command(
                fixture, product, cache, 'bridge',
                ['go', 'test', '-list', COMBINED_FILTER, './authority'],
                 image=go_image),
                output=True, timeout=240).stdout
            require_six_discovered_tests(discovered)
            require_a9_discovered_tests(discovered)
            pg_container = 'lic-a9-joint-' + uuid.uuid4().hex[:12]
            # Postgres is network-isolated: NO -p / exposed host port, even loopback.
            # Official postgres entrypoint drops root via gosu, which needs
            # CAP_SETUID/CAP_SETGID. Run directly as its built-in unprivileged
            # postgres user so the container can safely drop ALL capabilities.
            run(disposable_pg_command(pg_container, image=pg_image), timeout=180)
            deadline = time.monotonic() + 60
            while True:
                ready = subprocess.run(['docker', 'exec', pg_container, 'pg_isready',
                                        '-h', '127.0.0.1', '-U', 'postgres', '-d', PG_DATABASE],
                                       capture_output=True, text=True, timeout=12)
                if ready.returncode == 0:
                    break
                if time.monotonic() >= deadline:
                    raise TimeoutError('PostgreSQL 16 isolated container did not become ready')
                time.sleep(1)
            # Join DB network namespace without publishing any port or providing
            # outbound internet access. All dependencies were compiled beforehand.
            cmd = go_docker_command(
                fixture, product, cache, 'container:' + pg_container,
                ['go', 'test', '-count=1', '-json', '-timeout', '8m',
                 '-run', COMBINED_FILTER, './authority'], image=go_image)
            image_index = cmd.index(go_image)
            cmd[image_index:image_index] = [
                '--env', f'LIC_A8_TEST_DATABASE_URL=postgres://postgres@127.0.0.1:5432/{PG_DATABASE}?sslmode=disable',
                '--env', 'GOPROXY=off', '--env', 'GOSUMDB=off',
            ]
            try:
                executed = run(cmd, output=True, timeout=600).stdout
            except subprocess.CalledProcessError as exc:
                print('LIC_A9_TEST_FAILURE_SUMMARY: ' + safe_failed_e2e_summary(exc),
                      file=sys.stderr)
                raise
            require_combined_passed_tests(executed)
        outcome = 0
    except (OSError, ValueError, TimeoutError, subprocess.TimeoutExpired,
            subprocess.CalledProcessError) as exc:
        print(f'NOT_PASS: {exc}', file=sys.stderr)
    finally:
        if pg_container is not None:
            # Fail closed: a completed E2E test is NOT a success if its
            # temporary database could not be stopped and removed.
            try:
                cleaned = subprocess.run(['docker', 'stop', pg_container],
                                         capture_output=True, text=True, timeout=60)
                if cleaned.returncode != 0:
                    raise RuntimeError(cleaned.stderr or 'docker stop returned nonzero')
            except (OSError, subprocess.TimeoutExpired, RuntimeError) as exc:
                print(f'NOT_PASS: disposable PostgreSQL cleanup was not confirmed: {exc}',
                      file=sys.stderr)
                outcome = 3
    if outcome == 0:
        # Emit PASS *only after* the disposable database cleanup succeeds.
        emit_targeted_a9_result()
    return outcome


if __name__ == '__main__':
    raise SystemExit(main())
, './authority'],
                                   image=go_image),
                timeout=420)
            discovered = run(go_docker_command(
                fixture, product, cache, 'bridge',
                ['go', 'test', '-list', COMBINED_FILTER, './authority']),
                output=True, timeout=240).stdout
            require_six_discovered_tests(discovered)
            require_a9_discovered_tests(discovered)
            pg_container = 'lic-a9-joint-' + uuid.uuid4().hex[:12]
            # Postgres is network-isolated: NO -p / exposed host port, even loopback.
            # Official postgres entrypoint drops root via gosu, which needs
            # CAP_SETUID/CAP_SETGID. Run directly as its built-in unprivileged
            # postgres user so the container can safely drop ALL capabilities.
            run(disposable_pg_command(pg_container), timeout=180)
            deadline = time.monotonic() + 60
            while True:
                ready = subprocess.run(['docker', 'exec', pg_container, 'pg_isready',
                                        '-h', '127.0.0.1', '-U', 'postgres', '-d', PG_DATABASE],
                                       capture_output=True, text=True, timeout=12)
                if ready.returncode == 0:
                    break
                if time.monotonic() >= deadline:
                    raise TimeoutError('PostgreSQL 16 isolated container did not become ready')
                time.sleep(1)
            # Join DB network namespace without publishing any port or providing
            # outbound internet access. All dependencies were compiled beforehand.
            cmd = go_docker_command(
                fixture, product, cache, 'container:' + pg_container,
                ['go', 'test', '-count=1', '-json', '-timeout', '8m',
                 '-run', COMBINED_FILTER, './authority'])
            image_index = cmd.index(GO_IMAGE)
            cmd[image_index:image_index] = [
                '--env', f'LIC_A8_TEST_DATABASE_URL=postgres://postgres@127.0.0.1:5432/{PG_DATABASE}?sslmode=disable',
                '--env', 'GOPROXY=off', '--env', 'GOSUMDB=off',
            ]
            try:
                executed = run(cmd, output=True, timeout=600).stdout
            except subprocess.CalledProcessError as exc:
                print('LIC_A9_TEST_FAILURE_SUMMARY: ' + safe_failed_e2e_summary(exc),
                      file=sys.stderr)
                raise
            require_combined_passed_tests(executed)
        outcome = 0
    except (OSError, ValueError, TimeoutError, subprocess.TimeoutExpired,
            subprocess.CalledProcessError) as exc:
        print(f'NOT_PASS: {exc}', file=sys.stderr)
    finally:
        if pg_container is not None:
            # Fail closed: a completed E2E test is NOT a success if its
            # temporary database could not be stopped and removed.
            try:
                cleaned = subprocess.run(['docker', 'stop', pg_container],
                                         capture_output=True, text=True, timeout=60)
                if cleaned.returncode != 0:
                    raise RuntimeError(cleaned.stderr or 'docker stop returned nonzero')
            except (OSError, subprocess.TimeoutExpired, RuntimeError) as exc:
                print(f'NOT_PASS: disposable PostgreSQL cleanup was not confirmed: {exc}',
                      file=sys.stderr)
                outcome = 3
    if outcome == 0:
        # Emit PASS *only after* the disposable database cleanup succeeds.
        emit_targeted_a9_result()
    return outcome


if __name__ == '__main__':
    raise SystemExit(main())
