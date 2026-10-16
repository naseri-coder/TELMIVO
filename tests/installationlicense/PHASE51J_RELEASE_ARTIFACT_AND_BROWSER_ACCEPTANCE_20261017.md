# LIC-A9 Phase51-J — Release artifact fixture-exclusion and browser Build acceptance

Date: 2026-10-17.
Decision: **BRANCH-ONLY ARTIFACT-GATE CANDIDATE / ISOLATED Go1.23 RACE PASS / REAL PRODUCT GO1.27.2 DOCKER + BROWSER NOT_RUN / PRODUCTION NO-GO**.

## Exact lineage and safety constraints

- Parent: [Phase51-I Product Draft PR #113](https://github.com/naseri-coder/TELMIVO/pull/113), immutable commit `3886e92ca6fadfb2260d776e59c8451f35a48193`.
- Branch: `development/lic-a9-phase51j-release-artifact-and-browser-gates-20261017` only. No Merge to `main`, no GitHub Actions invocation, Termux, Product/Authority server, real signing keys, live provider messages, real PostgreSQL or NATS calls, or Production changes.
- Product Go module remains Go1.27.x. Model isolated Linux/amd64 has **Go1.23.2 only**; no Go1.27.2, PostgreSQL, NATS or Docker runtime available, no outbound shell DNS.

## Security finding and remediation

Phase51-I introduced `lic_a9_browser_test` and `integration` compile tags that intentionally allow ephemeral Ed25519-signed TEST-only Manager/lease construction in separate test executables. Production `deployments/docker/backend.Dockerfile` previously built all 8 Product executables using `go build` without explicitly verifying **actual selected GoFiles** or checking an inherited `GOFLAGS` value and binary build-info. An unintended inclusion of the signer/fixture source would violate the Production release boundary.

**Changes in this Draft:**

1. New standard-library-only Go tool: `scripts/release/phase51j_guard.go`, exact Git blob **`6ade0adee8577c241484dc2006906c62d02b3872`**. Two modes:
   - `selected`: streams the **actual** `go list -json -deps ./cmd/...` package graph; requires canonical Product runtime and license package, requires `license_startup_default.go` to be selected, and rejects `fixture_integration.go`, `browser_test_policy.go`, `license_startup_browser_fixture.go` and other browser-test-only selected source names. Rejects malformed/missing/duplicate package graph. This checks compiler-selected source, not mere presence of files in the repository.
   - `binary <path> go1.27.2`: invokes `go version -m <path>` and rejects any explicit build tags, missing/wrong **exact** Go compiler identity, missing executable build mode or nonzero/missing CGO disabled state. Version-prefix attacks such as `go1.27.20` versus `go1.27.2` are rejected.
   - The guard is a standard Go package main, **not** excluded via `//go:build ignore`; `go test ./scripts/release` can test it normally.
2. `deployments/docker/backend.Dockerfile`, exact Git blob **`21f7a1161a258640170ad0f470f9c2e39851f8ed`**: builder copies only the release guard Go source; evaluates package graph under `GOFLAGS= GOWORK=off` with Bash pipefail before any release binary build; builds each of eight release roles under `GOFLAGS= GOWORK=off CGO_ENABLED=0`; validates **every** emitted binary against pinned `go1.27.2` build metadata; failure aborts build. Base remains `golang:1.27.2-bookworm`, runtime remains Debian non-root UID 10001. No product runtime code or test signer enters final artifact by intent.
3. `scripts/release/phase51j_guard_test.go`, exact Git blob **`90da468f5c001e9a3f747fe145c78e65b545690e`**: positive standard package graph/build-info and negative real-selected test fixture, missing default, invalid graph, version prefix, CGO and tagged-binary scenarios.
4. `scripts/release/phase51j_dockerfile_contract_test.go`, exact Git blob **`3ada25e6a2cfc949ee2dd13bdd399f09da479082`**: source contract checks actual backend Dockerfile uses both pre-build source graph and per-binary metadata guards and doesn't explicitly enable browser tag. Source tests do not execute Docker daemon.

**Existing browser acceptance:** `scripts/test/phase51i_native_browser_api.sh` remains the opt-in local-only script from Phase51-I; it is intentionally NOT wired into a GitHub Actions workflow here (Actions quota and no execution). The existing `migration-check.yml` still builds a **normal** API binary for browser specs. Those Playwright suites cannot be interpreted as having actually exercised the new signed TEST-V2 browser API until a separate authorized isolated run.

## Actually executed, bounded evidence

- Isolated Go1.23.2 Linux module containing **byte-for-byte exact four GitHub blobs above**; all SHA-1 Git blob identities checked.
- `go test -race -count=10 -timeout=90s -v ./...`: **PASS, 180/180 named test-and-subtest RUN/PASS occurrences, zero FAIL or Race Detector issue**. Full log SHA256 **`25d333a4abd3ade01ecf44ce07ce884594287cb5d4fb6a7b5bb2acaeb758ad95`**. `go vet ./...` PASS; `gofmt -l` empty.
- Independent small **STUB Product package graph and binaries**, built with Go1.23.2, exercises **real Go tooling**, not simulated tool output:
  - `go list -json -deps ./cmd/...` + actual guard: CLEAN graph **PASS**.
  - `go list -tags=lic_a9_browser_test -json -deps ./cmd/...`: correctly **REJECTED**, nonzero exit, catches selected `fixture_integration.go`.
  - actual untagged binary compiled via `CGO_ENABLED=0 go build`, `go version -m` + guard pinned to **Go1.23.2**: **PASS**.
  - actual browser-tagged binary compiled via `CGO_ENABLED=0 go build -tags=lic_a9_browser_test`: correctly **REJECTED** by build metadata; nonzero exit.
  - This independent mock is **not** the real Product cmd/api binary and is not a Docker image.
- Evidence archive `/mnt/data/LIC_A9_PHASE51J_TEST_EVIDENCE.zip`, SHA256 **`4e180f45778a901699bdc076789bf8d25710578a04f879e4485587f18403cfd5`**, includes exact source files, Dockerfile, raw go test/go vet logs, selected-graph and binary rejection logs, actual `go version -m` output and hash manifest. No credential/key/product binary distributed.

## Mandatory NO-GO release gates

1. **Real TELMIVO Go1.27.2 full module/Docker build** of 8 apps, `go version -m` actual release binaries, clean selected dependencies, real Product V2 Runtime, Phase45–51 `go test -race`: **NOT_RUN**.
2. **Real PostgreSQL16, NATS JetStream, Redis, Playwright** synthetic Browser API, API runtime/session startup and genuine signed V2 fixture: **NOT_RUN**. Existing workflow does not test special Browser fixture. Native setup and customer journeys must be run in independent authorized sandbox before release.
3. This artifact gate is only **one release path**. Additional independently maintained image builders/installer/release scripts and GitHub Actions trigger settings must be inventoried and constrained before any signed easy-install or Production artifact claim. The image source itself is not a formal cryptographic supply-chain attestation.
4. Keep all PRs Phase45–51 **Draft** and unmerged until separate security review, genuine test evidence and explicit owner approval. Authority PG16/multi-UID session fencing, UNKNOWN outbox reconciliation and rollback/recovery remain blocked.

**Verdict: `PHASE51J_RELEASE_ARTIFACT_SOURCE_GATE_STAGED / GO1.23_EXACT_SOURCE_RACE_180_PASS / MOCK_CLEAN_BINARY_PASS_TAGGED_BINARY_DENIED / REAL_GO127_DOCKER_PRODUCT_SIGNED_V2_POSTGRES_NATS_BROWSER_NOT_RUN / PRODUCTION_NO_GO`.**
