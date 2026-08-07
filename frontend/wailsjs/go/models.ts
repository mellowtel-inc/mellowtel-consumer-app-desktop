export namespace account {

	export class SignUpResult {
	    confirmed: boolean;

	    static createFrom(source: any = {}) {
	        return new SignUpResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.confirmed = source["confirmed"];
	    }
	}
	export class State {
	    authenticated: boolean;
	    email: string;
	    emailVerified: boolean;
	    name: string;

	    static createFrom(source: any = {}) {
	        return new State(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.authenticated = source["authenticated"];
	        this.email = source["email"];
	        this.emailVerified = source["emailVerified"];
	        this.name = source["name"];
	    }
	}

}

export namespace config {

	export class Settings {
	    autoConnect: boolean;
	    launchOnStartup: boolean;
	    closeToTray: boolean;
	    notifications: boolean;
	    sharingIntensity: string;
	    bandwidthCap: string;
	    pauseScheduleFrom: string;
	    pauseScheduleTo: string;

	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.autoConnect = source["autoConnect"];
	        this.launchOnStartup = source["launchOnStartup"];
	        this.closeToTray = source["closeToTray"];
	        this.notifications = source["notifications"];
	        this.sharingIntensity = source["sharingIntensity"];
	        this.bandwidthCap = source["bandwidthCap"];
	        this.pauseScheduleFrom = source["pauseScheduleFrom"];
	        this.pauseScheduleTo = source["pauseScheduleTo"];
	    }
	}

}

export namespace node {

	export class Status {
	    connection: string;
	    detail: string;
	    paused: boolean;
	    approved: boolean;
	    chromeFound: boolean;
	    deviceId: string;
	    version: string;
	    jobsCompleted: number;
	    jobsFailed: number;
	    bytesUsedToday: number;
	    totalJobsCompleted: number;
	    totalJobsFailed: number;
	    successRate: number;
	    totalEarnedUsd: number;
	    sessionEarnedUsd: number;
	    earningsReady: boolean;

	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connection = source["connection"];
	        this.detail = source["detail"];
	        this.paused = source["paused"];
	        this.approved = source["approved"];
	        this.chromeFound = source["chromeFound"];
	        this.deviceId = source["deviceId"];
	        this.version = source["version"];
	        this.jobsCompleted = source["jobsCompleted"];
	        this.jobsFailed = source["jobsFailed"];
	        this.bytesUsedToday = source["bytesUsedToday"];
	        this.totalJobsCompleted = source["totalJobsCompleted"];
	        this.totalJobsFailed = source["totalJobsFailed"];
	        this.successRate = source["successRate"];
	        this.totalEarnedUsd = source["totalEarnedUsd"];
	        this.sessionEarnedUsd = source["sessionEarnedUsd"];
	        this.earningsReady = source["earningsReady"];
	    }
	}

}

