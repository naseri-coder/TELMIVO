#!/usr/bin/env python3
"""Check the actual native workflow/config contract; optionally enumerate Playwright."""
import argparse
import ast
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[2]

def selection(workflow: str, config: str) -> list[str]:
    loop = re.search(r"for spec in ([^;]+); do", workflow)
    match = re.search(r"testMatch\s*:\s*(\[[^\]]*\])", config)
    if not loop or not match:
        raise ValueError("native spec loop or explicit testMatch is missing")
    intended = loop[1].split()
    admitted = ast.literal_eval(match[1])
    if not intended or len(set(intended)) != len(intended):
        raise ValueError("empty/duplicate workflow spec selection")
    if not isinstance(admitted, list) or not all(isinstance(x, str) for x in admitted):
        raise ValueError("explicit filename testMatch required")
    if len(set(admitted)) != len(admitted) or set(intended) != set(admitted):
        raise ValueError(f"workflow/config drift: intended={intended}, admitted={admitted}")
    return intended

def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--enumerate", action="store_true")
    args = parser.parse_args()
    specs = selection((ROOT / '.github/workflows/migration-check.yml').read_text(),
                      (ROOT / 'web/miniapp/playwright.native.config.ts').read_text())
    for spec in specs:
        if not (ROOT / 'web/miniapp/e2e' / spec).is_file():
            raise ValueError(f"missing native spec {spec}")
        if args.enumerate:
            result = subprocess.run(['npx', '--no-install', 'playwright', 'test', '--config',
                                     'playwright.native.config.ts', f'/{re.escape(spec)}$', '--list'],
                                    cwd=ROOT / 'web/miniapp', text=True, capture_output=True)
            if result.returncode:
                raise RuntimeError(f'{spec}: Playwright enumeration failed\n{result.stdout}{result.stderr}')
            total = re.search(r'Total:\s+(\d+)\s+tests?\b', result.stdout)
            if not total or int(total[1]) < 1:
                raise ValueError(f"native spec selected zero tests: {spec}\n{result.stdout}")
            print(f'{spec}: {total[1]} tests selected')
    print(f'NATIVE_SELECTION_PASS: {len(specs)} intended specs' +
          (' enumerated' if args.enumerate else ' statically aligned; enumeration required'))

if __name__ == '__main__':
    main()
