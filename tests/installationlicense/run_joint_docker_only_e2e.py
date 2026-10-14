#!/usr/bin/env python3
"""LIC-A9 nonproduction joint E2E: only local Docker is needed for Go + PG16.

No GitHub Actions, on-host Go installation, live database, or production keys.
All source checkouts must pass the existing pinned Git provenance fences.
"""
import argparse
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

GO_IMAGE = 'golang:1.27.0-bookworm'
PG_IMAGE = 'postgres:16'
PG_DATABASE = 'lic_a9_joint_test'


def go_docker_command(fixture: Path, product: Path, cache: Path,
                      network: str, command: list[str]) -> list[str]:
    """Never mount host credentials; source product read-only and caches disposable."""
    if network != 'bridge' and not re.fullmatch(r'container:lic-a9-joint-[a-f0-9]{12}', network):
        raise ValueError('only disposable network namespaces are supported')
    if any(not p.is_absolute() for p in (fixture, product, cache)):
        raise ValueError('Docker source mounts must be absolute')
    return [
        'docker', 'run', '--rm', '--network', network,
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
        '--env', 'GOTELEMETRY=off', GO_IMAGE,
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
        endpoint = run(['docker', 'context', 'inspect', '--format',
                        '{{ .Endpoints.docker.Host }}'], output=True).stdout.strip()
        if not re.fullmatch(r'unix:///[\w./-]+\.sock', endpoint):
            raise ValueError('only local Unix Docker socket is permitted')
        run(['docker', 'info'], output=True)
        with tempfile.TemporaryDirectory(prefix='lic-a9-docker-only-') as temp:
            fixture = Path(temp) / 'fixture'
            cache = Path(temp) / 'go_cache'
            cache.mkdir()
            prepare = product / 'tests/installationlicense/prepare_authority_fixture.py'
            run([sys.executable, '-B', str(prepare), str(authority), str(fixture)], timeout=90)
            version = run(go_docker_command(fixture, product, cache, 'bridge', ['go', 'version']),
                          output=True, timeout=180).stdout
            if not re.search(r'\bgo version go1\.27\.0\b', version):
                raise ValueError('container Go version not exactly 1.27.0')
            # Compile all imported real sources and populate only an isolated Go cache,
            # BEFORE any PostgreSQL service exists. Failures are not called E2E PASS.
            run(go_docker_command(fixture, product, cache, 'bridge',
                                  ['go', 'test', '-count=1', '-run', '^$', './authority']),
                timeout=420)
            pg_container = 'lic-a9-joint-' + uuid.uuid4().hex[:12]
            # Postgres is network-isolated: NO -p / exposed host port, even loopback.
            run(['docker', 'run', '-d', '--rm', '--name', pg_container,
                 '--network', 'none', '--security-opt', 'no-new-privileges',
                 '--pids-limit', '128', '--memory', '512m', '--cpus', '1',
                 '--label', 'telmivo.purpose=lic-a9-disposable-joint-e2e',
                 '-e', 'POSTGRES_HOST_AUTH_METHOD=trust',
                 '-e', f'POSTGRES_DB={PG_DATABASE}', PG_IMAGE,
                 '-c', 'shared_buffers=32MB', '-c', 'max_connections=35'], timeout=180)
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
                ['go', 'test', '-count=1', '-v', '-timeout', '8m',
                 '-run', '^TestE2E[1-6]$', './authority'])
            image_index = cmd.index(GO_IMAGE)
            cmd[image_index:image_index] = [
                '--env', f'LIC_A8_TEST_DATABASE_URL=postgres://postgres@127.0.0.1:5432/{PG_DATABASE}?sslmode=disable',
                '--env', 'GOPROXY=off', '--env', 'GOSUMDB=off',
            ]
            run(cmd, timeout=600)
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
        print('LIC_A9_JOINT_DOCKER_GO127_POSTGRES16_E2E_PASS')
    return outcome


if __name__ == '__main__':
    raise SystemExit(main())
