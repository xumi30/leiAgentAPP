export namespace proxy {
	
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

