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
	
	export class MCPHubAuthor {
	    name: string;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPHubAuthor(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.url = source["url"];
	    }
	}
	export class MCPHubDeploymentConnection {
	    type: string;
	    command: string;
	    url: string;
	    args: string[];
	    headers: Record<string, string>;
	    env: Record<string, string>;
	    configSchema: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new MCPHubDeploymentConnection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.command = source["command"];
	        this.url = source["url"];
	        this.args = source["args"];
	        this.headers = source["headers"];
	        this.env = source["env"];
	        this.configSchema = source["configSchema"];
	    }
	}
	export class MCPHubSystemDependency {
	    name: string;
	    type: string;
	    checkCommand: string;
	    installInstructions: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new MCPHubSystemDependency(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.checkCommand = source["checkCommand"];
	        this.installInstructions = source["installInstructions"];
	    }
	}
	export class MCPHubDeploymentOption {
	    installationMethod: string;
	    description: string;
	    isRecommended: boolean;
	    connection: MCPHubDeploymentConnection;
	    installationDetails: Record<string, any>;
	    systemDependencies: MCPHubSystemDependency[];
	
	    static createFrom(source: any = {}) {
	        return new MCPHubDeploymentOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.installationMethod = source["installationMethod"];
	        this.description = source["description"];
	        this.isRecommended = source["isRecommended"];
	        this.connection = this.convertValues(source["connection"], MCPHubDeploymentConnection);
	        this.installationDetails = source["installationDetails"];
	        this.systemDependencies = this.convertValues(source["systemDependencies"], MCPHubSystemDependency);
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
	export class MCPHubPluginDetail {
	    identifier: string;
	    name: string;
	    description: string;
	    category: string;
	    version: string;
	    homepage: string;
	    icon: string;
	    connectionType: string;
	    installCount: number;
	    ratingAverage: number;
	    ratingCount: number;
	    isValidated: boolean;
	    isFeatured: boolean;
	    isOfficial: boolean;
	    haveCloudEndpoint: boolean;
	    author: MCPHubAuthor;
	    github: Record<string, any>;
	    overview: Record<string, any>;
	    tags: string[];
	    deploymentOptions: MCPHubDeploymentOption[];
	
	    static createFrom(source: any = {}) {
	        return new MCPHubPluginDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.identifier = source["identifier"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.category = source["category"];
	        this.version = source["version"];
	        this.homepage = source["homepage"];
	        this.icon = source["icon"];
	        this.connectionType = source["connectionType"];
	        this.installCount = source["installCount"];
	        this.ratingAverage = source["ratingAverage"];
	        this.ratingCount = source["ratingCount"];
	        this.isValidated = source["isValidated"];
	        this.isFeatured = source["isFeatured"];
	        this.isOfficial = source["isOfficial"];
	        this.haveCloudEndpoint = source["haveCloudEndpoint"];
	        this.author = this.convertValues(source["author"], MCPHubAuthor);
	        this.github = source["github"];
	        this.overview = source["overview"];
	        this.tags = source["tags"];
	        this.deploymentOptions = this.convertValues(source["deploymentOptions"], MCPHubDeploymentOption);
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
	export class MCPHubRegisterResult {
	    registered: boolean;
	    credentialsPath: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPHubRegisterResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.registered = source["registered"];
	        this.credentialsPath = source["credentialsPath"];
	        this.message = source["message"];
	    }
	}
	export class MCPHubSearchItem {
	    identifier: string;
	    name: string;
	    description: string;
	    author: string;
	    category: string;
	    connectionType: string;
	    installCount: number;
	    ratingAverage: number;
	    ratingCount: number;
	    commentCount: number;
	    version: string;
	    homepage: string;
	    icon: string;
	    manifestUrl: string;
	    installationMethods: string;
	    isValidated: boolean;
	    isFeatured: boolean;
	    isOfficial: boolean;
	    github: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new MCPHubSearchItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.identifier = source["identifier"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.author = source["author"];
	        this.category = source["category"];
	        this.connectionType = source["connectionType"];
	        this.installCount = source["installCount"];
	        this.ratingAverage = source["ratingAverage"];
	        this.ratingCount = source["ratingCount"];
	        this.commentCount = source["commentCount"];
	        this.version = source["version"];
	        this.homepage = source["homepage"];
	        this.icon = source["icon"];
	        this.manifestUrl = source["manifestUrl"];
	        this.installationMethods = source["installationMethods"];
	        this.isValidated = source["isValidated"];
	        this.isFeatured = source["isFeatured"];
	        this.isOfficial = source["isOfficial"];
	        this.github = source["github"];
	    }
	}
	export class MCPHubSearchResult {
	    items: MCPHubSearchItem[];
	    categories: string[];
	    currentPage: number;
	    pageSize: number;
	    totalCount: number;
	    totalPages: number;
	
	    static createFrom(source: any = {}) {
	        return new MCPHubSearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], MCPHubSearchItem);
	        this.categories = source["categories"];
	        this.currentPage = source["currentPage"];
	        this.pageSize = source["pageSize"];
	        this.totalCount = source["totalCount"];
	        this.totalPages = source["totalPages"];
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
	export class MCPHubStatus {
	    registered: boolean;
	    credentialsPath: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPHubStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.registered = source["registered"];
	        this.credentialsPath = source["credentialsPath"];
	        this.message = source["message"];
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
	    missingEnvKeys: string[];
	    warnings: string[];
	
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
	        this.missingEnvKeys = source["missingEnvKeys"];
	        this.warnings = source["warnings"];
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

