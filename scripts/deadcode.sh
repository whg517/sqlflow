#!/usr/bin/env bash
#
# Fail on code that nothing can reach — not from main, and not from a test.
#
# `unused` does not report exported symbols, because it cannot assume a package
# has no importers. For internal/ that assumption holds, and the gap let whole
# clusters rot in place: connpool kept a complete PostgreSQL pool that no caller
# had touched in either direction, and a superseded APITokenAuth middleware sat
# next to the live one under a name that read like the real thing.
#
# Reachability comes from x/tools/cmd/deadcode, which walks the call graph with
# type information rather than matching names. `-test` includes test binaries as
# roots, so anything a test exercises counts as reachable: this gate is about
# code nothing at all can run, not about code only tests run. That second, much
# larger set needs judgement — a test helper is supposed to be test-only — so it
# is reported by `make deadcode-report` and not gated.
set -euo pipefail

cd "$(dirname "$0")/.."

ALLOWLIST="scripts/deadcode.allow"

# Pinned rather than added to go.mod: `go get -tool` pulls x/tools' own
# dependency graph into this module's, and upgrading x/text that way broke
# go.sum for pgx and pkcs8. The version is fixed here so the gate cannot shift
# under a build.
DEADCODE="golang.org/x/tools/cmd/deadcode@v0.48.0"

# internal/db/ent is generated: ent's own codegen calls Annotations, Edges and
# Indexes, and the call graph cannot see that.
findings="$(go run "$DEADCODE" -test ./internal/... ./cmd/... \
  | grep -v '^internal/db/ent/' \
  | sed 's/: unreachable func: / /' \
  | sort || true)"

if [ -z "$findings" ]; then
  echo "deadcode: nothing unreachable"
  exit 0
fi

# An allowlist entry is a symbol name plus the reason it stays. Reasons are the
# point: an entry without one is indistinguishable from an oversight.
allowed="$(grep -vE '^\s*(#|$)' "$ALLOWLIST" 2>/dev/null | awk '{print $1}' | sort || true)"

unexpected=""
while IFS= read -r line; do
  [ -z "$line" ] && continue
  symbol="${line##* }"
  if ! echo "$allowed" | grep -qxF "$symbol"; then
    unexpected="${unexpected}${line}"$'\n'
  fi
done <<< "$findings"

if [ -n "$unexpected" ]; then
  echo "deadcode: nothing can reach these, and they are not in $ALLOWLIST:" >&2
  echo "$unexpected" >&2
  echo "Delete them, wire them up, or add them to $ALLOWLIST with the reason." >&2
  exit 1
fi

# A stale entry is as misleading as a missing one: it reads as permission for
# something that is no longer there.
stale=""
while IFS= read -r symbol; do
  [ -z "$symbol" ] && continue
  if ! echo "$findings" | grep -qF " $symbol"; then
    stale="${stale}${symbol}"$'\n'
  fi
done <<< "$allowed"

if [ -n "$stale" ]; then
  echo "deadcode: $ALLOWLIST lists symbols that are now reachable; remove them:" >&2
  echo "$stale" >&2
  exit 1
fi

echo "deadcode: $(echo "$findings" | wc -l | tr -d ' ') allowed finding(s), none unexpected"
