#!/usr/bin/env python3
"""Offline source-contract guard: static checks only; NOT E2E or Production approval."""
import argparse
import pathlib
import re
import subprocess
import sys

AUTHORITY_SHA = "68cdb6996f85981c36dd20ecb7435ec010351e9b"
LEGACY_SHA = "392fc89087d8dac001d1709d0f7973c2f28a8515"
ROUTES = ("activate", "refresh", "deactivate", "status")
PROOF_FIELDS = (
    "[]byte(method)", "[]byte(path)", "[]byte(query)",
    "[]byte(timestamp)", "[]byte(nonce)", "[]byte(key)",
    "[]byte(i.ID)", "pk[:]", "bh[:]")


def check_source(contract: str, client: str, fixture: str) -> list[str]:
    failures = []
    for route in ROUTES:
        if f"/v1/licenses/{route}" not in contract:
            failures.append(f"authority route missing: {route}")
    if "TELMIVO_INSTALLATION_REQUEST_V1" not in contract:
        failures.append("authority proof namespace missing")
    if "TELMIVO_INSTALLATION_REQUEST_V1\\x00" not in client:
        failures.append("product proof namespace missing")
    match = re.search(r"for\s+_,\s*f\s*:=\s*range\s*\[\]\[\]byte\s*\{([^}]+)\}", client, re.S)
    if not match:
        failures.append("product proof field tuple missing")
    else:
        fields = [re.sub(r"\s+", "", x) for x in match.group(1).split(",") if x.strip()]
        if tuple(fields) != PROOF_FIELDS:
            failures.append(f"product proof fields changed: {fields!r}")
    for marker in ('"X-Telmivo-Installation-ID"', '"X-Telmivo-Timestamp"',
                   '"X-Telmivo-Nonce"', '"X-Telmivo-Signature"',
                   '"Idempotency-Key"', '"application/json"'):
        if marker not in client:
            failures.append(f"product HTTP contract marker missing: {marker}")
    if f"sha='{AUTHORITY_SHA}'" not in fixture:
        failures.append("test fixture does not pin the selected PR32 Authority SHA")
    if f"sha='{LEGACY_SHA}'" in fixture:
        failures.append("legacy Authority SHA still pinned")
    if "git','-C',str(repo),'archive',sha" not in fixture:
        failures.append("fixture provenance is not pinned to git archive")
    return failures


def git_head(root: pathlib.Path) -> str:
    result = subprocess.run(["git", "-C", str(root), "rev-parse", "HEAD"],
                            capture_output=True, text=True, check=True)
    return result.stdout.strip()


def main(argv=None) -> int:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--authority-root", required=True, type=pathlib.Path)
    p.add_argument("--product-root", required=True, type=pathlib.Path)
    a = p.parse_args(argv)
    try:
        if git_head(a.authority_root) != AUTHORITY_SHA:
            print("FAIL: Authority checkout must be exact selected PR32 SHA", file=sys.stderr)
            return 2
        doc = (a.authority_root / "docs/m2m-api.md").read_text()
        client = (a.product_root / "internal/platform/installationlicense/client.go").read_text()
        fixture = (a.product_root / "tests/installationlicense/prepare_authority_fixture.py").read_text()
        failures = check_source(doc, client, fixture)
    except (OSError, subprocess.CalledProcessError) as exc:
        print(f"FAIL: cannot inspect source: {exc}", file=sys.stderr)
        return 2
    for failure in failures:
        print("FAIL: " + failure, file=sys.stderr)
    if failures:
        return 1
    print("PASS: current-source static M2M contract guard; NOT an E2E execution")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
