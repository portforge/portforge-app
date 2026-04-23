package metadata

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"portforge/models"
	"strings"
)

// LoadAll reads all mediaitem subdirectories and returns VideoGame items.
func LoadAll(dir string) ([]models.VideoGame, error) {
	entries, err := os.ReadDir(filepath.Join(dir, "VideoGame"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var games []models.VideoGame
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		game, err := LoadOne(dir, entry.Name())
		if err != nil || game == nil {
			continue
		}
		games = append(games, *game)
	}

	return games, nil
}

// LoadOne loads a single VideoGame from its mediaitem directory.
func LoadOne(baseDir, itemTitle string) (*models.VideoGame, error) {
	jsonPath := filepath.Join(baseDir, "VideoGame", itemTitle, ".mediaitem.json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}

	var game models.VideoGame
	if err := json.Unmarshal(data, &game); err != nil {
		return nil, err
	}

	if game.ItemType != "VideoGame" {
		return nil, nil
	}

	return &game, nil
}

// LoadAllVersions reads all VideoGameVersion mediaitem directories.
func LoadAllVersions(baseDir string) ([]models.VideoGameVersion, error) {
	entries, err := os.ReadDir(filepath.Join(baseDir, "VideoGameVersion"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var versions []models.VideoGameVersion
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		v, err := LoadOneVersion(baseDir, entry.Name())
		if err != nil || v == nil {
			continue
		}
		versions = append(versions, *v)
	}

	return versions, nil
}

// LoadOneVersion loads a single VideoGameVersion from its mediaitem directory.
func LoadOneVersion(baseDir, itemTitle string) (*models.VideoGameVersion, error) {
	jsonPath := filepath.Join(baseDir, "VideoGameVersion", itemTitle, ".mediaitem.json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}

	var v models.VideoGameVersion
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}

	if v.ItemType != "VideoGameVersion" {
		return nil, nil
	}

	return &v, nil
}

// ScanROMs returns a map of MD5 checksum → absolute file path for all non-hidden,
// non-JSON files in the mediaitem directory.
func ScanROMs(itemDir string) (map[string]string, error) {
	entries, err := os.ReadDir(itemDir)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}

		path := filepath.Join(itemDir, name)
		hash, err := md5sum(path)
		if err != nil {
			continue
		}
		result[hash] = path
	}
	return result, nil
}

// LoadAllRoms reads all VideoGameRom mediaitem directories.
func LoadAllRoms(baseDir string) ([]models.VideoGameRom, error) {
	entries, err := os.ReadDir(filepath.Join(baseDir, "VideoGameRom"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var roms []models.VideoGameRom
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		r, err := LoadOneRom(baseDir, entry.Name())
		if err != nil || r == nil {
			continue
		}
		roms = append(roms, *r)
	}
	return roms, nil
}

// LoadOneRom loads a single VideoGameRom from its mediaitem directory.
func LoadOneRom(baseDir, itemTitle string) (*models.VideoGameRom, error) {
	jsonPath := filepath.Join(baseDir, "VideoGameRom", itemTitle, ".mediaitem.json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}

	var r models.VideoGameRom
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	r.ItemTitle = itemTitle
	return &r, nil
}

// ScanROMLibrary returns a combined MD5 → filepath map for all files across
// all VideoGameRom subdirectories.
func ScanROMLibrary(baseDir string) (map[string]string, error) {
	romDir := filepath.Join(baseDir, "VideoGameRom")
	entries, err := os.ReadDir(romDir)
	if os.IsNotExist(err) {
		return make(map[string]string), nil
	}
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		hashes, err := ScanROMs(filepath.Join(romDir, entry.Name()))
		if err != nil {
			continue
		}
		for hash, path := range hashes {
			result[hash] = path
		}
	}
	return result, nil
}

// LoadInstallationSpecs reads the .install.json array for a VideoGameVersion.
// Returns nil (no error) if the file doesn't exist.
func LoadInstallationSpecs(baseDir, itemTitle string) ([]models.InstallationSpec, error) {
	data, err := os.ReadFile(filepath.Join(baseDir, "VideoGameVersion", itemTitle, ".install.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var specs []models.InstallationSpec
	if err := json.Unmarshal(data, &specs); err != nil {
		return nil, err
	}
	return specs, nil
}

// ReadInstallState reads .state/meta.json for a VideoGameVersion, if present.
// Returns nil (no error) if the file doesn't exist yet.
func ReadInstallState(versionDir string) (*models.InstallState, error) {
	data, err := os.ReadFile(filepath.Join(versionDir, ".state", "meta.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var state models.InstallState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// WriteInstallState writes .state/meta.json for a VideoGameVersion.
func WriteInstallState(versionDir string, state *models.InstallState) error {
	dir := filepath.Join(versionDir, ".state")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "meta.json"), data, 0644)
}

func md5sum(path string) (string, error) {
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
