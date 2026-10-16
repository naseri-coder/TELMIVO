#!/usr/bin/env python3
"""Static M2M compatibility checks. No network, PostgreSQL or production calls."""
import argparse
import ast
import pathlib
import re
import subprocess
import sys

AUTHORITY_SHA = "68cdb6996f85981c36dd20ecb7435ec010351e9b"
LEGACY_SHA = "392fc89087d8dac001d1709d0f7973c2f28a8515"
ROUTES = ("activate", "refresh", "deactivate", "status")
PROOF_FIELDS = ("[]byte(method)", "[]byte(path)", "[]byte(query)",
                "[]byte(timestamp)", "[]byte(nonce)", "[]byte(key)",
                "[]byte(i.ID)", "pk[:]", "bh[:]")


def check_source(contract: str, client: str, fixture: str) -> list[str]:
    failures = []
    for route in ROUTES:
        if f"/v1/licenses/{route}" not in contract:
            failures.append(f"authority route missing: {route}")
    if "TELMIVO_INSTALLATION_REQUEST_V1" not in contract:
        failures.append("authority proof namespace missing")
    if r"TELMIVO_INSTALLATION_REQUEST_V1\x00" not in client:
        failures.append("product proof namespace missing")
    match = re.search(r"for\s+_,\s*f\s*:=\s*range\s*\[\]\[\]byte\s*\{([^}]+)\}", client, re.S)
    if not match:
        failures.append("product proof fields missing")
    else:
        fields = tuple(re.sub(r"\s+", "", x) for x in match.group(1).split(",") if x.strip())
        if fields != PROOF_FIELDS:
            failures.append("product proof fields changed")
    for header in ('X-Telmivo-Installation-ID', 'X-Telmivo-Timestamp',
                   'X-Telmivo-Nonce', 'X-Telmivo-Signature',
                   'Idempotency-Key', 'application/json'):
        if f'"{header}"' not in client:
            failures.append("missing client header: " + header)
    try:
        parsed = ast.parse(fixture)
        assigns = [stmt for stmt in parsed.body if isinstance(stmt, ast.Assign)
                   and any(isinstance(t, ast.Name) and t.id == "sha" for t in stmt.targets)]
        if len(assigns) != 1 or ast.literal_eval(assigns[0].value) != AUTHORITY_SHA:
            failures.append("fixture source pin not PR32 (possibly legacy Authority SHA)")
        if '"archive", sha' not in fixture and "'archive',sha" not in fixture:
            failures.append("fixture source archive missing")
    except (SyntaxError, ValueError, TypeError):
        failures.append("fixture invalid Python syntax or nonconstant SHA")
    return failures


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--authority-root", required=True, type=pathlib.Path)
    parser.add_argument("--product-root", required=True, type=pathlib.Path)
    args = parser.parse_args(argv)
    try:
        head = subprocess.run(["git", "-C", str(args.authority_root), "rev-parse", "HEAD"],
                              check=True, capture_output=True, text=True).stdout.strip()
        if head != AUTHORITY_SHA:
            print("FAIL: Authority checkout must be exact PR32 SHA", file=sys.stderr)
            return 2
        contract = (args.authority_root / "docs/m2m-api.md").read_text()
        client = (args.product_root / "internal/platform/installationlicense/client.go").read_text()
        fixture = (args.product_root / "tests/installationlicense/prepare_authority_fixture.py").read_text()
    except (OSError, subprocess.CalledProcessError) as exc:
        print(f"FAIL: source unavailable: {exc}", file=sys.stderr)
        return 2
    failures = check_source(contract, client, fixture)
    for issue in failures:
        print("FAIL: " + issue, file=sys.stderr)
    if failures:
        return 1
    print("PASS: static M2M contract only; Go/PostgreSQL E2E still pending")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
