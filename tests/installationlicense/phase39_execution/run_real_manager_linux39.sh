#!/usr/bin/env bash
# LIC-A9 Phase39 — OFFLINE / disposable Linux ROOT test only.
# This is NOT a Production launch script and never enables external effects.
set -Eeuo pipefail
export GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off
fail() { printf 'LIC_A9_PHASE39_BLOCKED: %s\n' "$*" >&2; exit 78; }
[[ "${LIC_A9_DISPOSABLE:-}" == YES ]] || fail 'disposable user-owned Linux checkout consent required'
[[ "${LIC_A9_ALLOW_SIDE_EFFECTS:-}" != YES ]] || fail 'live effects prohibited'
[[ "${APP_ENV:-test}" != production && "${ENVIRONMENT:-test}" != production ]] || fail 'Production prohibited'
[[ "${GITHUB_ACTIONS:-}" != true ]] || fail 'Actions prohibited'
[[ "${LIC_A9_EXPECT_HEAD:-}" =~ ^[0-9a-f]{40}$ ]] || fail 'reviewed exact Phase39 full SHA required'
[[ $(uname -s) == Linux && $(id -u) == 0 ]] || fail 'must execute on isolated disposable Linux as UID0; Android/Termux is not an accepted test environment'
for exe in go git sha256sum mktemp grep; do command -v "$exe" >/dev/null || fail "missing $exe"; done
root="$(git rev-parse --show-toplevel 2>/dev/null)" || fail 'not a Git checkout'
[[ "$PWD" == "$root" ]] || fail 'run from checkout root'
[[ $(git symbolic-ref --quiet --short HEAD 2>/dev/null) == development/lic-a9-phase39-linux-real-manager-uid-supervisor-20261016 ]] || fail 'wrong branch'
[[ $(git rev-parse HEAD) == "$LIC_A9_EXPECT_HEAD" ]] || fail 'HEAD does not match reviewed SHA'
[[ -z $(git status --porcelain --untracked-files=all) ]] || fail 'checkout is not clean'
[[ -f go.mod && -f go.sum ]] || fail 'complete Product checkout required'
grep -Fxq 'module github.com/naseri-coder/TELMIVO' go.mod || fail 'wrong Product module'
grep -Fxq 'go 1.27.0' go.mod || fail 'do not downgrade Product module Go version'
[[ $(go env GOOS) == linux && $(go env CGO_ENABLED) == 1 ]] || fail 'native Linux CGO=1 is required for Go Race Detector'
version="$(go version)"
[[ "$version" =~ go([0-9]+)\.([0-9]+)(\.([0-9]+))?([[:space:]]|$) ]] || fail 'unrecognized Go toolchain'
(( BASH_REMATCH[1] > 1 || (BASH_REMATCH[1] == 1 && BASH_REMATCH[2] >= 27) )) || fail 'Go1.27+ required'
for p in internal/platform/installationlicense/phase39_real_product_supervisor_test.go tests/installationlicense/phase38_supervisor_durable_bridge/bridge.go tests/installationlicense/phase38_supervisor_durable_bridge/phase39_gate_test.go; do
  [[ -f "$p" ]] || fail "missing verified test code $p"
done
logs="${LIC_A9_LOG_DIR:-$HOME/telmivo-license-audit}"
[[ "$logs" == /* && ! -L "$logs" ]] || fail 'absolute nonsymlink log dir required'
mkdir -p "$logs"; chmod 0700 "$logs"
run_dir="$(mktemp -d "$logs/lic39-XXXXXX")" || fail 'cannot create log directory'
chmod 0700 "$run_dir"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/lic39-mod-XXXXXX")" || fail 'temp failed'
trap 'rm -rf -- "$tmp"' EXIT
cp go.mod "$tmp/phase39.mod"
cp go.sum "$tmp/phase39.sum"
cat >> "$tmp/phase39.mod" <<MOD
require telmivo.local/lic-a9-phase38-supervisor-durable-bridge v0.0.0
replace telmivo.local/lic-a9-phase38-supervisor-durable-bridge => $root/tests/installationlicense/phase38_supervisor_durable_bridge
MOD
printf 'HEAD=%s\nGO=%s\nSCOPE=REAL_PRODUCT_ROOT_LINUX_RACE_NO_PROVIDER\n' "$LIC_A9_EXPECT_HEAD" "$version" > "$run_dir/metadata.txt"
set +e
go test -mod=mod -modfile="$tmp/phase39.mod" -tags=lic39 -race -count=1 -timeout=180s -v -run '^TestLICA9Phase39' ./internal/platform/installationlicense 2>&1 | tee "$run_dir/real_manager39.log"
rc=${PIPESTATUS[0]}
set -e
[[ $rc -eq 0 ]] || fail "real Product Go1.27 tests FAILED with $rc; see $run_dir"
if grep -Eq '^--- SKIP: TestLICA9Phase39' "$run_dir/real_manager39.log"; then fail 'test skipped: not a release acceptance'; fi
if ! grep -Fxq 'PASS' "$run_dir/real_manager39.log"; then fail 'no actual Go PASS marker'; fi
go vet -mod=mod -modfile="$tmp/phase39.mod" -tags=lic39 ./internal/platform/installationlicense 2>&1 | tee "$run_dir/vet.log"
(cd "$run_dir" && sha256sum metadata.txt real_manager39.log vet.log > SHA256SUMS)
printf 'LIC_A9_PHASE39_REAL_PRODUCT_LINUX_RACE_FUNCTIONAL_PASS_NOT_PRODUCTION head=%s evidence=%s\n' "$LIC_A9_EXPECT_HEAD" "$run_dir"
