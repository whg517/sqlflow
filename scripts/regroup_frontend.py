#!/usr/bin/env python3
"""Reorganise web/src from package-by-kind into package-by-feature.

Pages, their API clients, their hooks and their feature-specific components move
together into web/src/features/<name>/, mirroring the backend's domain packages.
What genuinely has no owner — the HTTP client, the design-system primitives, the
layout chrome — moves to web/src/shared/.

The point is the same as on the backend: a change to one feature touches one
directory, so both a reviewer and an assistant can see the whole of it at once.

Run from the repository root. Imports are rewritten by path, so the "@/" alias
keeps working; run `npx tsc -b` and the test suite afterwards.
"""

import pathlib
import re
import subprocess
import sys

WEB = pathlib.Path("web/src")

# feature -> {source directory or file under web/src : destination subdirectory}
FEATURES = {
    "query": {
        "pages/Query": "pages/Query",
        "pages/SQLTemplatePage": "pages/SQLTemplatePage",
        "pages/SharedResultPage.tsx": "pages/SharedResultPage.tsx",
        "api/query.ts": "api/query.ts",
        "api/__tests__/query.api.test.ts": "api/__tests__/query.api.test.ts",
        "api/__tests__/query.helpers.test.ts": "api/__tests__/query.helpers.test.ts",
        "api/__tests__/query.stream.test.ts": "api/__tests__/query.stream.test.ts",
        "api/queryHistory.ts": "api/queryHistory.ts",
        "api/explain.ts": "api/explain.ts",
        "api/capabilities.ts": "api/capabilities.ts",
        "api/share.ts": "api/share.ts",
        "api/sql-template.ts": "api/sql-template.ts",
        "api/export.ts": "api/export.ts",
        "hooks/useDatasourceCapabilities.ts": "hooks/useDatasourceCapabilities.ts",
        "hooks/useSchemaCompletion.ts": "hooks/useSchemaCompletion.ts",
        "hooks/__tests__/useDatasourceCapabilities.test.ts": "hooks/__tests__/useDatasourceCapabilities.test.ts",
        "hooks/__tests__/useSchemaCompletion.test.ts": "hooks/__tests__/useSchemaCompletion.test.ts",
        "components/ExportDialog": "components/ExportDialog",
    },
    "ticket": {
        "pages/Ticket": "pages/Ticket",
        "pages/TicketNew": "pages/TicketNew",
        "api/ticket.ts": "api/ticket.ts",
        "api/__tests__/ticket.api.test.ts": "api/__tests__/ticket.api.test.ts",
        "api/__tests__/ticket.helpers.test.ts": "api/__tests__/ticket.helpers.test.ts",
        "api/__tests__/comment.api.test.ts": "api/__tests__/comment.api.test.ts",
        "api/__tests__/comment.helpers.test.ts": "api/__tests__/comment.helpers.test.ts",
        "api/approval.ts": "api/approval.ts",
        "api/comment.ts": "api/comment.ts",
        "api/sla.ts": "api/sla.ts",
        "components/AIReview": "components/AIReview",
        "components/GitInfoSection.tsx": "components/GitInfoSection.tsx",
    },
    "audit": {
        "pages/Audit": "pages/Audit",
        "pages/Reports": "pages/Reports",
        "api/audit.ts": "api/audit.ts",
        "api/__tests__/audit.api.test.ts": "api/__tests__/audit.api.test.ts",
        "api/__tests__/audit.helpers.test.ts": "api/__tests__/audit.helpers.test.ts",
        "api/report.ts": "api/report.ts",
        "api/userAnalytics.ts": "api/userAnalytics.ts",
    },
    "security": {
        "pages/Permissions": "pages/Permissions",
        "pages/PermRequestPage": "pages/PermRequestPage",
        "api/permission-request.ts": "api/permission-request.ts",
        "api/maskRule.ts": "api/maskRule.ts",
        "api/__tests__/maskRule.api.test.ts": "api/__tests__/maskRule.api.test.ts",
        "api/__tests__/maskRule.helpers.test.ts": "api/__tests__/maskRule.helpers.test.ts",
        "components/__tests__/SensitiveTableBadge.test.tsx": "components/__tests__/SensitiveTableBadge.test.tsx",
        "hooks/__tests__/useSensitiveTables.test.ts": "hooks/__tests__/useSensitiveTables.test.ts",
        "hooks/useSensitiveTables.ts": "hooks/useSensitiveTables.ts",
        "components/SensitiveTableBadge.tsx": "components/SensitiveTableBadge.tsx",
    },
    "iam": {
        "pages/Login": "pages/Login",
        "pages/Users": "pages/Users",
        "pages/TokenPage": "pages/TokenPage",
        "api/user.ts": "api/user.ts",
        "api/__tests__/user.api.test.ts": "api/__tests__/user.api.test.ts",
        "api/__tests__/user.helpers.test.ts": "api/__tests__/user.helpers.test.ts",
        "api/token.ts": "api/token.ts",
    },
    "ops": {
        "pages/Dashboard": "pages/Dashboard",
        "pages/Performance": "pages/Performance",
        "pages/Settings": "pages/Settings",
        "api/dashboard.ts": "api/dashboard.ts",
        "api/__tests__/dashboard.api.test.ts": "api/__tests__/dashboard.api.test.ts",
        "api/performance.ts": "api/performance.ts",
        "api/git.ts": "api/git.ts",
        "api/webhookSubscription.ts": "api/webhookSubscription.ts",
    },
}

