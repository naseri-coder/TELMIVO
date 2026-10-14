#!/usr/bin/env python3
"""Fail-closed offline LIC-A9 joint test with a disposable loopback PostgreSQL16.

Run ONLY on an authorized disposable development machine with Docker and Go 1.27.
No GitHub Actions, production environment, persistent databases or VPS changes.
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

AUTHORITY_MERGED_SHA = '939d14800c602dd9c31ff23fdf6e0688a440d4c2'
PRODUCT_MERGED_SHA = 'bc417045ca5e1b91a2377aba6dece348699f7bf4'


def git(root: Path, *args: str) -> str:
    return subprocess.run(['git', '-C', str(root), *args], text=True,
                          capture_output=True, check=True).stdout.strip()


def check_checkout(root: Path, expected: str) -> None:
    if not root.is_dir() or git(root, 'rev-parse', '--verify', 'HEAD^{commit}') != expected:
        raise ValueError(f'checkout not pinned to required SHA: {root}')
    if git(root, 'status', '--porcelain', '--untracked-files=all'):
        raise ValueError(f'checkout contains uncommitted or untracked files: {root}')


def check_product_checkout(root: Path, baseline: str = PRODUCT_MERGED_SHA) -> None:
    if not root.is_dir():
        raise ValueError('TELMIVO checkout is missing')
    git(root, 'rev-parse', '--verify', 'HEAD^{commit}')
    git(root, 'merge-base', '--is-ancestor', baseline, 'HEAD')
    if git(root, 'status', '--porcelain', '--untracked-files=all'):
        raise ValueError('TELMIVO checkout has uncommitted or untracked files')
    # Allow newly added *test tooling* on top of pinned product source, but
    # never allow a different Go/runtime module or old E2E fixture.
    protected = ('go.mod', 'go.sum', 'internal',
                 'tests/installationlicense/prepare_authority_fixture.py',
                 'tests/installationlicense/authority_e2e_test.go.template')
    git(root, 'diff', '--quiet', baseline, 'HEAD', '--', *protected)


def check_go_version(output: str) -> None:
    m = re.search(r'\bgo version go(\d+)\.(\d+)(?:\.\d+)?\b', output)
    if m is None or (int(m[1]), int(m[2])) < (1, 27):
        raise ValueError('Go 1.27+ is required for the merged product fixture')


def loopback_port(output: str) -> int:
    ports = re.findall(r'^127\.0\.0\.1:(\d+)\s*$', output, flags=re.M)
    if len(ports) != 1 or not (1024 <= int(ports[0]) <= 65535):
        raise ValueError('Docker did not provide exactly one loopback-only port mapping')
    return int(ports[0])


def run(args: list[str], *, cwd: Path | None = None, env: dict | None = None,
        output: bool = False, timeout: int = 120) -> subprocess.CompletedProcess:
    return subprocess.run(args, cwd=cwd, env=env, check=True,
                          text=True, capture_output=output, timeout=timeout)


def go_test_env() -> dict[str, str]:
    allowed = ('PATH', 'HOME', 'TMPDIR', 'GOCACHE', 'GOMODCACHE', 'GOPATH',
               'GOPROXY', 'GONOSUMDB', 'GOPRIVATE', 'SSL_CERT_FILE', 'SSL_CERT_DIR')
    env = {k: os.environ[k] for k in allowed if k in os.environ}
    env.update(GOTOOLCHAIN='local', GOWORK='off', GOTELEMETRY='off')
    return env


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--authority', type=Path, required=True)
    p.add_argument('--product', type=Path, required=True)
    p.add_argument('--acknowledge-disposable-host', action='store_true', required=True,
                   help='explicitly affirm this is NOT the TELMIVO production/main server')
    args = p.parse_args(argv)
    authority = args.authority.resolve()
    product = args.product.resolve()
    if authority == product:
        print('BLOCKED: independent authority and product checkouts are required', file=sys.stderr)
        return 3
    container = None
    try:
        for bad in ('TELMIVO_ENV', 'TELMIVO_LICENSE_TRUST_FILE',
                    'AUTHORITY_DATABASE_URL', 'DATABASE_URL', 'PGHOST',
                    'DOCKER_HOST', 'DOCKER_CONTEXT', 'DOCKER_TLS_VERIFY'):
            if os.environ.get(bad):
                raise ValueError(f'refusing inherited production-like environment variable {bad}')
        check_checkout(authority, AUTHORITY_MERGED_SHA)
        check_product_checkout(product)
        check_go_version(run(['go', 'version'], output=True).stdout)
        endpoint = run(['docker', 'context', 'inspect', '--format',
                        '{{ .Endpoints.docker.Host }}'], output=True).stdout.strip()
        if not re.fullmatch(r'unix:///[^\s]+\.sock', endpoint):
            raise ValueError('Docker context is not a local Unix socket')
        run(['docker', 'info'], output=True)
        with tempfile.TemporaryDirectory(prefix='lic-a9-joint-e2e-') as path:
            fixture = Path(path) / 'joint_fixture'
            preparer = product / 'tests/installationlicense/prepare_authority_fixture.py'
            run([sys.executable, '-B', str(preparer), str(authority), str(fixture)], timeout=90)
            env = go_test_env()
            print('Compiling exact joint source fixture (no database)...', flush=True)
            run(['go', 'test', '-count=1', '-run', '^$', './authority'],
                cwd=fixture, env=env, timeout=180)
            # Only a disposable localhost container is permitted. Never accept a DSN.
            container = 'lic-a9-joint-' + uuid.uuid4().hex[:12]
            run(['docker', 'run', '-d', '--rm', '--name', container,
                 '--label', 'telmivo.purpose=lic-a9-disposable-joint-e2e',
                 '--memory=512m', '--cpus=1', '-e', 'POSTGRES_HOST_AUTH_METHOD=trust',
                 '-e', 'POSTGRES_DB=lic_a9_joint_test',
                 '-p', '127.0.0.1::5432', 'postgres:16',
                 '-c', 'shared_buffers=32MB', '-c', 'max_connections=35'], timeout=120)
            port = loopback_port(run(['docker', 'port', container, '5432/tcp'], output=True).stdout)
            deadline = time.monotonic() + 60
            while True:
                ready = subprocess.run(['docker', 'exec', container, 'pg_isready',
                                        '-U', 'postgres', '-d', 'lic_a9_joint_test'],
                                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                if ready.returncode == 0:
                    break
                if time.monotonic() >= deadline:
                    raise TimeoutError('disposable PostgreSQL 16 did not become ready')
                time.sleep(1)
            env.update(LIC_A8_TEST_DATABASE_URL=(
                f'postgres://postgres@127.0.0.1:{port}/lic_a9_joint_test?sslmode=disable'))
            print('Executing synthetic Authority + product PostgreSQL tests...', flush=True)
            run(['go', 'test', '-count=1', '-v', '-timeout', '8m',
                 '-run', '^TestE2E[1-6]$', './authority'],
                cwd=fixture, env=env, timeout=540)
            print('LIC_A9_JOINT_GO127_POSTGRES16_E2E_PASS')
        return 0
    except (OSError, ValueError, subprocess.CalledProcessError,
            subprocess.TimeoutExpired, TimeoutError) as exc:
        print(f'NOT_PASS: {exc}', file=sys.stderr)
        return 3
    finally:
        if container is not None:
            subprocess.run(['docker', 'stop', container], capture_output=True,
                           text=True, timeout=45, check=False)


if __name__ == '__main__':
    raise SystemExit(main())
