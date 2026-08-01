#!/usr/bin/env python3
"""Move a group of files out of the flat internal/service package into a domain package.

Used to carry out the internal/service split one domain at a time. It is kept in
the repo rather than run ad hoc because each extraction needs the same four
mechanical steps applied consistently, and doing them by hand is how references
get missed silently:

  1. git mv the sources and their tests into internal/<domain>/
  2. rewrite the package clause
  3. rename symbols that would stutter once package-qualified
     (DatasourceService -> datasource.Service)
  4. requalify every reference in the rest of the tree and fix imports

It does not decide what belongs in a domain — that judgement stays with the
caller, who passes an explicit file list. Run `goimports -w` and the tests after.

Usage:
    python3 scripts/extract_domain.py <domain> <file>[:<newname>] ...

Each file is a base name under internal/service (without .go). A rename map is
given separately via --rename Old=New (repeatable).
"""

import argparse
import glob
import pathlib
import re
import subprocess
import sys

MODULE = "github.com/whg517/sqlflow"
SERVICE_DIR = pathlib.Path("internal/service")

# Identifiers at or below this length are too likely to collide with struct
# fields and locals to rewrite unqualified. See requalify_rest.
SAFE_BARE_REWRITE_LEN = 6


def run(*args):
    subprocess.run(args, check=True)


def move_files(domain, stems):
    """git mv each stem's source and every matching test into internal/<domain>."""
    dest = pathlib.Path("internal") / domain
    dest.mkdir(exist_ok=True)
    moved = []
    for stem in stems:
        src = SERVICE_DIR / f"{stem}.go"
        if not src.exists():
            sys.exit(f"extract_domain: {src} does not exist")
        run("git", "mv", str(src), str(dest / f"{stem}.go"))
        moved.append(dest / f"{stem}.go")
        tests = sorted(set(SERVICE_DIR.glob(f"{stem}_*test.go")) | set(SERVICE_DIR.glob(f"{stem}_test.go")))
        for test in tests:
            run("git", "mv", str(test), str(dest / test.name))
            moved.append(dest / test.name)
    return moved


def rewrite_moved(paths, domain, renames):
    """Set the package clause and apply renames inside the moved files."""
    for path in paths:
        text = path.read_text()
        text = re.sub(r"^package service$", f"package {domain}", text, flags=re.M)
        for old, new in renames.items():
            text = re.sub(rf"(?<![\w.]){re.escape(old)}\b", new, text)
        path.write_text(text)


def exported_symbols(domain):
    """Every exported top-level identifier the domain now owns.

    Collected from the moved sources rather than from an explicit list, because
    an omission here does not fail loudly: the reference simply stays bare and
    the build error names one symbol at a time.
    """
    symbols = set()
    for path in pathlib.Path("internal", domain).glob("*.go"):
        if path.name.endswith("_test.go"):
            continue
        text = path.read_text()
        symbols |= set(re.findall(r"^func ([A-Z]\w*)\(", text, re.M))
        symbols |= set(re.findall(r"^type ([A-Z]\w*)", text, re.M))
        symbols |= set(re.findall(r"^(?:var|const) ([A-Z]\w*)", text, re.M))
        for block in re.findall(r"^(?:var|const) \(\n(.*?)^\)", text, re.S | re.M):
            symbols |= set(re.findall(r"^\t([A-Z]\w*)\s*(?:=|\w)", block, re.M))
    return symbols


def requalify_rest(domain, renames, exported):
    """Point every reference outside the domain at the new package.

    Two shapes need handling: files that were in package service refer to the
    symbol bare, everything else refers to it as service.Symbol.
    """
    targets = [
        p for p in glob.glob("internal/**/*.go", recursive=True) + glob.glob("cmd/**/*.go", recursive=True)
        if f"/{domain}/" not in p and "/ent/" not in p and "/openapi/" not in p
    ]
    for path in map(pathlib.Path, targets):
        original = path.read_text()
        text = original
        in_service = re.search(r"^package service$", text, flags=re.M) is not None
        for old, new in renames.items():
            text = re.sub(rf"\bservice\.{re.escape(old)}\b", f"{domain}.{new}", text)
            if in_service:
                text = re.sub(rf"(?<![\w.]){re.escape(old)}\b", f"{domain}.{new}", text)
        for sym in exported:
            if sym in renames:
                continue
            # service.Symbol is unambiguous, so always safe to rewrite.
            text = re.sub(rf"\bservice\.{re.escape(sym)}\b", f"{domain}.{sym}", text)
            # A bare identifier is not. Rewriting a short, common name such as
            # Role also hits struct field declarations and composite-literal
            # keys, which is a syntax error rather than a wrong reference — but
            # a longer name like Config would be silently miscompiled into a
            # reference to the wrong package. Leave short names for the build
            # error to point at.
            if in_service and len(sym) > SAFE_BARE_REWRITE_LEN:
                text = re.sub(rf"(?<![\w.]){re.escape(sym)}\b", f"{domain}.{sym}", text)
        if text == original:
            continue
        if re.search(rf"\b{domain}\.\w", re.sub(r"//.*", "", text)) and f'"{MODULE}/internal/{domain}"' not in text:
            block = re.search(r"^import \(\n(.*?)^\)\n", text, re.S | re.M)
            if block:
                text = text[:block.end(1)] + f'\t"{MODULE}/internal/{domain}"\n' + text[block.end(1):]
        path.write_text(text)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("domain")
    parser.add_argument("stems", nargs="+")
    parser.add_argument("--rename", action="append", default=[], metavar="Old=New")
    args = parser.parse_args()

    renames = dict(pair.split("=", 1) for pair in args.rename)
    moved = move_files(args.domain, args.stems)
    rewrite_moved(moved, args.domain, renames)
    requalify_rest(args.domain, renames, exported_symbols(args.domain))
    print(f"moved {len(moved)} files into internal/{args.domain}")


if __name__ == "__main__":
    main()
