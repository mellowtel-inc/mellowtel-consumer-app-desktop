import { FormEvent, useMemo, useState } from 'react';
import { API, copyText, WaitlistSocialClaims } from '../api';

type Step = 'email' | 'profile' | 'social' | 'complete';

interface Props {
  onBack: () => void;
}

const COUNTRIES = [
  'Australia', 'Austria', 'Belgium', 'Brazil', 'Canada', 'China', 'Denmark',
  'Finland', 'France', 'Germany', 'India', 'Ireland', 'Italy', 'Japan',
  'Mexico', 'Netherlands', 'Norway', 'Poland', 'Portugal', 'Singapore',
  'South Korea', 'Spain', 'Sweden', 'Switzerland', 'United Kingdom',
  'United States',
];

const SOCIALS = [
  { id: 'x', label: 'Follow on X', detail: '@earnbearapp', url: 'https://x.com/earnbearapp' },
  { id: 'discord', label: 'Join Discord', detail: 'Community + support', url: 'https://discord.gg/QyUVXRr28a' },
  { id: 'tiktok', label: 'Follow on TikTok', detail: '@earnbear_app', url: 'https://www.tiktok.com/@earnbear_app' },
] as const;

function currentDevice(): string {
  const platform = `${navigator.platform} ${navigator.userAgent}`.toLowerCase();
  if (platform.includes('win')) return 'windows';
  if (platform.includes('mac')) return 'macos';
  return 'linux';
}