# Everything with no single owner. The HTTP client and the design system are
# used by every feature; the layout chrome belongs to the shell, not a feature.
SHARED = {
    "api/client.ts": "api/client.ts",
    "api/__tests__/client.test.ts": "api/__tests__/client.test.ts",
    "api/__tests__/response-normalization.test.ts": "api/__tests__/response-normalization.test.ts",
    "components/ui": "components/ui",
    "components/Layout.tsx": "components/Layout.tsx",
    "components/__tests__/Layout.test.tsx": "components/__tests__/Layout.test.tsx",
    "components/__tests__/Layout.permissions.test.tsx": "components/__tests__/Layout.permissions.test.tsx",
    "components/AuthGuard.tsx": "components/AuthGuard.tsx",
    "components/__tests__/AuthGuard.test.tsx": "components/__tests__/AuthGuard.test.tsx",
    "components/ErrorBoundary.tsx": "components/ErrorBoundary.tsx",
    "components/__tests__/ErrorBoundary.test.tsx": "components/__tests__/ErrorBoundary.test.tsx",
    "components/ErrorPage.tsx": "components/ErrorPage.tsx",
    "components/__tests__/ErrorPage.test.tsx": "components/__tests__/ErrorPage.test.tsx",
    "components/LazyLoad.tsx": "components/LazyLoad.tsx",
    "components/NetworkBanner.tsx": "components/NetworkBanner.tsx",
    "components/__tests__/NetworkBanner.test.tsx": "components/__tests__/NetworkBanner.test.tsx",
    "components/PageHeader.tsx": "components/PageHeader.tsx",
    "components/CommandPalette.tsx": "components/CommandPalette.tsx",
    "components/__tests__/CommandPalette.test.tsx": "components/__tests__/CommandPalette.test.tsx",
    "components/ChangePasswordDialog.tsx": "components/ChangePasswordDialog.tsx",
    "components/__tests__/ChangePasswordDialog.test.tsx": "components/__tests__/ChangePasswordDialog.test.tsx",
    "components/HighlightText.tsx": "components/HighlightText.tsx",
    "components/__tests__/HighlightText.test.tsx": "components/__tests__/HighlightText.test.tsx",
    "hooks/useTheme.ts": "hooks/useTheme.ts",
    "hooks/__tests__/useTheme.test.ts": "hooks/__tests__/useTheme.test.ts",
    "lib": "lib",
}


def git_mv(src: pathlib.Path, dst: pathlib.Path):
    dst.parent.mkdir(parents=True, exist_ok=True)
    subprocess.run(["git", "mv", str(src), str(dst)], check=True)


def build_moves():
    """Return [(old path under web/src, new path under web/src)] plus the alias map."""
    moves = []
    for feature, entries in FEATURES.items():
        for src, dst in entries.items():
            moves.append((src, f"features/{feature}/{dst}"))
    for src, dst in SHARED.items():
        moves.append((src, f"shared/{dst}"))
    return moves


def rewrite_imports(moves):
    """Point every "@/..." import at the file's new location.

    Longest source path first, so that "api/query.ts" is not shadowed by a
    shorter prefix that happens to match.
    """
    alias = []
    for src, dst in sorted(moves, key=lambda m: -len(m[0])):
        old = src[:-4] if src.endswith(".tsx") else src[:-3] if src.endswith(".ts") else src
        new = dst[:-4] if dst.endswith(".tsx") else dst[:-3] if dst.endswith(".ts") else dst
        alias.append((old, new))

    for path in WEB.rglob("*"):
        if path.suffix not in (".ts", ".tsx") or not path.is_file():
            continue
        text = original = path.read_text()
        for old, new in alias:
            # match "@/old" only at a path-segment boundary
            text = re.sub(rf'(["\'])@/{re.escape(old)}(?=["\'/])', rf'\1@/{new}', text)
        if text != original:
            path.write_text(text)


def main():
    if not WEB.is_dir():
        sys.exit("run from the repository root")
    moves = build_moves()
    moved = 0
    for src, dst in moves:
        source, dest = WEB / src, WEB / dst
        # Idempotent: a partially applied run must be resumable, since the
        # import rewrite only happens once every file is in place.
        if dest.exists() and not source.exists():
            continue
        if not source.exists():
            sys.exit(f"regroup_frontend: {source} does not exist")
        git_mv(source, dest)
        moved += 1
    rewrite_imports(moves)
    print(f"moved {moved} paths ({len(moves) - moved} already in place); "
          f"run `npx tsc -b` and the tests")


if __name__ == "__main__":
    main()
