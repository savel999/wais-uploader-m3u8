export namespace dl {
	
	export class TaskProgress {
	    url: string;
	    status: number;
	    progress: number;
	    Error: any;
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new TaskProgress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.status = source["status"];
	        this.progress = source["progress"];
	        this.Error = source["Error"];
	        this.warnings = source["warnings"];
	    }
	}

}

