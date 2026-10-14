#!/usr/bin/env python3
"""Run actual checked-out LIC-A9 static and Go unit checks; never invoke the DB.

Both source revisions must be explicit, clean, committed and immutable.
Toolchain/modules may use local build caches; this tool never deploys services.
"""
import argparse
import pathlib
import re
import subprocess
import sys

AUTHORITY_SHA = "68cdb6996f85981c36dd20ecb7435ec010351e9b"


def repository_sha(root: pathlib.Path) -> str:
    return subprocess.run(
        ["git", "-C", str(root), "rev-parse", "--verify", "HEAD^{commit}"],
        capture_output=True, text=True, check=True).stdout.strip()


def is_clean(root: pathlib.Path) -> bool:
    result = subprocess.run(
        ["git", "-C", str(root), "status", "--porcelain", "--untracked-files=all"],
        capture_output=True, text=True, check=True)
    return not result.stdout.strip()


def run(argv: list[str], cwd: pathlib.Path) -> None:
    subprocess.run(argv, cwd=cwd, check=True)


def main(argv=None) -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--authority", type=pathlib.Path, required=True)
    parser.add_argument("--product", type=pathlib.Path, required=True)
    parser.add_argument("--expected-product-sha", required=True,
                        help="Exact 40-hex Git commit SHA for TELMIVO, not a mutable branch")
    args = parser.parse_args(argv)
    if not re.fullmatch(r"[0-9a-f]{40}", args.expected_product_sha):
        print("BLOCKED: expected product SHA must be exactly 40 lowercase hex digits",
              file=sys.stderr)
        return 3
    authority, product = args.authority.resolve(), args.product.resolve()
    if authority == product:
        print("BLOCKED: Authority and product must be different checkouts", file=sys.stderr)
        return 3
    try:
        if repository_sha(authority) != AUTHORITY_SHA:
            print("BLOCKED: Authority checkout is not exact PR32 SHA", file=sys.stderr)
            return 3
        if repository_sha(product) != args.expected_product_sha:
            print("BLOCKED: TELMIVO checkout does not match pinned product SHA", file=sys.stderr)
            return 3
        if not is_clean(authority) or not is_clean(product):
            print("BLOCKED: uncommitted or untracked source in either checkout",
                  file=sys.stderr)
            return 3
        # Run source-derived checks before invoking Go, without any database.
        for pattern in ("test_contract_guard.py", "test_fixture_source_integrity.py",
                        "test_go_toolchain_gate.py"):
            run([sys.executable, "-B", "-m", "unittest", "discover",
                 "-s", "tests/installationlicense", "-p", pattern, "-v"], product)
        run([sys.executable, "-B", "tests/installationlicense/contract_guard.py",
             "--authority-root", str(authority), "--product-root", str(product)], product)
        run([sys.executable, "-B", "tests/installationlicense/go_toolchain_gate.py",
             "--product-go-mod", str(product / "go.mod")], product)
        run(["go", "test", "-count=1", "./internal/platform/installationlicense"], product)
    except (OSError, subprocess.CalledProcessError) as exc:
        print(f"NOT_PASS: test or prerequisite failed: {exc}", file=sys.stderr)
        return 3
    print("PASS: pinned clean-source static guards and TELMIVO installationlicense Go units")
    print("NOT_RUN: cross-repo Authority/PostgreSQL end-to-end license tests")
    return 0


if __name__ == "__main__":
    sys.exit(main())
