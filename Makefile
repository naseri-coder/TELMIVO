.PHONY: bootstrap up down logs build test test-unit test-contract test-integration test-financial lint fmt vet migrate migration-check security-check release-candidate-check
bootstrap:
	@test -f .env || cp .env.example .env
up:
	@sh scripts/dev/guard.sh
	docker compose up --build -d
down:
	docker compose down
logs:
	docker compose logs --tail=100 -f
build:
	go build ./cmd/...
	npm --prefix web/miniapp run build
test: test-unit test-contract
	npm --prefix web/miniapp test
test-unit:
	go test ./internal/...
test-contract:
	go test ./tests/contract -count=1
test-integration:
	@sh scripts/ci/check-test-target.sh
	go test -tags=integration ./tests/integration -count=1 -timeout=3m
test-financial:
	@sh scripts/ci/check-test-target.sh
	go test -tags=integration ./tests/integration -run 'TestWallet|TestFinancial' -count=1 -timeout=2m
lint: vet
	@test -z "$$(gofmt -l cmd internal db tests)"
	npm --prefix web/miniapp run lint
fmt:
	gofmt -w cmd internal db tests
vet:
	go vet ./...
migrate:
	@sh scripts/dev/guard.sh
	docker compose run --rm migrate
	docker compose run --rm grants
migration-check:
	@sh scripts/ci/check-test-target.sh
	go test -tags=integration ./tests/integration -run TestMigration -count=1
security-check:
	python3 scripts/security/scan.py
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...
	npm --prefix web/miniapp audit --audit-level=high

# Structural candidate verification only; NOT a signed installer approval.
# Manifest digest MUST come from an independent release record.
release-candidate-check:
	@test -n "$${PACKAGE_PATH:-}" && test -n "$${MANIFEST_SHA256:-}" || { echo "Set exported PACKAGE_PATH and independently pinned MANIFEST_SHA256"; exit 2; }
	python3 scripts/release/phase51k_package_guard.py --archive "$$PACKAGE_PATH" --manifest-sha256 "$$MANIFEST_SHA256"
