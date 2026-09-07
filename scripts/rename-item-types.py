#!/usr/bin/env python3
"""Rename MediaItem types across a catalog.

Renames the type directories and rewrites every reference to them: an item's own
_itemType, and the _itemType of each entry in a VideoGameVersion's
romDependencies. The item folders inside each type directory are untouched — only
the type name changes.

Idempotent: a catalog already renamed is left alone, so it is safe to re-run.

    rename-item-types.py --root ./mediaitems --dry-run
    rename-item-types.py --root ./mediaitems
"""

import argparse
import re
import sys
from pathlib import Path

# An ItemFile type names the medium a dump came off, so a cartridge dump is a
# CartRom and a disc dump a DiscImage — [Platform][Medium][Form] throughout.
# Dropping the medium would leave the name over-broad: an "N64Rom" could just as
# well be the console's own firmware, which belongs to a different type entirely.
RENAMES = {
    "N64Rom": "N64CartRom",
    "NESRom": "NESCartRom",
    "GBRom": "GBCartRom",
    "GBCRom": "GBCCartRom",
}

ITEM_FILE = ".mediaitem.json"


ITEM_TYPE_RE = re.compile(
    r'("_itemType"\s*:\s*")(' + "|".join(map(re.escape, RENAMES)) + r')(")'
)


def rewrite_json(path: Path, dry_run: bool) -> list[str]:
    """Rewrite every _itemType naming a renamed type, in place.

    This edits the raw text rather than round-tripping through json, so a file
    whose only change is one type name has a one-line diff. Reformatting 15,000
    files to change a single field in each would bury the change in noise. The
    pattern matches an item's own _itemType and the ones inside romDependencies
    alike, which is what we want — both refer to the same types.
    """
    try:
        raw = path.read_text()
    except OSError as e:
        print(f"  ! {path}: {e}", file=sys.stderr)
        return []

    changes = []

    def sub(m):
        changes.append(f"{m.group(2)} -> {RENAMES[m.group(2)]}")
        return m.group(1) + RENAMES[m.group(2)] + m.group(3)

    out = ITEM_TYPE_RE.sub(sub, raw)
    if changes and not dry_run:
        path.write_text(out)
    return changes


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--root", required=True, type=Path, help="the catalog directory")
    ap.add_argument("--dry-run", action="store_true")
    a = ap.parse_args()
    root, dry = a.root, a.dry_run

    if not root.is_dir():
        sys.exit(f"not a directory: {root}")

    # 1. Directories. Refuse rather than merge if the destination already exists
    #    with different contents; that is a half-finished migration, not a re-run.
    moved = []
    for old, new in RENAMES.items():
        src, dst = root / old, root / new
        if not src.is_dir():
            continue
        if dst.exists():
            sys.exit(f"both {old} and {new} exist under {root}; resolve by hand")
        moved.append((old, new, sum(1 for _ in src.iterdir())))
        if not dry:
            src.rename(dst)

    # 2. Every item file under every type directory, including the ones just moved
    #    and the version items whose romDependencies point at them.
    edited = files = 0
    for type_dir in sorted(p for p in root.iterdir() if p.is_dir() and not p.name.startswith(".")):
        for item_dir in sorted(p for p in type_dir.iterdir() if p.is_dir()):
            f = item_dir / ITEM_FILE
            if not f.is_file():
                continue
            files += 1
            changes = rewrite_json(f, dry)
            if changes:
                edited += 1
                if edited <= 8:
                    print(f"  {type_dir.name}/{item_dir.name}")
                    for c in changes[:3]:
                        print(f"      {c}")
                    if len(changes) > 3:
                        print(f"      … and {len(changes) - 3} more")

    if edited > 8:
        print(f"  … and {edited - 8} more item files")

    print(f"\ndirectories renamed: {len(moved)}")
    for old, new, n in moved:
        print(f"   {old:<14} -> {new:<18} ({n} items)")
    print(f"item files scanned:  {files}")
    print(f"item files changed:  {edited}")
    if dry:
        print("\n--dry-run: nothing written")


if __name__ == "__main__":
    main()
