#!/usr/bin/env python3
"""No-DB structural regression for A2 journal RLS and runtime grants."""
import importlib.util
import tempfile
import unittest
from pathlib import Path

HERE = Path(__file__).resolve()
spec = importlib.util.spec_from_file_location("a2_journal_isolation_audit", HERE.with_name("a2_journal_isolation_audit.py"))
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)


class JournalAudit(unittest.TestCase):
    def fixture(self, grant, *, safe=True):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        (root / "db" / "migrations").mkdir(parents=True)
        (root / "infrastructure" / "postgres").mkdir(parents=True)
        ddl = []
        for table in mod.JOURNALS:
            ddl.append(f"CREATE TABLE {table} (id uuid, workspace_id uuid);")
            if safe:
                ddl.append(f"ALTER TABLE {table} ENABLE ROW LEVEL SECURITY;")
                ddl.append(f"ALTER TABLE {table} FORCE ROW LEVEL SECURITY;")
                ddl.append(f"CREATE POLICY tenant_scope ON {table} USING (workspace_id::text=current_setting('app.workspace_id',true));")
        (root / "db" / "migrations" / "0020_a2_fixture.sql").write_text("\n".join(ddl))
        (root / "infrastructure" / "postgres" / "grants.sql").write_text(grant)
        return root

    def test_current_checkout_is_not_ready(self):
        root = HERE.parents[2]
        report = mod.inspect(root)
        self.assertEqual(report["status"], "NO_GO_DB_ISOLATION_GAPS")
        self.assertEqual(set(report["risk"]["journal_without_rls"]), set(mod.JOURNALS))
        self.assertEqual(set(report["risk"]["journal_without_workspace_column"]),
                         {"inbox_events", "dead_letter_events"})
        self.assertTrue(report["risk"]["blanket_runtime_table_write"])

    def test_synthetic_structural_preconditions_only(self):
        root = self.fixture("GRANT SELECT ON outbox_events TO telmivo_runtime;")
        report = mod.inspect(root)
        self.assertEqual(report["status"], "STRUCTURAL_PRECONDITIONS_ONLY_REVIEW_REQUIRED")
        self.assertEqual(report["risk"]["journal_without_rls"], [])
        self.assertEqual(report["risk"]["journal_without_workspace_column"], [])

    def test_blanket_grant_still_rejected(self):
        root = self.fixture("GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO telmivo_runtime;")
        self.assertTrue(mod.inspect(root)["risk"]["blanket_runtime_table_write"])
        self.assertEqual(mod.inspect(root)["status"], "NO_GO_DB_ISOLATION_GAPS")

    def test_missing_rls_detected(self):
        root = self.fixture("GRANT SELECT ON outbox_events TO telmivo_runtime;", safe=False)
        self.assertEqual(set(mod.inspect(root)["risk"]["journal_without_rls"]), set(mod.JOURNALS))

    def test_missing_migration_fails_closed(self):
        with tempfile.TemporaryDirectory() as tmp:
            with self.assertRaises(ValueError):
                mod.inspect(Path(tmp))


if __name__ == "__main__":
    unittest.main()
