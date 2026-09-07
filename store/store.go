package store

import (
	"crypto/md5"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
	"portforge/metadata"
	"portforge/models"
)

// Store wraps a SQLite connection and provides all persistence operations
// for the PortForge library index.
type Store struct {
	db           *sql.DB
	needsRebuild bool
}

// Open opens (or creates) the SQLite database at path and applies the schema.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path+"?_journal=WAL&_timeout=5000&_foreign_keys=on")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	// A stale schema is discarded before the tables are created, so CREATE IF NOT
	// EXISTS never meets a table of the wrong shape.
	if err := s.resetIfStale(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store schema reset: %w", err)
	}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store migration: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS media_items (
			item_title      TEXT PRIMARY KEY,
			item_type       TEXT NOT NULL,
			title           TEXT,
			platforms       TEXT,
			artwork         TEXT,
			vg_item_title   TEXT,
			vg_title        TEXT,
			vg_release_year INTEGER,
			platform        TEXT,
			has_update      INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS rom_formats (
			id          INTEGER PRIMARY KEY,
			item_title  TEXT NOT NULL REFERENCES media_items(item_title) ON DELETE CASCADE,
			filename    TEXT,
			format      TEXT,
			ext         TEXT,
			filesize    INTEGER,
			md5         TEXT,
			local_path  TEXT
		);
		CREATE INDEX IF NOT EXISTS rom_formats_item  ON rom_formats(item_title);
		CREATE INDEX IF NOT EXISTS rom_formats_md5   ON rom_formats(md5);
		CREATE TABLE IF NOT EXISTS version_rom_deps (
			version_item_title TEXT NOT NULL REFERENCES media_items(item_title) ON DELETE CASCADE,
			slot               INTEGER NOT NULL,
			slot_name          TEXT NOT NULL DEFAULT '',
			slot_required      INTEGER NOT NULL DEFAULT 1,
			rom_item_title     TEXT NOT NULL REFERENCES media_items(item_title) ON DELETE CASCADE,
			PRIMARY KEY (version_item_title, slot, rom_item_title)
		);
		CREATE INDEX IF NOT EXISTS idx_media_items_type_title ON media_items(item_type, title);
	`)
	return err
}

// schemaVersion is bumped whenever the shape of a table changes. Everything in
// this database is derived — the catalog supplies the items, SyncUserROMs the
// local paths, ScanUserLibraryUpdates the update flags — so a version mismatch
// is resolved by discarding the lot and rebuilding rather than by migrating
// column by column.
const schemaVersion = 2

// resetIfStale drops every table when the stored schema version does not match,
// and reports that a rebuild is owed. A fresh database reads version 0 and has
// no tables, so it costs nothing there.
func (s *Store) resetIfStale() error {
	var found int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&found); err != nil {
		return err
	}
	if found == schemaVersion {
		return nil
	}
	if _, err := s.db.Exec(`
		DROP TABLE IF EXISTS version_rom_deps;
		DROP TABLE IF EXISTS rom_formats;
		DROP TABLE IF EXISTS media_items;
	`); err != nil {
		return err
	}
	if _, err := s.db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion)); err != nil {
		return err
	}
	s.needsRebuild = found != 0 // nothing to rebuild for a database that was empty
	return nil
}

// NeedsCatalogRebuild reports whether opening the store discarded derived data
// that only RebuildCatalog can restore.
func (s *Store) NeedsCatalogRebuild() bool { return s.needsRebuild }

// IsEmpty returns true when the media_items table has no rows.
func (s *Store) IsEmpty() bool {
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM media_items`).Scan(&n)
	return n == 0
}

// ── Catalog rebuild ───────────────────────────────────────────────────────────

