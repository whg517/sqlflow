#!/bin/sh
set -e

# Wait for the data directory to be available (useful for volume mounts)
mkdir -p "${DB_PATH:-/app/data}"

echo "=== SQLFlow Starting ==="
echo "Time   : $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
echo "Version: ${VERSION:-dev}"
echo "Port   : ${SERVER_PORT:-8080}"
echo "========================"

exec /app/sqlflow
