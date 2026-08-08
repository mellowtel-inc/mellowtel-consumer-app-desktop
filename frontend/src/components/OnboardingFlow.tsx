import { useState } from 'react';
import earnbearMark from '../assets/brand/earnbear-mark.png';
import earnbearCoinPresenter from '../assets/mascot/earnbear-coin-presenter.png';
import earnbearPrivacy from '../assets/mascot/earnbear-privacy-control.png';
import earnbearQuiet from '../assets/mascot/earnbear-quiet-earning.png';

interface Props {
  onComplete: () => void;
}

const STEPS = [
  {
    eyebrow: 'Welcome to Earnbear',
    title: 'Put spare internet to work.',
    copy: 'Earnbear quietly uses bandwidth you are not using and turns those idle moments into rewards.',
    image: earnbearCoinPresenter,
    alt: 'Earnbear presenting a reward coin',
    notes: ['Simple setup', 'Runs in the background'],
  },
  {
    eyebrow: 'Always your choice',
    title: 'You set the pace.',
    copy: 'Choose a sharing level that feels right. Pause instantly, change your preferences, or close the app whenever you want.',
    image: earnbearPrivacy,
    alt: 'Earnbear standing beside privacy controls',
    notes: ['Pause anytime', 'Adjustable sharing'],
  },
  {
    eyebrow: 'Ready when you are',
    title: 'Small moments add up.',
    copy: 'Leave Earnbear running during coffee breaks, meetings, or quiet evenings, then watch your rewards gather.',
    image: earnbearQuiet,
    alt: 'Earnbear relaxing while rewards gather',
    notes: ['Track every reward', 'Stay in control'],
  },
];

export default function OnboardingFlow({ onComplete }: Props) {
  const [stepIndex, setStepIndex] = useState(0);
  const step = STEPS[stepIndex];
  const finalStep = stepIndex === STEPS.length - 1;

  return (
    <div className="onboarding-overlay" role="dialog" aria-modal="true" aria-labelledby="onboarding-title">
      <section className="onboarding-card">
        <header className="onboarding-header">
          <span className="onboarding-brand">
            <img src={earnbearMark} alt="" />
            <b>earnbear.app</b>
          </span>
          {!finalStep && (
            <button className="onboarding-skip" onClick={onComplete}>Skip</button>
          )}
        </header>

        <div className="onboarding-art">
          <span className="onboarding-halo" aria-hidden="true" />
          <img key={step.image} src={step.image} alt={step.alt} />
        </div>

        <div className="onboarding-copy">
          <span>{step.eyebrow}</span>
          <h1 id="onboarding-title">{step.title}</h1>
          <p>{step.copy}</p>
          <div className="onboarding-notes">
            {step.notes.map((note) => <small key={note}>✓ {note}</small>)}
          </div>
        </div>

        <footer className="onboarding-footer">
          <div className="onboarding-progress" aria-label={`Step ${stepIndex + 1} of ${STEPS.length}`}>
            {STEPS.map((_, index) => (
              <i key={index} className={index === stepIndex ? 'active' : ''} />
            ))}
          </div>
          <div className="onboarding-actions">
            {stepIndex > 0 && (
              <button className="onboarding-back" onClick={() => setStepIndex(stepIndex - 1)}>Back</button>
            )}
            <button
              className="onboarding-primary"
              onClick={() => finalStep ? onComplete() : setStepIndex(stepIndex + 1)}
            >
              {finalStep ? 'Open Earnbear' : 'Continue'}
            </button>
          </div>
        </footer>
      </section>
    </div>
  );
}
