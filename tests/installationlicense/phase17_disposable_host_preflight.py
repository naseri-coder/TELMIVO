#!/usr/bin/env python3
"""Read-only local-host prerequisites for a SEPARATELY approved LIC-A9 E2E VM.

Never starts, queries or installs Docker, Go, PostgreSQL, Git or any service.
Never performs network requests, reads secrets or certifies host ownership.
The result is an inventory for human review, NOT an authorization to run E2E.
"""
from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import re
import shutil
import stat
import sys

MIN_VCPU = 2
MIN_MEMORY_BYTES = 4 * 1024**3
MIN_FREE_BYTES = 10 * 1024**3
SCHEMA = 'TELMIVO_LIC_A9_PHASE17_HOST_PREFLIGHT_V1'
SENSITIVE_ENV_NAMES = frozenset({
    'DOCKER_HOST', 'DOCKER_CONTEXT', 'DOCKER_TLS_VERIFY', 'DOCKER_CERT_PATH',
    'DATABASE_URL', 'PGHOST', 'PGPASSWORD', 'PGSERVICE',
    'AUTHORITY_DATABASE_URL', 'AUTHORITY_TEST_DATABASE_URL',
    'TELMIVO_ENV', 'TELMIVO_LICENSE_TRUST_FILE', 'TELMIVO_LICENSE_STATE_DIR',
    'LIC_A8_TEST_DATABASE_URL', 'TELEGRAM_BOT_TOKEN',
    'AWS_ACCESS_KEY_ID', 'AWS_SECRET_ACCESS_KEY', 'GOOGLE_APPLICATION_CREDENTIALS',
    'KUBECONFIG',
})
VALID_PLATFORMS = frozenset({('ubuntu', '24.04'), ('debian', '13')})


def parse_os_release(raw: str) -> tuple[str, str]:
    """Read only ID and VERSION_ID; no command evaluation or expansion."""
    fields: dict[str, str] = {}
    for line in raw.splitlines():
        if '=' not in line or line.startswith('#'):
            continue
        key, value = line.split('=', 1)
        if key in ('ID', 'VERSION_ID'):
            value = value.strip().strip('"').strip("'")
            if not re.fullmatch(r'[A-Za-z0-9.\-_]+', value):
                raise ValueError('malformed OS identity')
            fields[key] = value.lower()
    return fields.get('ID', ''), fields.get('VERSION_ID', '')


def parse_memtotal(raw: str) -> int:
    match = re.search(r'^MemTotal:\s*(\d+)\s+kB\s*$', raw, re.MULTILINE)
    if not match:
        raise ValueError('unavailable physical memory')
    return int(match.group(1)) * 1024


def admissible_socket_path(path: str) -> bool:
    if not isinstance(path, str) or not path.startswith('/run/') and path != '/var/run/docker.sock':
        return False
    item = Path(path)
    if '..' in item.parts or not path.endswith('/docker.sock'):
        return False
    # Only accept the standard host Unix socket or a per-user runtime socket.
    return path == '/var/run/docker.sock' or bool(
        re.fullmatch(r'/run/user/[0-9]+/docker\.sock', path)
    )


