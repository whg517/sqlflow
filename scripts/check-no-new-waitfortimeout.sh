#!/usr/bin/env bash
# scripts/check-no-new-waitfortimeout.sh
#
# Guard against new page.waitForTimeout() calls in E2E specs.
#
# waitForTimeout is a hardcoded sleep and the primary source of E2E flakiness
# (see QA audit H2). It should be replaced with conditional waits
# (expect(locator).toBeVisible(), page.waitForResponse(), etc.). This script
# enforces a baseline so existing usages are grandfathered but any *new* sleep
# fails CI until either removed or explicitly added to the baseline.
#
# Usage: ./scripts/check-no-new-waitfortimeout.sh
set -euo pipefail

cd "$(git rev-parse --show-toplevel 2>/dev/null || echo .)"

# Baseline: allowed number of waitForTimeout occurrences across e2e/tests/.
# Lower this number as sleeps are removed; never raise it without review.
BASELINE=97

if ! command -v grep >/dev/null 2>&1; then
  echo "check-no-new-waitfortimeout: grep not found" >&2
  exit 2
fi

count=0
if [ -d e2e/tests ]; then
  count=$(grep -r --include='*.spec.ts' --include='*.spec.js' -c 'waitForTimeout' e2e/tests 2>/dev/null \
    | awk -F: '{s+=$2} END {print s+0}')
fi

echo "waitForTimeout occurrences in e2e/tests: $count (baseline: $BASELINE)"

if [ "$count" -gt "$BASELINE" ]; then
  echo "❌ FAIL: $count > baseline $BASELINE — new page.waitForTimeout() introduced." >&2
  echo "   Replace hardcoded sleeps with conditional waits, e.g.:" >&2
  echo "     await expect(locator).toBeVisible()" >&2
  echo "     await page.waitForResponse(url => url.pathname.includes('/api/...'))" >&2
  echo "   If a new sleep is genuinely unavoidable, raise BASELINE in this script" >&2
  echo "   and document why in the commit message." >&2
  exit 1
fi

echo "✅ OK: no new waitForTimeout calls."
exit 0
