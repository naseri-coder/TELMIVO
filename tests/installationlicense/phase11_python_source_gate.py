"""Read-only Python syntax and entrypoint gate for LIC-A9 disposable joint E2E.

This checks *source syntax*, not the integrity of a Docker image, external
signatures, or whether the Go/PostgreSQL E2E tests actually pass. It creates
no bytecode files and does not import or execute the inspected test scripts.
"""
import argparse
import ast
import json
from pathlib import Path


RUNNER = 'run_joint_docker_only_e2e.py'
REQUIRED = frozenset({
    'run_joint_docker_only_e2e.py',
    'run_joint_postgres_e2e.py',
    'prepare_authority_fixture.py',
    'test_joint_docker_only_e2e.py',
    'test_joint_runner.py',
    'test_phase02_authority_pin.py',
    'test_phase08_refreeze.py',
    'phase09_host_requirements.py',
    'test_phase09_host_requirements.py',
    'phase10_image_identity.py',
    'test_phase10_image_identity.py',
    'phase11_python_source_gate.py',
    'test_phase11_python_source_gate.py',
})


class SourceDenied(ValueError):
    pass


def _is_main_guard(node):
    if not isinstance(node, ast.If):
        return False
    test = node.test
    return (isinstance(test, ast.Compare)
            and isinstance(test.left, ast.Name)
            and test.left.id == '__name__'
            and len(test.ops) == 1 and isinstance(test.ops[0], ast.Eq)
            and len(test.comparators) == 1
            and isinstance(test.comparators[0], ast.Constant)
            and test.comparators[0].value == '__main__')


def _is_executable_main_entrypoint(node):
    """Accept only the canonical terminal raise SystemExit(main())."""
    if not _is_main_guard(node) or node.orelse or len(node.body) != 1:
        return False
    stmt = node.body[0]
    if not isinstance(stmt, ast.Raise) or stmt.cause is not None:
        return False
    call = stmt.exc
    if not (isinstance(call, ast.Call)
            and isinstance(call.func, ast.Name) and call.func.id == 'SystemExit'
            and len(call.args) == 1 and not call.keywords):
        return False
    arg = call.args[0]
    return (isinstance(arg, ast.Call)
            and isinstance(arg.func, ast.Name) and arg.func.id == 'main'
            and not arg.args and not arg.keywords)


def audit(root):
    """Fail closed on a missing source or any malformed Python before Docker."""
    directory = Path(root)
    if not directory.is_dir() or directory.is_symlink():
        raise SourceDenied('SOURCE_DIRECTORY_UNSAFE_OR_MISSING')
    paths = sorted(directory.glob('*.py'))
    present = {p.name for p in paths}
    if not REQUIRED.issubset(present):
        raise SourceDenied('PYTHON_SOURCE_INVENTORY_INCOMPLETE')
    for path in paths:
        if path.is_symlink() or not path.is_file() or path.parent != directory:
            raise SourceDenied('PYTHON_SOURCE_PATH_UNSAFE')
        try:
            source = path.read_bytes()
            tree = ast.parse(source, filename=path.name)
        except (SyntaxError, UnicodeError, ValueError) as exc:
            raise SourceDenied('PYTHON_SYNTAX_INVALID:' + path.name) from exc
        if path.name == RUNNER:
            main_guards = [node for node in tree.body if _is_main_guard(node)]
            if len(main_guards) != 1 or not _is_main_guard(tree.body[-1]):
                raise SourceDenied('RUNNER_MAIN_GUARD_NOT_UNIQUE_OR_TERMINAL')
            if (not _is_executable_main_entrypoint(tree.body[-1])
                    or not any(isinstance(node, ast.FunctionDef)
                               and node.name == 'main' for node in tree.body)):
                raise SourceDenied('RUNNER_MAIN_ENTRYPOINT_NOT_EXECUTABLE')
    return {
        'schema': 'TELMIVO_LIC_A9_PHASE11_PYTHON_SOURCE_GATE_V1',
        'verdict': 'SYNTAX_AND_RUNNER_ENTRYPOINT_PASS_ONLY',
        'inspected_files': len(paths),
        'go_pg16_e2e_executed': False,
        'docker_contacted': False,
        'production_authorized': False,
    }


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--test-source-dir', type=Path, required=True)
    args = parser.parse_args(argv)
    try:
        result = audit(args.test_source_dir)
    except (OSError, SourceDenied) as exc:
        reason = str(exc) if isinstance(exc, SourceDenied) else 'SOURCE_READ_DENIED'
        print(json.dumps({'verdict': 'SOURCE_DENIED', 'reason': reason,
                          'production_authorized': False}, sort_keys=True))
        return 3
    print(json.dumps(result, sort_keys=True))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
