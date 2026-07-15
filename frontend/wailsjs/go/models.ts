export namespace clipboard {
	
	export class ClipEntry {
	    id: number;
	    type: string;
	    content: string;
	    preview: string;
	    thumbnail?: string;
	    timestamp: number;
	    pinned: boolean;
	    sourceApp?: string;
	    sourceTitle?: string;
	    sourcePath?: string;
	    note?: string;
	    tags?: string;
	
	    static createFrom(source: any = {}) {
	        return new ClipEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.content = source["content"];
	        this.preview = source["preview"];
	        this.thumbnail = source["thumbnail"];
	        this.timestamp = source["timestamp"];
	        this.pinned = source["pinned"];
	        this.sourceApp = source["sourceApp"];
	        this.sourceTitle = source["sourceTitle"];
	        this.sourcePath = source["sourcePath"];
	        this.note = source["note"];
	        this.tags = source["tags"];
	    }
	}

}

export namespace main {
	
	export class AppInfo {
	    name: string;
	    version: string;
	    dataDir: string;
	    databasePath: string;
	    imageDir: string;
	    exePath: string;
	    startHidden: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.dataDir = source["dataDir"];
	        this.databasePath = source["databasePath"];
	        this.imageDir = source["imageDir"];
	        this.exePath = source["exePath"];
	        this.startHidden = source["startHidden"];
	    }
	}
	export class AppSettings {
	    capturePaused: boolean;
	    recordImages: boolean;
	    skipSensitiveText: boolean;
	    recordSourceInfo: boolean;
	    startAtLogin: boolean;
	    autoPaste: boolean;
	    theme: string;
	    minTextLength: number;
	    maxClips: number;
	    retentionDays: number;
	    maxImageStorageMB: number;
	    updateManifestURL: string;
	    autoBackupEnabled: boolean;
	    autoBackupDir: string;
	    autoBackupIntervalDays: number;
	    autoBackupMaxFiles: number;
	    excludedApps: string[];
	    excludedWindowTitles: string[];
	
	    static createFrom(source: any = {}) {
	        return new AppSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.capturePaused = source["capturePaused"];
	        this.recordImages = source["recordImages"];
	        this.skipSensitiveText = source["skipSensitiveText"];
	        this.recordSourceInfo = source["recordSourceInfo"];
	        this.startAtLogin = source["startAtLogin"];
	        this.autoPaste = source["autoPaste"];
	        this.theme = source["theme"];
	        this.minTextLength = source["minTextLength"];
	        this.maxClips = source["maxClips"];
	        this.retentionDays = source["retentionDays"];
	        this.maxImageStorageMB = source["maxImageStorageMB"];
	        this.updateManifestURL = source["updateManifestURL"];
	        this.autoBackupEnabled = source["autoBackupEnabled"];
	        this.autoBackupDir = source["autoBackupDir"];
	        this.autoBackupIntervalDays = source["autoBackupIntervalDays"];
	        this.autoBackupMaxFiles = source["autoBackupMaxFiles"];
	        this.excludedApps = source["excludedApps"];
	        this.excludedWindowTitles = source["excludedWindowTitles"];
	    }
	}
	export class ClipExportResult {
	    path: string;
	    type: string;
	    bytes: number;
	    cancelled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ClipExportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.type = source["type"];
	        this.bytes = source["bytes"];
	        this.cancelled = source["cancelled"];
	    }
	}
	export class ClipPage {
	    items: clipboard.ClipEntry[];
	    total: number;
	    limit: number;
	    offset: number;
	    hasMore: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ClipPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], clipboard.ClipEntry);
	        this.total = source["total"];
	        this.limit = source["limit"];
	        this.offset = source["offset"];
	        this.hasMore = source["hasMore"];
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
	export class ClipTag {
	    name: string;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new ClipTag(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.count = source["count"];
	    }
	}
	export class DiagnosticStats {
	    path: string;
	    logBytes: number;
	    files: number;
	    cancelled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DiagnosticStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.logBytes = source["logBytes"];
	        this.files = source["files"];
	        this.cancelled = source["cancelled"];
	    }
	}
	export class HotkeyConfig {
	    modifiers: number;
	    keyCode: number;
	    display: string;
	
	    static createFrom(source: any = {}) {
	        return new HotkeyConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.modifiers = source["modifiers"];
	        this.keyCode = source["keyCode"];
	        this.display = source["display"];
	    }
	}
	export class TargetWindowInfo {
	    available: boolean;
	    processName: string;
	    processPath: string;
	    title: string;
	
	    static createFrom(source: any = {}) {
	        return new TargetWindowInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.processName = source["processName"];
	        this.processPath = source["processPath"];
	        this.title = source["title"];
	    }
	}
	export class UpdateCheckResult {
	    currentVersion: string;
	    latestVersion: string;
	    updateAvailable: boolean;
	    manifestURL: string;
	    downloadURL: string;
	    releaseNotesURL: string;
	    sha256: string;
	    publishedAt: string;
	    checkedAt: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateCheckResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.currentVersion = source["currentVersion"];
	        this.latestVersion = source["latestVersion"];
	        this.updateAvailable = source["updateAvailable"];
	        this.manifestURL = source["manifestURL"];
	        this.downloadURL = source["downloadURL"];
	        this.releaseNotesURL = source["releaseNotesURL"];
	        this.sha256 = source["sha256"];
	        this.publishedAt = source["publishedAt"];
	        this.checkedAt = source["checkedAt"];
	        this.message = source["message"];
	    }
	}
	export class UpdateDownloadResult {
	    path: string;
	    bytes: number;
	    sha256: string;
	    verified: boolean;
	    cancelled: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateDownloadResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.bytes = source["bytes"];
	        this.sha256 = source["sha256"];
	        this.verified = source["verified"];
	        this.cancelled = source["cancelled"];
	        this.message = source["message"];
	    }
	}

}

