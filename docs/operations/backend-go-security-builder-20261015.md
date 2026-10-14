# TELMIVO backend Go security builder — 2026-10-15

**Mode:** source-only patch / offline regression; **not** a built or published container.

The TELMIVO backend production image builder previously used `FROM golang:1.27.1-bookworm AS build`, while the merged LIC-A9 E2E runner in PR56 was updated to `golang:1.27.2-bookworm`. Go 1.27.2 was released 2026-10-08 with security fixes in the Go toolchain, TLS, HTTP and related packages (official source: https://go.dev/doc/devel/release). Docker's official image catalog lists `golang:1.27.2-bookworm` (https://hub.docker.com/_/golang).

**Change:** only the Go build stage in `deployments/docker/backend.Dockerfile` updates to 1.27.2. The Debian runtime image, application source, binary list, install scripts, private key custody, deployment manifests, prod credentials and application Go module definition remain unchanged.

**Regression:** `tests/installationlicense/test_backend_go_builder.py` checks exact patched image vs LIC-A9 E2E image and Go language declaration, rejects downgrades (1.27.0, 1.27.1, 1.26.9), stage ambiguity and an unsupported module version. 4/4 local focused tests PASS. Full offline LIC-A9 Python suite result to be attached after execution.

**Limits:** Current isolated environment lacks Docker, network package fetch, and Go 1.27.2 executable, so no Docker image build, Go production compile, vulnerability scan or production release is asserted. **PostgreSQL 16 E2E remains NOT_RUN.** Merge destination is the nonproduction feature branch; Authority PR32 remains draft and unmerged.
