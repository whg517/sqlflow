#!/usr/bin/env bash
# pipeline-check.sh — 需求池状态确定性检查（git/文件系统层）
# 仅检查脚本可以做的事，不调用 reqmgr MCP
# 输出 JSON 到 stdout
# Usage: ./scripts/pipeline-check.sh
# Exit code: 0 = success, 1 = error

set -euo pipefail

cd "$(git rev-parse --show-toplevel 2>/dev/null || echo "$HOME/projects/sql-platform")"

# Check worktree list
worktree_count=$(git worktree list 2>/dev/null | wc -l)
worktree_list=$(git worktree list 2>/dev/null || echo "")

# Check unmerged local branches only (skip remotes to avoid duplicates)
unmerged_branches=$(git branch --no-merged main 2>/dev/null | grep -v HEAD | grep -v dependabot || echo "")

# Find branches without matching requirement IDs (potential orphans)
# Branches containing a known requirement ID (e.g. SF-ENG0051) are NOT orphans.
orphan_branches=""
if [ -n "$unmerged_branches" ]; then
  while IFS= read -r branch; do
    [ -z "$branch" ] && continue
    branch=$(echo "$branch" | sed 's/^[*+ ]*//')
    # Extract requirement ID from branch name (e.g. feat/SF-FEAT0046-dashboard)
    req_id=$(echo "$branch" | grep -oP '[A-Z]{2,}-[A-Z]{3,}\d+' | head -1 || true)
    if [ -z "$req_id" ]; then
      orphan_branches="$orphan_branches$branch\n"
    fi
  done <<< "$unmerged_branches"
fi
# Trim trailing newline for clean JSON
orphan_branches=$(echo -e "$orphan_branches" | sed '/^$/d')

# Output structured JSON
cat <<JSONEOF
{
  "timestamp": "$(date -Iseconds)",
  "worktree_count": $worktree_count,
  "worktree_list": $(echo "$worktree_list" | jq -R . | jq -s . 2>/dev/null || echo "[]"),
  "unmerged_branches": $(echo "$unmerged_branches" | jq -R . | jq -s . 2>/dev/null || echo "[]"),
  "orphan_branches": $(echo "$orphan_branches" | jq -R . | jq -s . 2>/dev/null || echo "[]")
}
JSONEOF
