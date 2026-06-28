export namespace main {
	
	export class LocalizationRow {
	    index: number;
	    rowNumber: number;
	    key: string;
	    english: string;
	    schinese: string;
	    noTranslate: string;
	    translatable: boolean;
	    cells: string[];
	
	    static createFrom(source: any = {}) {
	        return new LocalizationRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.rowNumber = source["rowNumber"];
	        this.key = source["key"];
	        this.english = source["english"];
	        this.schinese = source["schinese"];
	        this.noTranslate = source["noTranslate"];
	        this.translatable = source["translatable"];
	        this.cells = source["cells"];
	    }
	}
	export class LocalizationDocument {
	    path: string;
	    delimiter: string;
	    header: string[];
	    rows: LocalizationRow[];
	    englishIndex: number;
	    schineseIndex: number;
	    noTranslateIndex: number;
	    totalRows: number;
	    pendingRows: number;
	
	    static createFrom(source: any = {}) {
	        return new LocalizationDocument(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.delimiter = source["delimiter"];
	        this.header = source["header"];
	        this.rows = this.convertValues(source["rows"], LocalizationRow);
	        this.englishIndex = source["englishIndex"];
	        this.schineseIndex = source["schineseIndex"];
	        this.noTranslateIndex = source["noTranslateIndex"];
	        this.totalRows = source["totalRows"];
	        this.pendingRows = source["pendingRows"];
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
	
	export class ModelInfo {
	    id: string;
	    owner?: string;
	    created?: number;
	
	    static createFrom(source: any = {}) {
	        return new ModelInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.owner = source["owner"];
	        this.created = source["created"];
	    }
	}
	export class TranslateOptions {
	    baseUrl: string;
	    apiKey: string;
	    model: string;
	    batchSize: number;
	    concurrency: number;
	    overwrite: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TranslateOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.baseUrl = source["baseUrl"];
	        this.apiKey = source["apiKey"];
	        this.model = source["model"];
	        this.batchSize = source["batchSize"];
	        this.concurrency = source["concurrency"];
	        this.overwrite = source["overwrite"];
	    }
	}
	export class TranslationSummary {
	    total: number;
	    completed: number;
	    failed: number;
	    canceled: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new TranslationSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.completed = source["completed"];
	        this.failed = source["failed"];
	        this.canceled = source["canceled"];
	        this.message = source["message"];
	    }
	}

}

