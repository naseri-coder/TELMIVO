# LIC-A9 Phase 2J — Go 1.27.2 security-patched offline test runtime

**Date:** 2026-10-15

## Security reason

The disposable LIC-A9 joint runner was pinned to `golang:1.27.0-bookworm` (Go 1.27.0). The Go team released **Go 1.27.2 on 2026-10-08** with security fixes in the Go command, `crypto/tls`, `net/http`, `net/textproto`, `html/template`, and `os`, plus other bug fixes.

Official release history: https://go.dev/doc/devel/release

The official Docker Hub image `golang:1.27.2-bookworm` was independently verified available: https://hub.docker.com/_/golang/tags?name=bookworm

## Exact isolated scope

- Update only `tests/installationlicense/run_joint_docker_only_e2e.py` and its tests `test_joint_docker_only_e2e.py`.
- Use `GO_IMAGE = 'golang:1.27.2-bookworm'`.
- The runtime's `go version` gate requires **exactly Go 1.27.2** before attempting PostgreSQL.
- Regression cases now explicitly reject Go `1.26.8`, `1.27.0`, and `1.27.1` and prove **no PostgreSQL container is launched** when the Go version is wrong.
- Retain source provenance, Go test discovery/JSON evidence fences, non-root PostgreSQL, isolated networking, no published ports, and cleanup semantics.

No product application Go version, operational code, Authority source, database, secrets, production environment or GitHub Actions configuration was modified.

## Actual local validation

The four unchanged Python fixture/control modules and the two edited scripts were assembled from the **exact previously verified PR55 source** in an isolated local directory. A first test run failed because the older regex still checked Go 1.27.0: this was diagnosed and fixed. The final exact edited files were then executed with:

```sh
PYTHONDONTWRITEBYTECODE=1 python -B -m unittest discover -s . -p 'test_*.py' -v
```

**Final result: 36/36 PASS (exit 0, 1.088 sec).** The suite includes synthetic Git/Go executions and mocked Docker control paths; it does **not** run Docker, actual Go1.27.2, or PostgreSQL16.

Source blob hashes confirmed locally with `git hash-object`, then verified in GitHub:

- `run_joint_docker_only_e2e.py`: `75fa921f614b74aebec55d71ba2fb9e35ca04950`.
- `test_joint_docker_only_e2e.py`: `e8a37ed9061a6706aad796b3b3ea4eda9eb1b542`.
- SHA256 of complete local regression output: `13615eb9ee231f21e159c3ab687f8d5881e512754cdf14b1aee2b106bd95f8f3`.

**Remaining hard gate:** no independent Docker host or private checkout pairs are available in the current container. Authority PR32 must **not** be merged to main without all six genuine Go1.27.2/PostgreSQL16 E2E tests, actual JSON run/pass evidence and confirmed DB cleanup. Continue tracking in TELMIVO Issue #54.

**Verdict:** `PHASE2J_PATCHED_GO_IMAGE_TESTED_LOCALLY_36_OF_36_PASS`; `REAL_JOINT_PG16_E2E_NOT_RUN`.
