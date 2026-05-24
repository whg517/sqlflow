# ==============================================================================
# SQLFlow Makefile
# ==============================================================================

.PHONY: help build dev test lint fmt clean verify \
        docker-up docker-down docker-build \
        merge-cleanup

# ── Build ───────────────────────────────────────────────────────────────────

build: ## Build all (Go backend + frontend)
build: go-build web-build

go-build: ## Build Go backend binary
	go build ./...

web-build: ## Build frontend (tsc + vite)
	cd web && npm run build

# ── Development ─────────────────────────────────────────────────────────────

dev: ## Start full dev environment (Go backend + Vite)
dev: dev-backend dev-frontend

dev-backend: ## Start Go backend server (port 8080)
	go run ./cmd/... serve

dev-frontend: ## Start frontend dev server (Vite :5173)
	cd web && npm run dev

# ── Test ────────────────────────────────────────────────────────────────────

test: ## Run all tests (Go + frontend unit)
test: go-test web-test

go-test: ## Run Go tests
	go test ./...

web-test: ## Run frontend unit tests (Vitest)
	cd web && npm run test

# ── Quality ──────────────────────────────────────────────────────────────────

lint: ## Lint all (Go vet + ESLint)
lint: go-vet web-lint

go-vet: ## Run Go vet
	go vet ./...

web-lint: ## Run ESLint
	cd web && npm run lint

fmt: ## Format all code (go fmt + prettier)
	go fmt ./...
	cd web && npx prettier --write "src/**/*.{ts,tsx}"

verify: ## Full CI check (lint + build + test)
verify: lint build test

# ── Docker ──────────────────────────────────────────────────────────────────

docker-up: ## Start application stack
	docker compose up -d

docker-down: ## Stop application stack
	docker compose down

docker-build: ## Build application images
	docker compose build

# ── Maintenance ─────────────────────────────────────────────────────────────

clean: ## Remove build artifacts and caches
	rm -rf web/dist web/node_modules/.vite e2e/test-results
	go clean -cache

merge-cleanup: ## Remove worktree and branch (BRANCH=feat/xxx)
	@if [ -z "$(BRANCH)" ]; then \
		echo "Usage: make merge-cleanup BRANCH=feat/xxx"; exit 1; \
	fi
	./scripts/merge-cleanup.sh "$(BRANCH)"

# ── Help ────────────────────────────────────────────────────────────────────

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		sort -k1,1 | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m%s\n", $$1, $$2}'
