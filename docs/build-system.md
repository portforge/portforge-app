# PortForge Build System

All games require custom configurations before they can be played. PortForge supports this through an `.install.json` file placed alongside `.mediaitem.json` in a game's MediaItem folder.

**Platform availability** is determined solely by `.install.json`. If a spec exists whose `targetPlatforms` matches the current platform, the game is considered available.

---

## `.install.json` structure

The file contains a JSON **array** of install specs. PortForge uses the first spec whose `targetPlatforms` matches the current platform.

```json
[{
  "targetPlatforms": ["Linux"],
  "dependencies": ["make", "gcc", "python3"],
  "args": {
    "romVersion": {
      "type": "choice",
      "label": "ROM region",
      "options": [
        { "value": "eu", "label": "European (PAL)", "romTitle": "Super Mario 64 (Europe) (En,Fr,De)" },
        { "value": "us", "label": "US (NTSC)",       "romTitle": "Super Mario 64 (USA)" }
      ]
    }
  },
  "steps": [ ... ]
}]
```

| Field             | Type     | Description                                                                                                 |
| ----------------- | -------- | ----------------------------------------------------------------------------------------------------------- |
| `targetPlatforms` | string[] | Platforms this spec applies to: `"Linux"`, `"Mac"`, `"Windows"`. Omit to match all platforms.               |
| `dependencies`    | string[] | Command names that must be on `PATH`. PortForge aborts with a clear error if any are missing.               |
| `args`            | object   | Named user-configurable parameters shown in the UI before install (see [Args](#args)). Omit if none needed. |
| `steps`           | object[] | Ordered steps executed during install (see [Steps](#steps)).                                                |

---

## Args

Args let users configure installation variants before the install begins. PortForge presents a prompt in the UI for each arg and enables the Install button once all required selections are made.

### `type: "choice"`

Presents labeled option buttons.

```json
"romVersion": {
  "type": "choice",
  "label": "ROM region",
  "options": [
    { "value": "eu", "label": "European (PAL)", "romTitle": "Super Mario 64 (Europe) (En,Fr,De)" },
    { "value": "us", "label": "US (NTSC)",       "romTitle": "Super Mario 64 (USA)" }
  ]
}
```

| Field                | Description                                                                                                                                                          |
| -------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `label`              | Shown above the option buttons in the UI.                                                                                                                            |
| `options[].value`    | The value substituted into steps via `$argName` / `${argName}`.                                                                                                      |
| `options[].label`    | Human-readable label shown on the button.                                                                                                                            |
| `options[].romTitle` | Optional. Title of the `romDependency` required for this option. Used to show ✓/✗ ROM status per option and to resolve the ROM file during a `copy from:"rom"` step. |

PortForge automatically pre-selects an option if it is the only one whose ROM is present in the library.

### `type: "string"`

Presents a free-text input field. The typed value is available as `$argName` in steps.

---

## Arg interpolation

Any string field in a step supports shell-style `$varName` or `${varName}` substitution. PortForge replaces these with the user's selected arg values before executing each step.

```json
{ "step": "make", "args": ["VERSION=$romVersion", "TEXTURE_FIX=1"] }
{ "step": "cd",   "path": "build/Render96ex-${romVersion}" }
```

Use `${varName}` when the variable is immediately followed by more characters (e.g. `${romVersion}_pc`).

---

## Steps

Steps execute in order. The **working directory** starts at the root of the version's MediaItem folder (`mediaitems/VideoGameVersion/<ItemTitle>/`) and is updated by `cd` steps. All relative paths in steps are resolved against the current working directory at the time the step runs.

PortForge aborts on the first failure and preserves the folder for debugging. The UI offers a "Delete build folder" button for cleanup. On success, the `build/` subdirectory is deleted automatically.

---

### `cd` — change working directory

```json
{ "step": "cd", "path": "build/Render96ex-master" }
```

Updates the working directory for all subsequent steps. Relative paths are resolved from the current working directory, so `..` works as expected.

| Field  | Required | Description                                  |
| ------ | -------- | -------------------------------------------- |
| `path` | yes      | Path to change into. Supports interpolation. |

---

### `fetch` — download a remote file

```json
{ "step": "fetch", "url": "https://example.com/source.zip", "dest": "build/source.zip" }
```

| Field  | Required | Description                                                                                                                       |
| ------ | -------- | --------------------------------------------------------------------------------------------------------------------------------- |
| `url`  | yes      | URL to download. Supports interpolation.                                                                                          |
| `dest` | yes      | Destination path relative to the current working directory. Parent directories are created automatically. Supports interpolation. |

---

### `extract` — unpack an archive

```json
{ "step": "extract", "src": "build/source.zip", "dest": "build/" }
{ "step": "extract", "src": "build/models.7z",  "dest": "build/models/" }
```

| Field  | Required | Description                                                                                                                    |
| ------ | -------- | ------------------------------------------------------------------------------------------------------------------------------ |
| `src`  | yes      | Path to the archive file, relative to the current working directory. Supports interpolation.                                   |
| `dest` | yes      | Directory to extract into, relative to the current working directory. Created automatically if absent. Supports interpolation. |

Supported formats: `.zip` (extracted natively), `.7z` (requires `7z` on `PATH`). The format is inferred from the file extension. File permissions stored in the archive (e.g. execute bits) are preserved for zip.

---

### `copy` — copy a file or directory

Two modes depending on whether `from` is set.

**ROM copy** — copies a ROM from the library into the build tree:

```json
{ "step": "copy", "from": "rom", "arg": "romVersion", "dest": "baserom.${romVersion}.z64" }
```

**Local copy** — copies a file or directory:

```json
{ "step": "copy", "src": "assets/", "dest": "build/game/assets/" }
```

| Field  | Required | Description                                                                                                                                              |
| ------ | -------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `from` | no       | Set to `"rom"` to copy from the ROM library. Omit for local copies.                                                                                      |
| `arg`  | no       | _(ROM mode)_ Name of a `choice` arg whose selected option's `romTitle` identifies the ROM to copy.                                                       |
| `src`  | no       | _(ROM mode, no `arg`)_ Interpolated string matched against `romDependency` titles. _(Local mode)_ Source path relative to the current working directory. |
| `dest` | yes      | Destination path relative to the current working directory. Parent directories are created automatically. Supports interpolation.                        |

Directories are copied recursively. If `arg` is omitted in ROM mode, PortForge searches all `choice` args for one whose selected option has a `romTitle`.

---

### `move` — move a file or directory

```json
{ "step": "move", "src": "build/Render96ex-master", "dest": "build/source" }
```

| Field  | Required | Description                                                                                                                       |
| ------ | -------- | --------------------------------------------------------------------------------------------------------------------------------- |
| `src`  | yes      | Source path relative to the current working directory. Supports interpolation.                                                    |
| `dest` | yes      | Destination path relative to the current working directory. Parent directories are created automatically. Supports interpolation. |

Attempts an atomic rename first; falls back to copy + delete if source and destination are on different filesystems. Directories are handled recursively.

---

### `make` — run `make`

```json
{
  "step": "make",
  "args": ["--directory=.build/src", "VERSION=$romVersion", "TEXTURE_FIX=1"],
  "env": { "CC": "gcc" }
}
```

| Field  | Required | Description                                                                  |
| ------ | -------- | ---------------------------------------------------------------------------- |
| `args` | no       | Arguments passed to `make`. Each element supports interpolation.             |
| `env`  | no       | Key/value pairs merged on top of the current environment for this step only. |

Runs `make` in the current working directory. Standard output and standard error are streamed live to the PortForge UI.

> **Security note:** PortForge intentionally does not provide a generic `run` step. Shell interpreters (`bash`, `sh`, `python`, `node`, …) are not step types. The `make` step, `extract` step, and `fetch` step cover the vast majority of source-port build processes without allowing a malicious `.install.json` to execute arbitrary commands. The remaining trust surface is the URLs passed to `fetch`; checksum pinning is planned as a future hardening step.

---

### `createDir` — create a directory

```json
{ "step": "createDir", "path": "build/output" }
```

Creates the directory and all missing parent directories (`mkdir -p`).

| Field  | Required | Description                                                             |
| ------ | -------- | ----------------------------------------------------------------------- |
| `path` | yes      | Path relative to the current working directory. Supports interpolation. |

---

### `touch` — create an empty file

```json
{ "step": "touch", "path": "config/portable.txt" }
```

Creates an empty file at the given path, including any missing parent directories. Useful for marker files that build systems or games look for at startup.

| Field  | Required | Description                                                             |
| ------ | -------- | ----------------------------------------------------------------------- |
| `path` | yes      | Path relative to the current working directory. Supports interpolation. |

---

### `deletePath` — delete a file or directory

```json
{ "step": "deletePath", "path": "build/source.zip" }
```

Removes the file or directory at the given path (`rm -rf`). Silently succeeds if the path does not exist.

| Field  | Required | Description                                                             |
| ------ | -------- | ----------------------------------------------------------------------- |
| `path` | yes      | Path relative to the current working directory. Supports interpolation. |

---

### `defineExecutable` — register a launch target

```json
{ "step": "defineExecutable", "executable": "install/sm64.eu.f3dex2e", "title": "Play" }
```

Records the path of a launchable binary. Does not move or copy any files — the install author is responsible for placing files in their final location using `move`, `copy`, and `deletePath` steps before calling this.

| Field        | Required | Description                                                                                                                         |
| ------------ | -------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| `executable` | yes      | Path to the binary relative to the version's MediaItem folder. PortForge applies `chmod +x` on non-Windows. Supports interpolation. |
| `title`      | no       | Label shown on the Play button. Defaults to the executable filename.                                                                |

Multiple `defineExecutable` steps are allowed. The first becomes the default Play button; the rest appear in a dropdown.

---

## Directory layout

```
mediaitems/VideoGameVersion/<ItemTitle>/
├── .mediaitem.json    ← game metadata; download URL optional for build installs
├── .install.json      ← build spec (this file)
├── .state.json        ← written by PortForge after a successful install
└── install/           ← conventional destination for game files (not enforced)
    └── ...
```

There is no automatically created or deleted build directory. The install author manages the full layout using `createDir`, `move`, `deletePath`, and other steps.

---

## Full example — Render96 (SM64 port)

```json
[
  {
    "targetPlatforms": ["Linux"],
    "dependencies": ["make", "gcc", "python3", "sdl2-config", "pkg-config", "unzip"],
    "args": {
      "romVersion": {
        "type": "choice",
        "label": "ROM region",
        "options": [
          { "value": "eu", "label": "European (PAL)", "romTitle": "Super Mario 64 (Europe) (En,Fr,De)" },
          { "value": "us", "label": "US (NTSC)", "romTitle": "Super Mario 64 (USA)" }
        ]
      }
    },
    "steps": [
      { "step": "createDir", "path": ".build" },
      { "step": "fetch", "url": "https://github.com/Render96/Render96ex/archive/…/source.zip", "dest": ".build/source.zip" },
      { "step": "extract", "src": ".build/source.zip", "dest": ".build" },
      { "step": "deletePath", "path": ".build/source.zip" },
      { "step": "copy", "from": "rom", "arg": "romVersion", "dest": ".build/Render96ex-master/baserom.${romVersion}.z64" },
      { "step": "make", "args": ["--directory=.build/Render96ex-master", "VERSION=${romVersion}", "TEXTURE_FIX=1", "DISCORDRPC=0"] },
      { "step": "move", "src": ".build/Render96ex-master/build/${romVersion}_pc", "dest": "install" },
      { "step": "deletePath", "path": ".build" },
      { "step": "defineExecutable", "executable": "install/sm64.${romVersion}.f3dex2e", "title": "Render96 EX" }
    ]
  }
]
```
