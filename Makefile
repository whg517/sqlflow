# ==============================================================================
# SQLFlow Makefile
# ==============================================================================

SHELL := /bin/bash
.DEFAULT_GOAL := help

COL := 17

.PHONY: help \
        dev dev-api dev-web \
        build build-all \
        test test-all test-e2e test-e2e-real \
        lint lint-all fmt verify \
        docker-up docker-down docker-build \
        clean merge-cleanup \
        _go-build _go-test _go-vet

# ── DEVELOPMENT ──────────────────────────────────────────────────────────────

## dev            Start full dev environment (backend + frontend)
dev: dev-api dev-web

## dev-api        Start Go backend server (port 8080)
dev-api:
	go run ./cmd/... serve

## dev-web        Start frontend dev server (Vite :5173)
dev-web:
	cd web && npm run dev

# ── BUILD ───────────────────────────────────────────────────────────────────

## build          Build frontend (tsc + vite)
build:
	cd web && npm run build

## build-all      Build everything (Go + frontend)
build-all: _go-build build

# ── TEST ────────────────────────────────────────────────────────────────────

## test           Run frontend unit tests (Vitest)
test:
	cd web && npm run test

## test-all       Run all tests (Go + frontend)
test-all: _go-test test

## test-e2e       Run mock E2E (Playwright, no backend needed)
test-e2e:
	cd e2e && npm run test:mock

## test-e2e-real  Run real E2E (requires e2e docker-compose)
test-e2e-real:
	cd e2e && npm run test:real

# ── QUALITY ──────────────────────────────────────────────────────────────────

## lint           Lint frontend (ESLint)
lint:
	cd web && npm run lint

## lint-all       Lint all (Go vet + ESLint)
lint-all: _go-vet lint

## fmt            Format all code (go fmt + prettier)
fmt:
	go fmt ./...
	cd web && npx prettier --write "src/**/*.{ts,tsx}"

## verify         Full CI check: lint + typecheck + build + test
verify: lint-all
	cd web && npx tsc --noEmit && npm run build && npm run test
	go vet ./...
	go test ./...

# ── DOCKER ──────────────────────────────────────────────────────────────────

## docker-up      Start application stack
docker-up:
	docker compose up -d

## docker-down    Stop application stack
docker-down:
	docker compose down

## docker-build   Build application images
docker-build:
	docker compose build

# ── MAINTENANCE ─────────────────────────────────────────────────────────────

## clean          Remove build artifacts and caches
clean:
	rm -rf web/dist web/node_modules/.vite e2e/test-results
	go clean -cache

## merge-cleanup  Remove worktree and branch after merge (BRANCH=feat/xxx)
merge-cleanup:
	@if [ -z "$(BRANCH)" ]; then \
		echo "Usage: make merge-cleanup BRANCH=feat/xxx"; exit 1; \
	fi
	./scripts/merge-cleanup.sh "$(BRANCH)"

# ── INTERNAL ─────────────────────────────────────────────────────────────────

_go-build:
	@go build ./...

_go-test:
	@go test ./...

_go-vet:
	@go vet ./...

# ── HELP ────────────────────────────────────────────────────────────────────

## help           Show available targets
help:
	@echo ""
	@awk 'BEGIN { COL = ENVIRON["COL"] } \
		/^## [a-z]/ && !/^## help/ { \
			sub(/^## /, ""); \
			t = $$1; sub(/^[^ ]+[ \t]+/, ""); d = $$0; \
			g = 0; \
			if (t ~ /^dev/) g = 1; \
			else if (t ~ /^build/) g = 2; \
			else if (t ~ /^test/) g = 3; \
			else if (t ~ /^(lint|fmt|verify)/) g = 4; \
			else if (t ~ /^docker-/) g = 5; \
			else g = 6; \
			G[1] = "DEVELOPMENT"; G[2] = "BUILD"; G[3] = "TEST"; \
			G[4] = "QUALITY"; G[5] = "DOCKER"; G[6] = "MAINTENANCE"; \
			if (g != pg) { printf "\n  \033[1m%s\033[0m\n", G[g]; pg = g } \
			printf "    \033[36m%-" COL "s\033[0m %s\n", t, d \
		}' COL=$(COL) $(MAKEFILE_LIST)
	@echo ""
