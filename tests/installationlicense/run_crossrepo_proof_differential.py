#!/usr/bin/env python3
"""Offline Go differential from two pinned, clean GitHub checkouts.

Scope is request-signature byte framing only, NOT full Go1.27 build / DB E2E.
"""
import argparse
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile

AUTHORITY_SHA = "68cdb6996f85981c36dd20ecb7435ec010351e9b"
DOMAIN = r"TELMIVO_INSTALLATION_REQUEST_V1\x00"


def head(path: Path) -> str:
    return subprocess.run(["git", "-C", str(path), "rev-parse", "HEAD"],
                          check=True, capture_output=True, text=True).stdout.strip()


def clean(path: Path) -> bool:
    return not subprocess.run(
        ["git", "-C", str(path), "status", "--porcelain", "--untracked-files=all"],
        check=True, capture_output=True, text=True).stdout.strip()


def extract(source: str, prefix: str) -> str:
    """Fail closed on missing or duplicated known top-level Go declaration."""
    matches = list(re.finditer(r"(?m)^" + re.escape(prefix), source))
    if len(matches) != 1:
        raise ValueError(f"expected exactly one {prefix!r}")
    start = matches[0].start()
    end = source.find("\n}", start)
    if end == -1:
        raise ValueError(f"missing end of {prefix!r}")
    return source[start:end + 2] + "\n"


def extracted_go(client_source: str, authority_source: str) -> tuple[str, str]:
    client = extract(client_source, "func proof(")
    authority = extract(authority_source, "func SigningInput(")
    if client.count(DOMAIN) != 1:
        raise ValueError("product signature domain mismatch")
    constant = f'const RequestDomain = "{DOMAIN}"'
    if constant not in authority_source:
        raise ValueError("Authority signature domain mismatch")
    struct = extract(authority_source, "type Proof struct {")
    product_go = '''package wireproof
import ("crypto/ed25519";"crypto/sha256";"encoding/binary")
type Identity struct { ID string; PrivateKey ed25519.PrivateKey }
func (i Identity) PublicKey() ed25519.PublicKey { return i.PrivateKey.Public().(ed25519.PublicKey) }
''' + client
    authority_go = '''package wireproof
import ("bytes";"crypto/sha256";"encoding/binary")
''' + constant + "\n" + struct + authority
    return product_go, authority_go


def run_differential(client_source: str, authority_source: str, tests: str) -> str:
    product, authority = extracted_go(client_source, authority_source)
    with tempfile.TemporaryDirectory(prefix="lic-a9-wire-proof-") as location:
        root = Path(location)
        (root / "go.mod").write_text("module wireproof\n\ngo 1.23\n")
        (root / "client.go").write_text(product)
        (root / "authority.go").write_text(authority)
        (root / "wireproof_test.go").write_text(tests)
        env = os.environ.copy()
        env.update({"GOTOOLCHAIN": "local", "GOPROXY": "off", "GOSUMDB": "off"})
        result = subprocess.run(["go", "test", "-count=1", "-v", "./..."], cwd=root,
                                text=True, capture_output=True, timeout=90, env=env)
        if result.returncode:
            raise RuntimeError("Go proof differential failed:\n" + result.stdout + result.stderr)
        return result.stdout


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--authority", type=Path, required=True)
    parser.add_argument("--product", type=Path, required=True)
    parser.add_argument("--product-sha", required=True)
    args = parser.parse_args(argv)
    if not re.fullmatch("[0-9a-f]{40}", args.product_sha):
        print("BLOCKED: expected product commit must be exact SHA", file=sys.stderr)
        return 3
    authority, product = args.authority.resolve(), args.product.resolve()
    try:
        if authority == product or head(authority) != AUTHORITY_SHA or head(product) != args.product_sha:
            print("BLOCKED: checkout revision mismatch", file=sys.stderr)
            return 3
        if not clean(authority) or not clean(product):
            print("BLOCKED: uncommitted/untracked source", file=sys.stderr)
            return 3
        client = (product / "internal/platform/installationlicense/client.go").read_text()
        server = (authority / "internal/m2m/proof.go").read_text()
        tests = Path(__file__).with_name("wireproof_differential_test.go.txt").read_text()
        print(run_differential(client, server, tests), end="")
    except (OSError, ValueError, subprocess.CalledProcessError, subprocess.TimeoutExpired, RuntimeError) as exc:
        print(f"NOT_PASS: {exc}", file=sys.stderr)
        return 3
    print("WIRE_PROOF_DIFFERENTIAL_PASS (only signature wire framing)")
    print("NOT_RUN: full Go1.27 build and Authority/PostgreSQL E2E")
    return 0


if __name__ == "__main__":
    sys.exit(main())
