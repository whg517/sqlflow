# ==============================================================================
# SQLFlow Makefile
# ==============================================================================

.PHONY: help build dev test lint fmt clean verify \
        docker-up docker-down docker-build \
        merge-cleanup e2e-setup e2e-test e2e-teardown e2e-all docs

##@ Build

build: ## Build all (Go backend + frontend)
build: go-build web-build

go-build: ## Build Go backend binary
	go build ./...

web-build: ## Build frontend (tsc + vite)
	cd web && npm run build

##@ Development

PIDFILE := /tmp/sqlflow-dev.pid

dev: ## Start full dev environment (Go backend + Vite)
	@echo "Starting backend and frontend..."
	@$(MAKE) dev-backend & PID=$$!; echo $$PID > $(PIDFILE).back; \
		$(MAKE) dev-frontend & PID=$$!; echo $$PID > $(PIDFILE).front; \
		trap 'kill $$(cat $(PIDFILE).back) $$(cat $(PIDFILE).front) 2>/dev/null; rm -f $(PIDFILE).*' EXIT; \
		wait

dev-backend: ## Start Go backend server (port 8080)
	go run ./cmd/... serve

dev-frontend: ## Start frontend dev server (Vite :5173)
	cd web && npm run dev

##@ Test

test: ## Run all tests (Go + frontend unit)
test: go-test web-test

go-test: ## Run Go tests
	go test ./...

web-test: ## Run frontend unit tests (Vitest)
	cd web && npm run test

##@ Quality

lint: ## Lint all (golangci-lint + ESLint)
lint: go-lint web-lint

go-lint: ## Run golangci-lint
	golangci-lint run ./...

go-vet: ## Run Go vet (superseded by golangci-lint, kept for compat)
	go vet ./...

web-lint: ## Run ESLint
	cd web && npm run lint

fmt: ## Format all code (go fmt + goimports + prettier)
	golangci-lint fmt ./...
	cd web && npx prettier --write "src/**/*.{ts,tsx}"

docs: ## Generate Swagger API documentation
	$(shell go env GOPATH)/bin/swag init -g cmd/server/main.go -o docs/

verify: ## Full CI check (lint + build + test)
verify: lint build test

##@ E2E Tests (SF-QA0027)

e2e-setup: ## Start E2E test environment (docker-compose.test.yml)
	docker compose --env-file .env.test -f docker-compose.test.yml up -d --build --wait

e2e-test: ## Run E2E tests against test environment (unified real mode)
	cd e2e && npm run test:e2e

e2e-teardown: ## Stop E2E test environment
	docker compose --env-file .env.test -f docker-compose.test.yml down -v

e2e-all: ## Full E2E: setup + test + teardown (teardown always runs)
	$(MAKE) e2e-setup && $(MAKE) e2e-test; $(MAKE) e2e-teardown

##@ Docker

docker-up: ## Start application stack
	docker compose up -d

docker-down: ## Stop application stack
	docker compose down

docker-build: ## Build application images
	docker compose build

##@ Maintenance

clean: ## Remove build artifacts and caches
	rm -rf web/dist web/node_modules/.vite e2e/test-results
	go clean -cache

merge-cleanup: ## Remove worktree and branch (BRANCH=feat/xxx)
	@if [ -z "$(BRANCH)" ]; then \
		echo "Usage: make merge-cleanup BRANCH=feat/xxx"; exit 1; \
	fi
	./scripts/merge-cleanup.sh "$(BRANCH)"

##@ Help

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
