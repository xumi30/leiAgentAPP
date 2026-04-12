export namespace mcpbridge {
	
	export class ToolInfo {
	    name: string;
	    description?: string;
	    input_schema?: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new ToolInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.input_schema = source["input_schema"];
	    }
	}

}

export namespace proxy {
	
	export class LLMConfigRow {
	    name: string;
	    apiKey: string;
	    baseUrl: string;
	    model: string;
	    provider: string;
	    streamMode: string;
	    maxOutputTokens: number;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new LLMConfigRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.apiKey = source["apiKey"];
	        this.baseUrl = source["baseUrl"];
	        this.model = source["model"];
	        this.provider = source["provider"];
	        this.streamMode = source["streamMode"];
	        this.maxOutputTokens = source["maxOutputTokens"];
	        this.enabled = source["enabled"];
	    }
	}
	export class LLMConfigFormState {
	    primary: LLMConfigRow;
	    backends: LLMConfigRow[];
	    path: string;
	    usingExample: boolean;
	
	    static createFrom(source: any = {}) {
	        return new LLMConfigFormState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.primary = this.convertValues(source["primary"], LLMConfigRow);
	        this.backends = this.convertValues(source["backends"], LLMConfigRow);
	        this.path = source["path"];
	        this.usingExample = source["usingExample"];
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
	
	export class LLMConnectionStatus {
	    ok: boolean;
	    reachable: boolean;
	    phase: string;
	    message: string;
	    configPath: string;
	    backend?: string;
	    httpStatus?: number;
	
	    static createFrom(source: any = {}) {
	        return new LLMConnectionStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.reachable = source["reachable"];
	        this.phase = source["phase"];
	        this.message = source["message"];
	        this.configPath = source["configPath"];
	        this.backend = source["backend"];
	        this.httpStatus = source["httpStatus"];
	    }
	}
	export class MCPConfigRow {
	    label: string;
	    transportType: string;
	    url: string;
	    command: string;
	    argsText: string;
	    allowedTools: string;
	    headersText: string;
	    envText: string;
	    cachedTools: string[];
	    cachedToolDetails: mcpbridge.ToolInfo[];
	    lastCheckState: string;
	    lastCheckMessage: string;
	    lastCheckedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPConfigRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.transportType = source["transportType"];
	        this.url = source["url"];
	        this.command = source["command"];
	        this.argsText = source["argsText"];
	        this.allowedTools = source["allowedTools"];
	        this.headersText = source["headersText"];
	        this.envText = source["envText"];
	        this.cachedTools = source["cachedTools"];
	        this.cachedToolDetails = this.convertValues(source["cachedToolDetails"], mcpbridge.ToolInfo);
	        this.lastCheckState = source["lastCheckState"];
	        this.lastCheckMessage = source["lastCheckMessage"];
	        this.lastCheckedAt = source["lastCheckedAt"];
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
	export class MCPConfigFormState {
	    servers: MCPConfigRow[];
	    path: string;
	    usingExample: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MCPConfigFormState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.servers = this.convertValues(source["servers"], MCPConfigRow);
	        this.path = source["path"];
	        this.usingExample = source["usingExample"];
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
	
	export class MCPValidationResult {
	    ok: boolean;
	    message: string;
	    tools: string[];
	    toolDetails: mcpbridge.ToolInfo[];
	    label: string;
	    toolCount: number;
	    checkedAt: string;
	    configValid: boolean;
	    lastCheckState: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPValidationResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.message = source["message"];
	        this.tools = source["tools"];
	        this.toolDetails = this.convertValues(source["toolDetails"], mcpbridge.ToolInfo);
	        this.label = source["label"];
	        this.toolCount = source["toolCount"];
	        this.checkedAt = source["checkedAt"];
	        this.configValid = source["configValid"];
	        this.lastCheckState = source["lastCheckState"];
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

