#!/bin/sh
set -e

# Ensure data directory exists (useful for volume mounts)
mkdir -p /app/data

echo "=== SQLFlow Starting ==="
echo "Time    : $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
echo "Version : ${APP_VERSION:-dev}"
echo "Port    : ${SQLFLOW_SERVER_PORT:-8080}"
echo "Metrics : ${SQLFLOW_METRICS_ENABLED:-false}"
echo "========================"

exec /app/sqlflow