export default function WaitlistFlow({ onBack }: Props) {
  const [step, setStep] = useState<Step>('email');
  const [email, setEmail] = useState('');
  const [referralCode, setReferralCode] = useState('');
  const [country, setCountry] = useState('');
  const [devices, setDevices] = useState<string[]>([currentDevice()]);
  const [claims, setClaims] = useState<WaitlistSocialClaims>({ x: false, tiktok: false, discord: false });
  const [onboardingToken, setOnboardingToken] = useState('');
  const [inviteUrl, setInviteUrl] = useState('');
  const [acceptedInvites, setAcceptedInvites] = useState(0);
  const [pendingPoints, setPendingPoints] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [copied, setCopied] = useState(false);

  const selectedDeviceLabel = useMemo(() => devices.map((device) => device === 'macos' ? 'macOS' : device[0].toUpperCase() + device.slice(1)).join(', '), [devices]);

  const join = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError('');
    try {
      const result = await API.joinWaitlist(email.trim(), referralCode.trim().toLowerCase());
      const progress = result.progress;
      const restoredClaims = progress?.socialClaims || { x: false, tiktok: false, discord: false };
      setEmail(email.trim().toLowerCase());
      setOnboardingToken(result.onboardingToken);
      setInviteUrl(result.inviteUrl);
      setCountry(progress?.country || '');
      setDevices(progress?.devices?.length ? progress.devices : [currentDevice()]);
      setClaims(restoredClaims);
      setAcceptedInvites(progress?.acceptedInviteCount || 0);
      setPendingPoints(progress?.pendingReferralPoints || 0);
      setStep(!progress?.profileCompleted
        ? 'profile'
        : progress.onboardingCompleted || (restoredClaims.x && restoredClaims.tiktok && restoredClaims.discord)
          ? 'complete'
          : 'social');
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : String(requestError));
    } finally {
      setBusy(false);
    }
  };

  const toggleDevice = (device: string) => {
    setDevices((current) => current.includes(device)
      ? current.filter((item) => item !== device)
      : [...current, device]);
  };

  const saveProfile = async () => {
    setBusy(true);
    setError('');
    try {
      await API.saveWaitlistProfile(email, onboardingToken, country.trim(), devices, claims, false);
      setStep('social');
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : String(requestError));
    } finally {
      setBusy(false);
    }
  };

  const claimSocial = (platform: keyof WaitlistSocialClaims, url: string) => {
    setClaims((current) => ({ ...current, [platform]: true }));
    void API.openURL(url);
  };

  const finish = async () => {
    setBusy(true);
    setError('');
    try {
      await API.saveWaitlistProfile(email, onboardingToken, country.trim(), devices, claims, true);
      setStep('complete');
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : String(requestError));
    } finally {
      setBusy(false);
    }
  };

  const copyInvite = () => {
    copyText(inviteUrl);
    setCopied(true);
  };

  return (
    <section className="desktop-auth-panel waitlist-desktop-panel">
      {step === 'email' && (
        <>
          <div className="desktop-auth-copy">
            <span>Before public launch</span>
            <h1>Join the waitlist.</h1>
            <p>Save your place for desktop early access and restore an existing waitlist profile with the same email.</p>
          </div>
          <form className="desktop-auth-form" onSubmit={join}>
            <label><span>Email address</span><input type="email" value={email} onChange={(event) => setEmail(event.target.value)} autoComplete="email" placeholder="you@example.com" required /></label>
            <label><span>Invite code <small>(optional)</small></span><input type="text" value={referralCode} onChange={(event) => setReferralCode(event.target.value)} autoComplete="off" maxLength={32} placeholder="12- or 32-character code" /></label>
            {error && <p className="desktop-auth-feedback error" role="alert">{error}</p>}
            <button className="desktop-auth-primary" type="submit" disabled={busy}>{busy ? 'Saving your place…' : 'Join or restore waitlist'}</button>
          </form>
        </>
      )}

      {step === 'profile' && (
        <>
          <div className="desktop-auth-copy">
            <span>Your setup</span>
            <h1>Tell us where you’ll run Earnbear.</h1>
            <p>This helps us open early access in the right countries and on the right desktops.</p>
          </div>
          <div className="desktop-auth-form">
            <label><span>Country or region</span><input list="waitlist-countries" value={country} onChange={(event) => setCountry(event.target.value)} placeholder="Start typing…" /></label>
            <datalist id="waitlist-countries">{COUNTRIES.map((item) => <option value={item} key={item} />)}</datalist>
            <fieldset className="waitlist-device-picker">
              <legend>Which desktops would you use?</legend>
              <div>{['windows', 'macos', 'linux'].map((device) => <button type="button" className={devices.includes(device) ? 'selected' : ''} aria-pressed={devices.includes(device)} onClick={() => toggleDevice(device)} key={device}>{device === 'macos' ? 'macOS' : device[0].toUpperCase() + device.slice(1)}<i>{devices.includes(device) ? '✓' : '+'}</i></button>)}</div>
              <small>Selected: {selectedDeviceLabel || 'none'}</small>
            </fieldset>
            {error && <p className="desktop-auth-feedback error" role="alert">{error}</p>}
            <button className="desktop-auth-primary" type="button" onClick={() => void saveProfile()} disabled={busy || country.trim().length < 2 || devices.length === 0}>{busy ? 'Saving…' : 'Save profile'}</button>
          </div>
        </>
      )}

      {step === 'social' && (
        <>
          <div className="desktop-auth-copy">
            <span>Optional bonuses</span>
            <h1>Pick up extra points.</h1>
            <p>Open any community below and claim +100 pending points for each.</p>
          </div>
          <div className="waitlist-social-list">{SOCIALS.map((social) => <article key={social.id}><span>{social.id === 'tiktok' ? '♪' : social.id === 'discord' ? 'D' : 'X'}</span><div><b>{social.label}</b><small>{social.detail}</small></div><button type="button" className={claims[social.id] ? 'claimed' : ''} onClick={() => claimSocial(social.id, social.url)}>{claims[social.id] ? '✓ +100' : 'Open · +100'}</button></article>)}</div>
          {error && <p className="desktop-auth-feedback error" role="alert">{error}</p>}
          <button className="desktop-auth-primary" type="button" onClick={() => void finish()} disabled={busy}>{busy ? 'Saving…' : 'Continue to invite'}</button>
          <button className="waitlist-text-action" type="button" onClick={() => setStep('profile')}>Back to profile</button>
        </>
      )}

      {step === 'complete' && (
        <>
          <div className="desktop-auth-copy">
            <span>You’re on the list</span>
            <h1>Bring friends with you.</h1>
            <p>Your profile is saved. We’ll email you when your account batch opens.</p>
          </div>
          <div className="waitlist-complete-card">
            <div><span><b>{acceptedInvites}</b><small>friends waiting</small></span><span><b>{pendingPoints.toLocaleString()}</b><small>points pending</small></span></div>
            <label><span>Your private invite link</span><section><input value={inviteUrl} readOnly /><button type="button" onClick={copyInvite}>{copied ? 'Copied' : 'Copy'}</button></section></label>
            <small>Each qualifying friend reserves 500 points and earns you 10% of their eligible lifetime earnings.</small>
          </div>
          <button className="desktop-auth-primary" type="button" onClick={onBack}>Back to sign in</button>
          {!claims.x || !claims.tiktok || !claims.discord ? <button className="waitlist-text-action" type="button" onClick={() => setStep('social')}>Earn extra points</button> : null}
        </>
      )}

      {step === 'email' && <button className="waitlist-text-action" type="button" onClick={onBack}>Already have an account? Sign in</button>}
    </section>
  );
}
