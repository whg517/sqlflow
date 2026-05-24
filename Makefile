# SQLFlow Makefile
#
# Usage:
#   make                Show all available targets
#   make dev            Start frontend dev server
#   make go-build       Compile Go backend
#
# Note: GNU Make does not support colons in target names.
# Targets like test:e2e are invoked as make test-e2e.

.DEFAULT_GOAL := help

.PHONY: help dev build lint test clean merge-cleanup \
        go-build go-test go-fmt \
        docker-up docker-down docker-build \
        test-e2e test-e2e-real

# ----------------------------------------------------------
# Development
# ----------------------------------------------------------

## dev              Start frontend dev server (Vite on port 5173)
dev:
	cd web && npm run dev

## go-build         Compile Go backend
go-build:
	go build ./...

## go-test          Run Go tests
go-test:
	go test ./...

## go-fmt           Format Go code
go-fmt:
	go fmt ./...

## build            Build frontend for production (tsc + vite build)
build:
	cd web && npm run build

## lint             Run ESLint on web source
lint:
	cd web && npm run lint

# ----------------------------------------------------------
# Testing
# ----------------------------------------------------------

## test             Run unit tests (Vitest, 508 tests)
test:
	cd web && npm run test

## test-e2e         Run mock E2E tests (Playwright, route-mocked API)
test-e2e:
	cd e2e && npm run test:mock

## test-e2e-real    Run real E2E tests (requires e2e docker-compose)
test-e2e-real:
	cd e2e && npm run test:real

# ----------------------------------------------------------
# Docker
# ----------------------------------------------------------

## docker-up        Start Docker Compose services
docker-up:
	docker compose up -d

## docker-down      Stop Docker Compose services
docker-down:
	docker compose down

## docker-build     Build Docker images via docker-compose
docker-build:
	docker compose build

# ----------------------------------------------------------
# Maintenance
# ----------------------------------------------------------

## clean            Remove build artifacts, caches, and test results
clean:
	rm -rf web/dist web/node_modules/.vite e2e/test-results

## merge-cleanup    Clean up merged branch (usage: make merge-cleanup BRANCH=feat/xxx)
merge-cleanup:
	@if [ -z "$(BRANCH)" ]; then \
		echo "Error: BRANCH is required. Usage: make merge-cleanup BRANCH=feat/xxx"; \
		exit 1; \
	fi
	./scripts/merge-cleanup.sh "$(BRANCH)"

# ----------------------------------------------------------
# Help (auto-generated from ## comments above)
# ----------------------------------------------------------

## help             Show this help message
help:
	@echo ""
	@echo "SQLFlow — Available Targets"
	@echo "==========================="
	@echo ""
	@sed -n 's/^##  *\([^ ]*\)  */  \1 /p' Makefile | \
		while IFS= read -r line; do \
			target=$$(echo "$$line" | awk '{print $$1}'); \
			desc=$$(echo "$$line" | cut -c$$(( $${#target} + 3 ))-); \
			printf "  \033[36m%-18s\033[0m%s\n" "$$target" "$$desc"; \
		done
	@echo ""
