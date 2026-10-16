#!/usr/bin/env bash
# LIC-A9 Phase41: offline disposable native Linux amd64 root only.
set -Eeuo pipefail
export GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off
stop() { echo "LIC_A9_PHASE41_BLOCKED: $*" >&2; exit 78; }
[[ "$(printenv LIC_A9_DISPOSABLE 2>/dev/null || true)" == YES ]] || stop 'disposable environment consent required'
[[ "$(printenv LIC_A9_ROOT_LINUX_TEST_APPROVED 2>/dev/null || true)" == YES ]] || stop 'explicit root Linux test approval required'
[[ "$(printenv LIC_A9_ALLOW_SIDE_EFFECTS 2>/dev/null || true)" != YES ]] || stop 'real effects prohibited'
[[ "$(printenv APP_ENV 2>/dev/null || true)" != production && "$(printenv ENVIRONMENT 2>/dev/null || true)" != production ]] || stop 'production prohibited'
[[ "$(printenv GITHUB_ACTIONS 2>/dev/null || true)" != true ]] || stop 'Actions prohibited'
[[ $(uname -s) == Linux && $(uname -m) == x86_64 && $(id -u) == 0 ]] || stop 'isolated Linux amd64 UID0 needed; Android is unsupported'
[[ $(go env GOOS) == linux && $(go env GOARCH) == amd64 && $(go env CGO_ENABLED) == 1 ]] || stop 'native CGO Race Detector required'
[[ $(go version) =~ go1\.(2[7-9]|[3-9][0-9])\.[0-9]+[[:space:]] ]] || stop 'Go1.27+ required'
[[ "$(git rev-parse --show-toplevel)" == "$PWD" ]] || stop 'must run in Product checkout root'
[[ $(git symbolic-ref --short HEAD) == development/lic-a9-phase41-strict-real-manager-20261016 ]] || stop 'wrong branch'
want="$(printenv LIC_A9_EXPECT_HEAD 2>/dev/null || true)"
[[ "$want" =~ ^[0-9a-f]{40}$ && "$(git rev-parse HEAD)" == "$want" ]] || stop 'unreviewed Product HEAD'
[[ -z $(git status --porcelain --untracked-files=all) ]] || stop 'git checkout not clean'
grep -Fxq 'module github.com/naseri-coder/TELMIVO' go.mod || stop 'wrong module'
grep -Fxq 'go 1.27.0' go.mod || stop 'Product Go version altered'
base=internal/platform/installationlicense
bridge=tests/installationlicense/phase38_supervisor_durable_bridge
[[ $(git hash-object "$base/phase39_real_product_supervisor_test.go") == c35ebd60f8cfe1e5596bae2b42bdd62cf8abf12c ]] || stop 'Phase39 file hash mismatch'
[[ $(git hash-object "$base/phase41_strict_product_supervisor_test.go") == 84f9af29f71d4b8298cab953f59be65827e77fbd ]] || stop 'Phase41 test hash mismatch'
[[ $(git hash-object "$bridge/bridge.go") == 3fdd151390329bf4766d61f20f2f1b0018362f55 ]] || stop 'strict bridge hash mismatch'
logs="$(mktemp -d /tmp/lic41-logs.XXXXXX)"
chmod 0700 "$logs"
tmp="$(mktemp -d /tmp/lic41-mod.XXXXXX)"
trap 'rm -rf -- "$tmp"' EXIT
cp go.mod "$tmp/check.mod"
cp go.sum "$tmp/check.sum"
printf '\nrequire telmivo.local/lic-a9-phase38-supervisor-durable-bridge v0.0.0\nreplace telmivo.local/lic-a9-phase38-supervisor-durable-bridge => %s/%s\n' "$PWD" "$bridge" >> "$tmp/check.mod"
printf 'HEAD=%s\nGO=%s\nMODE=REAL_PRODUCT_LINUX_ROOT_NO_PROVIDER\n' "$want" "$(go version)" > "$logs/metadata.txt"
( cd "$bridge" && go test -race -count=1 -timeout=240s ./... ) 2>&1 | tee "$logs/bridge_race.log"
go test -mod=mod -modfile="$tmp/check.mod" -tags=lic39,lic41 -race -count=1 -timeout=240s -v -run '^TestLICA9Phase41' "./$base" 2>&1 | tee "$logs/real_manager.log"
[[ "$(grep -c '^--- PASS: TestLICA9Phase41' "$logs/real_manager.log")" == 5 ]] || stop 'five real Product tests required'
! grep -Eq '^--- (SKIP|FAIL): TestLICA9Phase41' "$logs/real_manager.log" || stop 'real Product test SKIP/FAIL'
go test -race -count=1 -timeout=480s -v "./$base" 2>&1 | tee "$logs/full_license_race.log"
! grep -Eq '^--- SKIP: TestLICA9Phase03Hardlink' "$logs/full_license_race.log" || stop 'real Linux hardlink security SKIP'
go vet -mod=mod -modfile="$tmp/check.mod" -tags=lic39,lic41 "./$base" 2>&1 | tee "$logs/vet.log"
( cd "$logs" && sha256sum metadata.txt *.log > SHA256SUMS )
printf 'LIC_A9_PHASE41_REAL_PRODUCT_LINUX_RACE_PASS_NOT_PRODUCTION head=%s logs=%s\n' "$want" "$logs"
