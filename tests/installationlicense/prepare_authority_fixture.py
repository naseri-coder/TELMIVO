#!/usr/bin/env python3
"""Prepare a TEST-ONLY Authority fixture from the exact Phase07 candidate source.

Usage:
    python3 prepare_authority_fixture.py AUTHORITY_REPO NEW_FIXTURE_DIR

This fixture copies the exact pinned Authority Phase07 source, not a stale
archive. The synthetic tests do NOT certify full production interoperability.
Never point its tests at a non-disposable database.
"""
import pathlib
import subprocess
import sys

sha = "bfceaef06d41aa185677a796652d17b07979058a"


def prepare(repo: pathlib.Path, dst: pathlib.Path) -> None:
    repo = repo.resolve()
    dst = dst.resolve()
    product = pathlib.Path(__file__).resolve().parents[2]
    if dst.exists() or dst.is_relative_to(repo) or dst.is_relative_to(product):
        raise SystemExit("fixture must be a new directory outside source worktrees")
    actual = subprocess.run(["git", "-C", str(repo), "rev-parse", "--verify", sha + "^{commit}"],
                            check=True, capture_output=True, text=True).stdout.strip()
    if actual != sha:
        raise SystemExit("Authority source commit not available at exact approved SHA")
    archive = subprocess.run(["git", "-C", str(repo), "archive", sha],
                             check=True, stdout=subprocess.PIPE).stdout
    dst.mkdir(mode=0o700)
    authority = dst / "authority"
    authority.mkdir()
    subprocess.run(["tar", "-x", "-C", str(authority)], input=archive, check=True)
    # The Git archive includes Authority's own go.mod and go.sum. Leaving
    # go.mod nested under <fixture>/authority creates a SECOND Go module:
    # 'go test ./authority' from the joint fixture root then cannot find it.
    # Remove only the ARCHIVED copies, never manifests from either checkout.
    authority_manifest = authority / "go.mod"
    if not authority_manifest.is_file() or not authority_manifest.read_text().startswith(
            "module github.com/naseri-coder/TELMIVO-LICENSE-AUTHORITY\n"):
        raise SystemExit("unexpected Authority module in disposable fixture")
    authority_manifest.unlink()
    (authority / "go.sum").unlink(missing_ok=True)
    old = "github.com/naseri-coder/TELMIVO-LICENSE-AUTHORITY"
    new = "github.com/naseri-coder/TELMIVO/lica8-e2e/authority"
    for p in authority.rglob("*.go"):
        source = p.read_text().replace(old, new)
        if (p.parent == authority / "internal/store/postgres"
                and p.name != "transport.go" and not p.name.endswith("_test.go")):
            source = source.replace("clock_timestamp()", "lic_a8_clock()")
        p.write_text(source)
    module = (product / "go.mod").read_text().replace(
        "module github.com/naseri-coder/TELMIVO",
        "module github.com/naseri-coder/TELMIVO/lica8-e2e", 1)
    module += ("\nrequire github.com/naseri-coder/TELMIVO v0.0.0\n"
               "replace github.com/naseri-coder/TELMIVO => " + str(product) + "\n")
    (dst / "go.mod").write_text(module)
    (dst / "go.sum").write_bytes((product / "go.sum").read_bytes())
    (authority / "client_test.go").write_bytes(
        (pathlib.Path(__file__).parent / "authority_e2e_test.go.template").read_bytes())
    print("TEST-ONLY exact Phase07 Authority fixture prepared from " + sha)


if __name__ == "__main__":
    if len(sys.argv) != 3:
        raise SystemExit("usage: prepare_authority_fixture.py AUTHORITY_REPO NEW_FIXTURE_DIR")
    prepare(pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2]))
