export namespace skillmgr {
	
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
	export class Config {
	    targetDirs: string[];
	    sources: SkillSourceConfig[];
	    validation: ValidationConfig;
	    scan: ScanConfig;
	    conflictHandling: string;
	    sourcePriority: string[];
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.targetDirs = source["targetDirs"];
	        this.sources = this.convertValues(source["sources"], SkillSourceConfig);
	        this.validation = this.convertValues(source["validation"], ValidationConfig);
	        this.scan = this.convertValues(source["scan"], ScanConfig);
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
	    sourceId: string;
	    sourceAlias?: string;
	    sourcePath: string;
	    targetPath?: string;
	    symlinkPath?: string;
	    targetStates?: SkillTarget[];
	    status: string;
	    hasSymlink: boolean;
	    symlinkTarget?: string;
	    isActive: boolean;
	    validationErrors?: string[];
	    files?: string[];
	    description?: string;
	    manifest?: SkillManifest;
	    previewFile?: string;
	    preview?: string;
	    updatedAt?: string;
	    lastScannedAt?: string;
	    conflictSources?: ConflictSource[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new Skill(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.sourceId = source["sourceId"];
	        this.sourceAlias = source["sourceAlias"];
	        this.sourcePath = source["sourcePath"];
	        this.targetPath = source["targetPath"];
	        this.symlinkPath = source["symlinkPath"];
	        this.targetStates = this.convertValues(source["targetStates"], SkillTarget);
	        this.status = source["status"];
	        this.hasSymlink = source["hasSymlink"];
	        this.symlinkTarget = source["symlinkTarget"];
	        this.isActive = source["isActive"];
	        this.validationErrors = source["validationErrors"];
	        this.files = source["files"];
	        this.description = source["description"];
	        this.manifest = this.convertValues(source["manifest"], SkillManifest);
	        this.previewFile = source["previewFile"];
	        this.preview = source["preview"];
	        this.updatedAt = source["updatedAt"];
	        this.lastScannedAt = source["lastScannedAt"];
	        this.conflictSources = this.convertValues(source["conflictSources"], ConflictSource);
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
	export class Inventory {
	    config: Config;
	    sources: SkillSource[];
	    skills: Skill[];
	    summary: Summary;
	
	    static createFrom(source: any = {}) {
	        return new Inventory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.config = this.convertValues(source["config"], Config);
	        this.sources = this.convertValues(source["sources"], SkillSource);
	        this.skills = this.convertValues(source["skills"], Skill);
	        this.summary = this.convertValues(source["summary"], Summary);
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
	
	
	
	
	
	
	

}

