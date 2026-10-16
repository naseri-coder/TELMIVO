#!/usr/bin/env python3
"""Read-only toolchain gate for TELMIVO / Authority joint E2E."""
import argparse
import pathlib
import re
import subprocess
import sys


def required_version(go_mod: str) -> tuple[int, int]:
    hits = re.findall(r'^go\s+(\d+)\.(\d+)(?:\.\d+)?\s*$', go_mod, re.M)
    if len(hits) != 1:
        raise ValueError('go.mod must declare exactly one Go version')
    return tuple(map(int, hits[0]))


def installed_version(output: str) -> tuple[int, int]:
    match = re.fullmatch(r'go version go(\d+)\.(\d+)(?:\.\d+)?(?:[-\w.]*)? \S+/\S+\s*', output)
    if not match:
        raise ValueError('unsupported go version output')
    return int(match.group(1)), int(match.group(2))


def assess(mod: str, version: str) -> tuple[bool, str]:
    required = required_version(mod)
    actual = installed_version(version)
    if actual < required:
        return False, f'BLOCKED: Go {required[0]}.{required[1]}+ required, installed {actual[0]}.{actual[1]}'
    return True, f'PASS: Go {actual[0]}.{actual[1]} >= {required[0]}.{required[1]}'


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument('--product-go-mod', type=pathlib.Path, required=True)
    ns = ap.parse_args()
    try:
        mod = ns.product_go_mod.read_text()
        proc = subprocess.run(['go', 'version'], capture_output=True, text=True, check=True)
        ok, result = assess(mod, proc.stdout)
    except (ValueError, OSError, subprocess.CalledProcessError) as ex:
        print(f'BLOCKED: {ex}', file=sys.stderr)
        return 2
    print(result)
    return 0 if ok else 3


if __name__ == '__main__':
    sys.exit(main())
