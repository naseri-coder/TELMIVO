#!/usr/bin/env python3
"""LIC-A9 Phase51-K: offline structural gate for candidate backend tar.gz bundles.

This does not authenticate the publisher or replace signed release manifests,
the phase51j Go build-info guard, or the actual installer acceptance tests.
Never extract an unverified archive. No access to Production is required.
"""
import argparse
import gzip
import hashlib
import io
import json
import re
import tarfile
from pathlib import Path

EXPECTED = ("api", "worker", "media-worker", "main-bot", "group-bot",
            "downloader-bot", "customer-bot-runtime", "migrate")
FILES = frozenset("bin/" + name for name in EXPECTED)
MANIFEST = "release.manifest.json"
SHA256 = re.compile(r"[0-9a-f]{64}\Z")
MAX_ARCHIVE = 512 * 1024 * 1024
MAX_BINARY = 128 * 1024 * 1024
MAX_MANIFEST = 16 * 1024

class ReleasePackageRejected(ValueError):
    pass

def _reject(reason):
    raise ReleasePackageRejected(reason)

def _unique_pairs(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            _reject("duplicate manifest property")
        result[key] = value
    return result

def verify(archive: Path, externally_pinned_manifest_sha256: str):
    if not SHA256.fullmatch(externally_pinned_manifest_sha256):
        _reject("independent manifest sha256 is mandatory")
    if not archive.is_file() or archive.is_symlink():
        _reject("archive must be an ordinary file")
    if archive.stat().st_size > MAX_ARCHIVE:
        _reject("archive exceeds compressed size cap")

    found = {}
    seen = set()
    with archive.open("rb") as fh, gzip.GzipFile(fileobj=fh) as stream:
        with tarfile.open(fileobj=stream, mode="r|") as tar:
            for member in tar:
                path = member.name
                if path not in FILES | {MANIFEST} or path in seen:
                    _reject("unexpected or duplicate member: " + path[:160])
                seen.add(path)
                if member.type not in (tarfile.REGTYPE, tarfile.AREGTYPE):
                    _reject("non-regular archive member")
                expected_mode = 0o644 if path == MANIFEST else 0o755
                if member.uid != 0 or member.gid != 0 or member.mode & 0o777 != expected_mode:
                    _reject("unsafe owner or member mode")
                if member.pax_headers:
                    _reject("extended archive headers not permitted")
                if member.size <= 0 or member.size > (MAX_MANIFEST if path == MANIFEST else MAX_BINARY):
                    _reject("invalid member size")
                handle = tar.extractfile(member)
                if handle is None:
                    _reject("archive member unreadable")
                digest = hashlib.sha256()
                head = b""
                count = 0
                manifest_bytes = b""
                while True:
                    block = handle.read(65536)
                    if not block:
                        break
                    count += len(block)
                    if count > member.size:
                        _reject("unexpected content size")
                    digest.update(block)
                    if path == MANIFEST:
                        manifest_bytes += block
                    elif len(head) < 5:
                        head += block[:5 - len(head)]
                if count != member.size:
                    _reject("truncated member")
                if path != MANIFEST and head != b"\x7fELF\x02":
                    _reject("release backend file is not a 64-bit ELF binary")
                found[path] = (count, digest.hexdigest(), manifest_bytes)

    if seen != FILES | {MANIFEST}:
        _reject("missing expected backend binaries or manifest")
    if found[MANIFEST][1] != externally_pinned_manifest_sha256:
        _reject("manifest digest does not match independent pin")
    try:
        manifest = json.loads(found[MANIFEST][2], object_pairs_hook=_unique_pairs)
    except (ValueError, UnicodeDecodeError) as exc:
        _reject("invalid or duplicate manifest JSON: " + str(exc))
    if (not isinstance(manifest, dict) or set(manifest) != {"schema", "product", "go", "files"}
        or type(manifest["schema"]) is not int or manifest["schema"] != 1
        or manifest["product"] != "TELMIVO_BACKEND"
        or manifest["go"] != "go1.27.2" or not isinstance(manifest["files"], dict)
        or set(manifest["files"]) != FILES):
        _reject("manifest identity/toolchain/file set invalid")
    for path in FILES:
        value = manifest["files"][path]
        if not isinstance(value, dict) or set(value) != {"sha256", "size"}:
            _reject("invalid manifest member entry")
        if type(value["size"]) is not int or value["size"] != found[path][0]:
            _reject("member size mismatch")
        if not isinstance(value["sha256"], str) or value["sha256"] != found[path][1]:
            _reject("member hash mismatch")
    return {"status": "STRUCTURAL_CANDIDATE_ONLY", "members": len(FILES),
            "pinned_manifest_sha256": externally_pinned_manifest_sha256}

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--archive", type=Path, required=True)
    parser.add_argument("--manifest-sha256", required=True)
    args = parser.parse_args()
    try:
        print(json.dumps(verify(args.archive, args.manifest_sha256), sort_keys=True))
    except (OSError, tarfile.TarError, EOFError, ReleasePackageRejected) as exc:
        parser.exit(1, "LIC_A9_PACKAGE_REJECTED: " + str(exc) + "\n")

if __name__ == "__main__":
    main()
