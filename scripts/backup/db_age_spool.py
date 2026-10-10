#!/usr/bin/env python3
"""Explicit-invocation PostgreSQL -> age ciphertext spool. NO timer/uploader/deletion.

The non-production suite exercises plumbing using synthetic streams. Real PostgreSQL
and age cryptographic/restore qualifications are separate, NOT established here.
"""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import uuid

PROJECTS = {"telmivo-main": "telmivo", "telmivo-license": None}
FORMAT_VERSION = "TELMIVO_ENCRYPTED_DB_SPOOL_V1"

class BackupError(Exception):
    pass


def _require_spool(root: Path) -> Path:
    if root.is_symlink() or (root.exists() and not root.is_dir()):
        raise BackupError("unsafe spool directory")
    root.mkdir(mode=0o700, parents=True, exist_ok=True)
    if root.is_symlink():
        raise BackupError("spool path changed to symlink")
    if root.stat().st_mode & 0o077:
        raise BackupError("spool directory must not grant group/other access (chmod 700)")
    return root.resolve(strict=True)


def _valid_recipient(value: str) -> bool:
    return bool(re.fullmatch(r"age1[0-9a-z]{35,100}", value))


def _run_capture(out_handle, source, synthetic_input, database, age_recipient,
                 pg_dump_binary, age_binary, env):
    if not _valid_recipient(age_recipient):
        raise BackupError("invalid public age recipient format")
    if source == "synthetic":
        if synthetic_input is None or not synthetic_input.is_file() or synthetic_input.is_symlink():
            raise BackupError("synthetic source must be an existing regular file")
        input_handle = synthetic_input.open("rb")
        dump_process = None
    else:
        if "PGPASSWORD" in env:
            raise BackupError("use PGPASSFILE; PGPASSWORD environment variable is forbidden")
        if not env.get("PGPASSFILE"):
            raise BackupError("PGPASSFILE is required for a live PostgreSQL read")
        if not Path(env["PGPASSFILE"]).is_file():
            raise BackupError("PGPASSFILE does not exist")
        if database != env.get("PGDATABASE", database):
            raise BackupError("PGDATABASE mismatch")
        if not env.get("PGUSER") or not env.get("PGHOST"):
            raise BackupError("PGUSER and PGHOST are required")
        args = [pg_dump_binary, "--no-password", "--format=custom",
                "--no-owner", "--no-acl", "--dbname=" + database]
        dump_process = subprocess.Popen(args, stdout=subprocess.PIPE,
                                        stderr=subprocess.DEVNULL, env=env)
        input_handle = dump_process.stdout
    try:
        encrypted = subprocess.Popen([age_binary, "-r", age_recipient],
                                     stdin=input_handle, stdout=out_handle,
                                     stderr=subprocess.DEVNULL, env=env)
        if dump_process is not None:
            input_handle.close()
        rc_age = encrypted.wait()
        rc_dump = dump_process.wait() if dump_process is not None else 0
        if rc_age != 0 or rc_dump != 0:
            raise BackupError("capture/encryption subprocess failed (pg_dump/age)")
    finally:
        if dump_process is None:
            input_handle.close()
        elif dump_process.poll() is None:
            dump_process.kill()
            dump_process.wait()


def _sync_directory(directory: Path):
    fd = os.open(str(directory), os.O_RDONLY | getattr(os, "O_DIRECTORY", 0))
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


