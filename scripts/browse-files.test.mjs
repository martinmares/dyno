import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const source = readFileSync(new URL('../assets/app.js', import.meta.url), 'utf8');
const context = vm.createContext({});
vm.runInContext(source.slice(source.indexOf('function formatBrowseFileTime('), source.indexOf('function initBrowseFiles(')), context);
const compare = context.compareBrowseFiles;
test('browse times use compact local dates with two-digit years and padded hours', () => {
  assert.equal(context.formatBrowseFileTime('2026-09-04T09:05:00'), '04.09.26 09:05');
  assert.equal(context.formatBrowseFileTime('invalid'), '');
});
function file(name, created = '', modified = '', directory = false) {
  return { dataset: { fileName: name, fileCreated: created, fileModified: modified }, hasAttribute: () => directory };
}

test('browse filename filter ignores case and accents and matches substrings', () => {
  assert.equal(context.browseFileMatches('PŘÍLIŠ_Žluťoučký.md', 'zlutoucky'), true);
  assert.equal(context.browseFileMatches('prilis.md', ' PŘÍLIŠ '), true);
  assert.equal(context.browseFileMatches('Žluťoučký.md', 'Z\u030clut\u030couc\u030cky\u0301'), true);
  assert.equal(context.browseFileMatches('CLI_CONTRACTS.md', 'contracts'), true);
  assert.equal(context.browseFileMatches('guide.md', ''), true);
  assert.equal(context.browseFileMatches('guide.md', 'other'), false);
});

test('browse names use natural order in both directions', () => {
  assert.ok(compare(file('2.md'), file('10.md'), 'name', 'asc') < 0);
  assert.ok(compare(file('2.md'), file('10.md'), 'name', 'desc') > 0);
});
test('browse timestamps sort numerically and tie-break by name', () => {
  assert.ok(compare(file('a', '9', '20'), file('b', '10', '10'), 'created', 'asc') < 0);
  assert.ok(compare(file('a', '9', '20'), file('b', '10', '10'), 'modified', 'desc') < 0);
  assert.ok(compare(file('a', '10'), file('b', '10'), 'created', 'desc') < 0);
});
test('missing birth times stay last in either direction without a modification-time fallback', () => {
  for (const direction of ['asc', 'desc']) {
    assert.ok(compare(file('a', '', '9999'), file('b', '10', '1'), 'created', direction) > 0);
    assert.ok(compare(file('a'), file('b'), 'created', direction) < 0);
  }
});
test('folders always stay above files and keep alphabetical order', () => {
  for (const direction of ['asc', 'desc']) {
    assert.ok(compare(file('z', '', '', true), file('a', '10'), 'created', direction) < 0);
    assert.ok(compare(file('a', '', '', true), file('b', '', '', true), 'name', direction) < 0);
  }
});
