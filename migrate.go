package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"portforge/storageunits"
)

// This file is a one-time migration of user data, not a compatibility layer.
// Nothing here is a fallback that PortForge keeps honouring: each function moves
// what it finds into the current arrangement, records that it has done so, and
// then does nothing on every subsequent launch. The whole file can be deleted
// once no installation predates it, and deleting it will change no behaviour for
// anyone already migrated.

// legacyTypeDirs maps every superseded ItemType directory name to its current
// one. A user's imported files live in folders named after these types, so a
// rename in the catalog strands whatever they have already imported.
//
// Three generations are covered because the names changed twice in three weeks:
// the original bare form, the short-lived *GameRom form, and the current
// [Platform][Medium][Form]. Several map onto one — PS1 dumps passed through both
// earlier spellings — which the merge below handles.
var legacyTypeDirs = map[string]string{
	"VideoGameVersion": "VideoGameFanPort",

	"N64Rom":     "N64CartRom",
	"N64GameRom": "N64CartRom",
	"NESRom":     "NESCartRom",
	"NESGameRom": "NESCartRom",
	"GBRom":      "GBCartRom",
	"GBCRom":     "GBCCartRom",

	"PS1Rom":         "PS1DiscImage",
	"PS1GameRom":     "PS1DiscImage",
	"Xbox360Rom":     "Xbox360DiscImage",
	"Xbox360GameRom": "Xbox360DiscImage",
}

// migrateLegacyTypeDirs renames a storage root's ItemType folders to their
// current names.
//
// It is idempotent and never destructive: a folder is renamed outright only when
// the new name is free, and when both exist the items are moved across one at a
// time, leaving anything that would collide where it is rather than overwriting a
// file the user has.
func migrateLegacyTypeDirs(root string) {
	if root == "" {
		return
	}
	for old, current := range legacyTypeDirs {
		oldDir := filepath.Join(root, old)
		if _, err := os.Stat(oldDir); err != nil {
			continue
		}
		newDir := filepath.Join(root, current)
		if _, err := os.Stat(newDir); os.IsNotExist(err) {
			_ = os.Rename(oldDir, newDir)
			continue
		}

		// Both exist: a partial migration, or two superseded names converging on
		// one current name. Move item by item so a collision costs nothing.
		entries, err := os.ReadDir(oldDir)
		if err != nil {
			continue
		}
		moved := 0
		for _, e := range entries {
			dest := filepath.Join(newDir, e.Name())
			if _, err := os.Stat(dest); err == nil {
				continue // already there under the new name; leave the old copy alone
			}
			if os.Rename(filepath.Join(oldDir, e.Name()), dest) == nil {
				moved++
			}
		}
		if moved == len(entries) {
			_ = os.Remove(oldDir) // succeeds only when empty
		}
	}
}

// legacySettingsFile is the private settings file PortForge kept before storage
// locations moved to the shared list. Its only surviving field is the library
// path.
type legacySettingsFile struct {
	DataPath string `json:"dataPath"`
}

// migrateLibraryPath moves a pre-existing library folder into the shared
// StorageUnit list.
//
// This is not seeding. Nothing is proposed on the user's behalf: the folder being
// migrated is one they chose explicitly in an earlier version, and losing it on
// upgrade would be a silent regression. The distinction that matters is that a
// migrated folder was already the user's answer to this same question.
//
// The old settings file is its own "not yet migrated" flag: consuming it renames
// it out of the way, so this runs exactly once without needing new state to
// record that it has. A user who later empties the list keeps it empty, because
// there is nothing left to migrate.
func migrateLibraryPath(units *storageunits.Manager, oldSettingsPath string) {
	if units == nil || oldSettingsPath == "" {
		return
	}
	data, err := os.ReadFile(oldSettingsPath)
	if err != nil {
		return
	}
	var s legacySettingsFile
	if json.Unmarshal(data, &s) != nil || s.DataPath == "" {
		// Unreadable or empty: retire it anyway. There is nothing to carry over,
		// and leaving it would retry this on every launch.
		_ = os.Rename(oldSettingsPath, oldSettingsPath+".migrated")
		return
	}

	// A duplicate is not a failure here — it means another launch, or the user,
	// already added this folder — so the file is retired either way.
	if _, err := units.Add(s.DataPath); err != nil {
		if _, statErr := os.Stat(s.DataPath); statErr != nil {
			return // the folder is gone right now; try again next launch
		}
	}
	_ = os.Rename(oldSettingsPath, oldSettingsPath+".migrated")
}

// legacySettingsPath is where that file used to live.
func legacySettingsPath() string {
	dir, err := configDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "settings.json")
}

// migrate applies every one-time upgrade, in the order they depend on each
// other, and is the whole of what startup does about them.
//
// It exists as a function so a test can run the real sequence rather than a
// hand-copied imitation of it. A test that reproduces these calls itself proves
// only that the migrations work when called — it cannot notice one going missing
// from startup, which is the failure that actually reaches a user.
func (a *App) migrate(legacySettings string) {
	// Carry a library folder chosen in an earlier version into the shared list
	// before deriving anything from it, so an upgrade does not present the
	// first-run screen to someone who already configured PortForge.
	if a.units != nil {
		migrateLibraryPath(a.units, legacySettings)
		a.syncDataPath()
	}

	// The ItemType folders under the storage root were renamed twice; a user's
	// imported files are filed by those names. Only this program's own root is
	// touched — the other units in the shared list may belong to programs whose
	// layout is not ours to rewrite.
	migrateLegacyTypeDirs(a.dataPath)

	// The same folder names appear again inside the update-detection snapshots,
	// which mirror the catalog's layout. Missing this one fails silently rather
	// than loudly: ScanUserLibraryUpdates reads only the current type names,
	// finds nothing under the superseded ones, and every port installed before
	// the rename quietly stops reporting catalog updates.
	migrateLegacyTypeDirs(a.userLibraryPath())
}
