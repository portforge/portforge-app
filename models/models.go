package models

type ParentItemType struct {
	Title         string `json:"title"`
	SchemaVersion string `json:"schemaVersion"`
}

// ItemRef is a lightweight subitem reference used to point at another MediaItem.
type ItemRef struct {
	ItemType    string `json:"_itemType"`
	ItemTitle   string `json:"_itemTitle"`
	Title       string `json:"title,omitempty"`
	ReleaseYear int    `json:"releaseYear,omitempty"`
}

type ROMFormat struct {
	Filename  string       `json:"filename"`
	Filesize  int64        `json:"filesize"`
	Format    string       `json:"format"`
	Ext       string       `json:"ext"`
	Checksums ROMChecksums `json:"checksums"`
}

type VideoGameRom struct {
	ItemType  string      `json:"_itemType"`
	ItemTitle string      `json:"_itemTitle"` // set from folder name on load
	Title     string      `json:"title"`
	Platform  string      `json:"platform"`
	Formats   []ROMFormat `json:"formats"`
}

type Platform struct {
	ItemType    string `json:"_itemType"`
	ItemTitle   string `json:"_itemTitle"`
	Title       string `json:"title"`
	ReleaseYear int    `json:"releaseYear,omitempty"`
}

// ROMFileMatch describes a dropped file that was matched to a known ROM format.
type ROMFileMatch struct {
	FilePath  string `json:"filePath"`  // absolute path of the dropped file
	FileName  string `json:"fileName"`  // base name for display
	ROMTitle  string `json:"romTitle"`  // _itemTitle of the matching VideoGameRom
	FormatExt string `json:"formatExt"` // file extension for display
}

// ROMDropSummary is returned by MatchDroppedROMs.
type ROMDropSummary struct {
	Matched   []ROMFileMatch `json:"matched"`
	Unmatched []string       `json:"unmatched"` // base names of files with no match
}

type ROMChecksums struct {
	MD5    string `json:"md5"`
	SHA1   string `json:"sha1"`
	SHA256 string `json:"sha256"`
	CRC32  string `json:"crc32"`
}

type ROMDependency struct {
	ItemType    string      `json:"_itemType"`
	Title       string      `json:"title"`
	Formats     []ROMFormat `json:"formats"`
	InstallPath string      `json:"installPath,omitempty"`
}

// BuildStep is a single step in a version's build process.
type BuildStep struct {
	Step string `json:"step"`
	If   string `json:"if,omitempty"` // optional condition; step is skipped when it evaluates to false
	// copy
	From string `json:"from,omitempty"`
	Arg  string `json:"arg,omitempty"`  // references an ArgSpec key; used by copy+rom to get the romTitle from the selected option
	Src  string `json:"src,omitempty"`
	Dest string `json:"dest,omitempty"`
	// make
	Args []string          `json:"args,omitempty"`
	Env  map[string]string `json:"env,omitempty"`
	// fetch
	URL string `json:"url,omitempty"`
	// file
	Path string `json:"path,omitempty"`
	// defineExecutable
	Executable string `json:"executable,omitempty"` // path to the binary relative to the version's MediaItem folder
	Title      string `json:"title,omitempty"`      // label shown on the Play button; defaults to the executable filename
}

// ArgOption is one selectable value for a choice arg.
type ArgOption struct {
	Value     string `json:"value"`
	Label     string `json:"label"`
	ROMTitle  string `json:"romTitle,omitempty"`  // exact romDependency title used when this option is selected
	ROMsReady bool   `json:"romsReady,omitempty"` // populated at runtime by PortForge, not stored in JSON
}

// ArgSpec describes a single install argument.
type ArgSpec struct {
	Type    string      `json:"type"`            // "choice" | "string"
	Label   string      `json:"label"`
	Options []ArgOption `json:"options,omitempty"` // for type "choice"
}

// ArgPrompt is returned to the frontend so it can collect arg values before installing.
type ArgPrompt struct {
	Name    string      `json:"name"`
	Type    string      `json:"type"`
	Label   string      `json:"label"`
	Options []ArgOption `json:"options,omitempty"`
}

