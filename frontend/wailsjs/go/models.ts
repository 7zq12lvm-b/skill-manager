export namespace skillmgr {
	
	export class SyncLLMConfig {
	    baseUrl?: string;
	    apiKey?: string;
	    model?: string;
	    temperature?: number;
	    maxTokens?: number;
	
	    static createFrom(source: any = {}) {
	        return new SyncLLMConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.baseUrl = source["baseUrl"];
	        this.apiKey = source["apiKey"];
	        this.model = source["model"];
	        this.temperature = source["temperature"];
	        this.maxTokens = source["maxTokens"];
	    }
	}
	export class Summary {
	    skillsFound: number;
	    enabled: number;
	    conflicts: number;
	    invalid: number;
	    errors: number;
	
	    static createFrom(source: any = {}) {
	        return new Summary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.skillsFound = source["skillsFound"];
	        this.enabled = source["enabled"];
	        this.conflicts = source["conflicts"];
	        this.invalid = source["invalid"];
	        this.errors = source["errors"];
	    }
	}
	export class SkillProfile {
	    summaryZh?: string;
	    useCasesZh?: string[];
	    generatedAt?: string;
	    model?: string;
	    sourceHash?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new SkillProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.summaryZh = source["summaryZh"];
	        this.useCasesZh = source["useCasesZh"];
	        this.generatedAt = source["generatedAt"];
	        this.model = source["model"];
	        this.sourceHash = source["sourceHash"];
	        this.error = source["error"];
	    }
	}
	export class ConflictSource {
	    skillId: string;
	    sourceId: string;
	    sourcePath: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new ConflictSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.skillId = source["skillId"];
	        this.sourceId = source["sourceId"];
	        this.sourcePath = source["sourcePath"];
	        this.status = source["status"];
	    }
	}
	export class SkillManifest {
	    name?: string;
	    description?: string;
	    license?: string;
	    compatibility?: string;
	    metadata?: Record<string, string>;
	    allowedTools?: string;
	    whenToUse?: string;
	    disableModelInvocation?: boolean;
	    userInvocable?: boolean;
	    argumentHint?: string;
	    arguments?: any;
	
	    static createFrom(source: any = {}) {
	        return new SkillManifest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.license = source["license"];
	        this.compatibility = source["compatibility"];
	        this.metadata = source["metadata"];
	        this.allowedTools = source["allowedTools"];
	        this.whenToUse = source["whenToUse"];
	        this.disableModelInvocation = source["disableModelInvocation"];
	        this.userInvocable = source["userInvocable"];
	        this.argumentHint = source["argumentHint"];
	        this.arguments = source["arguments"];
	    }
	}
	export class SkillTarget {
	    targetDir: string;
	    targetPath: string;
	    symlinkPath: string;
	    hasSymlink: boolean;
	    symlinkTarget?: string;
	    isActive: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new SkillTarget(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.targetDir = source["targetDir"];
	        this.targetPath = source["targetPath"];
	        this.symlinkPath = source["symlinkPath"];
	        this.hasSymlink = source["hasSymlink"];
	        this.symlinkTarget = source["symlinkTarget"];
	        this.isActive = source["isActive"];
	        this.error = source["error"];
	    }
	}
	export class Skill {
	    id: string;
	    name: string;
	    displayName?: string;
	    sourceId: string;
	    sourceKey?: string;
	    sourceAlias?: string;
	    sourcePath: string;
	    repoId?: string;
	    repoPath?: string;
	    repoSubpath?: string;
	    cloneUrl?: string;
	    syncId?: string;
	    targetName?: string;
	    previousTargetNames?: string[];
	    targetPath?: string;
	    symlinkPath?: string;
	    targetStates?: SkillTarget[];
	    status: string;
	    hasSymlink: boolean;
	    symlinkTarget?: string;
	    isActive: boolean;
	    canRemove: boolean;
	    ref?: string;
	    refMismatch: boolean;
	    validationErrors?: string[];
	    files?: string[];
	    description?: string;
	    manifest?: SkillManifest;
	    previewFile?: string;
	    preview?: string;
	    updatedAt?: string;
	    lastScannedAt?: string;
	    conflictSources?: ConflictSource[];
	    tags?: string[];
	    note?: string;
	    starred: boolean;
	    profile?: SkillProfile;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new Skill(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.displayName = source["displayName"];
	        this.sourceId = source["sourceId"];
	        this.sourceKey = source["sourceKey"];
	        this.sourceAlias = source["sourceAlias"];
	        this.sourcePath = source["sourcePath"];
	        this.repoId = source["repoId"];
	        this.repoPath = source["repoPath"];
	        this.repoSubpath = source["repoSubpath"];
	        this.cloneUrl = source["cloneUrl"];
	        this.syncId = source["syncId"];
	        this.targetName = source["targetName"];
	        this.previousTargetNames = source["previousTargetNames"];
	        this.targetPath = source["targetPath"];
	        this.symlinkPath = source["symlinkPath"];
	        this.targetStates = this.convertValues(source["targetStates"], SkillTarget);
	        this.status = source["status"];
	        this.hasSymlink = source["hasSymlink"];
	        this.symlinkTarget = source["symlinkTarget"];
	        this.isActive = source["isActive"];
	        this.canRemove = source["canRemove"];
	        this.ref = source["ref"];
	        this.refMismatch = source["refMismatch"];
	        this.validationErrors = source["validationErrors"];
	        this.files = source["files"];
	        this.description = source["description"];
	        this.manifest = this.convertValues(source["manifest"], SkillManifest);
	        this.previewFile = source["previewFile"];
	        this.preview = source["preview"];
	        this.updatedAt = source["updatedAt"];
	        this.lastScannedAt = source["lastScannedAt"];
	        this.conflictSources = this.convertValues(source["conflictSources"], ConflictSource);
	        this.tags = source["tags"];
	        this.note = source["note"];
	        this.starred = source["starred"];
	        this.profile = this.convertValues(source["profile"], SkillProfile);
	        this.error = source["error"];
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
	export class Repository {
	    id: string;
	    provider: string;
	    sourceKey: string;
	    repoId: string;
	    path: string;
	    alias?: string;
	    enabled: boolean;
	    cloneUrl?: string;
	    scanRoots?: string[];
	    ignorePaths?: string[];
	    skillCount: number;
	    installed: boolean;
	    lastScannedAt?: string;
	    isGitRepo: boolean;
	    currentRef?: string;
	    dirty: boolean;
	    errorCount: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new Repository(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.provider = source["provider"];
	        this.sourceKey = source["sourceKey"];
	        this.repoId = source["repoId"];
	        this.path = source["path"];
	        this.alias = source["alias"];
	        this.enabled = source["enabled"];
	        this.cloneUrl = source["cloneUrl"];
	        this.scanRoots = source["scanRoots"];
	        this.ignorePaths = source["ignorePaths"];
	        this.skillCount = source["skillCount"];
	        this.installed = source["installed"];
	        this.lastScannedAt = source["lastScannedAt"];
	        this.isGitRepo = source["isGitRepo"];
	        this.currentRef = source["currentRef"];
	        this.dirty = source["dirty"];
	        this.errorCount = source["errorCount"];
	        this.error = source["error"];
	    }
	}
	export class SkillSource {
	    id: string;
	    path: string;
	    alias?: string;
	    enabled: boolean;
	    isGitRepo: boolean;
	    gitRoot?: string;
	    skillCount: number;
	    lastScannedAt?: string;
	    errorCount: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new SkillSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.path = source["path"];
	        this.alias = source["alias"];
	        this.enabled = source["enabled"];
	        this.isGitRepo = source["isGitRepo"];
	        this.gitRoot = source["gitRoot"];
	        this.skillCount = source["skillCount"];
	        this.lastScannedAt = source["lastScannedAt"];
	        this.errorCount = source["errorCount"];
	        this.error = source["error"];
	    }
	}
	export class SyncConfig {
	    folder?: string;
	    lastAppliedAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new SyncConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.folder = source["folder"];
	        this.lastAppliedAt = source["lastAppliedAt"];
	    }
	}
	export class ScanConfig {
	    autoRescanOnStartup: boolean;
	    watchSourceFolders: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ScanConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.autoRescanOnStartup = source["autoRescanOnStartup"];
	        this.watchSourceFolders = source["watchSourceFolders"];
	    }
	}
	export class ValidationConfig {
	    mode: string;
	    requiredFiles: string[];
	    showInvalid: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ValidationConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.requiredFiles = source["requiredFiles"];
	        this.showInvalid = source["showInvalid"];
	    }
	}
	export class SourceInstallationOptions {
	    scanRoots?: string[];
	    ignorePaths?: string[];
	
	    static createFrom(source: any = {}) {
	        return new SourceInstallationOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scanRoots = source["scanRoots"];
	        this.ignorePaths = source["ignorePaths"];
	    }
	}
	export class SourceInstallation {
	    provider: string;
	    sourceId: string;
	    path: string;
	    alias?: string;
	    enabled: boolean;
	    options?: SourceInstallationOptions;
	
	    static createFrom(source: any = {}) {
	        return new SourceInstallation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.sourceId = source["sourceId"];
	        this.path = source["path"];
	        this.alias = source["alias"];
	        this.enabled = source["enabled"];
	        this.options = this.convertValues(source["options"], SourceInstallationOptions);
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
	export class Config {
	    version: number;
	    targetDirs: string[];
	    installations: SourceInstallation[];
	    validation: ValidationConfig;
	    scan: ScanConfig;
	    sync: SyncConfig;
	    conflictHandling: string;
	    sourcePriority: string[];
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.targetDirs = source["targetDirs"];
	        this.installations = this.convertValues(source["installations"], SourceInstallation);
	        this.validation = this.convertValues(source["validation"], ValidationConfig);
	        this.scan = this.convertValues(source["scan"], ScanConfig);
	        this.sync = this.convertValues(source["sync"], SyncConfig);
	        this.conflictHandling = source["conflictHandling"];
	        this.sourcePriority = source["sourcePriority"];
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
	export class Inventory {
	    config: Config;
	    sources: SkillSource[];
	    repositories?: Repository[];
	    skills: Skill[];
	    summary: Summary;
	    syncConfigured: boolean;
	    syncPath?: string;
	    syncError?: string;
	    llmConfig?: SyncLLMConfig;
	
	    static createFrom(source: any = {}) {
	        return new Inventory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.config = this.convertValues(source["config"], Config);
	        this.sources = this.convertValues(source["sources"], SkillSource);
	        this.repositories = this.convertValues(source["repositories"], Repository);
	        this.skills = this.convertValues(source["skills"], Skill);
	        this.summary = this.convertValues(source["summary"], Summary);
	        this.syncConfigured = source["syncConfigured"];
	        this.syncPath = source["syncPath"];
	        this.syncError = source["syncError"];
	        this.llmConfig = this.convertValues(source["llmConfig"], SyncLLMConfig);
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
	export class BulkDisableResult {
	    inventory: Inventory;
	    disabled: number;
	    alreadyDisabled: number;
	    skipped: number;
	    failed?: string[];
	
	    static createFrom(source: any = {}) {
	        return new BulkDisableResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.inventory = this.convertValues(source["inventory"], Inventory);
	        this.disabled = source["disabled"];
	        this.alreadyDisabled = source["alreadyDisabled"];
	        this.skipped = source["skipped"];
	        this.failed = source["failed"];
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
	export class BulkEnableResult {
	    inventory: Inventory;
	    enabled: number;
	    alreadyEnabled: number;
	    skipped: number;
	    failed?: string[];
	
	    static createFrom(source: any = {}) {
	        return new BulkEnableResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.inventory = this.convertValues(source["inventory"], Inventory);
	        this.enabled = source["enabled"];
	        this.alreadyEnabled = source["alreadyEnabled"];
	        this.skipped = source["skipped"];
	        this.failed = source["failed"];
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
	export class BulkTagResult {
	    inventory: Inventory;
	    updated: number;
	    unchanged: number;
	    skipped: number;
	    failed?: string[];
	
	    static createFrom(source: any = {}) {
	        return new BulkTagResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.inventory = this.convertValues(source["inventory"], Inventory);
	        this.updated = source["updated"];
	        this.unchanged = source["unchanged"];
	        this.skipped = source["skipped"];
	        this.failed = source["failed"];
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
	export class CloneRepositoryResult {
	    inventory: Inventory;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new CloneRepositoryResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.inventory = this.convertValues(source["inventory"], Inventory);
	        this.message = source["message"];
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
	
	
	
	export class PullSourceResult {
	    inventory: Inventory;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new PullSourceResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.inventory = this.convertValues(source["inventory"], Inventory);
	        this.message = source["message"];
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
	
	export class RepositoryConfig {
	    id: string;
	    repoId: string;
	    path: string;
	    alias?: string;
	    enabled: boolean;
	    cloneUrl?: string;
	    scanRoots?: string[];
	    ignorePaths?: string[];
	
	    static createFrom(source: any = {}) {
	        return new RepositoryConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.repoId = source["repoId"];
	        this.path = source["path"];
	        this.alias = source["alias"];
	        this.enabled = source["enabled"];
	        this.cloneUrl = source["cloneUrl"];
	        this.scanRoots = source["scanRoots"];
	        this.ignorePaths = source["ignorePaths"];
	    }
	}
	
	
	export class SkillFileEntry {
	    name: string;
	    path: string;
	    isDir: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SkillFileEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.isDir = source["isDir"];
	    }
	}
	export class SkillFilePreview {
	    path: string;
	    previewable: boolean;
	    content?: string;
	    reason?: string;
	
	    static createFrom(source: any = {}) {
	        return new SkillFilePreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.previewable = source["previewable"];
	        this.content = source["content"];
	        this.reason = source["reason"];
	    }
	}
	
	
	export class SkillProfileResult {
	    inventory: Inventory;
	    profile?: SkillProfile;
	    generated: boolean;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new SkillProfileResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.inventory = this.convertValues(source["inventory"], Inventory);
	        this.profile = this.convertValues(source["profile"], SkillProfile);
	        this.generated = source["generated"];
	        this.message = source["message"];
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
	
	export class SkillSourceConfig {
	    id: string;
	    path: string;
	    alias?: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SkillSourceConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.path = source["path"];
	        this.alias = source["alias"];
	        this.enabled = source["enabled"];
	    }
	}
	
	
	
	
	
	

}

