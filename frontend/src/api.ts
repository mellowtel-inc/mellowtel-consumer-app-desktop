// Thin, typed wrapper over the Wails-injected bridge. Using the runtime globals
// directly (instead of the generated wailsjs bindings) keeps the frontend
// working even before `wails dev` regenerates bindings for the current Go API.

export interface Status {
  connection: 'disconnected' | 'connecting' | 'connected';
  detail: string;
  paused: boolean;
  approved: boolean;
  chromeFound: boolean;
  deviceId: string;
  version: string;

  // Session counters
  jobsCompleted: number;
  jobsFailed: number;
  bytesUsedToday: number;

  // Lifetime totals
  totalJobsCompleted: number;
  totalJobsFailed: number;
  successRate: number;

  // Earnings — zero until the ledger backend exists
  totalEarnedUsd: number;
  sessionEarnedUsd: number;
  earningsReady: boolean;
}

export interface AuthState {
  authenticated: boolean;
  email: string;
  emailVerified: boolean;
  name: string;
}

export interface SignUpResult {
  confirmed: boolean;
}

export type BandwidthCap = 'unlimited' | '5gb' | '1gb';
export type SharingIntensity = 'low' | 'medium' | 'max';

export interface Settings {
  autoConnect: boolean;
  launchOnStartup: boolean;
  closeToTray: boolean;
  notifications: boolean;
  sharingIntensity: SharingIntensity;
  bandwidthCap: BandwidthCap;
  pauseScheduleFrom: string;
  pauseScheduleTo: string;
}

type AppBridge = {
  Connect(): Promise<void>;
  Disconnect(): Promise<void>;
  Toggle(): Promise<boolean>;
  GetStatus(): Promise<Status>;
  GetDeviceID(): Promise<string>;
  GetVersion(): Promise<string>;
  IsChromeInstalled(): Promise<boolean>;
  ChromeDownloadURL(): Promise<string>;
  GetSettings(): Promise<Settings>;
  SaveSettings(s: Settings): Promise<void>;
  OpenLogsFolder(): Promise<void>;
  GetLogPath(): Promise<string>;
  OpenURL(url: string): Promise<void>;
  ShowWindow(): Promise<void>;
  Quit(): Promise<void>;
  GetAuthState(): Promise<AuthState>;
  SignIn(email: string, password: string): Promise<AuthState>;
  SignUp(email: string, password: string, affiliateCode: string): Promise<SignUpResult>;
  ConfirmSignUp(email: string, code: string): Promise<void>;
  ResendSignUpCode(email: string): Promise<void>;
  SignOut(): Promise<void>;
};

type RuntimeBridge = {
  EventsOn(event: string, cb: (...data: any[]) => void): () => void;
  ClipboardSetText?(text: string): Promise<boolean>;
};

declare global {
  interface Window {
    go?: { main?: { App?: AppBridge } };
    runtime?: RuntimeBridge;
  }
}

function app(): AppBridge {
  const a = window.go?.main?.App;
  if (!a) throw new Error('Wails bridge not ready');
  return a;
}

export const API = {
  connect: () => app().Connect(),
  disconnect: () => app().Disconnect(),
  toggle: () => app().Toggle(),
  getStatus: () => app().GetStatus(),
  getSettings: () => app().GetSettings(),
  saveSettings: (s: Settings) => app().SaveSettings(s),
  isChromeInstalled: () => app().IsChromeInstalled(),
  chromeDownloadURL: () => app().ChromeDownloadURL(),
  openLogsFolder: () => app().OpenLogsFolder(),
  getLogPath: () => app().GetLogPath(),
  openURL: (url: string) => app().OpenURL(url),
  quit: () => app().Quit(),
  getAuthState: () => app().GetAuthState(),
  signIn: (email: string, password: string) => app().SignIn(email, password),
  signUp: (email: string, password: string, affiliateCode = '') => app().SignUp(email, password, affiliateCode),
  confirmSignUp: (email: string, code: string) => app().ConfirmSignUp(email, code),
  resendSignUpCode: (email: string) => app().ResendSignUpCode(email),
  signOut: () => app().SignOut(),
};

// subscribe attaches an event listener as soon as the Wails runtime is injected.
// The runtime is not guaranteed to exist at React mount time, so we retry
// briefly rather than silently dropping the subscription (which would leave the
// backend emitting to "no listeners" forever).
function subscribe(event: string, cb: (...data: any[]) => void): () => void {
  let unsub: (() => void) | null = null;
  let cancelled = false;

  const attach = () => {
    if (cancelled) return;
    const on = window.runtime?.EventsOn;
    if (on) {
      unsub = window.runtime!.EventsOn(event, cb);
      return;
    }
    setTimeout(attach, 100); // runtime not injected yet — retry
  };
  attach();

  return () => {
    cancelled = true;
    if (unsub) unsub();
  };
}

// onStatus subscribes to backend status pushes. Returns an unsubscribe fn.
export function onStatus(cb: (s: Status) => void): () => void {
  return subscribe('status:update', (s: Status) => cb(s));
}

export function onChromeMissing(cb: (url: string) => void): () => void {
  return subscribe('chrome:missing', (url: string) => cb(url));
}

export function copyText(text: string): void {
  const rt: any = window.runtime;
  if (rt?.ClipboardSetText) {
    rt.ClipboardSetText(text);
  } else if (navigator.clipboard) {
    navigator.clipboard.writeText(text).catch(() => {});
  }
}

// formatPoints is retained for reward displays that use a real server balance.
export function formatPoints(n: number): string {
  return `${(n || 0).toLocaleString(undefined, {
    minimumFractionDigits: 0,
    maximumFractionDigits: 2,
  })} points`;
}

// formatCount renders large job counts with thousands separators.
export function formatCount(n: number): string {
  return (n || 0).toLocaleString();
}

export function formatBytes(n: number): string {
  if (!n) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(n) / Math.log(1024));
  return `${(n / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}