// RebuildCatalog scans metadataPath and repopulates media_items and rom_formats.
// Existing has_update flags are preserved via an upsert.
func (s *Store) RebuildCatalog(metadataPath string) error {
	versions, err := metadata.LoadAllVersions(metadataPath)
	if err != nil {
		return fmt.Errorf("load versions: %w", err)
	}
	roms, err := metadata.LoadAllRoms(metadataPath)
	if err != nil {
		return fmt.Errorf("load roms: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, v := range versions {
		platformsJSON, _ := json.Marshal(v.Platforms)
		artworkJSON, _ := json.Marshal(v.Artwork)

		var vgItemTitle, vgTitle *string
		var vgYear *int
		if v.VideoGame != nil {
			vgItemTitle = &v.VideoGame.ItemTitle
			vgTitle = &v.VideoGame.Title
			yr := v.VideoGame.ReleaseYear
			vgYear = &yr
		}

		_, err := tx.Exec(`
			INSERT INTO media_items
				(item_title, item_type, title, platforms, artwork,
				 vg_item_title, vg_title, vg_release_year, has_update)
			VALUES (?, ?, ?, ?, ?,  ?, ?, ?,
				COALESCE((SELECT has_update FROM media_items WHERE item_title = ?), 0))
			ON CONFLICT(item_title) DO UPDATE SET
				item_type       = excluded.item_type,
				title           = excluded.title,
				platforms       = excluded.platforms,
				artwork         = excluded.artwork,
				vg_item_title   = excluded.vg_item_title,
				vg_title        = excluded.vg_title,
				vg_release_year = excluded.vg_release_year
		`, v.ItemTitle, metadata.PortItemType, v.Title, string(platformsJSON), string(artworkJSON),
			vgItemTitle, vgTitle, vgYear, v.ItemTitle)
		if err != nil {
			return fmt.Errorf("upsert version %q: %w", v.ItemTitle, err)
		}
	}

	for _, r := range roms {
		// r.Artwork is already populated by LoadOneRom (which calls ScanArtworkDir
		// as a fallback when the JSON has no artwork entries).
		artworkJSON, _ := json.Marshal(r.Artwork)
		_, err := tx.Exec(`
			INSERT INTO media_items
				(item_title, item_type, title, platform, artwork, has_update)
			VALUES (?, ?, ?, ?, ?,
				COALESCE((SELECT has_update FROM media_items WHERE item_title = ?), 0))
			ON CONFLICT(item_title) DO UPDATE SET
				item_type = excluded.item_type,
				title     = excluded.title,
				platform  = excluded.platform,
				artwork   = excluded.artwork
		`, r.ItemTitle, r.ItemType, r.Title, r.Platform, string(artworkJSON), r.ItemTitle)
		if err != nil {
			return fmt.Errorf("upsert rom %q: %w", r.ItemTitle, err)
		}

		if _, err := tx.Exec(`DELETE FROM rom_formats WHERE item_title = ?`, r.ItemTitle); err != nil {
			return err
		}
		for _, f := range r.Formats {
			_, err := tx.Exec(`
				INSERT INTO rom_formats (item_title, filename, format, ext, filesize, md5)
				VALUES (?, ?, ?, ?, ?, ?)
			`, r.ItemTitle, f.Filename, f.Format, f.Ext, f.Filesize, f.Checksums.MD5)
			if err != nil {
				return fmt.Errorf("insert format for %q: %w", r.ItemTitle, err)
			}
		}
	}

	// Rebuild version_rom_deps: link each VideoGameVersion to its ROM dependencies
	// by looking up (item_type, title) → item_title within the same transaction.
	if _, err := tx.Exec(`DELETE FROM version_rom_deps`); err != nil {
		return err
	}
	for _, v := range versions {
		for slot, req := range v.ROMDependencies {
			required := 0
			if req.Required {
				required = 1
			}
			for _, opt := range req.Options {
				var romItemTitle string
				err := tx.QueryRow(
					`SELECT item_title FROM media_items WHERE item_type = ? AND title = ?`,
					opt.ItemType, opt.Title,
				).Scan(&romItemTitle)
				if err != nil {
					continue // ROM not in catalog yet — skip silently
				}
				_, _ = tx.Exec(
					`INSERT OR IGNORE INTO version_rom_deps
						(version_item_title, slot, slot_name, slot_required, rom_item_title)
					 VALUES (?, ?, ?, ?, ?)`,
					v.ItemTitle, slot, req.Name, required, romItemTitle,
				)
			}
		}
	}

	return tx.Commit()
}

// ── User ROM library sync ─────────────────────────────────────────────────────

// SyncUserROMs scans all ROM type directories under dataPath, computes MD5 for
// every file, and updates local_path in rom_formats for matching entries.
func (s *Store) SyncUserROMs(dataPath string) error {
	if _, err := s.db.Exec(`UPDATE rom_formats SET local_path = NULL`); err != nil {
		return err
	}

	stmt, err := s.db.Prepare(`UPDATE rom_formats SET local_path = ? WHERE LOWER(md5) = LOWER(?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, itemType := range metadata.RomItemTypes {
		romDir := filepath.Join(dataPath, itemType)
		entries, err := os.ReadDir(romDir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			itemDir := filepath.Join(romDir, entry.Name())
			files, err := os.ReadDir(itemDir)
			if err != nil {
				continue
			}
			for _, f := range files {
				if f.IsDir() {
					continue
				}
				p := filepath.Join(itemDir, f.Name())
				hash, err := hashMD5(p)
				if err != nil {
					continue
				}
				stmt.Exec(p, hash)
			}
		}
	}
	return nil
}

// ── Queries used by listing pages ─────────────────────────────────────────────

// GetVersions returns all VideoGameVersion rows with fields needed for the listing.
func (s *Store) GetVersions() ([]models.VideoGameVersion, error) {
	rows, err := s.db.Query(`
		SELECT item_title, item_type, title, platforms, artwork,
		       vg_item_title, vg_title, vg_release_year
		FROM media_items WHERE item_type = ?
		ORDER BY LOWER(title)
	`, metadata.PortItemType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.VideoGameVersion
	for rows.Next() {
		var v models.VideoGameVersion
		var platforms, artwork sql.NullString
		var vgItemTitle, vgTitle sql.NullString
		var vgYear sql.NullInt64

		if err := rows.Scan(&v.ItemTitle, &v.ItemType, &v.Title, &platforms, &artwork,
			&vgItemTitle, &vgTitle, &vgYear); err != nil {
			return nil, err
		}
		if platforms.Valid {
			json.Unmarshal([]byte(platforms.String), &v.Platforms)
		}
		if artwork.Valid {
			json.Unmarshal([]byte(artwork.String), &v.Artwork)
		}
		if vgItemTitle.Valid || vgTitle.Valid || vgYear.Valid {
			v.VideoGame = &models.ItemRef{}
			if vgItemTitle.Valid {
				v.VideoGame.ItemTitle = vgItemTitle.String
			}
			if vgTitle.Valid {
				v.VideoGame.Title = vgTitle.String
			}
			if vgYear.Valid {
				v.VideoGame.ReleaseYear = int(vgYear.Int64)
			}
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetROMLocalPaths returns md5 → localPath for all ROM files present in the
// user library. Replaces ScanROMLibrary in install and prompt logic.
func (s *Store) GetROMLocalPaths() (map[string]string, error) {
	rows, err := s.db.Query(`
		SELECT LOWER(md5), local_path FROM rom_formats WHERE local_path IS NOT NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var hash, path string
		if err := rows.Scan(&hash, &path); err != nil {
			return nil, err
		}
		out[hash] = path
	}
	return out, rows.Err()
}

// GetROMCatalogIndex returns md5 → {itemTitle, ext, itemType, filename} for all catalog ROM formats.
// Used by MatchDroppedROMs instead of loading all ROM JSON files.
type ROMIndexEntry struct {
	ItemTitle string
	Ext       string
	ItemType  string
	Filename  string
}

func (s *Store) GetROMCatalogIndex() (map[string]ROMIndexEntry, error) {
	rows, err := s.db.Query(`
		SELECT LOWER(rf.md5), rf.item_title, rf.ext, mi.item_type, rf.filename
		FROM rom_formats rf
		JOIN media_items mi ON mi.item_title = rf.item_title
		WHERE rf.md5 != ''
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]ROMIndexEntry{}
	for rows.Next() {
		var hash string
		var e ROMIndexEntry
		if err := rows.Scan(&hash, &e.ItemTitle, &e.Ext, &e.ItemType, &e.Filename); err != nil {
			return nil, err
		}
		out[hash] = e
	}
	return out, rows.Err()
}

// GetROMFormatsForVersion returns all ROM formats for all romDependencies of the
// given version as a single JOIN, keyed by "itemType\x00depTitle". This replaces
// N per-dependency queries with one round-trip.
func (s *Store) GetROMFormatsForVersion(versionItemTitle string) (map[string][]models.ROMFormat, error) {
	rows, err := s.db.Query(`
		SELECT mi.item_type, mi.title, rf.filename, rf.format, rf.ext, rf.filesize, rf.md5
		FROM version_rom_deps vd
		JOIN rom_formats rf ON rf.item_title = vd.rom_item_title
		JOIN media_items mi ON mi.item_title = vd.rom_item_title
		WHERE vd.version_item_title = ?
		ORDER BY vd.rom_item_title, rf.id
	`, versionItemTitle)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][]models.ROMFormat)
	for rows.Next() {
		var itemType, title string
		var f models.ROMFormat
		var md5 string
		if err := rows.Scan(&itemType, &title, &f.Filename, &f.Format, &f.Ext, &f.Filesize, &md5); err != nil {
			return nil, err
		}
		f.Checksums.MD5 = md5
		out[itemType+"\x00"+title] = append(out[itemType+"\x00"+title], f)
	}
	return out, rows.Err()
}

// GetVersionROMReadiness returns version item title → whether at least one accepted
// dump of its ROM requirement is present locally. A version's romDependencies are
// alternatives (regional and revision variants of one game), so any single present
// file satisfies the whole set. Versions with no ROM dependencies are absent from
// the map, which lets callers distinguish "no requirement" from "unmet requirement".
func (s *Store) GetVersionROMReadiness() (map[string]bool, error) {
	// Per requirement, MAX() is the OR across its options; the outer MIN() is the
	// AND across requirements. The CASE narrows that AND to the required ones,
	// leaving optional requirements as NULL, which MIN ignores — and COALESCE
	// then makes a port whose requirements are ALL optional read as ready, which
	// is correct for one that installs and launches without any of them.
	rows, err := s.db.Query(`
		SELECT version_item_title,
		       COALESCE(MIN(CASE WHEN slot_required = 1 THEN satisfied END), 1)
		FROM (
			SELECT vd.version_item_title AS version_item_title,
			       vd.slot               AS slot,
			       vd.slot_required      AS slot_required,
			       MAX(CASE WHEN rf.local_path IS NOT NULL THEN 1 ELSE 0 END) AS satisfied
			FROM version_rom_deps vd
			LEFT JOIN rom_formats rf ON rf.item_title = vd.rom_item_title
			GROUP BY vd.version_item_title, vd.slot, vd.slot_required
		)
		GROUP BY version_item_title
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]bool)
	for rows.Next() {
		var title string
		var ready int
		if err := rows.Scan(&title, &ready); err != nil {
			return nil, err
		}
		out[title] = ready == 1
	}
	return out, rows.Err()
}

