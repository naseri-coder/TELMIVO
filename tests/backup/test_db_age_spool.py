"""Offline-only synthetic executable substitutes; does not qualify real age crypto/PG restore."""
import importlib.util
import json
import os
from pathlib import Path
import stat
import tempfile
import unittest

P = Path(__file__).resolve().parents[2] / "scripts" / "backup" / "db_age_spool.py"
spec = importlib.util.spec_from_file_location("db_age_spool", P)
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)
PUBLIC = "age1" + "q" * 58

class BackupSpoolTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.spool = self.root / "spool"
        self.dummy = self.root / "dummy.dump"
        self.dummy.write_bytes(b"PGDMP-SYNTHETIC-ONLY" * 100)
        self.age = self.root / "fake-age"
        self.age.write_text("#!/usr/bin/env python3\nimport sys\n"
                            "sys.stdout.buffer.write(b'AGE-MOCK-NOT-ENCRYPTION:' + sys.stdin.buffer.read())\n")
        self.age.chmod(0o700)
        self.dump = self.root / "fake-pg-dump"
        self.dump.write_text("#!/usr/bin/env python3\nimport sys\n"
                             "sys.stdout.buffer.write(b'PGDMP-SYNTHETIC-ONLY')\n")
        self.dump.chmod(0o700)

    def run_synthetic(self):
        return mod.capture(project="telmivo-main", spool=self.spool, recipient=PUBLIC,
                           source="synthetic", synthetic_input=self.dummy, age_binary=str(self.age))

    def test_capture_manifest_and_verify(self):
        res = self.run_synthetic()
        self.assertEqual(res["status"], "LOCAL_ONLY_UNQUALIFIED")
        data = json.loads((Path(res["path"]) / "manifest.json").read_text())
        self.assertFalse(data["remote_uploaded"])
        self.assertFalse(data["external_readback_verified"])
        self.assertFalse(data["semantic_restore_qualified"])
        self.assertFalse(data["recovery_pin_committed"])
        self.assertTrue(mod.inspect(self.spool)[0]["local_hash_valid"])

    def test_ciphertext_corruption_detected(self):
        res = self.run_synthetic()
        f = Path(res["path"]) / "database.dump.age"
        f.write_bytes(f.read_bytes() + b"tamper")
        self.assertFalse(mod.inspect(self.spool)[0]["local_hash_valid"])

    def test_missing_ciphertext_quarantined(self):
        res = self.run_synthetic()
        (Path(res["path"]) / "database.dump.age").unlink()
        self.assertEqual(mod.inspect(self.spool)[0]["error"], "LOCAL_CORRUPTION_OR_INVALID_MANIFEST")

    def test_distinct_captures_never_overwrite(self):
        a = self.run_synthetic()
        b = self.run_synthetic()
        self.assertNotEqual(a["point_id"], b["point_id"])
        self.assertEqual(len(mod.inspect(self.spool)), 2)

    def test_spool_mode_private(self):
        r = self.run_synthetic()
        self.assertEqual(stat.S_IMODE(self.spool.stat().st_mode), 0o700)
        self.assertEqual(stat.S_IMODE((Path(r["path"]) / "manifest.json").stat().st_mode), 0o600)

    def test_reject_group_readable_existing_spool(self):
        self.spool.mkdir(mode=0o700)
        self.spool.chmod(0o755)
        with self.assertRaises(mod.BackupError): self.run_synthetic()

    def test_reject_spool_symlink(self):
        target = self.root / "target"
        target.mkdir()
        self.spool.symlink_to(target)
        with self.assertRaises(mod.BackupError): self.run_synthetic()

    def test_reject_invalid_recipient(self):
        with self.assertRaises(mod.BackupError):
            mod.capture(project="telmivo-main", spool=self.spool, recipient="bad", source="synthetic",
                        synthetic_input=self.dummy, age_binary=str(self.age))

    def test_reject_license_live(self):
        with self.assertRaises(mod.BackupError):
            mod.capture(project="telmivo-license", spool=self.spool, recipient=PUBLIC,
                        source="postgres", database="telmivo_license", age_binary=str(self.age))

    def test_reject_wrong_main_database(self):
        with self.assertRaises(mod.BackupError):
            mod.capture(project="telmivo-main", spool=self.spool, recipient=PUBLIC,
                        source="postgres", database="postgres", age_binary=str(self.age))

    def test_reject_postgres_password_environment(self):
        with self.assertRaises(mod.BackupError):
            mod.capture(project="telmivo-main", spool=self.spool, recipient=PUBLIC,
                        source="postgres", database="telmivo", age_binary=str(self.age),
                        pg_dump_binary=str(self.dump), acknowledge_live=True, env={"PGPASSWORD":"secret", "PGPASSFILE":"/dev/null",
                                                               "PGUSER":"postgres", "PGHOST":"localhost"})

    def test_reject_missing_pgpassfile(self):
        with self.assertRaises(mod.BackupError):
            mod.capture(project="telmivo-main", spool=self.spool, recipient=PUBLIC,
                        source="postgres", database="telmivo", age_binary=str(self.age),
                        pg_dump_binary=str(self.dump), acknowledge_live=True, env={"PGUSER":"postgres","PGHOST":"localhost"})

    def test_reject_mixed_source(self):
        with self.assertRaises(mod.BackupError):
            mod.capture(project="telmivo-main", spool=self.spool, recipient=PUBLIC,
                        source="postgres", database="telmivo", synthetic_input=self.dummy)

    def test_reject_missing_synthetic(self):
        with self.assertRaises(mod.BackupError):
            mod.capture(project="telmivo-main", spool=self.spool, recipient=PUBLIC,
                        source="synthetic", synthetic_input=self.root / "nope", age_binary=str(self.age))

    def test_reject_synthetic_symlink(self):
        self.dummy.unlink()
        other = self.root / "other.dump"
        other.write_bytes(b"PGDMP")
        self.dummy.symlink_to(other)
        with self.assertRaises(mod.BackupError): self.run_synthetic()

    def test_failure_keeps_nonqualified_partial_not_manifest(self):
        bad = self.root / "fail-age"
        bad.write_text("#!/bin/sh\nexit 19\n")
        bad.chmod(0o700)
        with self.assertRaises(mod.BackupError):
            mod.capture(project="telmivo-main", spool=self.spool, recipient=PUBLIC,
                        source="synthetic", synthetic_input=self.dummy, age_binary=str(bad))
        self.assertEqual(mod.inspect(self.spool), [])
        self.assertEqual(len(list(self.spool.rglob("manifest.json"))), 0)

    def test_fake_pg_pipe_explicit_read_only_capture(self):
        pgpass = self.root / "pgpass"
        pgpass.write_text("test-only")
        res = mod.capture(project="telmivo-main", spool=self.spool, recipient=PUBLIC,
                          source="postgres", database="telmivo", pg_dump_binary=str(self.dump),
                          age_binary=str(self.age), acknowledge_live=True, env={"PGPASSFILE":str(pgpass),
                                                  "PGHOST":"test", "PGUSER":"test", "PGDATABASE":"telmivo",
                                                  "PATH":os.getenv("PATH", "")})
        self.assertTrue(mod.inspect(self.spool)[0]["local_hash_valid"])
        self.assertGreater(res["size_bytes"], 0)

    def test_explicit_authorization_required_for_live_read(self):
        with self.assertRaisesRegex(mod.BackupError, "separately authorized"):
            mod.capture(project="telmivo-main", spool=self.spool, recipient=PUBLIC,
                        source="postgres", database="telmivo", age_binary=str(self.age),
                        pg_dump_binary=str(self.dump), env={"PGUSER":"test","PGHOST":"localhost"})

    def test_no_live_upload_or_delete_logic(self):
        source = P.read_text()
        for keyword in ("requests.post(", "curl ", "boto3.", "unlink()"):
            self.assertNotIn(keyword, source)

if __name__ == "__main__":
    unittest.main()
