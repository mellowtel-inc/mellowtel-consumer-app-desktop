import { useEffect, useState, useCallback } from 'react';
import './App.css';
import {
  API,
  onStatus,
  onChromeMissing,
  formatPoints,
  formatCount,
  Status,
  AuthState,
} from './api';
import SettingsScreen from './components/SettingsScreen';
import ChromeModal from './components/ChromeModal';
import OnboardingFlow from './components/OnboardingFlow';
import AuthScreen from './components/AuthScreen';
import earnbearMark from '../../../public/brand/earnbear-mark.png';
import earnbearCoinPresenter from '../../../public/mascot-cutouts/earnbear-coin-presenter.png';
import earnbearPausedSad from '../../../public/mascot-cutouts/earnbear-paused-sad.png';
import earnbearToken from '../../../public/brand/earnbear-token.png';

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
  const [authState, setAuthState] = useState<AuthState | null>(null);
  const [authLoading, setAuthLoading] = useState(true);
  const [showOnboarding, setShowOnboarding] = useState(
    () => window.localStorage.getItem('earnbear-onboarding-complete') !== 'true'
  );

  useEffect(() => {
    API.getAuthState()
      .then(setAuthState)
      .catch(() => setAuthState({ authenticated: false, email: '', emailVerified: false, name: '' }))
      .finally(() => setAuthLoading(false));
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

  // Treat sharing as active as soon as the user taps connect. The native
  // connection can continue in the background without making the button
  // visibly step through a second, laggy-looking state.
  const active = !status.paused;

  const toggle = useCallback(async () => {
    setBusy(true);
    setStatus((current) => active
      ? { ...current, connection: 'disconnected', paused: true, detail: 'Paused — tap to start again.' }
      : { ...current, connection: 'connecting', paused: false, detail: 'Earnbear is active.' }
    );
    try {
      await API.toggle();
      setStatus(await API.getStatus());
    } catch {
      /* bridge not ready */
    } finally {
      setBusy(false);
    }
  }, [active]);

  const finishOnboarding = () => {
    window.localStorage.setItem('earnbear-onboarding-complete', 'true');
    setShowOnboarding(false);
  };

  const signOut = async () => {
    try {
      await API.signOut();
    } finally {
      setStatus(EMPTY_STATUS);
      setShowSettings(false);
      setAuthState({ authenticated: false, email: '', emailVerified: false, name: '' });
    }
  };

  if (authLoading) {
    return <div className="desktop-auth-loading"><img src={earnbearMark} alt="" /><span>Opening your den…</span></div>;
  }

  if (!authState?.authenticated) {
    return <AuthScreen onAuthenticated={setAuthState} />;
  }

  if (showSettings) {
    return (
      <SettingsScreen deviceId={status.deviceId} email={authState.email} onSignOut={signOut} onBack={() => setShowSettings(false)} />
    );
  }

  const buttonLabel = active ? 'Connected' : 'Paused';
  const statusDetail = active
    ? (status.connection === 'connected' ? status.detail : 'Earnbear is active.')
    : 'Paused — tap to start again.';
  const successLabel =
    status.totalJobsCompleted + status.totalJobsFailed > 0
      ? `${status.successRate.toFixed(1)}%`
      : '—';

  return (
    <div className="app">
      <header className="header">
        <div className="brand">
          <span className="logo-mark">
            <img src={earnbearMark} alt="" />
          </span>
          <span className="brand-name">earnbear.app</span>
        </div>
        <button className="icon-btn" title="Settings" onClick={() => setShowSettings(true)}>
          <GearIcon />
        </button>
      </header>

      <main className="main">
        <section className="earnings">
          <div className="earnings-label">Total earned</div>
          <div className="earnings-value">{formatPoints(status.totalJobsCompleted)}</div>
          <div className="earnings-session">
            + {formatPoints(status.jobsCompleted)} this session
          </div>
        </section>

        <div className={`sharing-stage ${active ? 'active' : 'paused'}`}>
          <div className="coin-stream" aria-hidden="true">
            {Array.from({ length: 7 }, (_, index) => (
              <img
                key={index}
                className={`stream-coin stream-coin-${index + 1}`}
                src={earnbearToken}
                alt=""
              />
            ))}
          </div>
          <img
            key={active ? 'active-mascot' : 'paused-mascot'}
            className="dashboard-mascot"
            src={active ? earnbearCoinPresenter : earnbearPausedSad}
            alt={active ? 'Earnbear presenting a reward coin' : 'Earnbear looking sad while sharing is paused'}
          />
          <button
            className={`power ${active ? 'on' : 'off'}`}
            onClick={toggle}
            disabled={busy}
            aria-label={active ? 'Pause sharing' : 'Start sharing'}
          >
            <PowerIcon />
            <span className="power-label">{buttonLabel}</span>
          </button>
        </div>

        <div className="status-line">
          <span className={`status-dot ${status.connection}`} />
          <span>{statusDetail}</span>
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
          onClick={() => API.openURL('https://earnbear.app/dashboard')}
        >
          Open dashboard <span className="arrow">↗</span>
        </button>
      </main>

      {chromeMissing && <ChromeModal onClose={() => setChromeMissing(false)} />}
      {showOnboarding && <OnboardingFlow onComplete={finishOnboarding} />}
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
