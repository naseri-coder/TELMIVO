#!/usr/bin/env python3
"""Run *actual checked-out* LIC-A9 contract/unit tests without a VPS.

Read-only to source and production infrastructure. The Go toolchain may write
its normal local build cache. Never opens a database or contacts Telegram.
"""
import argparse
import pathlib
import subprocess
import sys

AUTHORITY_SHA = "68cdb6996f85981c36dd20ecb7435ec010351e9b"


def run(argv: list[str], cwd: pathlib.Path) -> None:
    subprocess.run(argv, cwd=cwd, check=True)


def main(argv=None) -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--authority", type=pathlib.Path, required=True)
    parser.add_argument("--product", type=pathlib.Path, required=True)
    args = parser.parse_args(argv)
    authority, product = args.authority.resolve(), args.product.resolve()
    try:
        head = subprocess.run(
            ["git", "rev-parse", "HEAD"], cwd=authority,
            capture_output=True, text=True, check=True).stdout.strip()
        if head != AUTHORITY_SHA:
            print("BLOCKED: Authority checkout is not exact PR32 SHA", file=sys.stderr)
            return 3
        # Explicitly test source-derived checks before attempting Go.
        run([sys.executable, "-B", "-m", "unittest", "discover",
             "-s", "tests/installationlicense", "-p", "test_contract_guard.py", "-v"], product)
        run([sys.executable, "-B", "-m", "unittest", "discover",
             "-s", "tests/installationlicense", "-p", "test_fixture_source_integrity.py", "-v"], product)
        run([sys.executable, "-B", "-m", "unittest", "discover",
             "-s", "tests/installationlicense", "-p", "test_go_toolchain_gate.py", "-v"], product)
        run([sys.executable, "-B", "tests/installationlicense/contract_guard.py",
             "--authority-root", str(authority), "--product-root", str(product)], product)
        # Fail closed before Go invocation when declared toolchain is unavailable.
        run([sys.executable, "-B", "tests/installationlicense/go_toolchain_gate.py",
             "--product-go-mod", str(product / "go.mod")], product)
        run(["go", "test", "-count=1", "./internal/platform/installationlicense"], product)
    except (OSError, subprocess.CalledProcessError) as exc:
        print(f"NOT_PASS: test or prerequisite failed: {exc}", file=sys.stderr)
        return 3
    print("PASS: checked-out source static guards and installationlicense Go unit tests")
    print("NOT_RUN: Authority/PostgreSQL end-to-end licensing integration")
    return 0


if __name__ == "__main__":
    sys.exit(main())