def evaluate(facts: dict) -> dict:
    """Pure fail-closed decision; never certify the real host as disposable."""
    blocks: list[str] = []
    def deny(code: str, failed: bool) -> None:
        if failed:
            blocks.append(code)

    deny('UNSUPPORTED_OS', (facts.get('os_id'), facts.get('os_version')) not in VALID_PLATFORMS)
    deny('INSUFFICIENT_VCPU', not isinstance(facts.get('vcpu'), int)
         or facts['vcpu'] < MIN_VCPU)
    deny('INSUFFICIENT_RAM', not isinstance(facts.get('memory_bytes'), int)
         or facts['memory_bytes'] < MIN_MEMORY_BYTES)
    deny('INSUFFICIENT_FREE_DISK', not isinstance(facts.get('free_bytes'), int)
         or facts['free_bytes'] < MIN_FREE_BYTES)
    deny('PYTHON_310_REQUIRED', not facts.get('python_310'))
    deny('GIT_NOT_INSTALLED', not facts.get('git_cli'))
    deny('DOCKER_CLI_NOT_INSTALLED', not facts.get('docker_cli'))
    deny('LOCAL_DOCKER_SOCKET_MISSING', not facts.get('docker_socket'))
    deny('DOCKER_SOCKET_WORLD_WRITABLE', bool(facts.get('socket_world_writable')))
    deny('UNSAFE_DOCKER_SOCKET_PATH', not facts.get('socket_path_safe'))
    deny('SENSITIVE_ENVIRONMENT_PRESENT', bool(facts.get('sensitive_env_names')))
    deny('SCRATCH_DIRECTORY_UNSAFE', not facts.get('scratch_safe'))
    # Do not declare READY just because a Docker binary/socket appears on a VM.
    return {
        'schema': SCHEMA,
        'verdict': ('BLOCKED_NONPRODUCTION_HOST_PREREQUISITES' if blocks else
                    'STATIC_PREREQUISITES_PRESENT_EXTERNAL_APPROVAL_REQUIRED'),
        'blocking_codes': sorted(blocks),
        'sensitive_environment_variable_names': sorted(facts.get('sensitive_env_names', [])),
        'detected': {
            'os_supported': (facts.get('os_id'), facts.get('os_version')) in VALID_PLATFORMS,
            'vcpu_minimum_met': isinstance(facts.get('vcpu'), int) and facts['vcpu'] >= MIN_VCPU,
            'ram_minimum_met': isinstance(facts.get('memory_bytes'), int)
                               and facts['memory_bytes'] >= MIN_MEMORY_BYTES,
            'scratch_disk_minimum_met': isinstance(facts.get('free_bytes'), int)
                                        and facts['free_bytes'] >= MIN_FREE_BYTES,
            'docker_cli_present': bool(facts.get('docker_cli')),
            'local_unix_docker_socket_present': bool(facts.get('docker_socket')),
            'git_present': bool(facts.get('git_cli')),
        },
        'docker_daemon_contacted': False,
        'containers_created': False,
        'postgresql_started': False,
        'production_host_identity_verified': False,
        'independent_host_approval_verified': False,
        'private_repositories_cloned': False,
        'real_joint_e2e_executed': False,
        'production_authorized': False,
    }


def gather(*, scratch_dir: Path, docker_socket: str,
           environ=None, which=None) -> dict:
    """Host-local reads ONLY. Never invoke docker info or inspect secrets."""
    if which is None:
        which = shutil.which
    if environ is None:
        environ = os.environ
    try:
        ident = parse_os_release(Path('/etc/os-release').read_text())
    except (OSError, ValueError):
        ident = ('', '')
    try:
        mem = parse_memtotal(Path('/proc/meminfo').read_text())
    except (OSError, ValueError):
        mem = 0
    try:
        vcpu = len(os.sched_getaffinity(0))
    except (AttributeError, OSError):
        vcpu = os.cpu_count() or 0
    try:
        scratch_safe = (scratch_dir.is_absolute() and scratch_dir.is_dir()
                        and not scratch_dir.is_symlink()
                        and '..' not in scratch_dir.parts)
        statvfs = os.statvfs(scratch_dir) if scratch_safe else None
        free = statvfs.f_bavail * statvfs.f_frsize if statvfs else 0
    except OSError:
        scratch_safe, free = False, 0
    try:
        if admissible_socket_path(docker_socket):
            socket_stat = Path(docker_socket).lstat()
            is_socket = stat.S_ISSOCK(socket_stat.st_mode)
            world_write = bool(socket_stat.st_mode & stat.S_IWOTH)
        else:
            is_socket, world_write = False, False
    except OSError:
        is_socket, world_write = False, False
    return {
        'os_id': ident[0], 'os_version': ident[1], 'vcpu': vcpu,
        'memory_bytes': mem, 'free_bytes': free,
        'python_310': sys.version_info >= (3, 10),
        'git_cli': which('git') is not None,
        'docker_cli': which('docker') is not None,
        'socket_path_safe': admissible_socket_path(docker_socket),
        'docker_socket': is_socket,
        'socket_world_writable': world_write,
        'scratch_safe': scratch_safe,
        'sensitive_env_names': sorted(k for k in SENSITIVE_ENV_NAMES if environ.get(k)),
    }


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--scratch-dir', type=Path, default=Path('/tmp'))
    parser.add_argument('--docker-socket', default='/var/run/docker.sock')
    args = parser.parse_args(argv)
    result = evaluate(gather(scratch_dir=args.scratch_dir, docker_socket=args.docker_socket))
    print(json.dumps(result, sort_keys=True, ensure_ascii=False))
    # Exit 2 means review still required, NOT release GO. Exit 3 means blocked.
    return 3 if result['blocking_codes'] else 2


if __name__ == '__main__':
    raise SystemExit(main())
