import type { AuthState, Status } from './api';

export function sharingView(auth: AuthState | null, status: Status) {
  const accountApproved = Boolean(auth?.authenticated && auth.earningApproved);
  const approved = accountApproved && status.sharingAccess === 'approved';
  const active = approved && !status.paused;
  if (status.sharingAccess === 'device_conflict') {
    return { active: false, canStart: false, buttonLabel: 'Device blocked', statusText: status.detail || 'This device is revoked or linked to another account. Sharing is unavailable.', allowRetry: true };
  }
  if (status.sharingAccess === 'unavailable') {
    return { active: false, canStart: false, buttonLabel: 'Unavailable', statusText: status.detail || 'Could not verify approval. Sharing is unavailable.', allowRetry: true };
  }
  if (!accountApproved || status.sharingAccess === 'not_approved') {
    return { active: false, canStart: false, buttonLabel: 'Not approved', statusText: !accountApproved ? 'Your account is not approved to earn. Sharing is unavailable until approval.' : status.detail || 'This device is not approved to share.', allowRetry: true };
  }
  if (!approved) {
    return { active: false, canStart: false, buttonLabel: 'Checking', statusText: 'Checking approval before sharing can start…', allowRetry: false };
  }
  return { active, canStart: approved, buttonLabel: active ? 'Connected' : 'Paused', statusText: active ? status.detail || 'Connected — waiting for jobs.' : status.detail || 'Paused — tap to start again.', allowRetry: false };
}
