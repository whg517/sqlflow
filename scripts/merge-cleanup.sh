#!/usr/bin/env bash
# merge-cleanup.sh
# Usage: ./scripts/merge-cleanup.sh <branch-name>

set -euo pipefail

BRANCH="$1"
WORKTREE_DIR=".worktree/${BRANCH}"
REMOTE="origin"

echo "==> Cleaning up branch: $BRANCH"
echo "==> Worktree dir: $WORKTREE_DIR"
echo "==> Remote: $REMOTE"
echo ""

# 1. Remove worktree
echo "[1/4] Removing worktree: $WORKTREE_DIR ..."
if git worktree list | grep -q "$WORKTREE_DIR"; then
  if ! git worktree remove "$WORKTREE_DIR" 2>/dev/null; then
    echo "  Warning: worktree remove failed, trying force ..."
    git worktree remove --force "$WORKTREE_DIR"
  fi
  echo "  ✅ Worktree removed"
else
  echo "  ⏭️  Worktree not found, skipping"
fi

# 2. Delete local branch
echo "[2/4] Deleting local branch: $BRANCH ..."
if git branch --list | grep -q " $BRANCH$"; then
  git branch -D "$BRANCH"
  echo "  ✅ Local branch deleted"
else
  echo "  ⏭️  Local branch not found, skipping"
fi

# 3. Delete remote branch
echo "[3/4] Deleting remote branch: $REMOTE/$BRANCH ..."
if git branch -r | grep -q "$REMOTE/$BRANCH$"; then
  git push "$REMOTE" --delete "$BRANCH"
  echo "  ✅ Remote branch deleted"
else
  echo "  Remote branch not found, skipping"
fi

# 4. Verify clean state
echo "[4/4] Verifying clean state ..."
WORKTREE_COUNT=$(git worktree list | wc -l)
if [ "$WORKTREE_COUNT" -le 1 ]; then
  echo "  ✅ No stale worktrees"
else
  echo "  ⚠️  $WORKTREE_COUNT worktrees still exist"
fi

UNMERGED=$(git branch -a --no-merged main 2>/dev/null | grep -v dependabot | grep -v '^\*' || true)
if [ -z "$UNMERGED" ]; then
  echo "  ✅ No unmerged branches"
else
  echo "  ⚠️  Unmerged branches: $UNMERGED"
fi

echo ""
echo "==> Done. Current HEAD:"
git log --oneline -1
