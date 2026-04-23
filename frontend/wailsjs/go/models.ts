export namespace main {
	
	export class Settings {
	    mediaItemsPath: string;
	    dataPath: string;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mediaItemsPath = source["mediaItemsPath"];
	        this.dataPath = source["dataPath"];
	    }
	}

}

export namespace models {
	
	export class ArgOption {
	    value: string;
	    label: string;
	    romTitle?: string;
	    romsReady?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ArgOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.value = source["value"];
	        this.label = source["label"];
	        this.romTitle = source["romTitle"];
	        this.romsReady = source["romsReady"];
	    }
	}
	export class ArgPrompt {
	    name: string;
	    type: string;
	    label: string;
	    options?: ArgOption[];
	
	    static createFrom(source: any = {}) {
	        return new ArgPrompt(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.label = source["label"];
	        this.options = this.convertValues(source["options"], ArgOption);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Artwork {
	    artworkType: string;
	    fileExtension: string;
	    fileName: string;
	
	    static createFrom(source: any = {}) {
	        return new Artwork(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.artworkType = source["artworkType"];
	        this.fileExtension = source["fileExtension"];
	        this.fileName = source["fileName"];
	    }
	}
	export class DataSource {
	    sourceId: string;
	    lastUpdatedAt: string;
	    fields: string[];
	
	    static createFrom(source: any = {}) {
	        return new DataSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceId = source["sourceId"];
	        this.lastUpdatedAt = source["lastUpdatedAt"];
	        this.fields = source["fields"];
	    }
	}
	export class ExecutableEntry {
	    path: string;
	    title: string;
	
	    static createFrom(source: any = {}) {
	        return new ExecutableEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.title = source["title"];
	    }
	}
	export class Platform {
	    _itemType: string;
	    _itemTitle: string;
	    title: string;
	    releaseYear?: number;
	
	    static createFrom(source: any = {}) {
	        return new Platform(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this._itemType = source["_itemType"];
	        this._itemTitle = source["_itemTitle"];
	        this.title = source["title"];
	        this.releaseYear = source["releaseYear"];
	    }
	}
	export class GameVersion {
	    _itemType: string;
	    _itemTitle: string;
	    title?: string;
	    releaseYear: number;
	    versionType: string;
	    platforms: Platform[];
	
	    static createFrom(source: any = {}) {
	        return new GameVersion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this._itemType = source["_itemType"];
	        this._itemTitle = source["_itemTitle"];
	        this.title = source["title"];
	        this.releaseYear = source["releaseYear"];
	        this.versionType = source["versionType"];
	        this.platforms = this.convertValues(source["platforms"], Platform);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class InstallState {
	    installed: boolean;
	    installedVersion: string;
	    installDir: string;
	    executablePath: string;
	    executables?: ExecutableEntry[];
	    activeMods: string[];
	    installedAt: string;
	    totalPlaySeconds: number;
	    lastPlayedAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new InstallState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.installed = source["installed"];
	        this.installedVersion = source["installedVersion"];
	        this.installDir = source["installDir"];
	        this.executablePath = source["executablePath"];
	        this.executables = this.convertValues(source["executables"], ExecutableEntry);
	        this.activeMods = source["activeMods"];
	        this.installedAt = source["installedAt"];
	        this.totalPlaySeconds = source["totalPlaySeconds"];
	        this.lastPlayedAt = source["lastPlayedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ItemRef {
	    _itemType: string;
	    _itemTitle: string;
	    title?: string;
	    releaseYear?: number;
	
	    static createFrom(source: any = {}) {
	        return new ItemRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this._itemType = source["_itemType"];
	        this._itemTitle = source["_itemTitle"];
	        this.title = source["title"];
	        this.releaseYear = source["releaseYear"];
	    }
	}
	export class Mod {
	    _itemType: string;
	    title: string;
	    modType: string;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new Mod(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this._itemType = source["_itemType"];
	        this.title = source["title"];
	        this.modType = source["modType"];
	        this.description = source["description"];
	    }
	}
	export class ParentItemType {
	    title: string;
	    schemaVersion: string;
	
	    static createFrom(source: any = {}) {
	        return new ParentItemType(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.schemaVersion = source["schemaVersion"];
	    }
	}
	
	export class ROMChecksums {
	    md5: string;
	    sha1: string;
	    sha256: string;
	    crc32: string;
	
	    static createFrom(source: any = {}) {
	        return new ROMChecksums(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.md5 = source["md5"];
	        this.sha1 = source["sha1"];
	        this.sha256 = source["sha256"];
	        this.crc32 = source["crc32"];
	    }
	}
	export class ROMFormat {
	    filename: string;
	    filesize: number;
	    format: string;
	    ext: string;
	    checksums: ROMChecksums;
	
	    static createFrom(source: any = {}) {
	        return new ROMFormat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.filename = source["filename"];
	        this.filesize = source["filesize"];
	        this.format = source["format"];
	        this.ext = source["ext"];
	        this.checksums = this.convertValues(source["checksums"], ROMChecksums);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ROMDependency {
	    _itemType: string;
	    title: string;
	    formats: ROMFormat[];
	    installPath?: string;
	
	    static createFrom(source: any = {}) {
	        return new ROMDependency(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this._itemType = source["_itemType"];
	        this.title = source["title"];
	        this.formats = this.convertValues(source["formats"], ROMFormat);
	        this.installPath = source["installPath"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ROMFileMatch {
	    filePath: string;
	    fileName: string;
	    romTitle: string;
	    formatExt: string;
	
	    static createFrom(source: any = {}) {
	        return new ROMFileMatch(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.filePath = source["filePath"];
	        this.fileName = source["fileName"];
	        this.romTitle = source["romTitle"];
	        this.formatExt = source["formatExt"];
	    }
	}
	export class ROMDropSummary {
	    matched: ROMFileMatch[];
	    unmatched: string[];
	
	    static createFrom(source: any = {}) {
	        return new ROMDropSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.matched = this.convertValues(source["matched"], ROMFileMatch);
	        this.unmatched = source["unmatched"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class VideoGame {
	    _itemType: string;
	    _schemaVersion: string;
	    _parentItemType: ParentItemType;
	    _itemTitle: string;
	    releaseYear: number;
	    title: string;
	    sortTitle: string;
	    alternateTitles: string[];
	    versions: GameVersion[];
	    artwork: Artwork[];
	    _itemLanguage: string;
	    description: string;
	    tags: string[];
	    _createdAt: string;
	    lastUpdatedAt: string;
	    _dataSources: DataSource[];
	
	    static createFrom(source: any = {}) {
	        return new VideoGame(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this._itemType = source["_itemType"];
	        this._schemaVersion = source["_schemaVersion"];
	        this._parentItemType = this.convertValues(source["_parentItemType"], ParentItemType);
	        this._itemTitle = source["_itemTitle"];
	        this.releaseYear = source["releaseYear"];
	        this.title = source["title"];
	        this.sortTitle = source["sortTitle"];
	        this.alternateTitles = source["alternateTitles"];
	        this.versions = this.convertValues(source["versions"], GameVersion);
	        this.artwork = this.convertValues(source["artwork"], Artwork);
	        this._itemLanguage = source["_itemLanguage"];
	        this.description = source["description"];
	        this.tags = source["tags"];
	        this._createdAt = source["_createdAt"];
	        this.lastUpdatedAt = source["lastUpdatedAt"];
	        this._dataSources = this.convertValues(source["_dataSources"], DataSource);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class VideoGameRom {
	    _itemType: string;
	    _itemTitle: string;
	    title: string;
	    platform: string;
	    formats: ROMFormat[];
	
	    static createFrom(source: any = {}) {
	        return new VideoGameRom(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this._itemType = source["_itemType"];
	        this._itemTitle = source["_itemTitle"];
	        this.title = source["title"];
	        this.platform = source["platform"];
	        this.formats = this.convertValues(source["formats"], ROMFormat);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class VideoGameVersion {
	    _itemType: string;
	    _schemaVersion: string;
	    _itemTitle: string;
	    title?: string;
	    releaseYear: number;
	    versionType: string;
	    videoGame?: ItemRef;
	    platforms: string[];
	    mods?: Mod[];
	    romDependencies?: ROMDependency[];
	    artwork?: Artwork[];
	    description?: string;
	    tags?: string[];
	    _createdAt?: string;
	    lastUpdatedAt?: string;
	    _dataSources?: DataSource[];
	
	    static createFrom(source: any = {}) {
	        return new VideoGameVersion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this._itemType = source["_itemType"];
	        this._schemaVersion = source["_schemaVersion"];
	        this._itemTitle = source["_itemTitle"];
	        this.title = source["title"];
	        this.releaseYear = source["releaseYear"];
	        this.versionType = source["versionType"];
	        this.videoGame = this.convertValues(source["videoGame"], ItemRef);
	        this.platforms = source["platforms"];
	        this.mods = this.convertValues(source["mods"], Mod);
	        this.romDependencies = this.convertValues(source["romDependencies"], ROMDependency);
	        this.artwork = this.convertValues(source["artwork"], Artwork);
	        this.description = source["description"];
	        this.tags = source["tags"];
	        this._createdAt = source["_createdAt"];
	        this.lastUpdatedAt = source["lastUpdatedAt"];
	        this._dataSources = this.convertValues(source["_dataSources"], DataSource);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