def capture(*, project: str, spool: Path, recipient: str, source: str,
            synthetic_input=None, database=None, pg_dump_binary="pg_dump",
            age_binary="age", env=None, now=None, acknowledge_live=False):
    if project not in PROJECTS:
        raise BackupError("unknown project")
    if source not in ("synthetic", "postgres"):
        raise BackupError("unknown source")
    if source == "postgres" and not acknowledge_live:
        raise BackupError("live database read requires separately authorized --acknowledge-live-read")
    if project == "telmivo-license" and source == "postgres":
        raise BackupError("license Production database/cadence not yet provisioned or qualified")
    if source == "postgres" and database != PROJECTS[project]:
        raise BackupError("live capture is restricted to confirmed main TELMIVO database")
    if source == "postgres" and synthetic_input is not None:
        raise BackupError("mixed source arguments")
    if source == "synthetic" and database is not None:
        raise BackupError("synthetic input must not specify database")
    if env is None:
        env = os.environ.copy()
    when = now or datetime.now(timezone.utc)
    if when.tzinfo is None:
        raise BackupError("timestamp must be timezone aware")
    when = when.astimezone(timezone.utc)
    os.umask(0o077)
    root = _require_spool(spool)
    point_id = when.strftime("%Y%m%dT%H%M%SZ") + "-" + uuid.uuid4().hex[:12]
    point = root / project / point_id
    point.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    if point.parent.is_symlink():
        raise BackupError("project spool must not be a symlink")
    point.mkdir(mode=0o700)
    staging = point / "backup.bundle.tar.age.pending"
    encrypted_path = point / "database.dump.age"
    manifest_path = point / "manifest.json"
    try:
        fd = os.open(staging, os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, "O_NOFOLLOW", 0), 0o600)
        with os.fdopen(fd, "wb") as out:
            _run_capture(out, source, synthetic_input, database, recipient,
                         pg_dump_binary, age_binary, env)
            out.flush()
            os.fsync(out.fileno())
        digest = hashlib.sha256()
        size = 0
        with staging.open("rb") as encrypted:
            for chunk in iter(lambda: encrypted.read(1024 * 1024), b""):
                digest.update(chunk)
                size += len(chunk)
        if size <= 0:
            raise BackupError("empty ciphertext")
        os.replace(staging, encrypted_path)
        data = {"format": FORMAT_VERSION, "project": project,
                "point_id": point_id, "created_at_utc": when.isoformat(),
                "source": source, "database": database if source == "postgres" else "SYNTHETIC_ONLY",
                "cipher": "age", "encrypted_file": encrypted_path.name,
                "encrypted_length": size, "sha256": digest.hexdigest(),
                "remote_uploaded": False, "external_readback_verified": False,
                "semantic_restore_qualified": False, "recovery_pin_committed": False}
        mtemp = point / "manifest.json.pending"
        mfd = os.open(mtemp, os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, "O_NOFOLLOW", 0), 0o600)
        with os.fdopen(mfd, "w", encoding="utf-8") as out:
            json.dump(data, out, sort_keys=True, separators=(",", ":"))
            out.write("\n")
            out.flush()
            os.fsync(out.fileno())
        os.replace(mtemp, manifest_path)
        _sync_directory(point)
        _sync_directory(point.parent)
        return {"project": project, "point_id": point_id, "size_bytes": size,
                "ciphertext_sha256": digest.hexdigest(), "path": str(point),
                "status": "LOCAL_ONLY_UNQUALIFIED"}
    except BaseException:
        raise


def inspect(spool: Path):
    """Read-only local inventory. No deletion or cloud connection."""
    root = spool.resolve(strict=True)
    results = []
    for m in sorted(root.glob("*/20*/manifest.json")):
        if m.is_symlink():
            continue
        try:
            data = json.loads(m.read_text(encoding="utf-8"))
            if data.get("format") != FORMAT_VERSION:
                continue
            f = m.parent / data["encrypted_file"]
            if f.is_symlink() or not f.is_file():
                raise BackupError("ciphertext missing or symlinked")
            h = hashlib.sha256()
            with f.open("rb") as inp:
                for chunk in iter(lambda: inp.read(1024 * 1024), b""):
                    h.update(chunk)
            valid = h.hexdigest() == data["sha256"] and f.stat().st_size == data["encrypted_length"]
            results.append({"point": data["point_id"], "project": data["project"],
                            "bytes": data["encrypted_length"], "local_hash_valid": valid,
                            "recovery_qualified": False})
        except (ValueError, KeyError, OSError, BackupError):
            results.append({"point": m.parent.name, "error": "LOCAL_CORRUPTION_OR_INVALID_MANIFEST",
                            "recovery_qualified": False})
    return results


def main(argv=None):
    ap = argparse.ArgumentParser(description="TELMIVO non-production encrypted backup spool; no deploy/timers/upload")
    sub = ap.add_subparsers(dest="action", required=True)
    make = sub.add_parser("capture")
    make.add_argument("--project", choices=PROJECTS, required=True)
    make.add_argument("--spool", type=Path, required=True)
    make.add_argument("--recipient", required=True, help="PUBLIC age recipient only")
    make.add_argument("--source", choices=["synthetic", "postgres"], required=True)
    make.add_argument("--synthetic-input", type=Path)
    make.add_argument("--database")
    make.add_argument("--acknowledge-live-read", action="store_true",
                      help="DANGEROUS: requires separate explicit authorization to read a live database")
    make.add_argument("--pg-dump-binary", default="pg_dump")
    make.add_argument("--age-binary", default="age")
    inv = sub.add_parser("inspect")
    inv.add_argument("--spool", type=Path, required=True)
    args = ap.parse_args(argv)
    try:
        if args.action == "inspect":
            print(json.dumps(inspect(args.spool), sort_keys=True))
        else:
            res = capture(project=args.project, spool=args.spool, recipient=args.recipient,
                          source=args.source, synthetic_input=args.synthetic_input,
                          database=args.database, age_binary=args.age_binary,
                          pg_dump_binary=args.pg_dump_binary,
                          acknowledge_live=args.acknowledge_live_read)
            print(json.dumps(res, sort_keys=True))
    except (OSError, BackupError, subprocess.SubprocessError) as exc:
        print("BACKUP_BLOCKED: " + str(exc), file=sys.stderr)
        return 2
    return 0

if __name__ == "__main__":
    sys.exit(main())
