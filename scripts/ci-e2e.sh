#!/usr/bin/env bash
# ci-e2e.sh — Run full E2E test suite against docker-compose.test.yml stack
# Usage: ./scripts/ci-e2e.sh [setup|test|teardown|all]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
ENV_FILE="${PROJECT_ROOT}/.env.test"
COMPOSE_FILE="${PROJECT_ROOT}/docker-compose.test.yml"

# Defaults
ACTION="${1:-all}"
E2E_PROJECT="${E2E_PROJECT:-real}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() { echo -e "${GREEN}[ci-e2e]${NC} $1"; }
warn() { echo -e "${YELLOW}[ci-e2e]${NC} $1"; }
err() { echo -e "${RED}[ci-e2e]${NC} $1"; }

check_env() {
  if [ ! -f "$ENV_FILE" ]; then
    err ".env.test not found. Copy .env.test.example to .env.test first."
    exit 1
  fi
}

setup() {
  check_env
  log "Starting test environment..."
  docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" up -d --build --wait
  log "Test environment ready."
}

teardown() {
  log "Stopping test environment..."
  docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" down -v
  log "Test environment stopped."
}

run_tests() {
  check_env
  log "Running E2E tests (project=${E2E_PROJECT})..."

  cd "$PROJECT_ROOT/e2e"

  npx playwright test --project="$E2E_PROJECT" \
    --reporter=list \
    --retries="${CI_RETRIES:-2}" \
    --timeout=60000

  log "E2E tests passed!"
}

all() {
  setup
  # shellcheck disable=SC2064
  trap 'teardown' EXIT
  run_tests
}

case "$ACTION" in
  setup)   setup ;;
  test)    run_tests ;;
  teardown) teardown ;;
  all)     all ;;
  *)
    echo "Usage: $0 [setup|test|teardown|all]"
    exit 1
    ;;
esac
