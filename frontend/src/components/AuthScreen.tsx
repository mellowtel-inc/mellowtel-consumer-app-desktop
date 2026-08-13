import { FormEvent, useState } from 'react';
import { API, AuthState } from '../api';
import earnbearMark from '../assets/brand/earnbear-mark.png';
import earnbearToken from '../assets/brand/earnbear-token.png';

type Mode = 'signin' | 'signup' | 'confirm';

interface Props {
  onAuthenticated: (state: AuthState) => void;
}
const COPY = {
  signin: {
    eyebrow: 'Welcome back',
    title: 'Sign in to your den.',
    copy: 'Keep this device, your points, and the web dashboard under one Earnbear account.',
    action: 'Sign in',
  },
  signup: {
    eyebrow: 'New to Earnbear',
    title: 'Create your account.',
    copy: 'One account follows your devices and keeps every point in the right place.',
    action: 'Create account',
  },
  confirm: {
    eyebrow: 'Check your inbox',
    title: 'Verify your email.',
    copy: 'Enter the six-digit code we sent you to finish setting up Earnbear.',
    action: 'Verify email',
  },
};

export default function AuthScreen({ onAuthenticated }: Props) {
  const [mode, setMode] = useState<Mode>('signin');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [repeatPassword, setRepeatPassword] = useState('');
  const [code, setCode] = useState('');
  const [affiliateCode, setAffiliateCode] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');

  const switchMode = (next: Mode) => {
    setMode(next);
    setError('');
    setMessage('');
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setError('');
    setMessage('');
    if (mode === 'signup' && password !== repeatPassword) {
      setError('Those passwords do not match.');
      return;
    }
    setBusy(true);
    try {
      if (mode === 'signin') {
        onAuthenticated(await API.signIn(email, password));
        return;
      }
      if (mode === 'signup') {
        const result = await API.signUp(email, password, affiliateCode);
        if (result.confirmed) {
          switchMode('signin');
          setMessage('Account created. Sign in to continue.');
        } else {
          switchMode('confirm');
          setMessage('Your verification code is on its way.');
        }
        return;
      }
      await API.confirmSignUp(email, code);
      switchMode('signin');
      setMessage('Email verified. Sign in to connect this device.');
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : String(requestError));
    } finally {
      setBusy(false);
    }
  };

  const resend = async () => {
    setBusy(true);
    setError('');
    try {
      await API.resendSignUpCode(email);
      setMessage('A fresh verification code is on its way.');
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : String(requestError));
    } finally {
      setBusy(false);
    }
  };

  const current = COPY[mode];

  return (
    <div className="desktop-auth">
      <header className="desktop-auth-header">
        <span><img src={earnbearMark} alt="" /><b>earnbear.app</b></span>
        <button onClick={() => API.openURL('https://earnbear.app')}>Visit website ↗</button>
      </header>

      <div className="desktop-auth-art" aria-hidden="true">
        <span className="desktop-auth-halo halo-one" />
        <span className="desktop-auth-halo halo-two" />
        <img className="desktop-auth-bear" src={earnbearMark} alt="" />
        <img className="desktop-auth-coin auth-coin-one" src={earnbearToken} alt="" />
        <img className="desktop-auth-coin auth-coin-two" src={earnbearToken} alt="" />
      </div>

      <section className="desktop-auth-panel">
        <div className="desktop-auth-copy">
          <span>{current.eyebrow}</span>
          <h1>{current.title}</h1>
          <p>{current.copy}</p>
        </div>

        <form className="desktop-auth-form" onSubmit={submit}>
          <label>
            <span>Email address</span>
            <input type="email" value={email} onChange={(event) => setEmail(event.target.value)} autoComplete="email" placeholder="you@example.com" required />
          </label>

          {mode === 'confirm' ? (
            <label>
              <span>Six-digit code</span>
              <input className="desktop-code-input" type="text" value={code} onChange={(event) => setCode(event.target.value.replace(/\D/g, ''))} inputMode="numeric" autoComplete="one-time-code" minLength={6} maxLength={6} placeholder="000000" required />
            </label>
          ) : (
            <label>
              <span>Password</span>
              <input type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete={mode === 'signin' ? 'current-password' : 'new-password'} minLength={8} placeholder="At least 8 characters" required />
            </label>
          )}

          {mode === 'signup' && (
            <>
              <label>
                <span>Confirm password</span>
                <input type="password" value={repeatPassword} onChange={(event) => setRepeatPassword(event.target.value)} autoComplete="new-password" minLength={8} placeholder="Repeat your password" required />
              </label>
              <label>
                <span>Creator code <small>(optional)</small></span>
                <input type="text" value={affiliateCode} onChange={(event) => setAffiliateCode(event.target.value.toUpperCase())} autoComplete="off" maxLength={32} placeholder="CREATOR" />
              </label>
            </>
          )}

          {error && <p className="desktop-auth-feedback error" role="alert">{error}</p>}
          {message && <p className="desktop-auth-feedback success" role="status">{message}</p>}

          <button className="desktop-auth-primary" type="submit" disabled={busy}>{busy ? 'One moment…' : current.action}</button>
        </form>

        <div className="desktop-auth-links">
          {mode === 'signin' && <><button onClick={() => API.openURL('https://earnbear.app/auth?mode=forgot')}>Forgot password?</button><p>New here? <button onClick={() => switchMode('signup')}>Create an account</button></p></>}
          {mode === 'signup' && <p>Already have an account? <button onClick={() => switchMode('signin')}>Sign in</button></p>}
          {mode === 'confirm' && <><button onClick={resend} disabled={busy}>Send another code</button><p><button onClick={() => switchMode('signin')}>Back to sign in</button></p></>}
        </div>
      </section>
    </div>
  );
}
