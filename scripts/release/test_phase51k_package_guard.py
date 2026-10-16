#!/usr/bin/env python3
"""No-Docker, no-network tests for the LIC-A9 proposed bundle guard."""
import hashlib
import importlib.util
import io
import json
import tarfile
import tempfile
import unittest
from pathlib import Path

SOURCE = Path(__file__).resolve().with_name("phase51k_package_guard.py")
spec = importlib.util.spec_from_file_location("telmivo_phase51k_package_guard", SOURCE)
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)


class Phase51KPackageGate(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.archive = Path(self.tmp.name) / "candidate.tar.gz"

    def candidate(self, mutate=None, manifest_change=None, missing=None, extra=None):
        binaries = {"bin/" + name: b"\x7fELF\x02" + name.encode() * 3 for name in gate.EXPECTED}
        listed = {
            name: {"sha256": hashlib.sha256(data).hexdigest(), "size": len(data)}
            for name, data in binaries.items()
        }
        doc = {"schema": 1, "product": "TELMIVO_BACKEND", "go": "go1.27.2", "files": listed}
        if manifest_change:
            manifest_change(doc)
        encoded = json.dumps(doc, separators=(",", ":"), sort_keys=True).encode()
        pin = hashlib.sha256(encoded).hexdigest()
        items = [(name, data) for name, data in binaries.items() if name != missing]
        items.append((gate.MANIFEST, encoded))
        if extra:
            items.append(extra)
        with tarfile.open(self.archive, "w:gz", format=tarfile.GNU_FORMAT) as tar:
            for name, data in items:
                member = tarfile.TarInfo(name)
                member.size = len(data)
                member.mode = 0o644 if name == gate.MANIFEST else 0o755
                member.uid = member.gid = member.mtime = 0
                if mutate and name == mutate[0]:
                    mutate[1](member)
                tar.addfile(member, io.BytesIO(data))
        return pin

    def reject(self, pin):
        with self.assertRaises(gate.ReleasePackageRejected):
            gate.verify(self.archive, pin)

    def test_canonical_candidate(self):
        pin = self.candidate()
        result = gate.verify(self.archive, pin)
        self.assertEqual(result["status"], "STRUCTURAL_CANDIDATE_ONLY")
        self.assertEqual(result["members"], 8)

    def test_missing_independent_manifest_pin(self):
        self.candidate()
        self.reject("")

    def test_wrong_independent_manifest_pin(self):
        self.candidate()
        self.reject("0" * 64)

    def test_missing_binary(self):
        self.reject(self.candidate(missing="bin/worker"))

    def test_unlisted_file(self):
        self.reject(self.candidate(extra=("keys/software_release_private.key", b"not-a-key")))

    def test_path_traversal(self):
        self.reject(self.candidate(extra=("../api", b"\x7fELF\x02secret")))

    def test_duplicate_archive_entry(self):
        self.reject(self.candidate(extra=("bin/api", b"\x7fELF\x02same")))

    def test_symlink(self):
        def make_symlink(item):
            item.type = tarfile.SYMTYPE
            item.linkname = "../../etc/passwd"
            item.size = 0
        # tarfile writes symlink without data, so package reader must reject it.
        self.reject(self.candidate(mutate=("bin/api", make_symlink)))

    def test_world_writable_binary(self):
        self.reject(self.candidate(mutate=("bin/api", lambda i: setattr(i, "mode", 0o777))))

    def test_untrusted_file_owner(self):
        self.reject(self.candidate(mutate=("bin/api", lambda i: setattr(i, "uid", 1000))))

    def test_manifest_wrong_go_version(self):
        self.reject(self.candidate(manifest_change=lambda d: d.__setitem__("go", "go1.27.3")))

    def test_manifest_wrong_file_hash(self):
        def bad_hash(d):
            d["files"]["bin/api"]["sha256"] = "f" * 64
        self.reject(self.candidate(manifest_change=bad_hash))

    def test_manifest_wrong_member_size(self):
        def bad_size(d):
            d["files"]["bin/worker"]["size"] += 1
        self.reject(self.candidate(manifest_change=bad_size))

    def test_manifest_additional_field(self):
        self.reject(self.candidate(manifest_change=lambda d: d.__setitem__("signed", True)))

    def test_truncated_archive(self):
        pin = self.candidate()
        data = self.archive.read_bytes()
        self.archive.write_bytes(data[:len(data)//2])
        with self.assertRaises((gate.ReleasePackageRejected, OSError, EOFError, tarfile.TarError)):
            gate.verify(self.archive, pin)


if __name__ == "__main__":
    unittest.main()
