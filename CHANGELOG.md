# Changelog

All notable changes to PortForge are recorded here.

## v0.2.0-alpha — 2026-09-07

The largest release so far. PortForge narrowed from a do-everything app into one program
in a suite of MediaItem tools, gained shared storage locations, a rebuilt interface, and
support for building ports across platforms and versions.

**Upgrading is automatic.** Item type folders are renamed and an existing library folder is
carried into the new shared storage list on first launch. Nothing needs to be moved by hand,
and no configuration is lost.

### Added

**Storage locations.** The single library folder is now a list of storage locations shared
with every other program in the suite, stored alongside the PortForge config rather than
inside it. Locations can be added, renamed, reordered and removed, each showing its free
space and the share PortForge uses. The first location in the list is where PortForge writes;
order is priority, and an unplugged drive is reported as unavailable rather than quietly
skipped, so a library never relocates itself behind your back.

**A ROMs page.** Every ROM the ports in your catalog can use, grouped by port. Each
requirement expands to list all of its accepted dumps with filename, size, format and
checksums, and marks the ones you already have. Files are matched by checksum, so a rename
or a different extension makes no difference.

**Cross-platform and multi-version builds.** Ports can declare several versions and several
target platforms, with a picker for each on the game page. Architecture is part of the
platform name (`Mac-arm64`, `Windows-x64`), so a port can target one architecture without
claiming the other. A port's recommended version is honoured where it differs from its
newest.

**Build output.** The game page shows live build output while a port installs and keeps it
on screen after a failure, which is when it is actually read. A banner tracks an install
from anywhere in the app, with progress and a stop button. Full output is still written to
`install.log` in the game's data folder.

**Install spec validation.** A spec that uses a variable it never declares is now rejected
before anything is downloaded or compiled, naming the variable. Previously such a spec built
for as long as it took to reach the offending step and then failed with whatever the build
tool made of the literal text.

**Server mode.** `portforge -server` serves the interface over HTTP and prints a URL instead
of opening a window, for systems whose native web view renders it poorly. `-addr` chooses
the address. This is a workaround rather than a supported mode, and choosing folders still
requires the desktop window.

**New ports in the catalog:** Gen1Recomp and re:Blue, along with Game Boy, Game Boy Color
and PlayStation ROM support.

### Changed

**Rebuilt interface.** A text sidebar replaces the icon rail. Cover art is no longer written
over: status, label and version moved below the title into their own row. Artwork is rounded
less than the surrounding chrome, tiles share a uniform shape so titles line up across a row,
and non-standard art letterboxes rather than being cropped. Settings was rebuilt on the
shared palette with real storage figures and catalog information. New typefaces throughout
(Outfit and IBM Plex Mono).

**Artwork is resized before it reaches the interface**, with a thumbnail cache. Catalog
artwork is stored at print resolution because it is shared across the suite; handing it
straight to the web view was making the whole app stutter.

**ROM requirements are a list of requirements, each satisfied by any one of its options.**
A three-disc game asks for each disc and accepts any region for each, rather than
enumerating every valid combination. A requirement marked required is one the port cannot be
*played* without — increasingly ports build without ROMs and extract their assets on first
run — so a port can install and launch with requirements outstanding.

**Item types were renamed** to name the platform, medium and form: `VideoGameFanPort` for
ports, and `N64CartRom`, `NESCartRom`, `GBCartRom`, `GBCCartRom`, `PS1DiscImage`,
`Xbox360DiscImage` for ROMs. Existing folders are renamed on first launch.

**Install specs are now `.forge.json`** and may carry file-level defaults shared by every
build in the file. The build engine is [Forge](https://github.com/zamiba/forge), now a
published dependency rather than a local one, so PortForge builds from a fresh clone.

**The catalog shrank from 1.9 GB to 27 MB** by keeping only the ROMs that a port in the
catalog actually references.

### Removed

- **Disc dumping** — optical drive detection, redumper integration and the dumper screen.
- **Emulator launching** — DuckStation integration and ROM launching. Launching installed
  ports is unaffected.
- **The standalone ROM browser**, replaced by the ROMs page described above.
- **PortForge's private settings file.** Its only setting was the library folder, which now
  comes from the shared storage list; an existing one is consumed during the upgrade.

### Fixed

- Artwork failed to load after the item type rename.
- A banner at the top of the window pushed the bottom of the page out of view.
- The Now Playing screen did not cover the sidebar, leaving navigation lit and clickable
  underneath it.
- The build output header showed a bare number that was really the display cap, so it always
  read `500`; it now says how many lines are shown and that older ones were trimmed.
- ROM scanning only looked at one item type, so ROMs filed under any other platform were
  never found.
- Super Mario 64 Render96 could not be installed: its spec used an undeclared variable, and
  separately declared no version.
- Settings added a duplicate progress listener every time it was opened.
- Removed the hover animation on library covers, which was the most visible stutter on Linux.

---

Earlier releases are listed at
<https://github.com/zamiba/portforge-app/releases>.
