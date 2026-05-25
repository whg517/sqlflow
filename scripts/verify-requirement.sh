#!/usr/bin/env bash
# verify-requirement.sh — 验证需求代码是否存在于 main 分支
# Usage: ./scripts/verify-requirement.sh <requirement-id>
# Example: ./scripts/verify-requirement.sh SF-FEAT0021
# Exit code: 0 = found, 1 = not found

set -euo pipefail

REQ_ID="${1:-}"
if [ -z "$REQ_ID" ]; then
  echo '{"status":"error","message":"usage: verify-requirement.sh <requirement-id>"}'
  exit 1
fi

cd "$(git rev-parse --show-toplevel 2>/dev/null || echo "$HOME/projects/sql-platform")"

branch_result=""
main_result=""

# Step 1: check unmerged branches
if git branch -a --no-merged main | grep -q "$REQ_ID" 2>/dev/null; then
  branch_info=$(git branch -a --no-merged main | grep "$REQ_ID" | head -1 | xargs)
  branch_result="found"
else
  branch_result="not_found"
  branch_info=""
fi

# Step 2: check main commit history
if git log --oneline --grep="$REQ_ID" main -1 2>/dev/null | grep -q .; then
  main_info=$(git log --oneline --grep="$REQ_ID" main -1 | xargs)
  main_result="found"
else
  main_result="not_found"
  main_info=""
fi

# Determine overall status
if [ "$branch_result" = "found" ] || [ "$main_result" = "found" ]; then
  status="found"
else
  status="not_found"
fi

# Output JSON
cat <<EOF
{
  "requirement": "$REQ_ID",
  "status": "$status",
  "branch": {"result": "$branch_result", "info": "$branch_info"},
  "main": {"result": "$main_result", "info": "$main_info"}
}
EOF

[ "$status" = "found" ] && exit 0 || exit 1
