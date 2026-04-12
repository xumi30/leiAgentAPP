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

}

