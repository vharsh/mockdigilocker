import { test } from 'node:test';
import assert from 'node:assert/strict';
import { maskMobile, statusTone, httpTone, genderLabel } from './format.ts';

test('maskMobile keeps last 4 digits', () => {
  assert.equal(maskMobile('9999900001'), '••••••0001');
});
test('status tones', () => {
  assert.equal(statusTone('valid'), 'green');
  assert.equal(statusTone('revoked'), 'red');
  assert.equal(statusTone('retrying'), 'amber');
  assert.equal(httpTone(503), 'red');
  assert.equal(httpTone(404), 'amber');
  assert.equal(genderLabel('T'), 'Transgender');
});
