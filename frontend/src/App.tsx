import { useEffect, useState, useCallback } from 'react';
import './App.css';
import {
  API,
  onStatus,
  onChromeMissing,
  formatUSD,
  formatCount,
  Status,
} from './api';
import SettingsScreen from './components/SettingsScreen';
import ChromeModal from './components/ChromeModal';

const EMPTY_STATUS: Status = {
  connection: 'disconnected',
  detail: 'Paused',
  paused: true,
  approved: false,
  chromeFound: true,
  deviceId: '',
  version: '',
  jobsCompleted: 0,
  jobsFailed: 0,
  bytesUsedToday: 0,
  totalJobsCompleted: 0,
  totalJobsFailed: 0,
  successRate: 0,
  totalEarnedUsd: 0,
  sessionEarnedUsd: 0,
  earningsReady: false,
};

export default function App() {
  const [status, setStatus] = useState<Status>(EMPTY_STATUS);
  const [busy, setBusy] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [chromeMissing, setChromeMissing] = useState(false);

  useEffect(() => {
    API.getStatus().then(setStatus).catch(() => {});
    const off = onStatus(setStatus);
    const offChrome = onChromeMissing(() => setChromeMissing(true));

    // Backstop: keep the UI honest even if an event is missed.
    const poll = setInterval(() => {
      API.getStatus().then(setStatus).catch(() => {});
    }, 2000);

    return () => {
      off();
      offChrome();
      clearInterval(poll);
    };
  }, []);

  const connected = status.connection === 'connected';
  const connecting = status.connection === 'connecting';

  const toggle = useCallback(async () => {
    setBusy(true);
    try {
      await API.toggle();
      setStatus(await API.getStatus());
    } catch {
      /* bridge not ready */
    } finally {
      setBusy(false);
    }
  }, []);

  if (showSettings) {
    return (
      <SettingsScreen deviceId={status.deviceId} onBack={() => setShowSettings(false)} />
    );
  }

  const buttonLabel = connecting ? 'Connecting' : connected ? 'Connected' : 'Paused';
  const successLabel =
    status.totalJobsCompleted + status.totalJobsFailed > 0
      ? `${status.successRate.toFixed(1)}%`
      : '—';

  return (
    <div className="app">
      <header className="header">
        <div className="brand">
          <span className="logo-mark" />
          <span className="brand-name">Mellowtel</span>
        </div>
        <button className="icon-btn" title="Settings" onClick={() => setShowSettings(true)}>
          <GearIcon />
        </button>
      </header>

      <main className="main">
        <section className="earnings">
          <div className="earnings-label">Total earned</div>
          <div className="earnings-value">{formatUSD(status.totalEarnedUsd)}</div>
          <div className="earnings-session">
            + {formatUSD(status.sessionEarnedUsd)} this session
          </div>
        </section>

        <button
          className={`power ${connected ? 'on' : connecting ? 'connecting' : 'off'}`}
          onClick={toggle}
          disabled={busy || connecting}
          aria-label={connected ? 'Pause sharing' : 'Start sharing'}
        >
          <PowerIcon />
          <span className="power-label">{buttonLabel}</span>
        </button>

        <div className="status-line">
          <span className={`status-dot ${status.connection}`} />
          <span>{status.detail}</span>
        </div>

        {!status.chromeFound && (
          <button className="warn-pill" onClick={() => setChromeMissing(true)}>
            Chrome not found — tap to fix
          </button>
        )}

        <section className="stats">
          <div className="stat">
            <div className="stat-value">{formatCount(status.totalJobsCompleted)}</div>
            <div className="stat-label">jobs done</div>
          </div>
          <div className="stat">
            <div className="stat-value">{formatCount(status.totalJobsFailed)}</div>
            <div className="stat-label">failed</div>
          </div>
          <div className="stat">
            <div className="stat-value accent">{successLabel}</div>
            <div className="stat-label">success</div>
          </div>
        </section>

        <button
          className="dashboard-btn"
          onClick={() => API.openURL('https://www.mellowtel.com')}
        >
          Open dashboard <span className="arrow">↗</span>
        </button>
      </main>

      {chromeMissing && <ChromeModal onClose={() => setChromeMissing(false)} />}
    </div>
  );
}

function PowerIcon() {
  return (
    <svg className="power-icon" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path
        d="M12 3v9"
        stroke="currentColor"
        strokeWidth="2.2"
        strokeLinecap="round"
      />
      <path
        d="M18.4 6.6a9 9 0 1 1-12.8 0"
        stroke="currentColor"
        strokeWidth="2.2"
        strokeLinecap="round"
      />
    </svg>
  );
}

function GearIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" width="18" height="18" aria-hidden="true">
      <circle cx="12" cy="12" r="3" stroke="currentColor" strokeWidth="1.8" />
      <path
        d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1A1.7 1.7 0 0 0 8.9 19a1.7 1.7 0 0 0-1.9.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.9 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1A1.7 1.7 0 0 0 5 8.9a1.7 1.7 0 0 0-.3-1.9l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.9.3H9.5a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.9-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.9v.1a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"
        stroke="currentColor"
        strokeWidth="1.5"
      />
    </svg>
  );
}
