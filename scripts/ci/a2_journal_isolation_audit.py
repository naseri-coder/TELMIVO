#!/usr/bin/env python3
"""LIC-A9 A2: read-only source gate for journal tenant/worker DB boundaries.

This gate intentionally reports outstanding schema gaps and NEVER makes a
database or network connection. An RLS token in SQL is not a security proof:
passing these structural prerequisites still requires real PostgreSQL tests.
"""
import argparse
import json
import re
from pathlib import Path

JOURNALS = ("outbox_events", "inbox_events", "dead_letter_events")


def inspect(root: Path):
    migrations = root / "db" / "migrations"
    grants = root / "infrastructure" / "postgres" / "grants.sql"
    sqlfiles = sorted(migrations.glob("[0-9][0-9][0-9][0-9]_*.sql"))
    if not sqlfiles or not grants.is_file():
        raise ValueError("required migration and grants sources are missing")
    sql = "\n".join(f.read_text(encoding="utf-8") for f in sqlfiles)
    privileges = grants.read_text(encoding="utf-8")
    observations = {}
    for table in JOURNALS:
        name = re.escape(table)
        create = re.search(r"\bCREATE\s+TABLE\s+" + name + r"\s*\((.*?)\);", sql, re.I | re.S)
        enabled = re.search(r"\bALTER\s+TABLE\s+" + name + r"\s+ENABLE\s+ROW\s+LEVEL\s+SECURITY\b", sql, re.I)
        forced = re.search(r"\bALTER\s+TABLE\s+" + name + r"\s+FORCE\s+ROW\s+LEVEL\s+SECURITY\b", sql, re.I)
        policy = re.search(r"\bCREATE\s+POLICY\s+\w+\s+ON\s+" + name + r"\b", sql, re.I)
        observations[table] = {
            "present": create is not None,
            "workspace_column": bool(create and re.search(r"\bworkspace_id\s+uuid\b", create.group(1), re.I)),
            "rls_enabled": enabled is not None,
            "rls_forced": forced is not None,
            "explicit_policy": policy is not None,
        }
    broad_grant = re.search(
        r"\bGRANT\s+SELECT\s*,\s*INSERT\s*,\s*UPDATE\s*,\s*DELETE\s+"
        r"ON\s+ALL\s+TABLES\s+IN\s+SCHEMA\s+public\s+TO\s+telmivo_runtime\b",
        privileges, re.I,
    )
    risk = {
        "blanket_runtime_table_write": broad_grant is not None,
        "journal_without_rls": [t for t, data in observations.items()
                                if not (data["rls_enabled"] and data["rls_forced"] and data["explicit_policy"])],
        "journal_without_workspace_column": [t for t, data in observations.items()
                                             if not data["workspace_column"]],
    }
    gap = bool(risk["blanket_runtime_table_write"]
               or risk["journal_without_rls"]
               or risk["journal_without_workspace_column"])
    return {
        "status": "NO_GO_DB_ISOLATION_GAPS" if gap else "STRUCTURAL_PRECONDITIONS_ONLY_REVIEW_REQUIRED",
        "checked_migrations": len(sqlfiles),
        "journals": observations,
        "risk": risk,
        "limitations": "Source inspection only; does not execute RLS, attest worker identity, or certify release",
    }


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[2])
    args = parser.parse_args()
    result = inspect(args.root)
    print(json.dumps(result, indent=2, sort_keys=True))
    raise SystemExit(2 if result["status"] == "NO_GO_DB_ISOLATION_GAPS" else 0)


if __name__ == "__main__":
    main()
