export namespace core {
	
	export class CleanableItem {
	    id: string;
	    name: string;
	    description: string;
	    path?: string;
	    size: number;
	    selected: boolean;
	    safety: string;
	    // Go type: time
	    mod_time?: any;
	    metadata?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new CleanableItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.path = source["path"];
	        this.size = source["size"];
	        this.selected = source["selected"];
	        this.safety = source["safety"];
	        this.mod_time = this.convertValues(source["mod_time"], null);
	        this.metadata = source["metadata"];
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
	export class CleanResult {
	    cleaned_items: CleanableItem[];
	    freed_bytes: number;
	    errors?: any[];
	    duration: number;
	
	    static createFrom(source: any = {}) {
	        return new CleanResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cleaned_items = this.convertValues(source["cleaned_items"], CleanableItem);
	        this.freed_bytes = source["freed_bytes"];
	        this.errors = source["errors"];
	        this.duration = source["duration"];
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
	
	export class CustomScanRule {
	    id: string;
	    name: string;
	    path: string;
	    pattern: string;
	    recursive: boolean;
	    min_age_days: number;
	    safety: string;
	
	    static createFrom(source: any = {}) {
	        return new CustomScanRule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.pattern = source["pattern"];
	        this.recursive = source["recursive"];
	        this.min_age_days = source["min_age_days"];
	        this.safety = source["safety"];
	    }
	}
	export class RetentionConfig {
	    snapper_min_age_days: number;
	    pacman_keep_versions: number;
	    journal_vacuum_days: number;
	    dev_cache_idle_days: number;
	
	    static createFrom(source: any = {}) {
	        return new RetentionConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.snapper_min_age_days = source["snapper_min_age_days"];
	        this.pacman_keep_versions = source["pacman_keep_versions"];
	        this.journal_vacuum_days = source["journal_vacuum_days"];
	        this.dev_cache_idle_days = source["dev_cache_idle_days"];
	    }
	}
	export class ModuleConfig {
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ModuleConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	    }
	}
	export class GeneralConfig {
	    dry_run_default: boolean;
	    snapshot_safeguard: boolean;
	    log_level: string;
	
	    static createFrom(source: any = {}) {
	        return new GeneralConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dry_run_default = source["dry_run_default"];
	        this.snapshot_safeguard = source["snapshot_safeguard"];
	        this.log_level = source["log_level"];
	    }
	}
	export class Config {
	    version: string;
	    general: GeneralConfig;
	    modules: Record<string, ModuleConfig>;
	    retention: RetentionConfig;
	    custom_rules: CustomScanRule[];
	    exclude_paths?: string[];
	    dry_run_default?: boolean;
	    snapshot_safeguard?: boolean;
	    pacman_retention?: number;
	    log_level?: string;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.general = this.convertValues(source["general"], GeneralConfig);
	        this.modules = this.convertValues(source["modules"], ModuleConfig, true);
	        this.retention = this.convertValues(source["retention"], RetentionConfig);
	        this.custom_rules = this.convertValues(source["custom_rules"], CustomScanRule);
	        this.exclude_paths = source["exclude_paths"];
	        this.dry_run_default = source["dry_run_default"];
	        this.snapshot_safeguard = source["snapshot_safeguard"];
	        this.pacman_retention = source["pacman_retention"];
	        this.log_level = source["log_level"];
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
	
	
	
	export class MountStats {
	    path: string;
	    device: string;
	    fs_type: string;
	    total_bytes: number;
	    used_bytes: number;
	    free_bytes: number;
	    used_percent: number;
	    dev_id: number;
	
	    static createFrom(source: any = {}) {
	        return new MountStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.device = source["device"];
	        this.fs_type = source["fs_type"];
	        this.total_bytes = source["total_bytes"];
	        this.used_bytes = source["used_bytes"];
	        this.free_bytes = source["free_bytes"];
	        this.used_percent = source["used_percent"];
	        this.dev_id = source["dev_id"];
	    }
	}
	
	export class ScanResult {
	    items: CleanableItem[];
	    total_size: number;
	    // Go type: time
	    scanned_at: any;
	    error?: any;
	    metadata?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new ScanResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], CleanableItem);
	        this.total_size = source["total_size"];
	        this.scanned_at = this.convertValues(source["scanned_at"], null);
	        this.error = source["error"];
	        this.metadata = source["metadata"];
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
	export class SystemStorageInfo {
	    root: MountStats;
	    home: MountStats;
	    has_separate_home: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SystemStorageInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root = this.convertValues(source["root"], MountStats);
	        this.home = this.convertValues(source["home"], MountStats);
	        this.has_separate_home = source["has_separate_home"];
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

export namespace distro {
	
	export class DistroInfo {
	    id: string;
	    id_like: string[];
	    name: string;
	    pretty_name: string;
	    version_id: string;
	    family: string;
	    raw?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new DistroInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.id_like = source["id_like"];
	        this.name = source["name"];
	        this.pretty_name = source["pretty_name"];
	        this.version_id = source["version_id"];
	        this.family = source["family"];
	        this.raw = source["raw"];
	    }
	}

}

export namespace gui {
	
	export class GUIModuleSummary {
	    id: string;
	    name: string;
	    category: string;
	    description: string;
	    available: boolean;
	    requires_elevation: boolean;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GUIModuleSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.category = source["category"];
	        this.description = source["description"];
	        this.available = source["available"];
	        this.requires_elevation = source["requires_elevation"];
	        this.enabled = source["enabled"];
	    }
	}

}

