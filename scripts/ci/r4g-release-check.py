#!/usr/bin/env python3
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[2]

def fail(message: str) -> None:
    print(f"R4G_RELEASE_CHECK_FAIL: {message}", file=sys.stderr)
    raise SystemExit(1)

def require(path: str) -> Path:
    p = ROOT / path
    if not p.is_file():
        fail(f"missing required file: {path}")
    return p

def text(path: str) -> str:
    return require(path).read_text(encoding="utf-8")

migrations = sorted((ROOT / "db/migrations").glob("[0-9][0-9][0-9][0-9]_*.sql"))
if len(migrations) != 23:
    fail(f"expected 23 migrations, found {len(migrations)}")
for index, path in enumerate(migrations, start=1):
    prefix = f"{index:04d}_"
    if not path.name.startswith(prefix):
        fail(f"migration chain gap/order mismatch at {path.name}; expected {prefix}*")
if migrations[-1].name != "0023_ai_assistant_2.sql":
    fail(f"unexpected final migration: {migrations[-1].name}")
if list((ROOT / "db/migrations").glob("0024_*.sql")):
    fail("R4G must not introduce migration 0024 without an identified schema defect")

required_docs = [
    "docs/R4A-customer-bot-runtime.md",
    "docs/R4B-channel-manager.md",
    "docs/R4C-uploader-storage-platform.md",
    "docs/R4D-commerce-checkout-payments.md",
    "docs/R4E-automation-2.md",
    "docs/R4F-ai-assistant-2.md",
    "docs/R4G-v0.2.0-final-hardening.md",
    "docs/operations/v0.2.0-release-readiness.md",
]
for path in required_docs:
    require(path)

browser_specs = [
    "web/miniapp/e2e/channels.native.spec.ts",
    "web/miniapp/e2e/uploader.native.spec.ts",
    "web/miniapp/e2e/payments.native.spec.ts",
    "web/miniapp/e2e/automation2.native.spec.ts",
    "web/miniapp/e2e/ai2.native.spec.ts",
]
for path in browser_specs:
    require(path)

native = text(".github/workflows/migration-check.yml")
for marker in [
    "TestR4B",
    "TestR4C",
    "TestR4D",
    "TestR4E",
    "TestR4F",
    "TestR4G(MigrationChainAndTenantHardening|CrossFeatureContinuity)",
    "TestR4GRecovered",
]:
    if marker not in native:
        fail(f"native release matrix missing {marker}")
for spec in [Path(p).name for p in browser_specs]:
    if spec not in native:
        fail(f"native browser matrix missing {spec}")

security = text(".github/workflows/security.yml")
for command in [
    "python3 scripts/security/scan.py",
    "govulncheck@latest ./...",
    "npm --prefix web/miniapp audit --audit-level=high",
]:
    if command not in security:
        fail(f"security gate missing: {command}")

production = text(".github/workflows/production-readiness.yml")
for required in [
    "docker build -f deployments/docker/backend.Dockerfile",
    "docker build -f deployments/docker/miniapp.Dockerfile",
    "customer-bot-runtime",
    "entrypoint id telmivo-backend:production-readiness -u",
]:
    if required not in production:
        fail(f"production-readiness contract missing: {required}")

for workflow in [
    ".github/workflows/ci-go.yml",
    ".github/workflows/ci-web.yml",
    ".github/workflows/build-images.yml",
    ".github/workflows/security.yml",
    ".github/workflows/production-readiness.yml",
]:
    if "'feature/r4*'" not in text(workflow):
        fail(f"R4 stacked branch trigger missing from {workflow}")

readme = text("README.md")
if "Twenty-three checksummed migrations (0001–0023)" not in readme:
    fail("README migration count is not synchronized with v0.2.0")
if "R4G v0.2.0 Final Hardening" not in readme:
    fail("README does not link the R4G final-hardening boundary")

require(".github/workflows/r4-release-readiness.yml")

print("R4G_RELEASE_CHECK_PASS")
print("migrations=23")
print("browser_matrix=R4B,R4C,R4D,R4E,R4F")
print("release_state=SOURCE_READY_VALIDATION_REQUIRED")
