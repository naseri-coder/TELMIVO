#!/usr/bin/env bash
# LIC-A9 Phase35: Termux-only, no server/Actions, real Product package tests.
# GOOS=android does not support Go's race detector; DO NOT label this a race PASS.
set -Eeuo pipefail
export LC_ALL=C GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off
fail() { printf 'LIC_A9_TERMUX_BLOCKED: %s\n' "$*" >&2; exit 78; }

[[ "${LIC_A9_DISPOSABLE:-}" == YES ]] || fail 'explicit disposable checkout consent missing'
[[ -n "${PREFIX:-}" && "${PREFIX}" == */files/usr ]] || fail 'run in Termux on Android, not a Production Linux host'
[[ "${GITHUB_ACTIONS:-}" != true ]] || fail 'GitHub Actions prohibited'
[[ "${APP_ENV:-test}" != production && "${ENVIRONMENT:-test}" != production ]] || fail 'Production prohibited'
[[ "${LIC_A9_ALLOW_SIDE_EFFECTS:-}" != YES ]] || fail 'external side effects prohibited'
[[ "${LIC_A9_EXPECT_HEAD:-}" =~ ^[0-9a-f]{40}$ ]] || fail 'exact immutable reviewed 40-character SHA required'
for p in go git bash sha256sum mktemp; do command -v "$p" >/dev/null || fail "missing $p"; done
root="$(git rev-parse --show-toplevel 2>/dev/null)" || fail 'must be in a cloned Git checkout'
[[ "$(pwd -P)" == "$(cd "$root" && pwd -P)" ]] || fail 'run only from checkout root'
branch="$(git symbolic-ref --quiet --short HEAD 2>/dev/null)" || fail 'detached HEAD refused'
[[ "$branch" == development/lic-a9-phase35-offline-go127-execution-gate-20261016 ]] || fail "wrong development branch: $branch"
head="$(git rev-parse HEAD 2>/dev/null)" || fail 'cannot resolve HEAD'
[[ "$head" == "$LIC_A9_EXPECT_HEAD" ]] || fail "reviewed HEAD mismatch: $head"
[[ -z "$(git status --porcelain --untracked-files=all)" ]] || fail 'uncommitted or untracked files refused'
[[ -f go.mod && -f go.sum ]] || fail 'not full Product checkout'
grep -Fxq 'module github.com/naseri-coder/TELMIVO' go.mod || fail 'wrong Go module'
grep -Fxq 'go 1.27.0' go.mod || fail 'root Go version differs from reviewed module'
# Verify actual cryptographic implementation, old real V2 fixture and phase34 test.
while read -r path expected; do
  [[ -f "$path" ]] || fail "missing $path"
  [[ "$(git hash-object "$path")" == "$expected" ]] || fail "reviewed blob mismatch: $path"
done <<'MANIFEST'
internal/platform/installationlicense/state.go c5c81548a1b063825eccc9c3d69cc6006160d4cd
internal/platform/installationlicense/phase19_session_fencing_test.go 3fe0316182760ccf6504b7f36e7be8cd19dacd77
internal/platform/installationlicense/phase34_real_manager_effect_test.go d32c4dac201fcb78216b633830a6d70e37b7f0b5
MANIFEST
version="$(go version)" || fail 'cannot read Go toolchain version'
[[ "$version" =~ go([0-9]+)\.([0-9]+)(\.([0-9]+))?([[:space:]]|$) ]] || fail "unrecognized Go version: $version"
major="${BASH_REMATCH[1]}" minor="${BASH_REMATCH[2]}"
((major > 1 || (major == 1 && minor >= 27))) || fail "Go 1.27+ required: $version"
goos="$(go env GOOS)" || fail 'go env GOOS failed'
[[ "$goos" == android || "$goos" == linux ]] || fail "unexpected Go target: $goos"
# Termux GOOS=android: Go race detector unsupported. Never auto-fallback from
# an attempted -race run and never claim release acceptance from mobile tests.
logs="${LIC_A9_LOG_DIR:-$HOME/telmivo-license-audit}"
[[ "$logs" == /* && ! -L "$logs" ]] || fail 'log directory path must be absolute and not a symlink'
mkdir -p "$logs" || fail 'cannot create log directory'
chmod 0700 "$logs" || fail 'cannot protect log directory'
run_dir="$(mktemp -d "$logs/lic-a9-phase35-XXXXXX")" || fail 'cannot create isolated run directory'
chmod 0700 "$run_dir"
printf 'HEAD=%s\nBRANCH=%s\nVERSION=%s\nGOOS=%s\nMODE=REAL_PRODUCT_FUNCTIONAL_ONLY_NO_RACE_CLAIM\n' "$head" "$branch" "$version" "$goos" | tee "$run_dir/metadata.log"
# No network access: package import audit confirmed these files depend on Go stdlib.
# A module-cache miss is a BLOCK, not permission to enable GOPROXY or Actions.
run() {
  local label="$1"; shift
  printf '\n== %s ==\n' "$label" | tee "$run_dir/$label.log"
  "$@" 2>&1 | tee -a "$run_dir/$label.log"
}
run real_phase34_manager go test -count=1 -timeout=300s -run '^TestLICA9Phase34' ./internal/platform/installationlicense
run full_license_package go test -count=1 -timeout=420s ./internal/platform/installationlicense
run license_vet go vet ./internal/platform/installationlicense
( cd "$run_dir"; sha256sum metadata.log real_phase34_manager.log full_license_package.log license_vet.log > SHA256SUMS )
printf 'LIC_A9_TERMUX_FUNCTIONAL_PASS_NOT_RACE_NOT_PRODUCTION head=%s logs=%s\n' "$head" "$run_dir"
