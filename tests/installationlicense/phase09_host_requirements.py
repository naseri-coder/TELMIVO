#!/usr/bin/env python3
"""Read-only host capability scan for LIC-A9 joint Go+PG16 Docker tests.

Never invokes Docker, Go, Git, systemd, SQL, network or services. Presence
of Docker CLI alone cannot certify the local daemon or container isolation.
"""
import json
import os
import shutil
import sys

BLOCKING_ENV = (
    'TELMIVO_ENV', 'TELMIVO_LICENSE_TRUST_FILE', 'TELMIVO_LICENSE_STATE_DIR',
    'AUTHORITY_DATABASE_URL', 'AUTHORITY_TEST_DATABASE_URL',
    'DATABASE_URL', 'PGHOST', 'DOCKER_HOST', 'DOCKER_CONTEXT',
    'DOCKER_TLS_VERIFY', 'LIC_A8_TEST_DATABASE_URL',
)


def scan(which=shutil.which, env=None):
    if env is None:
        env = os.environ
    inherited = [name for name in BLOCKING_ENV if env.get(name)]
    cli = which('docker') is not None
    git = which('git') is not None
    python = sys.version_info >= (3, 10)
    if inherited:
        verdict = 'BLOCKED_POSSIBLE_PRODUCTION_ENV'
    elif not cli:
        verdict = 'BLOCKED_NO_DOCKER_CLIENT'
    elif not git or not python:
        verdict = 'BLOCKED_MISSING_LOCAL_PREREQUISITE'
    else:
        verdict = 'STATIC_PREREQUISITES_SEEN_ENGINE_NOT_VERIFIED'
    return {
        'schema': 'TELMIVO_LIC_A9_PHASE09_STATIC_PREFLIGHT_V1',
        'verdict': verdict,
        'docker_client_present': cli,
        'git_client_present': git,
        'python_310_plus': python,
        'forbidden_environment_variable_names': inherited,
        'local_go127_required': False,
        'local_postgresql16_required': False,
        'docker_daemon_contacted': False,
        'docker_images_checked': False,
        'go_tests_run': False,
        'postgresql_tests_run': False,
        'production_changed': False,
        'release_certified': False,
    }


def main():
    result = scan()
    print(json.dumps(result, sort_keys=True))
    return 3 if result['verdict'].startswith('BLOCKED_') else 2


if __name__ == '__main__':
    raise SystemExit(main())
