# ==============================================================================
# SQLFlow Makefile
# ==============================================================================

.PHONY: help build dev test lint fmt clean verify \
        docker-up docker-down docker-build docker-up-dev docker-down-dev docker-build-dev \
        merge-cleanup docs \
        cover go-cover web-cover

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

go-test: ## Run Go tests (with race detector)
	go test -race -count=1 ./...

web-test: ## Run frontend unit tests (Vitest)
	cd web && npm run test

cover: ## Generate coverage reports (Go + frontend)
cover: go-cover web-cover

go-cover: ## Generate Go coverage report (coverage.out + HTML)
	go test -race -count=1 -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out | tail -1
	@echo "HTML report: open coverage.html"
	go tool cover -html=coverage.out -o coverage.html

web-cover: ## Generate frontend coverage report
	cd web && npm run test:coverage

##@ Quality

lint: ## Lint all (golangci-lint + ESLint)
lint: go-lint web-lint

go-lint: ## Run golangci-lint
	golangci-lint run ./...

go-vet: ## Run Go vet (superseded by golangci-lint, kept for compat)
	go vet ./...

arch: ## Check package layering (also runs as part of `make test`)
	go test ./internal/arch/ -count=1

web-lint: ## Run ESLint
	cd web && npm run lint

fmt: ## Format all code (go fmt + goimports + prettier)
	golangci-lint fmt ./...
	cd web && npx prettier --write "src/**/*.{ts,tsx}"

docs: ## Generate OpenAPI package used by Swagger UI
	$(shell go env GOPATH)/bin/swag init -g cmd/server/main.go -o internal/api/openapi --ot go --packageName docs

verify: ## Full CI check (lint + build + test)
verify: lint build test

##@ Docker

docker-up: ## Start production stack
	docker compose up -d --wait

docker-down: ## Stop production stack
	docker compose down

docker-build: ## Build production images
	docker compose build

docker-build-dev: ## Build dev images
	docker compose -f docker-compose.yml -f docker-compose.dev.yml build

docker-up-dev: ## Start dev stack (with debug ports, no resource limits)
	docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d --wait

docker-down-dev: ## Stop dev stack
	docker compose -f docker-compose.yml -f docker-compose.dev.yml down

docker-logs: ## Tail application logs
	docker compose logs -f sqlflow

docker-logs-mysql: ## Tail MySQL logs
	docker compose logs -f mysql

docker-ps: ## Show running containers
	docker compose ps

##@ Maintenance

clean: ## Remove build artifacts and caches
	rm -rf web/dist web/node_modules/.vite
	go clean -cache

merge-cleanup: ## Remove worktree and branch (BRANCH=feat/xxx)
	@if [ -z "$(BRANCH)" ]; then \
		echo "Usage: make merge-cleanup BRANCH=feat/xxx"; exit 1; \
	fi
	./scripts/merge-cleanup.sh "$(BRANCH)"

##@ Help

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
