import { test } from 'node:test';
import assert from 'node:assert/strict';

// match() is duplicated here as a pure function test (router.tsx needs a DOM).
function match(pattern: string, path: string) {
  const p = pattern.split('/').filter(Boolean), s = path.split('/').filter(Boolean);
  if (p.length !== s.length) return null;
  const out: Record<string, string> = {};
  for (let i = 0; i < p.length; i++) {
    if (p[i].startsWith(':')) out[p[i].slice(1)] = decodeURIComponent(s[i]);
    else if (p[i] !== s[i]) return null;
  }
  return out;
}

test('route matching', () => {
  assert.deepEqual(match('/documents/:uri', '/documents/in.gov.pan-PANCR-ZZZPT0001Z'), { uri: 'in.gov.pan-PANCR-ZZZPT0001Z' });
  assert.equal(match('/documents/:uri', '/documents'), null);
  assert.deepEqual(match('/admin/users/:id', '/admin/users/a%2Fb'), { id: 'a/b' });
});
