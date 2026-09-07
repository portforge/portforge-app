#!/usr/bin/env python3
"""Derive the shipped MediaItems catalog from the full one.

The full catalog mirrors Redump/No-Intro — every dump for every console. The app
only ever needs the ports and the ROMs those ports depend on, which is under 1% of
it. This walks the ports, follows their references, and writes out just that
closure, so nobody has to hand-maintain which ROM items ship.

Run it again whenever a port is added or its romDependencies change; adding a port
that needs a five-release, three-disc game picks up all fifteen ROM items on its
own.

    derive-catalog.py --source ../mediaitems-full --out ./mediaitems
    derive-catalog.py --source ./mediaitems --out /tmp/derived --dry-run
"""

import argparse
import json
import shutil
import sys
from collections import defaultdict
from pathlib import Path

ITEM_FILE = ".mediaitem.json"
# Copied verbatim from the catalog root.
ROOT_FILES = ["README.md"]


def load_item(d: Path):
    f = d / ITEM_FILE
    if not f.is_file():
        return None
    try:
        return json.loads(f.read_text())
    except json.JSONDecodeError as e:
        print(f"  ! {d.name}/{ITEM_FILE}: {e}", file=sys.stderr)
        return None


def dir_size(p: Path) -> int:
    return sum(f.stat().st_size for f in p.rglob("*") if f.is_file())


def human(n: int) -> str:
    for unit in ("B", "KB", "MB", "GB"):
        if n < 1024 or unit == "GB":
            return f"{n:.1f} {unit}" if unit != "B" else f"{n} B"
        n /= 1024


def index_by_title(source: Path, item_type: str):
    """Map both the folder name and the title it carries to that folder.

    A ROM item's folder is "<title> · <PLATFORM>" while a dependency names only the
    title, so both spellings have to resolve.
    """
    out = defaultdict(list)
    d = source / item_type
    if not d.is_dir():
        return out
    for p in sorted(d.iterdir()):
        if not p.is_dir():
            continue
        out[p.name].append(p)
        base = p.name.rsplit(" · ", 1)[0]
        if base != p.name:
            out[base].append(p)
    return out


def derive(source: Path, out: Path, dry_run: bool):
    versions_dir = source / "VideoGameVersion"
    if not versions_dir.is_dir():
        sys.exit(f"no VideoGameVersion directory under {source}")

    keep = defaultdict(set)        # item_type -> {Path}
    dangling = []                  # references with no item behind them
    indexes = {}

    versions = [d for d in sorted(versions_dir.iterdir()) if d.is_dir()]
    for d in versions:
        item = load_item(d)
        if item is None:
            continue
        keep["VideoGameVersion"].add(d)

        # The base game this port is of.
        vg = item.get("videoGame")
        if vg and vg.get("_itemTitle"):
            idx = indexes.setdefault("VideoGame", index_by_title(source, "VideoGame"))
            hits = idx.get(vg["_itemTitle"], [])
            if hits:
                keep["VideoGame"].update(hits)
            else:
                dangling.append(("VideoGame", vg["_itemTitle"], d.name))

        # Every ROM the port can be built from. These are alternatives, so all of
        # them ship — the user may own any one.
        for dep in item.get("romDependencies") or []:
            t, title = dep.get("_itemType"), dep.get("title")
            if not t or not title:
                dangling.append((t or "?", title or "?", d.name))
                continue
            idx = indexes.setdefault(t, index_by_title(source, t))
            hits = idx.get(title, [])
            if hits:
                keep[t].update(hits)
                if len(hits) > 1:
                    print(f"  ! {title!r} matches {len(hits)} folders under {t}; keeping all")
            else:
                dangling.append((t, title, d.name))

    # ---- report ----------------------------------------------------------
    print(f"source: {source}")
    all_types = sorted({p.name for p in source.iterdir() if p.is_dir() and not p.name.startswith(".")})
    total_before = total_after = 0
    print(f"\n{'type':<20} {'kept':>7} {'of':>7}   {'size kept':>10} {'was':>10}")
    for t in all_types:
        present = [p for p in (source / t).iterdir() if p.is_dir()]
        kept = keep.get(t, set())
        before = sum(dir_size(p) for p in present)
        after = sum(dir_size(p) for p in kept)
        total_before += before
        total_after += after
        print(f"{t:<20} {len(kept):>7} {len(present):>7}   {human(after):>10} {human(before):>10}")
    print(f"{'-' * 60}")
    pct = 100 * (1 - total_after / total_before) if total_before else 0
    print(f"{'TOTAL':<20} {sum(len(v) for v in keep.values()):>7} "
          f"{sum(len(list((source / t).iterdir())) for t in all_types):>7}   "
          f"{human(total_after):>10} {human(total_before):>10}   ({pct:.1f}% smaller)")

    if dangling:
        print(f"\n{len(dangling)} dangling reference(s) — nothing to copy, and nothing "
              f"pruning can fix:")
        for t, title, src in dangling:
            print(f"   {t}/{title!r}  (referenced by {src})")

    if dry_run:
        print("\n--dry-run: nothing written")
        return 0

    # ---- write -----------------------------------------------------------
    if out.exists():
        if any(out.iterdir()):
            sys.exit(f"refusing to write into non-empty {out}")
    out.mkdir(parents=True, exist_ok=True)

    written = 0
    for t, paths in keep.items():
        (out / t).mkdir(parents=True, exist_ok=True)
        for p in sorted(paths):
            shutil.copytree(p, out / t / p.name)
            written += 1
    for name in ROOT_FILES:
        f = source / name
        if f.is_file():
            shutil.copy2(f, out / name)

    print(f"\nwrote {written} items to {out}")

    # ---- self-check ------------------------------------------------------
    # The derived catalog has to stand on its own: every dependency the ports
    # declare must resolve inside it, or a build would fail at the copy step.
    bad = 0
    for d in sorted((out / "VideoGameVersion").iterdir()):
        item = load_item(d)
        if item is None:
            continue
        for dep in item.get("romDependencies") or []:
            t, title = dep.get("_itemType"), dep.get("title")
            idx = index_by_title(out, t)
            if not idx.get(title):
                print(f"  ! UNRESOLVED in output: {t}/{title!r} for {d.name}")
                bad += 1
    if bad:
        print(f"self-check FAILED: {bad} unresolved dependencies")
        return 1
    print("self-check passed: every romDependency resolves inside the derived catalog")
    return 0


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--source", required=True, type=Path, help="the full catalog")
    ap.add_argument("--out", type=Path, help="where to write the derived catalog")
    ap.add_argument("--dry-run", action="store_true", help="report only, write nothing")
    a = ap.parse_args()
    if not a.dry_run and not a.out:
        ap.error("--out is required unless --dry-run is given")
    sys.exit(derive(a.source, a.out or Path("/dev/null"), a.dry_run))


if __name__ == "__main__":
    main()
