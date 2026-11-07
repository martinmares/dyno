import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const app = readFileSync(new URL('../assets/app.js', import.meta.url), 'utf8');
const source = app.slice(app.indexOf('function searchMatchRanges('), app.indexOf('function applySearchHighlights('));
const context = vm.createContext({});
vm.runInContext(source, context);
const matches = (value, query) => Array.from(context.searchMatchRanges(value, query), range => value.slice(range.start, range.end));

test('destination highlights Czech without accents, case or word-order sensitivity', () => {
  assert.deepEqual(matches('Přístupová práva pro PostgreSQL', 'postgresql pristupova'), ['Přístupová', 'PostgreSQL']);
  assert.deepEqual(matches('Pr\u030ci\u0301stup', 'pristup'), ['Pr\u030ci\u0301stup']);
});

test('destination preserves phrase boundaries and merges overlapping matches', () => {
  assert.deepEqual(matches('database runtime permissions and database permissions', '"database permissions"'), ['database permissions']);
  assert.deepEqual(matches('PostgreSQL', 'post postgresql'), ['PostgreSQL']);
  assert.deepEqual(matches('database\n  permissions', '"database permissions"'), ['database\n  permissions']);
});

test('prefix highlights cover complete words without swallowing punctuation or adjacent words', () => {
  assert.deepEqual(matches('Cílové rozdělení schémat PostgreSQL, rozdělená data.', 'Postgres rozdělen'), ['rozdělení', 'PostgreSQL', 'rozdělená']);
  assert.deepEqual(matches('rozděleni\u0301, další', 'rozdelen'), ['rozděleni\u0301']);
  assert.deepEqual(matches('😀 PostgreSQL; tsm-pd-backend', 'Postgres pd'), ['PostgreSQL', 'pd']);
  assert.deepEqual(matches('A<script> text', '<script>'), ['<script>']);
});

test('destination handles UTF-16 offsets, combining accents, and literal identifiers safely', () => {
  assert.deepEqual(matches('😀 REQ0129276 a <script>', 'REQ0129276 <script>'), ['REQ0129276', '<script>']);
  assert.deepEqual(matches('a\u0301', 'a'), ['a\u0301']);
  assert.deepEqual(matches('tsm-pd-backend release/RE63', 'release/RE63'), ['release/RE63']);
  assert.deepEqual(matches('no results', '""'), []);
});

test('Enter opens full search with site prefix and metadata filters preserved', () => {
  const ctx = vm.createContext({ URL });
  vm.runInContext(app.slice(app.indexOf('function searchResultsURL('), app.indexOf("document.addEventListener('DOMContentLoaded'", app.indexOf('function searchResultsURL('))), ctx);
  assert.equal(ctx.searchResultsURL('/docs/_search?meta.owner=alice&page=3', ' PostgreSQL role ', 'http://localhost:3000/docs/guide'), '/docs/_search?meta.owner=alice&q=PostgreSQL+role');
});
