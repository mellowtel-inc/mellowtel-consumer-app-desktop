import test from 'node:test';
import assert from 'node:assert/strict';
import { sharingView } from './sharing-state.ts';

const approvedUser = { authenticated: true, earningApproved: true };
const paused = { sharingAccess: 'approved', paused: true, detail: 'Paused — tap to start again.' };

test('unapproved account stays locked even with stale approved device state', () => {
 const view = sharingView({ authenticated: true, earningApproved: false }, { ...paused, paused: false });
 assert.equal(view.canStart, false);
 assert.equal(view.active, false);
 assert.equal(view.buttonLabel, 'Not approved');
 assert.match(view.statusText, /not approved to earn/);
 assert.doesNotMatch(view.statusText, /tap to start/);
});
test('device denial is visible and the sharing button stays locked', () => {
 const view = sharingView(approvedUser, { ...paused, sharingAccess: 'not_approved', detail: 'This device is not approved to share.' });
 assert.equal(view.canStart, false);
 assert.equal(view.buttonLabel, 'Not approved');
 assert.equal(view.statusText, 'This device is not approved to share.');
 assert.equal(view.allowRetry, true);
});
test('failed verification preserves its reason and never invites starting', () => {
 const view = sharingView(approvedUser, { ...paused, sharingAccess: 'unavailable', detail: 'Could not verify account approval. Sharing is unavailable.' });
 assert.equal(view.canStart, false);
 assert.equal(view.buttonLabel, 'Unavailable');
 assert.equal(view.statusText, 'Could not verify account approval. Sharing is unavailable.');
});
test('registration conflict shows a device block instead of unknown approval', () => {
 const view = sharingView(approvedUser, { ...paused, sharingAccess: 'device_conflict', detail: 'This device is revoked or linked to another account. Sharing is unavailable.' });
 assert.equal(view.canStart, false);
 assert.equal(view.active, false);
 assert.equal(view.buttonLabel, 'Device blocked');
 assert.match(view.statusText, /revoked or linked/);
 assert.equal(view.allowRetry, true);
});
test('unknown and in-progress approval both disable starting', () => {
 for (const sharingAccess of ['unknown', 'checking']) {
  const view = sharingView(approvedUser, { ...paused, sharingAccess });
  assert.equal(view.canStart, false);
  assert.equal(view.active, false);
  assert.equal(view.buttonLabel, 'Checking');
  assert.equal(view.allowRetry, false);
 }
});
test('approved user who paused can resume, and active user can pause', () => {
 assert.equal(sharingView(approvedUser, paused).canStart, true);
 assert.equal(sharingView(approvedUser, paused).buttonLabel, 'Paused');
 const connected = sharingView(approvedUser, { ...paused, paused: false, detail: 'Connected — waiting for jobs.' });
 assert.equal(connected.active, true);
 assert.equal(connected.buttonLabel, 'Connected');
});