export namespace storage {
	
	export class BackupStats {
	    path: string;
	    clips: number;
	    textClips: number;
	    imageClips: number;
	    images: number;
	    settings: number;
	    skippedImages: number;
	    cancelled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BackupStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.clips = source["clips"];
	        this.textClips = source["textClips"];
	        this.imageClips = source["imageClips"];
	        this.images = source["images"];
	        this.settings = source["settings"];
	        this.skippedImages = source["skippedImages"];
	        this.cancelled = source["cancelled"];
	    }
	}
	export class Stats {
	    totalClips: number;
	    textClips: number;
	    imageClips: number;
	    pinnedClips: number;
	    imageBytes: number;
	    databaseBytes: number;
	    missingImages: number;
	    unpinnedClips: number;
	    thumbnailBytes: number;
	    settingsEntries: number;
	
	    static createFrom(source: any = {}) {
	        return new Stats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalClips = source["totalClips"];
	        this.textClips = source["textClips"];
	        this.imageClips = source["imageClips"];
	        this.pinnedClips = source["pinnedClips"];
	        this.imageBytes = source["imageBytes"];
	        this.databaseBytes = source["databaseBytes"];
	        this.missingImages = source["missingImages"];
	        this.unpinnedClips = source["unpinnedClips"];
	        this.thumbnailBytes = source["thumbnailBytes"];
	        this.settingsEntries = source["settingsEntries"];
	    }
	}
	export class ClearResult {
	    deletedClips: number;
	    deletedImages: number;
	    pinnedKept: number;
	    stats: Stats;
	
	    static createFrom(source: any = {}) {
	        return new ClearResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.deletedClips = source["deletedClips"];
	        this.deletedImages = source["deletedImages"];
	        this.pinnedKept = source["pinnedKept"];
	        this.stats = this.convertValues(source["stats"], Stats);
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
	export class MaintenanceResult {
	    missingImagesRemoved: number;
	    databaseBytesBefore: number;
	    databaseBytesAfter: number;
	    totalClipsBefore: number;
	    totalClipsAfter: number;
	    vacuumed: boolean;
	    optimized: boolean;
	    stats: Stats;
	
	    static createFrom(source: any = {}) {
	        return new MaintenanceResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.missingImagesRemoved = source["missingImagesRemoved"];
	        this.databaseBytesBefore = source["databaseBytesBefore"];
	        this.databaseBytesAfter = source["databaseBytesAfter"];
	        this.totalClipsBefore = source["totalClipsBefore"];
	        this.totalClipsAfter = source["totalClipsAfter"];
	        this.vacuumed = source["vacuumed"];
	        this.optimized = source["optimized"];
	        this.stats = this.convertValues(source["stats"], Stats);
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