// IsROMPresent returns true when the given MD5 has a local_path in rom_formats.
func (s *Store) IsROMPresent(md5 string) bool {
	var p sql.NullString
	s.db.QueryRow(`SELECT local_path FROM rom_formats WHERE LOWER(md5) = LOWER(?) LIMIT 1`, md5).Scan(&p)
	return p.Valid
}

// ── Update tracking ───────────────────────────────────────────────────────────

// GetItemUpdate returns the persistent has_update flag for an item.
func (s *Store) GetItemUpdate(itemTitle string) bool {
	var v int
	s.db.QueryRow(`SELECT has_update FROM media_items WHERE item_title = ?`, itemTitle).Scan(&v)
	return v == 1
}

// SetItemUpdate writes the has_update flag for an item.
func (s *Store) SetItemUpdate(itemTitle string, hasUpdate bool) error {
	v := 0
	if hasUpdate {
		v = 1
	}
	_, err := s.db.Exec(`UPDATE media_items SET has_update = ? WHERE item_title = ?`, v, itemTitle)
	return err
}

// ScanUserLibraryUpdates compares user library copies against the catalog and
// updates the has_update flag for every item that differs.
func (s *Store) ScanUserLibraryUpdates(metadataPath, dataPath string) error {
	libraryBase := filepath.Join(dataPath, "library")

	for _, mediaType := range append([]string{metadata.PortItemType}, metadata.RomItemTypes...) {
		entries, err := os.ReadDir(filepath.Join(libraryBase, mediaType))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			itemTitle := e.Name()
			catalogDir := filepath.Join(metadataPath, mediaType, itemTitle)
			libraryDir := filepath.Join(libraryBase, mediaType, itemTitle)
			hasUpdate := !dirsMatch(catalogDir, libraryDir)
			if err := s.SetItemUpdate(itemTitle, hasUpdate); err != nil {
				return err
			}
		}
	}
	return nil
}

// ── File comparison helpers ───────────────────────────────────────────────────

func dirsMatch(a, b string) bool {
	match := true
	filepath.Walk(a, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		// Skip symlinks: hashSHA256 calls os.Open which would follow the
		// symlink and potentially read content outside the catalog tree.
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		rel, _ := filepath.Rel(a, path)
		if !hashesMatch(path, filepath.Join(b, rel)) {
			match = false
			return filepath.SkipAll
		}
		return nil
	})
	return match
}

func hashesMatch(a, b string) bool {
	ha, err := hashSHA256(a)
	if err != nil {
		return false
	}
	hb, err := hashSHA256(b)
	if err != nil {
		return false
	}
	return ha == hb
}

func hashSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashMD5(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
