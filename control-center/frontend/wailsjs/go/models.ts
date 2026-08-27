export namespace audit {
	
	export class Entry {
	    id: number;
	    // Go type: time
	    timestamp: any;
	    action: string;
	    agent_id: string;
	    user: string;
	    details: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.timestamp = this.convertValues(source["timestamp"], null);
	        this.action = source["action"];
	        this.agent_id = source["agent_id"];
	        this.user = source["user"];
	        this.details = source["details"];
	        this.status = source["status"];
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

export namespace client {
	
	export class AgentInfo {
	    id: string;
	    host: string;
	    port: number;
	    auth_token?: string;
	    name: string;
	    os: string;
	    arch: string;
	    connected: boolean;
	    // Go type: time
	    last_seen: any;
	    system_info?: protocol.SystemInfoPayload;
	
	    static createFrom(source: any = {}) {
	        return new AgentInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.auth_token = source["auth_token"];
	        this.name = source["name"];
	        this.os = source["os"];
	        this.arch = source["arch"];
	        this.connected = source["connected"];
	        this.last_seen = this.convertValues(source["last_seen"], null);
	        this.system_info = this.convertValues(source["system_info"], protocol.SystemInfoPayload);
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

export namespace protocol {
	
	export class CPUInfo {
	    percent: number;
	    cores: number;
	    model: string;
	
	    static createFrom(source: any = {}) {
	        return new CPUInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.percent = source["percent"];
	        this.cores = source["cores"];
	        this.model = source["model"];
	    }
	}
	export class CommandResultPayload {
	    stdout: string;
	    stderr: string;
	    exit_code: number;
	    duration_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new CommandResultPayload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.stdout = source["stdout"];
	        this.stderr = source["stderr"];
	        this.exit_code = source["exit_code"];
	        this.duration_ms = source["duration_ms"];
	    }
	}
	export class DirEntry {
	    name: string;
	    path: string;
	    is_dir: boolean;
	    size: number;
	    mode: string;
	    mod_time: string;
	
	    static createFrom(source: any = {}) {
	        return new DirEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.is_dir = source["is_dir"];
	        this.size = source["size"];
	        this.mode = source["mode"];
	        this.mod_time = source["mod_time"];
	    }
	}
	export class DirContentsPayload {
	    path: string;
	    entries: DirEntry[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new DirContentsPayload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.entries = this.convertValues(source["entries"], DirEntry);
	        this.total = source["total"];
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
	
	export class DiskInfo {
	    mount: string;
	    fs_type: string;
	    total: number;
	    used: number;
	    free: number;
	    percent: number;
	
	    static createFrom(source: any = {}) {
	        return new DiskInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mount = source["mount"];
	        this.fs_type = source["fs_type"];
	        this.total = source["total"];
	        this.used = source["used"];
	        this.free = source["free"];
	        this.percent = source["percent"];
	    }
	}
	export class MemoryInfo {
	    total: number;
	    used: number;
	    free: number;
	    percent: number;
	
	    static createFrom(source: any = {}) {
	        return new MemoryInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.used = source["used"];
	        this.free = source["free"];
	        this.percent = source["percent"];
	    }
	}
	export class NetInfo {
	    ip: string;
	    mac: string;
	    hostname: string;
	
	    static createFrom(source: any = {}) {
	        return new NetInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ip = source["ip"];
	        this.mac = source["mac"];
	        this.hostname = source["hostname"];
	    }
	}
	export class ScreenshotDataPayload {
	    format: string;
	    data: number[];
	    width: number;
	    height: number;
	
	    static createFrom(source: any = {}) {
	        return new ScreenshotDataPayload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.format = source["format"];
	        this.data = source["data"];
	        this.width = source["width"];
	        this.height = source["height"];
	    }
	}
	export class SystemInfoPayload {
	    hostname: string;
	    os: string;
	    platform: string;
	    arch: string;
	    cpu: CPUInfo;
	    memory: MemoryInfo;
	    disks: DiskInfo[];
	    uptime: number;
	    net: NetInfo;
	    agent_version: string;
	
	    static createFrom(source: any = {}) {
	        return new SystemInfoPayload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hostname = source["hostname"];
	        this.os = source["os"];
	        this.platform = source["platform"];
	        this.arch = source["arch"];
	        this.cpu = this.convertValues(source["cpu"], CPUInfo);
	        this.memory = this.convertValues(source["memory"], MemoryInfo);
	        this.disks = this.convertValues(source["disks"], DiskInfo);
	        this.uptime = source["uptime"];
	        this.net = this.convertValues(source["net"], NetInfo);
	        this.agent_version = source["agent_version"];
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

export namespace scripting {
	
	export class Script {
	    name: string;
	    content: string;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Script(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.content = source["content"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
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

export namespace session {
	
	export class Session {
	    id: number;
	    name: string;
	    host: string;
	    port: number;
	    auth_token?: string;
	    tls?: boolean;
	    ca_file?: string;
	    server_name?: string;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    last_connected: any;
	
	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.auth_token = source["auth_token"];
	        this.tls = source["tls"];
	        this.ca_file = source["ca_file"];
	        this.server_name = source["server_name"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.last_connected = this.convertValues(source["last_connected"], null);
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

