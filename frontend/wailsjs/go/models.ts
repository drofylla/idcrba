export namespace main {
	
	export class MyKadData {
	    name: string;
	    ic_number: string;
	    sex: string;
	    date_of_birth: string;
	    state_of_birth: string;
	    address_1: string;
	    address_2: string;
	    address_3: string;
	    postcode: string;
	    city: string;
	    religion: string;
	    read_time: string;
	
	    static createFrom(source: any = {}) {
	        return new MyKadData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.ic_number = source["ic_number"];
	        this.sex = source["sex"];
	        this.date_of_birth = source["date_of_birth"];
	        this.state_of_birth = source["state_of_birth"];
	        this.address_1 = source["address_1"];
	        this.address_2 = source["address_2"];
	        this.address_3 = source["address_3"];
	        this.postcode = source["postcode"];
	        this.city = source["city"];
	        this.religion = source["religion"];
	        this.read_time = source["read_time"];
	    }
	}
	export class ReaderStatus {
	    connected: boolean;
	    reader: string;
	    message: string;
	    has_card: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ReaderStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connected = source["connected"];
	        this.reader = source["reader"];
	        this.message = source["message"];
	        this.has_card = source["has_card"];
	    }
	}

}

