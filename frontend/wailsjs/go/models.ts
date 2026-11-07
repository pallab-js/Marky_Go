export namespace domain {
	
	export class NoteDocument {
	    path: string;
	    absPath: string;
	    content: string;
	    mtimeNs: number;
	    sizeBytes: number;
	    contentHash: string;
	    lineEnding: string;
	
	    static createFrom(source: any = {}) {
	        return new NoteDocument(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.absPath = source["absPath"];
	        this.content = source["content"];
	        this.mtimeNs = source["mtimeNs"];
	        this.sizeBytes = source["sizeBytes"];
	        this.contentHash = source["contentHash"];
	        this.lineEnding = source["lineEnding"];
	    }
	}
	export class SaveResult {
	    saved: boolean;
	    conflict: boolean;
	    conflictKind?: string;
	    message?: string;
	    document?: NoteDocument;
	
	    static createFrom(source: any = {}) {
	        return new SaveResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.saved = source["saved"];
	        this.conflict = source["conflict"];
	        this.conflictKind = source["conflictKind"];
	        this.message = source["message"];
	        this.document = this.convertValues(source["document"], NoteDocument);
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
	export class SearchOptions {
	    limit: number;
	
	    static createFrom(source: any = {}) {
	        return new SearchOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.limit = source["limit"];
	    }
	}
	export class SearchResult {
	    path: string;
	    title: string;
	    mtimeNs: number;
	    snippet: string;
	    rank?: number;
	
	    static createFrom(source: any = {}) {
	        return new SearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.title = source["title"];
	        this.mtimeNs = source["mtimeNs"];
	        this.snippet = source["snippet"];
	        this.rank = source["rank"];
	    }
	}
	export class Settings {
	    recentVaults: string[];
	    lastActiveVault?: string;
	    autosaveMs: number;
	    previewDefault: boolean;
	    configVersion: number;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.recentVaults = source["recentVaults"];
	        this.lastActiveVault = source["lastActiveVault"];
	        this.autosaveMs = source["autosaveMs"];
	        this.previewDefault = source["previewDefault"];
	        this.configVersion = source["configVersion"];
	    }
	}
	export class TreeNode {
	    name: string;
	    path: string;
	    absPath: string;
	    isDir: boolean;
	    modTime?: string;
	    size?: number;
	    children?: TreeNode[];
	
	    static createFrom(source: any = {}) {
	        return new TreeNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.absPath = source["absPath"];
	        this.isDir = source["isDir"];
	        this.modTime = source["modTime"];
	        this.size = source["size"];
	        this.children = this.convertValues(source["children"], TreeNode);
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
	export class VaultInfo {
	    rootPath: string;
	    name: string;
	    lastOpenedAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new VaultInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rootPath = source["rootPath"];
	        this.name = source["name"];
	        this.lastOpenedAt = source["lastOpenedAt"];
	    }
	}

}

