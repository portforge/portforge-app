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
	Artwork   []Artwork   `json:"artwork,omitempty"`
}

type Platform struct {
	ItemType    string `json:"_itemType"`
	ItemTitle   string `json:"_itemTitle"`
	Title       string `json:"title"`
	ReleaseYear int    `json:"releaseYear,omitempty"`
}

// ROMFileMatch describes a dropped file that was matched to a known ROM format.
type ROMFileMatch struct {
	FilePath       string `json:"filePath"`       // absolute path of the dropped file
	FileName       string `json:"fileName"`       // base name for display
	ROMTitle       string `json:"romTitle"`       // _itemTitle of the matching VideoGameRom
	ROMType        string `json:"romType"`        // _itemType of the matching ROM
	FormatExt      string `json:"formatExt"`      // file extension for display
	FormatFilename string `json:"formatFilename"` // canonical filename from the matched format, used to rename on import
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

// ROMOption is one dump that can satisfy a requirement. Formats are not stored
// in the version's own JSON — they are hydrated from the standalone ROM
// MediaItem so checksum data lives in exactly one place.
type ROMOption struct {
	ItemType string      `json:"_itemType"`
	Title    string      `json:"title"`
	Formats  []ROMFormat `json:"formats,omitempty"`
}

// ROMRequirement is one thing a port needs, satisfied by any one of its
// options. Requirements combine with AND, options within a requirement with OR,
// which is what lets a three-disc game say "each disc, any region" without
// enumerating the fifteen valid combinations.
//
// Required means the port is unusable without it, not that the build needs it.
// Newer ports increasingly extract assets on first run rather than at build
// time, so a port can install and launch with nothing here satisfied and still
// declare a requirement it genuinely cannot be played without.
type ROMRequirement struct {
	// Name identifies the requirement: it is the heading the UI shows ("Disc 2")
	// and the string a spec step addresses with copy from:"rom" src:"Disc 2". It
	// is unique within a version, and it is an identifier rather than a caption —
	// renaming one silently breaks any spec that references it.
	Name     string      `json:"name,omitempty"`
	Required bool        `json:"required"`
	Options  []ROMOption `json:"options"`
}

// ROMDependencies is a version's full set of ROM requirements.
type ROMDependencies []ROMRequirement

// ROMLibraryPort is one port's requirements as the ROM library shows them.
type ROMLibraryPort struct {
	ItemTitle    string          `json:"_itemTitle"`
	Title        string          `json:"title"`
	Requirements ROMDependencies `json:"romDependencies"`
}

// ROMLibrary is every ROM requirement across every port, with a single presence
// map covering all of them.
//
// Satisfaction is deliberately not computed here. Whether a requirement is met
// is a rule about options and formats that the game page already applies, and
// two implementations of it would eventually disagree — so the backend supplies
// the facts (which dumps exist, which are present) and one shared frontend
// helper draws the conclusion for both views.
type ROMLibrary struct {
	Ports []ROMLibraryPort `json:"ports"`
	// Status maps a format's MD5 to whether the user has that file, keyed
	// exactly as the catalog writes it so a lookup needs no normalising.
	Status map[string]bool `json:"status"`
}

// AllOptions returns every option across every requirement, for the callers that
// only care whether a given dump is referenced at all.
func (d ROMDependencies) AllOptions() []ROMOption {
	var out []ROMOption
	for _, r := range d {
		out = append(out, r.Options...)
	}
	return out
}

// The install spec types — steps, args and the spec itself — live in
// github.com/zamiba/forge/engine, which owns executing them. The types below
// are PortForge's own: what it shows in the UI and what it persists to disk.

// ArgOption is one selectable value for a choice arg, as shown in the UI.
type ArgOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ArgPrompt is returned to the frontend so it can collect arg values before installing.
type ArgPrompt struct {
	Name    string      `json:"name"`
	Type    string      `json:"type"`
	Label   string      `json:"label"`
	Options []ArgOption `json:"options,omitempty"`
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
	Executables      []ExecutableEntry `json:"executables,omitempty"`
	ActiveMods       []string          `json:"activeMods"`
	Args             map[string]string `json:"args,omitempty"`           // arg values the build was made with
	TargetPlatform   string            `json:"targetPlatform,omitempty"` // platform the build targeted
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

// SoftwareVersion is one released version of a port, as listed in a
// VideoGameVersion's versions array. Title matches the version field of an object
// in the same item's .forge.json, which is what makes a release's notes findable
// from the version the user picked. Content is markdown.
type SoftwareVersion struct {
	ItemType string `json:"_itemType"`
	Title    string `json:"title"`
	Date     string `json:"date,omitempty"`
	Content  string `json:"content,omitempty"`
}

// VideoGameVersion is a top-level MediaItem representing a standalone version/port.
type VideoGameVersion struct {
	ItemType        string            `json:"_itemType"`
	SchemaVersion   string            `json:"_schemaVersion"`
	ItemTitle       string            `json:"_itemTitle"`
	Title           string            `json:"title,omitempty"`
	ReleaseYear     int               `json:"releaseYear"`
	VersionType     string            `json:"versionType"`
	VideoGame       *ItemRef          `json:"videoGame,omitempty"`
	Platforms       []string          `json:"platforms"`
	Mods            []Mod             `json:"mods,omitempty"`
	Versions        []SoftwareVersion `json:"versions,omitempty"`
	ROMDependencies ROMDependencies   `json:"romDependencies,omitempty"`
	Artwork         []Artwork         `json:"artwork,omitempty"`
	Description     string            `json:"description,omitempty"`
	Tags            []string          `json:"tags,omitempty"`
	CreatedAt       string            `json:"_createdAt,omitempty"`
	LastUpdatedAt   string            `json:"lastUpdatedAt,omitempty"`
	DataSources     []DataSource      `json:"_dataSources,omitempty"`
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
