#!/usr/bin/env bash
# LIC-A9 Phase35 - nonproduction, offline, fail-closed real Product test runner.
# Tests are NOT executed merely by committing this script to GitHub.
set -Eeuo pipefail
export LC_ALL=C GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off
fail() { printf 'LIC_A9_PHASE35_BLOCKED: %s\n' "$*" >&2; exit 78; }

[[ "${LIC_A9_DISPOSABLE:-}" == YES ]] || fail 'explicit disposable test-environment approval required'
[[ "${GITHUB_ACTIONS:-}" != true ]] || fail 'GitHub Actions forbidden by project policy'
[[ "${APP_ENV:-}" != production && "${ENVIRONMENT:-}" != production ]] || fail 'production environment denied'
[[ "${LIC_A9_ALLOW_SIDE_EFFECTS:-}" != YES ]] || fail 'external side effects may not be enabled'
[[ -n "${LIC_A9_EXPECT_HEAD:-}" && "${LIC_A9_EXPECT_HEAD}" =~ ^[0-9a-f]{40}$ ]] || fail 'immutable expected Product commit SHA required'
command -v git >/dev/null 2>&1 || fail 'git not present'
command -v go >/dev/null 2>&1 || fail 'Go not present'
command -v sha256sum >/dev/null 2>&1 || fail 'sha256sum not present'

root="$(git rev-parse --show-toplevel 2>/dev/null)" || fail 'not a Git checkout'
[[ "$PWD" == "$root" ]] || fail 'run only from Product checkout root'
branch="$(git symbolic-ref --quiet --short HEAD 2>/dev/null)" || fail 'detached HEAD denied (use reviewed phase35 branch)'
[[ "$branch" == development/lic-a9-phase35-* ]] || fail 'wrong branch (never run main/production)'
head="$(git rev-parse HEAD 2>/dev/null)" || fail 'cannot identify immutable Product HEAD'
[[ "$head" == "$LIC_A9_EXPECT_HEAD" ]] || fail 'HEAD differs from reviewed exact SHA'
[[ -z "$(git status --porcelain --untracked-files=all)" ]] || fail 'dirty checkout refused'
[[ -f go.mod && -f go.sum ]] || fail 'not a complete Product checkout'
grep -Eq '^module github\.com/naseri-coder/TELMIVO$' go.mod || fail 'wrong Product module'
grep -Eq '^go 1\.27\.0$' go.mod || fail 'expected module Go 1.27.0 not present'
[[ -f internal/platform/installationlicense/phase34_real_manager_effect_test.go ]] || fail 'Phase34 genuine Manager.Effect test missing'
[[ -f internal/platform/installationlicense/phase19_session_fencing_test.go ]] || fail 'signed V2 fixture missing'

version="$(go version 2>/dev/null)" || fail 'cannot query Go version'
[[ "$version" =~ go([0-9]+)\.([0-9]+)(\.([0-9]+))?([[:space:]]|$) ]] || fail "unrecognized Go version: $version"
major="${BASH_REMATCH[1]}"; minor="${BASH_REMATCH[2]}"
(( major > 1 || (major == 1 && minor >= 27) )) || fail "Go 1.27+ required, found $version"
[[ "$(go env GOOS)" == linux ]] || fail 'Linux-only audit runner'
[[ "$(go env CGO_ENABLED)" == 1 ]] || fail '-race requires enabled CGO for this target'

logbase="${LIC_A9_LOG_DIR:-${TMPDIR:-/tmp}}"
[[ -d "$logbase" && ! -L "$logbase" ]] || fail 'invalid log parent'
logdir="$(mktemp -d "$logbase/lic-a9-phase35-XXXXXX")" || fail 'cannot create private audit log directory'
chmod 0700 "$logdir"
printf 'LIC_A9_PHASE35_RUN_STARTED head=%s branch=%s toolchain=%s\n' "$head" "$branch" "$version" | tee "$logdir/meta.log"
printf '%s\n' 'Nonproduction synthetic signed V2 fixtures only; no Authority or external-provider E2E.' | tee -a "$logdir/meta.log"
printf 'Audit logs: %s\n' "$logdir"

# Offline: NEVER auto-download a toolchain or modules. A missing dependency
# is a BLOCK, not authorization to use a server or GitHub Actions.
run() {
  local label="$1"; shift
  printf '\n== %s ==\n' "$label" | tee "$logdir/$label.log"
  "$@" 2>&1 | tee -a "$logdir/$label.log"
}
run phase34_signed_v2 go test -race -count=1 -timeout=180s -run '^TestLICA9Phase34' ./internal/platform/installationlicense
run full_installationlicense go test -race -count=1 -timeout=240s ./internal/platform/installationlicense
run installationlicense_vet go vet ./internal/platform/installationlicense
(
 cd "$logdir"
 sha256sum meta.log phase34_signed_v2.log full_installationlicense.log installationlicense_vet.log > SHA256SUMS
)
printf '\nLIC_A9_PHASE35_EXECUTION_PASS exact_head=%s logs=%s\n' "$head" "$logdir"
