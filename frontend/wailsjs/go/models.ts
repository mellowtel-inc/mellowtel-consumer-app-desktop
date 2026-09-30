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
	export class WaitlistProfileResult {
	    socialBonusPoints: number;

	    static createFrom(source: any = {}) {
	        return new WaitlistProfileResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.socialBonusPoints = source["socialBonusPoints"];
	    }
	}
	export class WaitlistSocialClaims {
	    x: boolean;
	    tiktok: boolean;
	    discord: boolean;

	    static createFrom(source: any = {}) {
	        return new WaitlistSocialClaims(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.x = source["x"];
	        this.tiktok = source["tiktok"];
	        this.discord = source["discord"];
	    }
	}
	export class WaitlistProgress {
	    country: string;
	    devices: string[];
	    socialClaims: WaitlistSocialClaims;
	    profileCompleted: boolean;
	    onboardingCompleted: boolean;
	    acceptedInviteCount: number;
	    pendingReferralPoints: number;

	    static createFrom(source: any = {}) {
	        return new WaitlistProgress(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.country = source["country"];
	        this.devices = source["devices"];
	        this.socialClaims = this.convertValues(source["socialClaims"], WaitlistSocialClaims);
	        this.profileCompleted = source["profileCompleted"];
	        this.onboardingCompleted = source["onboardingCompleted"];
	        this.acceptedInviteCount = source["acceptedInviteCount"];
	        this.pendingReferralPoints = source["pendingReferralPoints"];
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
	export class WaitlistResult {
	    alreadyJoined: boolean;
	    onboardingToken: string;
	    inviteUrl: string;
	    referralAccepted: boolean;
	    progress?: WaitlistProgress;

	    static createFrom(source: any = {}) {
	        return new WaitlistResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.alreadyJoined = source["alreadyJoined"];
	        this.onboardingToken = source["onboardingToken"];
	        this.inviteUrl = source["inviteUrl"];
	        this.referralAccepted = source["referralAccepted"];
	        this.progress = this.convertValues(source["progress"], WaitlistProgress);
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
