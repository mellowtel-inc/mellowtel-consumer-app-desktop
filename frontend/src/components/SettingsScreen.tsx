import { useEffect, useState } from 'react';
import { API, copyText, Settings, SharingIntensity } from '../api';

interface Props {
  deviceId: string;
  email: string;
  onSignOut: () => void;
  onBack: () => void;
}

const DEFAULTS: Settings = {
  autoConnect: false,
  launchOnStartup: false,
  closeToTray: true,
  notifications: true,
  sharingIntensity: 'medium',
  bandwidthCap: 'unlimited',
  pauseScheduleFrom: '',
  pauseScheduleTo: '',
};

const INTENSITY_COPY: Record<SharingIntensity, string> = {
  low: 'Light — minimal impact, ideal while you’re working.',
  medium: 'Balanced — a steady amount of sharing without slowing you down.',
  max: 'Maximum — earn the most, best when you’re away.',
};

export default function SettingsScreen({ deviceId, email, onSignOut, onBack }: Props) {
  const [settings, setSettings] = useState<Settings>(DEFAULTS);
  const [loaded, setLoaded] = useState(false);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    API.getSettings()
      .then((s) => setSettings({ ...DEFAULTS, ...s }))
      .catch(() => {})
      .finally(() => setLoaded(true));
  }, []);

  // Settings save immediately — no Save button, which keeps the flow light.
  const update = <K extends keyof Settings>(key: K, value: Settings[K]) => {
    const next = { ...settings, [key]: value };
    setSettings(next);
    API.saveSettings(next).catch(() => {});
  };

  const doCopy = () => {
    copyText(deviceId);
    setCopied(true);
    setTimeout(() => setCopied(false), 1400);
  };

  const initials = deviceId ? deviceId.slice(-2).toUpperCase() : 'ML';

  return (
    <div className="app">
      <header className="header">
        <div className="brand">
          <button className="icon-btn back" onClick={onBack} aria-label="Back">
            ←
          </button>
          <span className="brand-name">Settings</span>
        </div>
      </header>

      <div className="settings-scroll">
        {!loaded ? (
          <div className="loading">Loading…</div>
        ) : (
          <>
            <SectionLabel>Account</SectionLabel>
            <div className="card account-card">
              <span className="account-avatar">{email ? email.slice(0, 1).toUpperCase() : 'E'}</span>
              <div className="device-meta">
                <div className="device-name">Earnbear account</div>
                <div className="device-id">{email}</div>
              </div>
              <button className="link-action signout" onClick={onSignOut}>Sign out</button>
            </div>

            <SectionLabel>Device</SectionLabel>
            <div className="card device-card">
              <span className="avatar">{initials}</span>
              <div className="device-meta">
                <div className="device-name">This device</div>
                <div className="device-id">{deviceId || '—'}</div>
              </div>
              <button className="link-action" onClick={doCopy}>
                {copied ? 'Copied' : 'Copy'}
              </button>
            </div>

            <SectionLabel>Preferences</SectionLabel>
            <ToggleRow
              title="Auto-connect on launch"
              subtitle="Start sharing as soon as the app opens"
              checked={settings.autoConnect}
              onChange={(v) => update('autoConnect', v)}
            />
            <ToggleRow
              title="Launch on system startup"
              subtitle="Open Earnbear when your computer starts"
              checked={settings.launchOnStartup}
              onChange={(v) => update('launchOnStartup', v)}
            />
            <ToggleRow
              title="Notifications"
              subtitle="Milestones and status changes"
              checked={settings.notifications}
              onChange={(v) => update('notifications', v)}
            />

            <SectionLabel>Sharing intensity</SectionLabel>
            <div className="card">
              <div className="segmented">
                {(['low', 'medium', 'max'] as SharingIntensity[]).map((level) => (
                  <button
                    key={level}
                    className={`segment ${settings.sharingIntensity === level ? 'active' : ''}`}
                    onClick={() => update('sharingIntensity', level)}
                  >
                    {level === 'low' ? 'Low' : level === 'medium' ? 'Medium' : 'Max'}
                  </button>
                ))}
              </div>
              <p className="hint">{INTENSITY_COPY[settings.sharingIntensity]}</p>
            </div>

            <SectionLabel>Application</SectionLabel>
            <button className="card quit-card" onClick={() => API.quit()}>
              <span>
                <b>Quit Earnbear</b>
                <small>Stop sharing and close the app completely</small>
              </span>
              <strong>Quit</strong>
            </button>

          </>
        )}
      </div>
    </div>
  );
}

function SectionLabel({ children }: { children: React.ReactNode }) {
  return <div className="section-label">{children}</div>;
}

function ToggleRow({
  title,
  subtitle,
  checked,
  onChange,
}: {
  title: string;
  subtitle: string;
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <div className="card toggle-card" onClick={() => onChange(!checked)}>
      <div className="toggle-meta">
        <div className="row-title">{title}</div>
        <div className="row-sub">{subtitle}</div>
      </div>
      <span className={`switch ${checked ? 'on' : ''}`}>
        <span className="knob" />
      </span>
    </div>
  );
}
