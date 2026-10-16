"""Strict offline network+module gate for disposable LIC-A9 Docker Go tests.

The Go Docker image and modules must be independently provisioned and reviewed
before the test run. None of these helpers contact a daemon, source repo,
registry, network or a real license server. They only validate/return argv.
"""
import re

NETWORK_PATTERN = re.compile(r'container:lic-a9-joint-[0-9a-f]{12}\Z')
OFFLINE_ENV = (
    'GOTOOLCHAIN=local',
    'GOWORK=off',
    'GOTELEMETRY=off',
    'GOPROXY=off',
    'GOSUMDB=off',
)


def validate_go_network(network: str) -> str:
    if type(network) is not str:
        raise ValueError('Go network must be explicit and reviewed')
    if network == 'none' or NETWORK_PATTERN.fullmatch(network):
        return network
    raise ValueError('no bridge, host or unreviewed Go container network permitted')


def offline_go_env_flags() -> list[str]:
    return [arg for setting in OFFLINE_ENV for arg in ('--env', setting)]


def audit_go_docker_argv(argv: list[str]) -> None:
    """Check builder output, including a single isolated network and offline Go env.

    Do not trust argv that merely *includes* safe flags: duplicate Docker
    --network or --env options could override those flags. Reject duplicates
    and unreviewed environment variables that permit dependency downloads.
    """
    if type(argv) is not list or any(type(part) is not str for part in argv):
        raise ValueError('not a reviewed Docker argv')
    if argv[:3] != ['docker', 'run', '--pull=never']:
        raise ValueError('Docker must run a locally pinned image, never pull')
    if argv.count('--network') != 1:
        raise ValueError('Docker must specify exactly one network namespace')
    idx = argv.index('--network')
    if idx + 1 >= len(argv):
        raise ValueError('missing network namespace value')
    validate_go_network(argv[idx + 1])
    env = []
    for i, token in enumerate(argv):
        if token in ('--env', '-e'):
            if i + 1 >= len(argv):
                raise ValueError('incomplete Docker environment argument')
            env.append(argv[i + 1])
        if (token in ('--publish', '-p', '--publish-all', '-P', '--net',
                       '--cap-add', '--privileged')
                or token.startswith(('--network=', '--net=', '--publish=',
                                     '-p=', '--env=', '-e=', '--cap-add=',
                                     '--privileged='))):
            raise ValueError('Docker override of isolation policy is forbidden')
    for name in ('GOTOOLCHAIN', 'GOWORK', 'GOTELEMETRY', 'GOPROXY', 'GOSUMDB'):
        values = [entry for entry in env if entry.split('=', 1)[0] == name]
        if len(values) != 1 or values[0] != next(x for x in OFFLINE_ENV if x.startswith(name + '=')):
            raise ValueError('missing, duplicated or unsafe Go offline environment: ' + name)
    if not any(x == '--cap-drop' for x in argv) or '--privileged' in argv:
        raise ValueError('privileged Go test container is forbidden')
