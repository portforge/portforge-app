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
	db *sql.DB
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
	`)
	return err
}

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
			VALUES (?, 'VideoGameVersion', ?, ?, ?,  ?, ?, ?,
				COALESCE((SELECT has_update FROM media_items WHERE item_title = ?), 0))
			ON CONFLICT(item_title) DO UPDATE SET
				item_type       = 'VideoGameVersion',
				title           = excluded.title,
				platforms       = excluded.platforms,
				artwork         = excluded.artwork,
				vg_item_title   = excluded.vg_item_title,
				vg_title        = excluded.vg_title,
				vg_release_year = excluded.vg_release_year
		`, v.ItemTitle, v.Title, string(platformsJSON), string(artworkJSON),
			vgItemTitle, vgTitle, vgYear, v.ItemTitle)
		if err != nil {
			return fmt.Errorf("upsert version %q: %w", v.ItemTitle, err)
		}
	}

	for _, r := range roms {
		_, err := tx.Exec(`
			INSERT INTO media_items
				(item_title, item_type, title, platform, has_update)
			VALUES (?, 'VideoGameRom', ?, ?,
				COALESCE((SELECT has_update FROM media_items WHERE item_title = ?), 0))
			ON CONFLICT(item_title) DO UPDATE SET
				item_type = 'VideoGameRom',
				title     = excluded.title,
				platform  = excluded.platform
		`, r.ItemTitle, r.Title, r.Platform, r.ItemTitle)
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

	return tx.Commit()
}

// ── User ROM library sync ─────────────────────────────────────────────────────

// SyncUserROMs scans {dataPath}/VideoGameRom/, computes MD5 for every file,
// and updates local_path in rom_formats for matching entries.
func (s *Store) SyncUserROMs(dataPath string) error {
	if _, err := s.db.Exec(`UPDATE rom_formats SET local_path = NULL`); err != nil {
		return err
	}

	romDir := filepath.Join(dataPath, "VideoGameRom")
	entries, err := os.ReadDir(romDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	stmt, err := s.db.Prepare(`UPDATE rom_formats SET local_path = ? WHERE LOWER(md5) = LOWER(?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

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
	return nil
}

// ── Queries used by listing pages ─────────────────────────────────────────────

// GetVersions returns all VideoGameVersion rows with fields needed for the listing.
func (s *Store) GetVersions() ([]models.VideoGameVersion, error) {
	rows, err := s.db.Query(`
		SELECT item_title, title, platforms, artwork,
		       vg_item_title, vg_title, vg_release_year
		FROM media_items WHERE item_type = 'VideoGameVersion'
		ORDER BY LOWER(title)
	`)
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

		if err := rows.Scan(&v.ItemTitle, &v.Title, &platforms, &artwork,
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

// GetRoms returns all VideoGameRom rows with their formats (display fields only).
func (s *Store) GetRoms() ([]models.VideoGameRom, error) {
	rows, err := s.db.Query(`
		SELECT item_title, title, platform
		FROM media_items WHERE item_type = 'VideoGameRom'
		ORDER BY LOWER(title)
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roms []models.VideoGameRom
	idx := map[string]int{}
	for rows.Next() {
		var r models.VideoGameRom
		if err := rows.Scan(&r.ItemTitle, &r.Title, &r.Platform); err != nil {
			return nil, err
		}
		idx[r.ItemTitle] = len(roms)
		roms = append(roms, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	frows, err := s.db.Query(`
		SELECT item_title, filename, format, ext, filesize
		FROM rom_formats ORDER BY item_title, id
	`)
	if err != nil {
		return nil, err
	}
	defer frows.Close()

	for frows.Next() {
		var itemTitle string
		var f models.ROMFormat
		if err := frows.Scan(&itemTitle, &f.Filename, &f.Format, &f.Ext, &f.Filesize); err != nil {
			return nil, err
		}
		if i, ok := idx[itemTitle]; ok {
			roms[i].Formats = append(roms[i].Formats, f)
		}
	}
	return roms, frows.Err()
}

// GetRomLibraryStatus returns itemTitle → whether any format file is present.
func (s *Store) GetRomLibraryStatus() (map[string]bool, error) {
	rows, err := s.db.Query(`
		SELECT item_title, MAX(CASE WHEN local_path IS NOT NULL THEN 1 ELSE 0 END)
		FROM rom_formats GROUP BY item_title
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]bool{}
	for rows.Next() {
		var title string
		var have int
		if err := rows.Scan(&title, &have); err != nil {
			return nil, err
		}
		out[title] = have == 1
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

// GetROMCatalogIndex returns md5 → {itemTitle, ext} for all catalog ROM formats.
// Used by MatchDroppedROMs instead of loading all ROM JSON files.
type ROMIndexEntry struct {
	ItemTitle string
	Ext       string
}

func (s *Store) GetROMCatalogIndex() (map[string]ROMIndexEntry, error) {
	rows, err := s.db.Query(`
		SELECT LOWER(md5), item_title, ext FROM rom_formats WHERE md5 != ''
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]ROMIndexEntry{}
	for rows.Next() {
		var hash string
		var e ROMIndexEntry
		if err := rows.Scan(&hash, &e.ItemTitle, &e.Ext); err != nil {
			return nil, err
		}
		out[hash] = e
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

	for _, mediaType := range []string{"VideoGameVersion", "VideoGameRom"} {
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
