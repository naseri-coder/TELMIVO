#!/usr/bin/env python3
"""Offline HTTPS wire test from exact clean TELMIVO and Authority checkouts.

Only copies selected Go declarations into a disposable local standard-library
module; does not run either application, database, live Telegram or production.
"""
import argparse
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile

AUTHORITY_SHA = "68cdb6996f85981c36dd20ecb7435ec010351e9b"
DOMAIN = r'TELMIVO_INSTALLATION_REQUEST_V1\x00'


def git(root: Path, *args: str) -> str:
    return subprocess.run(["git", "-C", str(root), *args], check=True,
                          capture_output=True, text=True).stdout.strip()


def extract(source: str, prefix: str) -> str:
    positions = list(re.finditer(r"(?m)^" + re.escape(prefix), source))
    if len(positions) != 1:
        raise ValueError("missing or ambiguous source declaration: " + prefix)
    start = positions[0].start()
    end = source.find("\n}", start)
    if end < start:
        raise ValueError("unclosed Go declaration: " + prefix)
    return source[start:end + 2] + "\n"


def files_from_source(client: str, authority: str, strict: str) -> dict[str, str]:
    if DOMAIN not in client or f'const RequestDomain = "{DOMAIN}"' not in authority:
        raise ValueError("request signing domain mismatch")
    stub = '''package wire
import("bytes";"context";"crypto/ed25519";"crypto/rand";"crypto/sha256";"encoding/base64";"encoding/binary";"errors";"io";"net/http";"strconv";"time")
type Manager struct{}
type Identity struct{ID string;PrivateKey ed25519.PrivateKey}
func (i Identity) PublicKey() ed25519.PublicKey{return i.PrivateKey.Public().(ed25519.PublicKey)}
type Operation struct{Kind,Key string;Body []byte}
var ErrUnavailable=errors.New("LICENSE_AUTHORITY_UNAVAILABLE")
var ErrState=errors.New("INSTALLATION_LICENSE_STATE_INVALID")
'''
    product = stub + "\n".join(extract(client, x) for x in (
        "type Client struct {", "type authorityStatus struct {", "type reply struct {",
        "func proof(", "func (cl *Client) request("))
    auth = '''package wire
import("bytes";"crypto/ed25519";"crypto/sha256";"encoding/base64";"encoding/binary";"net/http";"strconv";"time")
''' + f'const RequestDomain = "{DOMAIN}"\n' + extract(authority, "type Proof struct {") + \
        extract(authority, "func SigningInput(") + extract(authority, "func VerifyProof(")
    json_codec = 'package wire\nimport("bytes";"encoding/json";"io")\n' + extract(strict, "func StrictJSON(")
    return {"client.go": product, "authority.go": auth, "strict.go": json_codec}


def run(authority_root: Path, product_root: Path, product_sha: str) -> str:
    ar, pr = authority_root.resolve(), product_root.resolve()
    if ar == pr:
        raise ValueError("Authority and TELMIVO must have separate worktrees")
    if git(ar, "rev-parse", "HEAD") != AUTHORITY_SHA or git(pr, "rev-parse", "HEAD") != product_sha:
        raise ValueError("checked-out commit does not match selected SHA")
    if git(ar, "status", "--porcelain", "--untracked-files=all") or \
            git(pr, "status", "--porcelain", "--untracked-files=all"):
        raise ValueError("source checkout must be clean, including untracked files")
    src = files_from_source(
        (pr / "internal/platform/installationlicense/client.go").read_text(),
        (ar / "internal/m2m/proof.go").read_text(),
        (pr / "internal/platform/installationlicense/json.go").read_text())
    src["wire_test.go"] = Path(__file__).with_name("https_wire_test.go.txt").read_text()
    with tempfile.TemporaryDirectory(prefix="lic-a9-https-wire-") as folder:
        root = Path(folder)
        (root / "go.mod").write_text("module lic-a9-https-wire\n\ngo 1.23\n")
        for name, body in src.items():
            (root / name).write_text(body)
        env = dict(os.environ, GOTOOLCHAIN="local", GOPROXY="off",
                   GOSUMDB="off", GOWORK="off")
        process = subprocess.run(["go", "test", "-count=1", "-v", "./..."], cwd=root,
                                 capture_output=True, text=True, timeout=90, env=env)
        if process.returncode:
            raise RuntimeError("Go HTTPS wire test failed\n" + process.stdout + process.stderr)
        return process.stdout


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--authority", type=Path, required=True)
    parser.add_argument("--product", type=Path, required=True)
    parser.add_argument("--product-sha", required=True)
    args = parser.parse_args()
    if not re.fullmatch(r"[0-9a-f]{40}", args.product_sha):
        print("BLOCKED: product SHA must be exact lowercase 40-hex", file=sys.stderr)
        return 3
    try:
        print(run(args.authority, args.product, args.product_sha), end="")
    except (ValueError, OSError, subprocess.CalledProcessError,
            subprocess.TimeoutExpired, RuntimeError) as exc:
        print("NOT_PASS: " + str(exc), file=sys.stderr)
        return 3
    print("HTTPS_WIRE_INTEROPERABILITY_PASS -- test-only, no business database")
    print("NOT_RUN: full Go 1.27 application or PostgreSQL activation E2E")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