// InstallationSpec is one entry in the .install.json array.
type InstallationSpec struct {
	Version         string             `json:"version,omitempty"`
	TargetPlatforms []string           `json:"targetPlatforms,omitempty"`
	Dependencies    []string           `json:"dependencies,omitempty"`
	Args            map[string]ArgSpec `json:"args,omitempty"`
	Steps           []BuildStep        `json:"steps"`
	BuildPaths      []string           `json:"buildPaths,omitempty"`
	UninstallSteps  []BuildStep        `json:"uninstallSteps,omitempty"`
}

// ExecutableEntry describes a launchable executable produced by an install.
type ExecutableEntry struct {
	Path  string `json:"path"`
	Title string `json:"title"`
}

type InstallState struct {
	Installed        bool              `json:"installed"`
	InstalledVersion string            `json:"installedVersion"`
	InstallDir       string            `json:"installDir"`
	ExecutablePath   string            `json:"executablePath"` // legacy; use Executables[0] when present
	Executables      []ExecutableEntry `json:"executables,omitempty"`
	ActiveMods       []string          `json:"activeMods"`
	InstalledAt      string            `json:"installedAt"`
	TotalPlaySeconds int64             `json:"totalPlaySeconds"`
	LastPlayedAt     string            `json:"lastPlayedAt,omitempty"`
}

type Mod struct {
	ItemType    string `json:"_itemType"`
	Title       string `json:"title"`
	ModType     string `json:"modType"`
	Description string `json:"description"`
}

type Artwork struct {
	ArtworkType   string `json:"artworkType"`
	FileExtension string `json:"fileExtension"`
	FileName      string `json:"fileName"`
}

type DataSource struct {
	SourceID      string   `json:"sourceId"`
	LastUpdatedAt string   `json:"lastUpdatedAt"`
	Fields        []string `json:"fields"`
}

// GameVersion is a lightweight subitem reference embedded in a VideoGame's versions array.
type GameVersion struct {
	ItemType    string     `json:"_itemType"`
	ItemTitle   string     `json:"_itemTitle"`
	Title       string     `json:"title,omitempty"`
	ReleaseYear int        `json:"releaseYear"`
	VersionType string     `json:"versionType"`
	Platforms   []Platform `json:"platforms"`
}

// VideoGameVersion is a top-level MediaItem representing a standalone version/port.
type VideoGameVersion struct {
	ItemType        string          `json:"_itemType"`
	SchemaVersion   string          `json:"_schemaVersion"`
	ItemTitle       string          `json:"_itemTitle"`
	Title           string          `json:"title,omitempty"`
	ReleaseYear     int             `json:"releaseYear"`
	VersionType     string          `json:"versionType"`
	VideoGame       *ItemRef        `json:"videoGame,omitempty"`
	Platforms       []string        `json:"platforms"`
	Mods            []Mod           `json:"mods,omitempty"`
	ROMDependencies []ROMDependency `json:"romDependencies,omitempty"`
	Artwork         []Artwork       `json:"artwork,omitempty"`
	Description     string          `json:"description,omitempty"`
	Tags            []string        `json:"tags,omitempty"`
	CreatedAt       string          `json:"_createdAt,omitempty"`
	LastUpdatedAt   string          `json:"lastUpdatedAt,omitempty"`
	DataSources     []DataSource    `json:"_dataSources,omitempty"`
}

type VideoGame struct {
	ItemType        string         `json:"_itemType"`
	SchemaVersion   string         `json:"_schemaVersion"`
	ParentItemType  ParentItemType `json:"_parentItemType"`
	ItemTitle       string         `json:"_itemTitle"`
	ReleaseYear     int            `json:"releaseYear"`
	Title           string         `json:"title"`
	SortTitle       string         `json:"sortTitle"`
	AlternateTitles []string       `json:"alternateTitles"`
	Versions        []GameVersion  `json:"versions"`
	Artwork         []Artwork      `json:"artwork"`
	ItemLanguage    string         `json:"_itemLanguage"`
	Description     string         `json:"description"`
	Tags            []string       `json:"tags"`
	CreatedAt       string         `json:"_createdAt"`
	LastUpdatedAt   string         `json:"lastUpdatedAt"`
	DataSources     []DataSource   `json:"_dataSources"`
}
